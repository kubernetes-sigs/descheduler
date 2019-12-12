/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package core

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/algorithm"
	algorithmpredicates "k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/priorities"
	priorityutil "k8s.io/kubernetes/pkg/scheduler/algorithm/priorities/util"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	extenderv1 "k8s.io/kubernetes/pkg/scheduler/apis/extender/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	internalcache "k8s.io/kubernetes/pkg/scheduler/internal/cache"
	internalqueue "k8s.io/kubernetes/pkg/scheduler/internal/queue"
	schedulerlisters "k8s.io/kubernetes/pkg/scheduler/listers"
	fakelisters "k8s.io/kubernetes/pkg/scheduler/listers/fake"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

var (
	errPrioritize = fmt.Errorf("priority map encounters an error")
	order         = []string{"false", "true", "matches", "nopods", algorithmpredicates.MatchInterPodAffinityPred}
)

func falsePredicate(pod *v1.Pod, meta algorithmpredicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []algorithmpredicates.PredicateFailureReason, error) {
	return false, []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate}, nil
}

func truePredicate(pod *v1.Pod, meta algorithmpredicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []algorithmpredicates.PredicateFailureReason, error) {
	return true, nil, nil
}

func matchesPredicate(pod *v1.Pod, meta algorithmpredicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []algorithmpredicates.PredicateFailureReason, error) {
	node := nodeInfo.Node()
	if node == nil {
		return false, nil, fmt.Errorf("node not found")
	}
	if pod.Name == node.Name {
		return true, nil, nil
	}
	return false, []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate}, nil
}

func hasNoPodsPredicate(pod *v1.Pod, meta algorithmpredicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []algorithmpredicates.PredicateFailureReason, error) {
	if len(nodeInfo.Pods()) == 0 {
		return true, nil, nil
	}
	return false, []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate}, nil
}

func numericMapPriority(pod *v1.Pod, meta interface{}, nodeInfo *schedulernodeinfo.NodeInfo) (framework.NodeScore, error) {
	node := nodeInfo.Node()
	score, err := strconv.Atoi(node.Name)
	if err != nil {
		return framework.NodeScore{}, err
	}

	return framework.NodeScore{
		Name:  node.Name,
		Score: int64(score),
	}, nil
}

func reverseNumericReducePriority(pod *v1.Pod, meta interface{}, sharedLister schedulerlisters.SharedLister, result framework.NodeScoreList) error {
	var maxScore float64
	minScore := math.MaxFloat64

	for _, hostPriority := range result {
		maxScore = math.Max(maxScore, float64(hostPriority.Score))
		minScore = math.Min(minScore, float64(hostPriority.Score))
	}
	for i, hostPriority := range result {
		result[i] = framework.NodeScore{
			Name:  hostPriority.Name,
			Score: int64(maxScore + minScore - float64(hostPriority.Score)),
		}
	}

	return nil
}

func trueMapPriority(pod *v1.Pod, meta interface{}, nodeInfo *schedulernodeinfo.NodeInfo) (framework.NodeScore, error) {
	return framework.NodeScore{
		Name:  nodeInfo.Node().Name,
		Score: 1,
	}, nil
}

func falseMapPriority(pod *v1.Pod, meta interface{}, nodeInfo *schedulernodeinfo.NodeInfo) (framework.NodeScore, error) {
	return framework.NodeScore{}, errPrioritize
}

func getNodeReducePriority(pod *v1.Pod, meta interface{}, sharedLister schedulerlisters.SharedLister, result framework.NodeScoreList) error {
	for _, host := range result {
		if host.Name == "" {
			return fmt.Errorf("unexpected empty host name")
		}
	}
	return nil
}

// emptyPluginRegistry is a test plugin set used by the default scheduler.
var emptyPluginRegistry = framework.Registry{}
var emptyFramework, _ = framework.NewFramework(emptyPluginRegistry, nil, []schedulerapi.PluginConfig{})
var emptySnapshot = nodeinfosnapshot.NewEmptySnapshot()

// FakeFilterPlugin is a test filter plugin used by default scheduler.
type FakeFilterPlugin struct {
	numFilterCalled         int32
	failedNodeReturnCodeMap map[string]framework.Code
}

// Name returns name of the plugin.
func (fp *FakeFilterPlugin) Name() string {
	return "fake-filter-plugin"
}

// reset is used to reset filter plugin.
func (fp *FakeFilterPlugin) reset() {
	fp.numFilterCalled = 0
	fp.failedNodeReturnCodeMap = map[string]framework.Code{}
}

// Filter is a test function that returns an error or nil, depending on the
// value of "failedNodeReturnCodeMap".
func (fp *FakeFilterPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) *framework.Status {
	atomic.AddInt32(&fp.numFilterCalled, 1)

	if returnCode, ok := fp.failedNodeReturnCodeMap[nodeInfo.Node().Name]; ok {
		return framework.NewStatus(returnCode, fmt.Sprintf("injecting failure for pod %v", pod.Name))
	}

	return nil
}

// newPlugin returns a plugin factory with specified Plugin.
func newPlugin(plugin framework.Plugin) framework.PluginFactory {
	return func(_ *runtime.Unknown, fh framework.FrameworkHandle) (framework.Plugin, error) {
		return plugin, nil
	}
}

func makeNodeList(nodeNames []string) []*v1.Node {
	result := make([]*v1.Node, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		result = append(result, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}})
	}
	return result
}

func TestSelectHost(t *testing.T) {
	scheduler := genericScheduler{}
	tests := []struct {
		name          string
		list          framework.NodeScoreList
		possibleHosts sets.String
		expectsErr    bool
	}{
		{
			name: "unique properly ordered scores",
			list: []framework.NodeScore{
				{Name: "machine1.1", Score: 1},
				{Name: "machine2.1", Score: 2},
			},
			possibleHosts: sets.NewString("machine2.1"),
			expectsErr:    false,
		},
		{
			name: "equal scores",
			list: []framework.NodeScore{
				{Name: "machine1.1", Score: 1},
				{Name: "machine1.2", Score: 2},
				{Name: "machine1.3", Score: 2},
				{Name: "machine2.1", Score: 2},
			},
			possibleHosts: sets.NewString("machine1.2", "machine1.3", "machine2.1"),
			expectsErr:    false,
		},
		{
			name: "out of order scores",
			list: []framework.NodeScore{
				{Name: "machine1.1", Score: 3},
				{Name: "machine1.2", Score: 3},
				{Name: "machine2.1", Score: 2},
				{Name: "machine3.1", Score: 1},
				{Name: "machine1.3", Score: 3},
			},
			possibleHosts: sets.NewString("machine1.1", "machine1.2", "machine1.3"),
			expectsErr:    false,
		},
		{
			name:          "empty priority list",
			list:          []framework.NodeScore{},
			possibleHosts: sets.NewString(),
			expectsErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// increase the randomness
			for i := 0; i < 10; i++ {
				got, err := scheduler.selectHost(test.list)
				if test.expectsErr {
					if err == nil {
						t.Error("Unexpected non-error")
					}
				} else {
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					if !test.possibleHosts.Has(got) {
						t.Errorf("got %s is not in the possible map %v", got, test.possibleHosts)
					}
				}
			}
		})
	}
}

func TestGenericScheduler(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()

	filterPlugin := &FakeFilterPlugin{}
	filterPluginRegistry := framework.Registry{filterPlugin.Name(): newPlugin(filterPlugin)}
	filterFramework, err := framework.NewFramework(filterPluginRegistry, &schedulerapi.Plugins{
		Filter: &schedulerapi.PluginSet{
			Enabled: []schedulerapi.Plugin{
				{
					Name: filterPlugin.Name(),
				},
			},
		},
	}, []schedulerapi.PluginConfig{})
	if err != nil {
		t.Errorf("Unexpected error when initialize scheduling framework, err :%v", err.Error())
	}

	tests := []struct {
		name                          string
		predicates                    map[string]algorithmpredicates.FitPredicate
		prioritizers                  []priorities.PriorityConfig
		alwaysCheckAllPredicates      bool
		nodes                         []string
		pvcs                          []v1.PersistentVolumeClaim
		pod                           *v1.Pod
		pods                          []*v1.Pod
		buildPredMeta                 bool // build predicates metadata or not
		filterFailedNodeReturnCodeMap map[string]framework.Code
		expectedHosts                 sets.String
		wErr                          error
	}{
		{
			predicates: map[string]algorithmpredicates.FitPredicate{"false": falsePredicate},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			name:       "test 1",
			wErr: &FitError{
				Pod:         &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
				NumAllNodes: 2,
				FailedPredicates: FailedPredicateMap{
					"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
					"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
				},
				FilteredNodesStatuses: framework.NodeToStatusMap{},
			},
		},
		{
			predicates:    map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			nodes:         []string{"machine1", "machine2"},
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignore", UID: types.UID("ignore")}},
			expectedHosts: sets.NewString("machine1", "machine2"),
			name:          "test 2",
			wErr:          nil,
		},
		{
			// Fits on a machine where the pod ID matches the machine name
			predicates:    map[string]algorithmpredicates.FitPredicate{"matches": matchesPredicate},
			nodes:         []string{"machine1", "machine2"},
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine2", UID: types.UID("machine2")}},
			expectedHosts: sets.NewString("machine2"),
			name:          "test 3",
			wErr:          nil,
		},
		{
			predicates:    map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers:  []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:         []string{"3", "2", "1"},
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignore", UID: types.UID("ignore")}},
			expectedHosts: sets.NewString("3"),
			name:          "test 4",
			wErr:          nil,
		},
		{
			predicates:    map[string]algorithmpredicates.FitPredicate{"matches": matchesPredicate},
			prioritizers:  []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:         []string{"3", "2", "1"},
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			expectedHosts: sets.NewString("2"),
			name:          "test 5",
			wErr:          nil,
		},
		{
			predicates: map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers: []priorities.PriorityConfig{
				{
					Map:    numericMapPriority,
					Weight: 1,
				},
				{
					Map:    numericMapPriority,
					Reduce: reverseNumericReducePriority,
					Weight: 2,
				},
			},
			nodes:         []string{"3", "2", "1"},
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			expectedHosts: sets.NewString("1"),
			name:          "test 6",
			wErr:          nil,
		},
		{
			predicates:   map[string]algorithmpredicates.FitPredicate{"true": truePredicate, "false": falsePredicate},
			prioritizers: []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:        []string{"3", "2", "1"},
			pod:          &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			name:         "test 7",
			wErr: &FitError{
				Pod:         &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
				NumAllNodes: 3,
				FailedPredicates: FailedPredicateMap{
					"3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
					"2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
					"1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
				},
				FilteredNodesStatuses: framework.NodeToStatusMap{},
			},
		},
		{
			predicates: map[string]algorithmpredicates.FitPredicate{
				"nopods":  hasNoPodsPredicate,
				"matches": matchesPredicate,
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")},
					Spec: v1.PodSpec{
						NodeName: "2",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			pod:          &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			prioritizers: []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:        []string{"1", "2"},
			name:         "test 8",
			wErr: &FitError{
				Pod:         &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
				NumAllNodes: 2,
				FailedPredicates: FailedPredicateMap{
					"1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
					"2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate},
				},
				FilteredNodesStatuses: framework.NodeToStatusMap{},
			},
		},
		{
			// Pod with existing PVC
			predicates: map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			nodes:      []string{"machine1", "machine2"},
			pvcs:       []v1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "existingPVC"}}},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ignore", UID: types.UID("ignore")},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "existingPVC",
								},
							},
						},
					},
				},
			},
			expectedHosts: sets.NewString("machine1", "machine2"),
			name:          "existing PVC",
			wErr:          nil,
		},
		{
			// Pod with non existing PVC
			predicates: map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			nodes:      []string{"machine1", "machine2"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ignore", UID: types.UID("ignore")},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "unknownPVC",
								},
							},
						},
					},
				},
			},
			name: "unknown PVC",
			wErr: fmt.Errorf("persistentvolumeclaim \"unknownPVC\" not found"),
		},
		{
			// Pod with deleting PVC
			predicates: map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			nodes:      []string{"machine1", "machine2"},
			pvcs:       []v1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "existingPVC", DeletionTimestamp: &metav1.Time{}}}},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ignore", UID: types.UID("ignore")},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "existingPVC",
								},
							},
						},
					},
				},
			},
			name: "deleted PVC",
			wErr: fmt.Errorf("persistentvolumeclaim \"existingPVC\" is being deleted"),
		},
		{
			// alwaysCheckAllPredicates is true
			predicates:               map[string]algorithmpredicates.FitPredicate{"true": truePredicate, "matches": matchesPredicate, "false": falsePredicate},
			alwaysCheckAllPredicates: true,
			nodes:                    []string{"1"},
			pod:                      &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
			name:                     "test alwaysCheckAllPredicates is true",
			wErr: &FitError{
				Pod:         &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
				NumAllNodes: 1,
				FailedPredicates: FailedPredicateMap{
					"1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrFakePredicate, algorithmpredicates.ErrFakePredicate},
				},
				FilteredNodesStatuses: framework.NodeToStatusMap{},
			},
		},
		{
			predicates:   map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers: []priorities.PriorityConfig{{Map: falseMapPriority, Weight: 1}, {Map: trueMapPriority, Reduce: getNodeReducePriority, Weight: 2}},
			nodes:        []string{"2", "1"},
			pod:          &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2"}},
			name:         "test error with priority map",
			wErr:         errors.NewAggregate([]error{errPrioritize, errPrioritize}),
		},
		{
			name: "test even pods spread predicate - 2 nodes with maxskew=1",
			predicates: map[string]algorithmpredicates.FitPredicate{
				"matches": algorithmpredicates.EvenPodsSpreadPredicate,
			},
			nodes: []string{"machine1", "machine2"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "p", UID: types.UID("p"), Labels: map[string]string{"foo": ""}},
				Spec: v1.PodSpec{
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1"), Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "machine1",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			buildPredMeta: true,
			expectedHosts: sets.NewString("machine2"),
			wErr:          nil,
		},
		{
			name: "test even pods spread predicate - 3 nodes with maxskew=2",
			predicates: map[string]algorithmpredicates.FitPredicate{
				"matches": algorithmpredicates.EvenPodsSpreadPredicate,
			},
			nodes: []string{"machine1", "machine2", "machine3"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "p", UID: types.UID("p"), Labels: map[string]string{"foo": ""}},
				Spec: v1.PodSpec{
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           2,
							TopologyKey:       "hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1a", UID: types.UID("pod1a"), Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "machine1",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1b", UID: types.UID("pod1b"), Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "machine1",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod2", UID: types.UID("pod2"), Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "machine2",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			buildPredMeta: true,
			expectedHosts: sets.NewString("machine2", "machine3"),
			wErr:          nil,
		},
		{
			name:                          "test with filter plugin returning Unschedulable status",
			predicates:                    map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers:                  []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:                         []string{"3"},
			pod:                           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-filter", UID: types.UID("test-filter")}},
			expectedHosts:                 nil,
			filterFailedNodeReturnCodeMap: map[string]framework.Code{"3": framework.Unschedulable},
			wErr: &FitError{
				Pod:              &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-filter", UID: types.UID("test-filter")}},
				NumAllNodes:      1,
				FailedPredicates: FailedPredicateMap{},
				FilteredNodesStatuses: framework.NodeToStatusMap{
					"3": framework.NewStatus(framework.Unschedulable, "injecting failure for pod test-filter"),
				},
			},
		},
		{
			name:                          "test with filter plugin returning UnschedulableAndUnresolvable status",
			predicates:                    map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers:                  []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:                         []string{"3"},
			pod:                           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-filter", UID: types.UID("test-filter")}},
			expectedHosts:                 nil,
			filterFailedNodeReturnCodeMap: map[string]framework.Code{"3": framework.UnschedulableAndUnresolvable},
			wErr: &FitError{
				Pod:              &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-filter", UID: types.UID("test-filter")}},
				NumAllNodes:      1,
				FailedPredicates: FailedPredicateMap{},
				FilteredNodesStatuses: framework.NodeToStatusMap{
					"3": framework.NewStatus(framework.UnschedulableAndUnresolvable, "injecting failure for pod test-filter"),
				},
			},
		},
		{
			name:                          "test with partial failed filter plugin",
			predicates:                    map[string]algorithmpredicates.FitPredicate{"true": truePredicate},
			prioritizers:                  []priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
			nodes:                         []string{"1", "2"},
			pod:                           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-filter", UID: types.UID("test-filter")}},
			expectedHosts:                 nil,
			filterFailedNodeReturnCodeMap: map[string]framework.Code{"1": framework.Unschedulable},
			wErr:                          nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			filterPlugin.failedNodeReturnCodeMap = test.filterFailedNodeReturnCodeMap

			cache := internalcache.New(time.Duration(0), wait.NeverStop)
			for _, pod := range test.pods {
				cache.AddPod(pod)
			}
			for _, name := range test.nodes {
				cache.AddNode(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"hostname": name}}})
			}
			pvcs := []v1.PersistentVolumeClaim{}
			pvcs = append(pvcs, test.pvcs...)

			pvcLister := fakelisters.PersistentVolumeClaimLister(pvcs)

			predMetaProducer := algorithmpredicates.EmptyMetadataProducer
			if test.buildPredMeta {
				f := &algorithmpredicates.MetadataProducerFactory{}
				predMetaProducer = f.GetPredicateMetadata
			}
			scheduler := NewGenericScheduler(
				cache,
				internalqueue.NewSchedulingQueue(nil, nil),
				test.predicates,
				predMetaProducer,
				test.prioritizers,
				priorities.EmptyMetadataProducer,
				emptySnapshot,
				filterFramework,
				[]algorithm.SchedulerExtender{},
				nil,
				pvcLister,
				informerFactory.Policy().V1beta1().PodDisruptionBudgets().Lister(),
				test.alwaysCheckAllPredicates,
				false,
				schedulerapi.DefaultPercentageOfNodesToScore,
				false)
			result, err := scheduler.Schedule(context.Background(), framework.NewCycleState(), test.pod)
			if !reflect.DeepEqual(err, test.wErr) {
				t.Errorf("Unexpected error: %v, expected: %v", err.Error(), test.wErr)
			}
			if test.expectedHosts != nil && !test.expectedHosts.Has(result.SuggestedHost) {
				t.Errorf("Expected: %s, got: %s", test.expectedHosts, result.SuggestedHost)
			}
			if test.wErr == nil && len(test.nodes) != result.EvaluatedNodes {
				t.Errorf("Expected EvaluatedNodes: %d, got: %d", len(test.nodes), result.EvaluatedNodes)
			}

			filterPlugin.reset()
		})
	}
}

// makeScheduler makes a simple genericScheduler for testing.
func makeScheduler(predicates map[string]algorithmpredicates.FitPredicate, nodes []*v1.Node) *genericScheduler {
	cache := internalcache.New(time.Duration(0), wait.NeverStop)
	for _, n := range nodes {
		cache.AddNode(n)
	}

	s := NewGenericScheduler(
		cache,
		internalqueue.NewSchedulingQueue(nil, nil),
		predicates,
		algorithmpredicates.EmptyMetadataProducer,
		nil,
		priorities.EmptyMetadataProducer,
		emptySnapshot,
		emptyFramework,
		nil, nil, nil, nil, false, false,
		schedulerapi.DefaultPercentageOfNodesToScore, false)
	cache.UpdateNodeInfoSnapshot(s.(*genericScheduler).nodeInfoSnapshot)
	return s.(*genericScheduler)

}

func TestFindFitAllError(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()
	predicates := map[string]algorithmpredicates.FitPredicate{"true": truePredicate, "matches": matchesPredicate}
	nodes := makeNodeList([]string{"3", "2", "1"})
	scheduler := makeScheduler(predicates, nodes)

	_, predicateMap, _, err := scheduler.findNodesThatFit(context.Background(), framework.NewCycleState(), &v1.Pod{})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(predicateMap) != len(nodes) {
		t.Errorf("unexpected failed predicate map: %v", predicateMap)
	}

	for _, node := range nodes {
		t.Run(node.Name, func(t *testing.T) {
			failures, found := predicateMap[node.Name]
			if !found {
				t.Errorf("failed to find node in %v", predicateMap)
			}
			if len(failures) != 1 || failures[0] != algorithmpredicates.ErrFakePredicate {
				t.Errorf("unexpected failures: %v", failures)
			}
		})
	}
}

func TestFindFitSomeError(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()
	predicates := map[string]algorithmpredicates.FitPredicate{"true": truePredicate, "matches": matchesPredicate}
	nodes := makeNodeList([]string{"3", "2", "1"})
	scheduler := makeScheduler(predicates, nodes)

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "1", UID: types.UID("1")}}
	_, predicateMap, _, err := scheduler.findNodesThatFit(context.Background(), framework.NewCycleState(), pod)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(predicateMap) != (len(nodes) - 1) {
		t.Errorf("unexpected failed predicate map: %v", predicateMap)
	}

	for _, node := range nodes {
		if node.Name == pod.Name {
			continue
		}
		t.Run(node.Name, func(t *testing.T) {
			failures, found := predicateMap[node.Name]
			if !found {
				t.Errorf("failed to find node in %v", predicateMap)
			}
			if len(failures) != 1 || failures[0] != algorithmpredicates.ErrFakePredicate {
				t.Errorf("unexpected failures: %v", failures)
			}
		})
	}
}

type predicateCallCounter struct {
	count int
}

func (c *predicateCallCounter) truePredicate() algorithmpredicates.FitPredicate {
	return func(pod *v1.Pod, meta algorithmpredicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []algorithmpredicates.PredicateFailureReason, error) {
		c.count++
		return truePredicate(pod, meta, nodeInfo)
	}
}

func TestFindFitPredicateCallCounts(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()

	tests := []struct {
		name          string
		pod           *v1.Pod
		expectedCount int
	}{
		{
			name:          "nominated pods have lower priority, predicate is called once",
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "1", UID: types.UID("1")}, Spec: v1.PodSpec{Priority: &highPriority}},
			expectedCount: 1,
		},
		{
			name:          "nominated pods have higher priority, predicate is called twice",
			pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "1", UID: types.UID("1")}, Spec: v1.PodSpec{Priority: &lowPriority}},
			expectedCount: 2,
		},
	}

	for _, test := range tests {
		pc := predicateCallCounter{}
		predicates := map[string]algorithmpredicates.FitPredicate{"true": pc.truePredicate()}
		nodes := makeNodeList([]string{"1"})

		cache := internalcache.New(time.Duration(0), wait.NeverStop)
		for _, n := range nodes {
			cache.AddNode(n)
		}

		queue := internalqueue.NewSchedulingQueue(nil, nil)
		scheduler := NewGenericScheduler(
			cache,
			queue,
			predicates,
			algorithmpredicates.EmptyMetadataProducer,
			nil,
			priorities.EmptyMetadataProducer,
			emptySnapshot,
			emptyFramework,
			nil, nil, nil, nil, false, false,
			schedulerapi.DefaultPercentageOfNodesToScore, false).(*genericScheduler)
		cache.UpdateNodeInfoSnapshot(scheduler.nodeInfoSnapshot)
		queue.UpdateNominatedPodForNode(&v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("nominated")}, Spec: v1.PodSpec{Priority: &midPriority}}, "1")

		_, _, _, err := scheduler.findNodesThatFit(context.Background(), framework.NewCycleState(), test.pod)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if test.expectedCount != pc.count {
			t.Errorf("predicate was called %d times, expected is %d", pc.count, test.expectedCount)
		}
	}
}

func makeNode(node string, milliCPU, memory int64) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
				"pods":            *resource.NewQuantity(100, resource.DecimalSI),
			},
			Allocatable: v1.ResourceList{

				v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
				"pods":            *resource.NewQuantity(100, resource.DecimalSI),
			},
		},
	}
}

func TestHumanReadableFitError(t *testing.T) {
	err := &FitError{
		Pod:         &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "2", UID: types.UID("2")}},
		NumAllNodes: 3,
		FailedPredicates: FailedPredicateMap{
			"1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderMemoryPressure},
			"2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderDiskPressure},
			"3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderDiskPressure},
		},
	}
	if strings.Contains(err.Error(), "0/3 nodes are available") {
		if strings.Contains(err.Error(), "2 node(s) had disk pressure") && strings.Contains(err.Error(), "1 node(s) had memory pressure") {
			return
		}
	}
	t.Errorf("Error message doesn't have all the information content: [" + err.Error() + "]")
}

// The point of this test is to show that you:
// - get the same priority for a zero-request pod as for a pod with the defaults requests,
//   both when the zero-request pod is already on the machine and when the zero-request pod
//   is the one being scheduled.
// - don't get the same score no matter what we schedule.
func TestZeroRequest(t *testing.T) {
	// A pod with no resources. We expect spreading to count it as having the default resources.
	noResources := v1.PodSpec{
		Containers: []v1.Container{
			{},
		},
	}
	noResources1 := noResources
	noResources1.NodeName = "machine1"
	// A pod with the same resources as a 0-request pod gets by default as its resources (for spreading).
	small := v1.PodSpec{
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU: resource.MustParse(
							strconv.FormatInt(priorityutil.DefaultMilliCPURequest, 10) + "m"),
						v1.ResourceMemory: resource.MustParse(
							strconv.FormatInt(priorityutil.DefaultMemoryRequest, 10)),
					},
				},
			},
		},
	}
	small2 := small
	small2.NodeName = "machine2"
	// A larger pod.
	large := v1.PodSpec{
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU: resource.MustParse(
							strconv.FormatInt(priorityutil.DefaultMilliCPURequest*3, 10) + "m"),
						v1.ResourceMemory: resource.MustParse(
							strconv.FormatInt(priorityutil.DefaultMemoryRequest*3, 10)),
					},
				},
			},
		},
	}
	large1 := large
	large1.NodeName = "machine1"
	large2 := large
	large2.NodeName = "machine2"
	tests := []struct {
		pod           *v1.Pod
		pods          []*v1.Pod
		nodes         []*v1.Node
		name          string
		expectedScore int64
	}{
		// The point of these next two tests is to show you get the same priority for a zero-request pod
		// as for a pod with the defaults requests, both when the zero-request pod is already on the machine
		// and when the zero-request pod is the one being scheduled.
		{
			pod:   &v1.Pod{Spec: noResources},
			nodes: []*v1.Node{makeNode("machine1", 1000, priorityutil.DefaultMemoryRequest*10), makeNode("machine2", 1000, priorityutil.DefaultMemoryRequest*10)},
			name:  "test priority of zero-request pod with machine with zero-request pod",
			pods: []*v1.Pod{
				{Spec: large1}, {Spec: noResources1},
				{Spec: large2}, {Spec: small2},
			},
			expectedScore: 250,
		},
		{
			pod:   &v1.Pod{Spec: small},
			nodes: []*v1.Node{makeNode("machine1", 1000, priorityutil.DefaultMemoryRequest*10), makeNode("machine2", 1000, priorityutil.DefaultMemoryRequest*10)},
			name:  "test priority of nonzero-request pod with machine with zero-request pod",
			pods: []*v1.Pod{
				{Spec: large1}, {Spec: noResources1},
				{Spec: large2}, {Spec: small2},
			},
			expectedScore: 250,
		},
		// The point of this test is to verify that we're not just getting the same score no matter what we schedule.
		{
			pod:   &v1.Pod{Spec: large},
			nodes: []*v1.Node{makeNode("machine1", 1000, priorityutil.DefaultMemoryRequest*10), makeNode("machine2", 1000, priorityutil.DefaultMemoryRequest*10)},
			name:  "test priority of larger pod with machine with zero-request pod",
			pods: []*v1.Pod{
				{Spec: large1}, {Spec: noResources1},
				{Spec: large2}, {Spec: small2},
			},
			expectedScore: 230,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			// This should match the configuration in defaultPriorities() in
			// pkg/scheduler/algorithmprovider/defaults/defaults.go if you want
			// to test what's actually in production.
			priorityConfigs := []priorities.PriorityConfig{
				{Map: priorities.LeastRequestedPriorityMap, Weight: 1},
				{Map: priorities.BalancedResourceAllocationMap, Weight: 1},
			}
			selectorSpreadPriorityMap, selectorSpreadPriorityReduce := priorities.NewSelectorSpreadPriority(
				informerFactory.Core().V1().Services().Lister(),
				informerFactory.Core().V1().ReplicationControllers().Lister(),
				informerFactory.Apps().V1().ReplicaSets().Lister(),
				informerFactory.Apps().V1().StatefulSets().Lister(),
			)
			pc := priorities.PriorityConfig{Map: selectorSpreadPriorityMap, Reduce: selectorSpreadPriorityReduce, Weight: 1}
			priorityConfigs = append(priorityConfigs, pc)

			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, test.nodes))

			metaDataProducer := priorities.NewMetadataFactory(
				informerFactory.Core().V1().Services().Lister(),
				informerFactory.Core().V1().ReplicationControllers().Lister(),
				informerFactory.Apps().V1().ReplicaSets().Lister(),
				informerFactory.Apps().V1().StatefulSets().Lister(),
				1,
			)

			metaData := metaDataProducer(test.pod, test.nodes, snapshot)

			scheduler := NewGenericScheduler(
				nil,
				nil,
				nil,
				nil,
				priorityConfigs,
				metaDataProducer,
				emptySnapshot,
				emptyFramework,
				[]algorithm.SchedulerExtender{},
				nil,
				nil,
				nil,
				false,
				false,
				schedulerapi.DefaultPercentageOfNodesToScore,
				false).(*genericScheduler)
			scheduler.nodeInfoSnapshot = snapshot

			list, err := scheduler.prioritizeNodes(
				context.Background(),
				framework.NewCycleState(),
				test.pod,
				metaData,
				test.nodes,
			)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			for _, hp := range list {
				if hp.Score != test.expectedScore {
					t.Errorf("expected %d for all priorities, got list %#v", test.expectedScore, list)
				}
			}
		})
	}
}

func printNodeToVictims(nodeToVictims map[*v1.Node]*extenderv1.Victims) string {
	var output string
	for node, victims := range nodeToVictims {
		output += node.Name + ": ["
		for _, pod := range victims.Pods {
			output += pod.Name + ", "
		}
		output += "]"
	}
	return output
}

func checkPreemptionVictims(expected map[string]map[string]bool, nodeToPods map[*v1.Node]*extenderv1.Victims) error {
	if len(expected) == len(nodeToPods) {
		for k, victims := range nodeToPods {
			if expPods, ok := expected[k.Name]; ok {
				if len(victims.Pods) != len(expPods) {
					return fmt.Errorf("unexpected number of pods. expected: %v, got: %v", expected, printNodeToVictims(nodeToPods))
				}
				prevPriority := int32(math.MaxInt32)
				for _, p := range victims.Pods {
					// Check that pods are sorted by their priority.
					if *p.Spec.Priority > prevPriority {
						return fmt.Errorf("pod %v of node %v was not sorted by priority", p.Name, k)
					}
					prevPriority = *p.Spec.Priority
					if _, ok := expPods[p.Name]; !ok {
						return fmt.Errorf("pod %v was not expected. Expected: %v", p.Name, expPods)
					}
				}
			} else {
				return fmt.Errorf("unexpected machines. expected: %v, got: %v", expected, printNodeToVictims(nodeToPods))
			}
		}
	} else {
		return fmt.Errorf("unexpected number of machines. expected: %v, got: %v", expected, printNodeToVictims(nodeToPods))
	}
	return nil
}

var smallContainers = []v1.Container{
	{
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				"cpu": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMilliCPURequest, 10) + "m"),
				"memory": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMemoryRequest, 10)),
			},
		},
	},
}
var mediumContainers = []v1.Container{
	{
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				"cpu": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMilliCPURequest*2, 10) + "m"),
				"memory": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMemoryRequest*2, 10)),
			},
		},
	},
}
var largeContainers = []v1.Container{
	{
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				"cpu": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMilliCPURequest*3, 10) + "m"),
				"memory": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMemoryRequest*3, 10)),
			},
		},
	},
}
var veryLargeContainers = []v1.Container{
	{
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				"cpu": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMilliCPURequest*5, 10) + "m"),
				"memory": resource.MustParse(
					strconv.FormatInt(priorityutil.DefaultMemoryRequest*5, 10)),
			},
		},
	},
}
var negPriority, lowPriority, midPriority, highPriority, veryHighPriority = int32(-100), int32(0), int32(100), int32(1000), int32(10000)

var startTime = metav1.Date(2019, 1, 1, 1, 1, 1, 0, time.UTC)

var startTime20190102 = metav1.Date(2019, 1, 2, 1, 1, 1, 0, time.UTC)
var startTime20190103 = metav1.Date(2019, 1, 3, 1, 1, 1, 0, time.UTC)
var startTime20190104 = metav1.Date(2019, 1, 4, 1, 1, 1, 0, time.UTC)
var startTime20190105 = metav1.Date(2019, 1, 5, 1, 1, 1, 0, time.UTC)
var startTime20190106 = metav1.Date(2019, 1, 6, 1, 1, 1, 0, time.UTC)
var startTime20190107 = metav1.Date(2019, 1, 7, 1, 1, 1, 0, time.UTC)

// TestSelectNodesForPreemption tests selectNodesForPreemption. This test assumes
// that podsFitsOnNode works correctly and is tested separately.
func TestSelectNodesForPreemption(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()

	filterPlugin := &FakeFilterPlugin{}
	filterPluginRegistry := framework.Registry{filterPlugin.Name(): newPlugin(filterPlugin)}
	filterFramework, err := framework.NewFramework(filterPluginRegistry, &schedulerapi.Plugins{
		Filter: &schedulerapi.PluginSet{
			Enabled: []schedulerapi.Plugin{
				{
					Name: filterPlugin.Name(),
				},
			},
		},
	}, []schedulerapi.PluginConfig{})
	if err != nil {
		t.Errorf("Unexpected error when initialize scheduling framework, err :%v", err.Error())
	}

	tests := []struct {
		name                    string
		predicates              map[string]algorithmpredicates.FitPredicate
		nodes                   []string
		pod                     *v1.Pod
		pods                    []*v1.Pod
		filterReturnCode        framework.Code
		expected                map[string]map[string]bool // Map from node name to a list of pods names which should be preempted.
		expectednumFilterCalled int32
		addAffinityPredicate    bool
	}{
		{
			name:       "a pod that does not fit on any machine",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": falsePredicate},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "new", UID: types.UID("new")}, Spec: v1.PodSpec{Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{},
			expectednumFilterCalled: 2,
		},
		{
			name:       "a pod that fits with no preemption",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": truePredicate},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "new", UID: types.UID("new")}, Spec: v1.PodSpec{Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {}, "machine2": {}},
			expectednumFilterCalled: 4,
		},
		{
			name:       "a pod that fits on one machine with no preemption",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": matchesPredicate},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {}},
			expectednumFilterCalled: 3,
		},
		{
			name:       "a pod that fits on both machines when lower priority pods are preempted",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {"a": true}, "machine2": {"b": true}},
			expectednumFilterCalled: 4,
		},
		{
			name:       "a pod that would fit on the machines, but other pods running are higher priority",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &lowPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{},
			expectednumFilterCalled: 2,
		},
		{
			name:       "medium priority pod is preempted, but lower priority one stays as it is small",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "c", UID: types.UID("c")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {"b": true}, "machine2": {"c": true}},
			expectednumFilterCalled: 5,
		},
		{
			name:       "mixed priority pods are preempted",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "c", UID: types.UID("c")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "d", UID: types.UID("d")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &highPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "e", UID: types.UID("e")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {"b": true, "c": true}},
			expectednumFilterCalled: 5,
		},
		{
			name:       "mixed priority pods are preempted, pick later StartTime one when priorities are equal",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190107}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190106}},
				{ObjectMeta: metav1.ObjectMeta{Name: "c", UID: types.UID("c")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190105}},
				{ObjectMeta: metav1.ObjectMeta{Name: "d", UID: types.UID("d")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &highPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190104}},
				{ObjectMeta: metav1.ObjectMeta{Name: "e", UID: types.UID("e")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190103}}},
			expected:                map[string]map[string]bool{"machine1": {"a": true, "c": true}},
			expectednumFilterCalled: 5,
		},
		{
			name:       "pod with anti-affinity is preempted",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:   "machine1",
				Labels: map[string]string{"pod": "preemptor"}}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a"), Labels: map[string]string{"service": "securityscan"}}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1", Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "pod",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"preemptor", "value2"},
										},
									},
								},
								TopologyKey: "hostname",
							},
						},
					}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "d", UID: types.UID("d")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &highPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "e", UID: types.UID("e")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}}},
			expected:                map[string]map[string]bool{"machine1": {"a": true}, "machine2": {}},
			expectednumFilterCalled: 4,
			addAffinityPredicate:    true,
		},
		{
			name: "preemption to resolve even pods spread FitError",
			predicates: map[string]algorithmpredicates.FitPredicate{
				"matches": algorithmpredicates.EvenPodsSpreadPredicate,
			},
			nodes: []string{"node-a/zone1", "node-b/zone1", "node-x/zone2"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "p",
					Labels: map[string]string{"foo": ""},
				},
				Spec: v1.PodSpec{
					Priority: &highPriority,
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "zone",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
						{
							MaxSkew:           1,
							TopologyKey:       "hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-a1", UID: types.UID("pod-a1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-a", Priority: &midPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-a2", UID: types.UID("pod-a2"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-a", Priority: &lowPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-b1", UID: types.UID("pod-b1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-b", Priority: &lowPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-x1", UID: types.UID("pod-x1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-x", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-x2", UID: types.UID("pod-x2"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-x", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
			},
			expected: map[string]map[string]bool{
				"node-a": {"pod-a2": true},
				"node-b": {"pod-b1": true},
			},
			expectednumFilterCalled: 6,
		},
		{
			name:       "get Unschedulable in the preemption phase when the filter plugins filtering the nodes",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("a")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("b")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}}},
			filterReturnCode:        framework.Unschedulable,
			expected:                map[string]map[string]bool{},
			expectednumFilterCalled: 2,
		},
	}
	labelKeys := []string{"hostname", "zone", "region"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			filterFailedNodeReturnCodeMap := map[string]framework.Code{}
			cache := internalcache.New(time.Duration(0), wait.NeverStop)
			for _, pod := range test.pods {
				cache.AddPod(pod)
			}
			for _, name := range test.nodes {
				filterFailedNodeReturnCodeMap[name] = test.filterReturnCode
				cache.AddNode(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"hostname": name}}})
			}

			factory := &algorithmpredicates.MetadataProducerFactory{}
			filterPlugin.failedNodeReturnCodeMap = filterFailedNodeReturnCodeMap
			scheduler := NewGenericScheduler(
				nil,
				internalqueue.NewSchedulingQueue(nil, nil),
				test.predicates,
				factory.GetPredicateMetadata,
				nil,
				priorities.EmptyMetadataProducer,
				emptySnapshot,
				filterFramework,
				[]algorithm.SchedulerExtender{},
				nil,
				nil,
				informerFactory.Policy().V1beta1().PodDisruptionBudgets().Lister(),
				false,
				false,
				schedulerapi.DefaultPercentageOfNodesToScore,
				false)
			g := scheduler.(*genericScheduler)

			assignDefaultStartTime(test.pods)

			nodes := []*v1.Node{}
			for _, n := range test.nodes {
				node := makeNode(n, 1000*5, priorityutil.DefaultMemoryRequest*5)
				// if possible, split node name by '/' to form labels in a format of
				// {"hostname": node.Name[0], "zone": node.Name[1], "region": node.Name[2]}
				node.ObjectMeta.Labels = make(map[string]string)
				for i, label := range strings.Split(node.Name, "/") {
					node.ObjectMeta.Labels[labelKeys[i]] = label
				}
				node.Name = node.ObjectMeta.Labels["hostname"]
				nodes = append(nodes, node)
			}
			if test.addAffinityPredicate {
				n := fakelisters.NewNodeInfoLister([]*v1.Node{nodes[0]})
				p := fakelisters.PodLister(test.pods)
				test.predicates[algorithmpredicates.MatchInterPodAffinityPred] = algorithmpredicates.NewPodAffinityPredicate(n, p)
			}

			g.nodeInfoSnapshot = nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, nodes))
			// newnode simulate a case that a new node is added to the cluster, but nodeNameToInfo
			// doesn't have it yet.
			newnode := makeNode("newnode", 1000*5, priorityutil.DefaultMemoryRequest*5)
			newnode.ObjectMeta.Labels = map[string]string{"hostname": "newnode"}
			nodes = append(nodes, newnode)
			state := framework.NewCycleState()
			nodeToPods, err := g.selectNodesForPreemption(context.Background(), state, test.pod, nodes, nil)
			if err != nil {
				t.Error(err)
			}

			if test.expectednumFilterCalled != filterPlugin.numFilterCalled {
				t.Errorf("expected filterPlugin.numFilterCalled is %d,nut got %d", test.expectednumFilterCalled, filterPlugin.numFilterCalled)
			}

			if err := checkPreemptionVictims(test.expected, nodeToPods); err != nil {
				t.Error(err)
			}

			filterPlugin.reset()
		})
	}
}

// TestPickOneNodeForPreemption tests pickOneNodeForPreemption.
func TestPickOneNodeForPreemption(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()
	tests := []struct {
		name       string
		predicates map[string]algorithmpredicates.FitPredicate
		nodes      []string
		pod        *v1.Pod
		pods       []*v1.Pod
		expected   []string // any of the items is valid
	}{
		{
			name:       "No node needs preemption",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}}},
			expected: []string{"machine1"},
		},
		{
			name:       "a pod that fits on both machines when lower priority pods are preempted",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}}},
			expected: []string{"machine1", "machine2"},
		},
		{
			name:       "a pod that fits on a machine with no preemption",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}}},
			expected: []string{"machine3"},
		},
		{
			name:       "machine with min highest priority pod is picked",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
			},
			expected: []string{"machine3"},
		},
		{
			name:       "when highest priorities are the same, minimum sum of priorities is picked",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
			},
			expected: []string{"machine2"},
		},
		{
			name:       "when highest priority and sum are the same, minimum number of pods is picked",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.3", UID: types.UID("m1.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.4", UID: types.UID("m1.4")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &negPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.3", UID: types.UID("m3.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
			},
			expected: []string{"machine2"},
		},
		{
			// pickOneNodeForPreemption adjusts pod priorities when finding the sum of the victims. This
			// test ensures that the logic works correctly.
			name:       "sum of adjusted priorities is considered",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.3", UID: types.UID("m1.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &negPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.3", UID: types.UID("m3.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
			},
			expected: []string{"machine2"},
		},
		{
			name:       "non-overlapping lowest high priority, sum priorities, and number of pods",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3", "machine4"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &veryHighPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.3", UID: types.UID("m1.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.3", UID: types.UID("m3.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.4", UID: types.UID("m3.4")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m4.1", UID: types.UID("m4.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine4"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m4.2", UID: types.UID("m4.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine4"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m4.3", UID: types.UID("m4.3")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine4"}, Status: v1.PodStatus{StartTime: &startTime}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m4.4", UID: types.UID("m4.4")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &negPriority, NodeName: "machine4"}, Status: v1.PodStatus{StartTime: &startTime}},
			},
			expected: []string{"machine1"},
		},
		{
			name:       "same priority, same number of victims, different start time for each machine's pod",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190103}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190103}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190104}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190104}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190102}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190102}},
			},
			expected: []string{"machine2"},
		},
		{
			name:       "same priority, same number of victims, different start time for all pods",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190105}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190103}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190106}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190102}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190104}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190107}},
			},
			expected: []string{"machine3"},
		},
		{
			name:       "different priority, same number of victims, different start time for all pods",
			predicates: map[string]algorithmpredicates.FitPredicate{"matches": algorithmpredicates.PodFitsResources},
			nodes:      []string{"machine1", "machine2", "machine3"},
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "machine1", UID: types.UID("machine1")}, Spec: v1.PodSpec{Containers: veryLargeContainers, Priority: &highPriority}},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190105}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{StartTime: &startTime20190103}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190107}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.2", UID: types.UID("m2.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine2"}, Status: v1.PodStatus{StartTime: &startTime20190102}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &lowPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190104}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.2", UID: types.UID("m3.2")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{StartTime: &startTime20190106}},
			},
			expected: []string{"machine2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var nodes []*v1.Node
			for _, n := range test.nodes {
				nodes = append(nodes, makeNode(n, priorityutil.DefaultMilliCPURequest*5, priorityutil.DefaultMemoryRequest*5))
			}
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, nodes))
			fwk, _ := framework.NewFramework(emptyPluginRegistry, nil, []schedulerapi.PluginConfig{}, framework.WithSnapshotSharedLister(snapshot))

			factory := algorithmpredicates.MetadataProducerFactory{}
			g := &genericScheduler{
				framework:             fwk,
				nodeInfoSnapshot:      snapshot,
				predicates:            test.predicates,
				predicateMetaProducer: factory.GetPredicateMetadata,
			}
			assignDefaultStartTime(test.pods)

			state := framework.NewCycleState()
			candidateNodes, _ := g.selectNodesForPreemption(context.Background(), state, test.pod, nodes, nil)
			node := pickOneNodeForPreemption(candidateNodes)
			found := false
			for _, nodeName := range test.expected {
				if node.Name == nodeName {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("unexpected node: %v", node)
			}
		})
	}
}

func TestNodesWherePreemptionMightHelp(t *testing.T) {
	// Prepare 4 node names.
	nodeNames := make([]string, 0, 4)
	for i := 1; i < 5; i++ {
		nodeNames = append(nodeNames, fmt.Sprintf("machine%d", i))
	}

	tests := []struct {
		name          string
		failedPredMap FailedPredicateMap
		nodesStatuses framework.NodeToStatusMap
		expected      map[string]bool // set of expected node names. Value is ignored.
	}{
		{
			name: "No node should be attempted",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeSelectorNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTaintsTolerationsNotMatch},
				"machine4": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeLabelPresenceViolated},
			},
			expected: map[string]bool{},
		},
		{
			name: "ErrPodAffinityNotMatch should be tried as it indicates that the pod is unschedulable due to inter-pod affinity or anti-affinity",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnschedulable},
			},
			expected: map[string]bool{"machine1": true, "machine4": true},
		},
		{
			name: "pod with both pod affinity and anti-affinity should be tried",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
			},
			expected: map[string]bool{"machine1": true, "machine3": true, "machine4": true},
		},
		{
			name: "ErrPodAffinityRulesNotMatch should not be tried as it indicates that the pod is unschedulable due to inter-pod affinity, but ErrPodAffinityNotMatch should be tried as it indicates that the pod is unschedulable due to inter-pod affinity or anti-affinity",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityRulesNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityNotMatch},
			},
			expected: map[string]bool{"machine2": true, "machine3": true, "machine4": true},
		},
		{
			name: "Mix of failed predicates works fine",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeSelectorNotMatch, algorithmpredicates.ErrNodeUnderDiskPressure, algorithmpredicates.NewInsufficientResourceError(v1.ResourceMemory, 1000, 500, 300)},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName, algorithmpredicates.ErrDiskConflict},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.NewInsufficientResourceError(v1.ResourceMemory, 1000, 600, 400)},
				"machine4": []algorithmpredicates.PredicateFailureReason{},
			},
			expected: map[string]bool{"machine3": true, "machine4": true},
		},
		{
			name: "Node condition errors should be considered unresolvable",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderDiskPressure},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderPIDPressure},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnderMemoryPressure},
			},
			expected: map[string]bool{"machine4": true},
		},
		{
			name: "Node condition errors and ErrNodeUnknownCondition should be considered unresolvable",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeNotReady},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeNetworkUnavailable},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrNodeUnknownCondition},
			},
			expected: map[string]bool{"machine4": true},
		},
		{
			name: "ErrVolume... errors should not be tried as it indicates that the pod is unschedulable due to no matching volumes for pod on node",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrVolumeZoneConflict},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrVolumeNodeConflict},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrVolumeBindConflict},
			},
			expected: map[string]bool{"machine4": true},
		},
		{
			name: "ErrTopologySpreadConstraintsNotMatch should be tried as it indicates that the pod is unschedulable due to topology spread constraints",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTopologySpreadConstraintsNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTopologySpreadConstraintsNotMatch},
			},
			expected: map[string]bool{"machine1": true, "machine3": true, "machine4": true},
		},
		{
			name:          "UnschedulableAndUnresolvable status should be skipped but Unschedulable should be tried",
			failedPredMap: FailedPredicateMap{},
			nodesStatuses: framework.NodeToStatusMap{
				"machine2": framework.NewStatus(framework.UnschedulableAndUnresolvable, ""),
				"machine3": framework.NewStatus(framework.Unschedulable, ""),
				"machine4": framework.NewStatus(framework.UnschedulableAndUnresolvable, ""),
			},
			expected: map[string]bool{"machine1": true, "machine3": true},
		},
		{
			name: "Failed predicates and statuses should be evaluated",
			failedPredMap: FailedPredicateMap{
				"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityNotMatch},
				"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodAffinityNotMatch},
				"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
				"machine4": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrPodNotMatchHostName},
			},
			nodesStatuses: framework.NodeToStatusMap{
				"machine1": framework.NewStatus(framework.Unschedulable, ""),
				"machine2": framework.NewStatus(framework.UnschedulableAndUnresolvable, ""),
				"machine3": framework.NewStatus(framework.Unschedulable, ""),
				"machine4": framework.NewStatus(framework.UnschedulableAndUnresolvable, ""),
			},
			expected: map[string]bool{"machine1": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fitErr := FitError{
				FailedPredicates:      test.failedPredMap,
				FilteredNodesStatuses: test.nodesStatuses,
			}
			nodes := nodesWherePreemptionMightHelp(nodeinfosnapshot.CreateNodeInfoMap(nil, makeNodeList(nodeNames)), &fitErr)
			if len(test.expected) != len(nodes) {
				t.Errorf("number of nodes is not the same as expected. exptectd: %d, got: %d. Nodes: %v", len(test.expected), len(nodes), nodes)
			}
			for _, node := range nodes {
				if _, found := test.expected[node.Name]; !found {
					t.Errorf("node %v is not expected.", node.Name)
				}
			}
		})
	}
}

func TestPreempt(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()
	defaultFailedPredMap := FailedPredicateMap{
		"machine1": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.NewInsufficientResourceError(v1.ResourceMemory, 1000, 500, 300)},
		"machine2": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrDiskConflict},
		"machine3": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.NewInsufficientResourceError(v1.ResourceMemory, 1000, 600, 400)},
	}
	// Prepare 3 node names.
	defaultNodeNames := []string{}
	for i := 1; i < 4; i++ {
		defaultNodeNames = append(defaultNodeNames, fmt.Sprintf("machine%d", i))
	}
	var (
		preemptLowerPriority = v1.PreemptLowerPriority
		preemptNever         = v1.PreemptNever
	)
	tests := []struct {
		name          string
		pod           *v1.Pod
		pods          []*v1.Pod
		extenders     []*FakeExtender
		failedPredMap FailedPredicateMap
		nodeNames     []string
		predicate     algorithmpredicates.FitPredicate
		buildPredMeta bool
		expectedNode  string
		expectedPods  []string // list of preempted pods
	}{
		{
			name: "basic preemption logic",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			expectedNode: "machine1",
			expectedPods: []string{"m1.1", "m1.2"},
		},
		{
			name: "One node doesn't need any preemption",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			expectedNode: "machine3",
			expectedPods: []string{},
		},
		{
			name: "preemption for topology spread constraints",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "p",
					Labels: map[string]string{"foo": ""},
				},
				Spec: v1.PodSpec{
					Priority: &highPriority,
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "zone",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
						{
							MaxSkew:           1,
							TopologyKey:       "hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-a1", UID: types.UID("pod-a1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-a", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-a2", UID: types.UID("pod-a2"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-a", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-b1", UID: types.UID("pod-b1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-b", Priority: &lowPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-x1", UID: types.UID("pod-x1"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-x", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-x2", UID: types.UID("pod-x2"), Labels: map[string]string{"foo": ""}},
					Spec:       v1.PodSpec{NodeName: "node-x", Priority: &highPriority},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
			},
			failedPredMap: FailedPredicateMap{
				"node-a": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTopologySpreadConstraintsNotMatch},
				"node-b": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTopologySpreadConstraintsNotMatch},
				"node-x": []algorithmpredicates.PredicateFailureReason{algorithmpredicates.ErrTopologySpreadConstraintsNotMatch},
			},
			predicate:     algorithmpredicates.EvenPodsSpreadPredicate,
			buildPredMeta: true,
			nodeNames:     []string{"node-a/zone1", "node-b/zone1", "node-x/zone2"},
			expectedNode:  "node-b",
			expectedPods:  []string{"pod-b1"},
		},
		{
			name: "Scheduler extenders allow only machine1, otherwise machine3 would have been chosen",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			extenders: []*FakeExtender{
				{
					predicates: []fitPredicate{truePredicateExtender},
				},
				{
					predicates: []fitPredicate{machine1PredicateExtender},
				},
			},
			expectedNode: "machine1",
			expectedPods: []string{"m1.1", "m1.2"},
		},
		{
			name: "Scheduler extenders do not allow any preemption",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			extenders: []*FakeExtender{
				{
					predicates: []fitPredicate{falsePredicateExtender},
				},
			},
			expectedNode: "",
			expectedPods: []string{},
		},
		{
			name: "One scheduler extender allows only machine1, the other returns error but ignorable. Only machine1 would be chosen",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			extenders: []*FakeExtender{
				{
					predicates: []fitPredicate{errorPredicateExtender},
					ignorable:  true,
				},
				{
					predicates: []fitPredicate{machine1PredicateExtender},
				},
			},
			expectedNode: "machine1",
			expectedPods: []string{"m1.1", "m1.2"},
		},
		{
			name: "One scheduler extender allows only machine1, but it is not interested in given pod, otherwise machine1 would have been chosen",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptLowerPriority},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &midPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},

				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &midPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			extenders: []*FakeExtender{
				{
					predicates:   []fitPredicate{machine1PredicateExtender},
					unInterested: true,
				},
				{
					predicates: []fitPredicate{truePredicateExtender},
				},
			},
			expectedNode: "machine3",
			expectedPods: []string{},
		},
		{
			name: "no preempting in pod",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: &preemptNever},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			expectedNode: "",
			expectedPods: nil,
		},
		{
			name: "PreemptionPolicy is nil",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", UID: types.UID("pod1")}, Spec: v1.PodSpec{
				Containers:       veryLargeContainers,
				Priority:         &highPriority,
				PreemptionPolicy: nil},
			},
			pods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.1", UID: types.UID("m1.1")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m1.2", UID: types.UID("m1.2")}, Spec: v1.PodSpec{Containers: smallContainers, Priority: &lowPriority, NodeName: "machine1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m2.1", UID: types.UID("m2.1")}, Spec: v1.PodSpec{Containers: largeContainers, Priority: &highPriority, NodeName: "machine2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
				{ObjectMeta: metav1.ObjectMeta{Name: "m3.1", UID: types.UID("m3.1")}, Spec: v1.PodSpec{Containers: mediumContainers, Priority: &midPriority, NodeName: "machine3"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			expectedNode: "machine1",
			expectedPods: []string{"m1.1", "m1.2"},
		},
	}

	labelKeys := []string{"hostname", "zone", "region"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			t.Logf("===== Running test %v", t.Name())
			stop := make(chan struct{})
			cache := internalcache.New(time.Duration(0), stop)
			for _, pod := range test.pods {
				cache.AddPod(pod)
			}
			cachedNodeInfoMap := map[string]*schedulernodeinfo.NodeInfo{}
			nodeNames := defaultNodeNames
			if len(test.nodeNames) != 0 {
				nodeNames = test.nodeNames
			}
			for i, name := range nodeNames {
				node := makeNode(name, 1000*5, priorityutil.DefaultMemoryRequest*5)
				// if possible, split node name by '/' to form labels in a format of
				// {"hostname": node.Name[0], "zone": node.Name[1], "region": node.Name[2]}
				node.ObjectMeta.Labels = make(map[string]string)
				for i, label := range strings.Split(node.Name, "/") {
					node.ObjectMeta.Labels[labelKeys[i]] = label
				}
				node.Name = node.ObjectMeta.Labels["hostname"]
				cache.AddNode(node)
				nodeNames[i] = node.Name

				// Set nodeInfo to extenders to mock extenders' cache for preemption.
				cachedNodeInfo := schedulernodeinfo.NewNodeInfo()
				cachedNodeInfo.SetNode(node)
				cachedNodeInfoMap[node.Name] = cachedNodeInfo
			}
			extenders := []algorithm.SchedulerExtender{}
			for _, extender := range test.extenders {
				// Set nodeInfoMap as extenders cached node information.
				extender.cachedNodeNameToInfo = cachedNodeInfoMap
				extenders = append(extenders, extender)
			}
			predicate := algorithmpredicates.PodFitsResources
			if test.predicate != nil {
				predicate = test.predicate
			}
			predMetaProducer := algorithmpredicates.EmptyMetadataProducer
			if test.buildPredMeta {
				f := &algorithmpredicates.MetadataProducerFactory{}
				predMetaProducer = f.GetPredicateMetadata
			}
			scheduler := NewGenericScheduler(
				cache,
				internalqueue.NewSchedulingQueue(nil, nil),
				map[string]algorithmpredicates.FitPredicate{"matches": predicate},
				predMetaProducer,
				[]priorities.PriorityConfig{{Map: numericMapPriority, Weight: 1}},
				priorities.EmptyMetadataProducer,
				emptySnapshot,
				emptyFramework,
				extenders,
				nil,
				informerFactory.Core().V1().PersistentVolumeClaims().Lister(),
				informerFactory.Policy().V1beta1().PodDisruptionBudgets().Lister(),
				false,
				false,
				schedulerapi.DefaultPercentageOfNodesToScore,
				true)
			state := framework.NewCycleState()
			scheduler.(*genericScheduler).snapshot()
			// Call Preempt and check the expected results.
			failedPredMap := defaultFailedPredMap
			if test.failedPredMap != nil {
				failedPredMap = test.failedPredMap
			}
			node, victims, _, err := scheduler.Preempt(context.Background(), state, test.pod, error(&FitError{Pod: test.pod, FailedPredicates: failedPredMap}))
			if err != nil {
				t.Errorf("unexpected error in preemption: %v", err)
			}
			if node != nil && node.Name != test.expectedNode {
				t.Errorf("expected node: %v, got: %v", test.expectedNode, node.GetName())
			}
			if node == nil && len(test.expectedNode) != 0 {
				t.Errorf("expected node: %v, got: nothing", test.expectedNode)
			}
			if len(victims) != len(test.expectedPods) {
				t.Errorf("expected %v pods, got %v.", len(test.expectedPods), len(victims))
			}
			for _, victim := range victims {
				found := false
				for _, expPod := range test.expectedPods {
					if expPod == victim.Name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("pod %v is not expected to be a victim.", victim.Name)
				}
				// Mark the victims for deletion and record the preemptor's nominated node name.
				now := metav1.Now()
				victim.DeletionTimestamp = &now
				test.pod.Status.NominatedNodeName = node.Name
			}
			// Call preempt again and make sure it doesn't preempt any more pods.
			node, victims, _, err = scheduler.Preempt(context.Background(), state, test.pod, error(&FitError{Pod: test.pod, FailedPredicates: failedPredMap}))
			if err != nil {
				t.Errorf("unexpected error in preemption: %v", err)
			}
			if node != nil && len(victims) > 0 {
				t.Errorf("didn't expect any more preemption. Node %v is selected for preemption.", node)
			}
			close(stop)
		})
	}
}

func TestNumFeasibleNodesToFind(t *testing.T) {
	tests := []struct {
		name                     string
		percentageOfNodesToScore int32
		numAllNodes              int32
		wantNumNodes             int32
	}{
		{
			name:         "not set percentageOfNodesToScore and nodes number not more than 50",
			numAllNodes:  10,
			wantNumNodes: 10,
		},
		{
			name:                     "set percentageOfNodesToScore and nodes number not more than 50",
			percentageOfNodesToScore: 40,
			numAllNodes:              10,
			wantNumNodes:             10,
		},
		{
			name:         "not set percentageOfNodesToScore and nodes number more than 50",
			numAllNodes:  1000,
			wantNumNodes: 420,
		},
		{
			name:                     "set percentageOfNodesToScore and nodes number more than 50",
			percentageOfNodesToScore: 40,
			numAllNodes:              1000,
			wantNumNodes:             400,
		},
		{
			name:         "not set percentageOfNodesToScore and nodes number more than 50*125",
			numAllNodes:  6000,
			wantNumNodes: 300,
		},
		{
			name:                     "set percentageOfNodesToScore and nodes number more than 50*125",
			percentageOfNodesToScore: 40,
			numAllNodes:              6000,
			wantNumNodes:             2400,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &genericScheduler{
				percentageOfNodesToScore: tt.percentageOfNodesToScore,
			}
			if gotNumNodes := g.numFeasibleNodesToFind(tt.numAllNodes); gotNumNodes != tt.wantNumNodes {
				t.Errorf("genericScheduler.numFeasibleNodesToFind() = %v, want %v", gotNumNodes, tt.wantNumNodes)
			}
		})
	}
}

func assignDefaultStartTime(pods []*v1.Pod) {
	now := metav1.Now()
	for i := range pods {
		pod := pods[i]
		if pod.Status.StartTime == nil {
			pod.Status.StartTime = &now
		}
	}
}

func TestFairEvaluationForNodes(t *testing.T) {
	defer algorithmpredicates.SetPredicatesOrderingDuringTest(order)()
	predicates := map[string]algorithmpredicates.FitPredicate{"true": truePredicate}
	numAllNodes := 500
	nodeNames := make([]string, 0, numAllNodes)
	for i := 0; i < numAllNodes; i++ {
		nodeNames = append(nodeNames, strconv.Itoa(i))
	}
	nodes := makeNodeList(nodeNames)
	g := makeScheduler(predicates, nodes)
	// To make numAllNodes % nodesToFind != 0
	g.percentageOfNodesToScore = 30
	nodesToFind := int(g.numFeasibleNodesToFind(int32(numAllNodes)))

	// Iterating over all nodes more than twice
	for i := 0; i < 2*(numAllNodes/nodesToFind+1); i++ {
		nodesThatFit, _, _, err := g.findNodesThatFit(context.Background(), framework.NewCycleState(), &v1.Pod{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(nodesThatFit) != nodesToFind {
			t.Errorf("got %d nodes filtered, want %d", len(nodesThatFit), nodesToFind)
		}
		if g.nextStartNodeIndex != (i+1)*nodesToFind%numAllNodes {
			t.Errorf("got %d lastProcessedNodeIndex, want %d", g.nextStartNodeIndex, (i+1)*nodesToFind%numAllNodes)
		}
	}
}
