/*
Copyright 2015 The Kubernetes Authors.

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

package cmd

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/kubectl/cmd/templates"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	"k8s.io/kubernetes/pkg/kubectl/util/i18n"
)

type DrainOptions struct {
	client             kubernetes.Interface
	restClient         *restclient.RESTClient
	Factory            cmdutil.Factory
	Force              bool
	DryRun             bool
	GracePeriodSeconds int
	IgnoreDaemonsets   bool
	Timeout            time.Duration
	backOff            clockwork.Clock
	DeleteLocalData    bool
	Selector           string
	mapper             meta.RESTMapper
	nodeInfos          []*resource.Info
	Out                io.Writer
	ErrOut             io.Writer
	typer              runtime.ObjectTyper
}

// Takes a pod and returns a bool indicating whether or not to operate on the
// pod, an optional warning message, and an optional fatal error.
type podFilter func(corev1.Pod) (include bool, w *warning, f *fatal)
type warning struct {
	string
}
type fatal struct {
	string
}

const (
	EvictionKind        = "Eviction"
	EvictionSubresource = "pods/eviction"

	kDaemonsetFatal      = "DaemonSet-managed pods (use --ignore-daemonsets to ignore)"
	kDaemonsetWarning    = "Ignoring DaemonSet-managed pods"
	kLocalStorageFatal   = "pods with local storage (use --delete-local-data to override)"
	kLocalStorageWarning = "Deleting pods with local storage"
	kUnmanagedFatal      = "pods not managed by ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet (use --force to override)"
	kUnmanagedWarning    = "Deleting pods not managed by ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet"
)

var (
	cordon_long = templates.LongDesc(i18n.T(`
		Mark node as unschedulable.`))

	cordon_example = templates.Examples(i18n.T(`
		# Mark node "foo" as unschedulable.
		kubectl cordon foo`))
)

func NewCmdCordon(f cmdutil.Factory, out io.Writer) *cobra.Command {
	options := &DrainOptions{Factory: f, Out: out}

	cmd := &cobra.Command{
		Use:     "cordon NODE",
		Short:   i18n.T("Mark node as unschedulable"),
		Long:    cordon_long,
		Example: cordon_example,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.SetupDrain(cmd, args))
			cmdutil.CheckErr(options.RunCordonOrUncordon(true))
		},
	}
	cmd.Flags().StringVarP(&options.Selector, "selector", "l", options.Selector, "Selector (label query) to filter on")
	cmdutil.AddDryRunFlag(cmd)
	return cmd
}

var (
	uncordon_long = templates.LongDesc(i18n.T(`
		Mark node as schedulable.`))

	uncordon_example = templates.Examples(i18n.T(`
		# Mark node "foo" as schedulable.
		$ kubectl uncordon foo`))
)

func NewCmdUncordon(f cmdutil.Factory, out io.Writer) *cobra.Command {
	options := &DrainOptions{Factory: f, Out: out}

	cmd := &cobra.Command{
		Use:     "uncordon NODE",
		Short:   i18n.T("Mark node as schedulable"),
		Long:    uncordon_long,
		Example: uncordon_example,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.SetupDrain(cmd, args))
			cmdutil.CheckErr(options.RunCordonOrUncordon(false))
		},
	}
	cmd.Flags().StringVarP(&options.Selector, "selector", "l", options.Selector, "Selector (label query) to filter on")
	cmdutil.AddDryRunFlag(cmd)
	return cmd
}

var (
	drain_long = templates.LongDesc(i18n.T(`
		Drain node in preparation for maintenance.

		The given node will be marked unschedulable to prevent new pods from arriving.
		'drain' evicts the pods if the APIServer supports eviction
		(http://kubernetes.io/docs/admin/disruptions/). Otherwise, it will use normal DELETE
		to delete the pods.
		The 'drain' evicts or deletes all pods except mirror pods (which cannot be deleted through
		the API server).  If there are DaemonSet-managed pods, drain will not proceed
		without --ignore-daemonsets, and regardless it will not delete any
		DaemonSet-managed pods, because those pods would be immediately replaced by the
		DaemonSet controller, which ignores unschedulable markings.  If there are any
		pods that are neither mirror pods nor managed by ReplicationController,
		ReplicaSet, DaemonSet, StatefulSet or Job, then drain will not delete any pods unless you
		use --force.  --force will also allow deletion to proceed if the managing resource of one
		or more pods is missing.

		'drain' waits for graceful termination. You should not operate on the machine until
		the command completes.

		When you are ready to put the node back into service, use kubectl uncordon, which
		will make the node schedulable again.

		![Workflow](http://kubernetes.io/images/docs/kubectl_drain.svg)`))

	drain_example = templates.Examples(i18n.T(`
		# Drain node "foo", even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet on it.
		$ kubectl drain foo --force

		# As above, but abort if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet, and use a grace period of 15 minutes.
		$ kubectl drain foo --grace-period=900`))
)

func NewCmdDrain(f cmdutil.Factory, out, errOut io.Writer) *cobra.Command {
	options := &DrainOptions{Factory: f, Out: out, ErrOut: errOut, backOff: clockwork.NewRealClock()}

	cmd := &cobra.Command{
		Use:     "drain NODE",
		Short:   i18n.T("Drain node in preparation for maintenance"),
		Long:    drain_long,
		Example: drain_example,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.SetupDrain(cmd, args))
			cmdutil.CheckErr(options.RunDrain())
		},
	}
	cmd.Flags().BoolVar(&options.Force, "force", false, "Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet.")
	cmd.Flags().BoolVar(&options.IgnoreDaemonsets, "ignore-daemonsets", false, "Ignore DaemonSet-managed pods.")
	cmd.Flags().BoolVar(&options.DeleteLocalData, "delete-local-data", false, "Continue even if there are pods using emptyDir (local data that will be deleted when the node is drained).")
	cmd.Flags().IntVar(&options.GracePeriodSeconds, "grace-period", -1, "Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used.")
	cmd.Flags().DurationVar(&options.Timeout, "timeout", 0, "The length of time to wait before giving up, zero means infinite")
	cmd.Flags().StringVarP(&options.Selector, "selector", "l", options.Selector, "Selector (label query) to filter on")
	cmdutil.AddDryRunFlag(cmd)
	return cmd
}

// SetupDrain populates some fields from the factory, grabs command line
// arguments and looks up the node using Builder
func (o *DrainOptions) SetupDrain(cmd *cobra.Command, args []string) error {
	var err error
	o.Selector = cmdutil.GetFlagString(cmd, "selector")

	if len(args) == 0 && !cmd.Flags().Changed("selector") {
		return cmdutil.UsageErrorf(cmd, fmt.Sprintf("USAGE: %s [flags]", cmd.Use))
	}
	if len(args) > 0 && len(o.Selector) > 0 {
		return cmdutil.UsageErrorf(cmd, "error: cannot specify both a node name and a --selector option")
	}
	if len(args) > 0 && len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, fmt.Sprintf("USAGE: %s [flags]", cmd.Use))
	}

	o.DryRun = cmdutil.GetFlagBool(cmd, "dry-run")

	if o.client, err = o.Factory.KubernetesClientSet(); err != nil {
		return err
	}

	o.restClient, err = o.Factory.RESTClient()
	if err != nil {
		return err
	}

	o.nodeInfos = []*resource.Info{}
	o.mapper, o.typer = o.Factory.Object()

	cmdNamespace, _, err := o.Factory.DefaultNamespace()
	if err != nil {
		return err
	}

	builder := o.Factory.NewBuilder().
		Internal().
		NamespaceParam(cmdNamespace).DefaultNamespace().
		ResourceNames("nodes", args...).
		SingleResourceType().
		Flatten()

	if len(o.Selector) > 0 {
		builder = builder.LabelSelectorParam(o.Selector).
			ResourceTypes("nodes")
	}

	r := builder.Do()

	if err = r.Err(); err != nil {
		return err
	}

	return r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		if info.Mapping.Resource != "nodes" {
			return fmt.Errorf("error: expected resource of type node, got %q", info.Mapping.Resource)
		}

		o.nodeInfos = append(o.nodeInfos, info)
		return nil
	})
}

// RunDrain runs the 'drain' command
func (o *DrainOptions) RunDrain() error {
	if err := o.RunCordonOrUncordon(true); err != nil {
		return err
	}

	drainedNodes := sets.NewString()
	var fatal error

	for _, info := range o.nodeInfos {
		var err error
		if !o.DryRun {
			err = o.deleteOrEvictPodsSimple(info)
		}
		if err == nil || o.DryRun {
			drainedNodes.Insert(info.Name)
			o.Factory.PrintSuccess(o.mapper, false, o.Out, "node", info.Name, o.DryRun, "drained")
		} else {
			fmt.Fprintf(o.ErrOut, "error: unable to drain node %q, aborting command...\n\n", info.Name)
			remainingNodes := []string{}
			fatal = err
			for _, remainingInfo := range o.nodeInfos {
				if drainedNodes.Has(remainingInfo.Name) {
					continue
				}
				remainingNodes = append(remainingNodes, remainingInfo.Name)
			}

			if len(remainingNodes) > 0 {
				fmt.Fprintf(o.ErrOut, "There are pending nodes to be drained:\n")
				for _, nodeName := range remainingNodes {
					fmt.Fprintf(o.ErrOut, " %s\n", nodeName)
				}
			}
			break
		}
	}

	return fatal
}

func (o *DrainOptions) deleteOrEvictPodsSimple(nodeInfo *resource.Info) error {
	pods, err := o.getPodsForDeletion(nodeInfo)
	if err != nil {
		return err
	}

	err = o.deleteOrEvictPods(pods)
	if err != nil {
		pendingPods, newErr := o.getPodsForDeletion(nodeInfo)
		if newErr != nil {
			return newErr
		}
		fmt.Fprintf(o.ErrOut, "There are pending pods in node %q when an error occurred: %v\n", nodeInfo.Name, err)
		for _, pendingPod := range pendingPods {
			fmt.Fprintf(o.ErrOut, "%s/%s\n", "pod", pendingPod.Name)
		}
	}
	return err
}

func (o *DrainOptions) getController(namespace string, controllerRef *metav1.OwnerReference) (interface{}, error) {
	switch controllerRef.Kind {
	case "ReplicationController":
		return o.client.Core().ReplicationControllers(namespace).Get(controllerRef.Name, metav1.GetOptions{})
	case "DaemonSet":
		return o.client.Extensions().DaemonSets(namespace).Get(controllerRef.Name, metav1.GetOptions{})
	case "Job":
		return o.client.Batch().Jobs(namespace).Get(controllerRef.Name, metav1.GetOptions{})
	case "ReplicaSet":
		return o.client.Extensions().ReplicaSets(namespace).Get(controllerRef.Name, metav1.GetOptions{})
	case "StatefulSet":
		return o.client.AppsV1beta1().StatefulSets(namespace).Get(controllerRef.Name, metav1.GetOptions{})
	}
	return nil, fmt.Errorf("Unknown controller kind %q", controllerRef.Kind)
}

func (o *DrainOptions) getPodController(pod corev1.Pod) (*metav1.OwnerReference, error) {
	controllerRef := metav1.GetControllerOf(&pod)
	if controllerRef == nil {
		return nil, nil
	}

	// We assume the only reason for an error is because the controller is
	// gone/missing, not for any other cause.
	// TODO(mml): something more sophisticated than this
	// TODO(juntee): determine if it's safe to remove getController(),
	// so that drain can work for controller types that we don't know about
	_, err := o.getController(pod.Namespace, controllerRef)
	if err != nil {
		return nil, err
	}
	return controllerRef, nil
}

func (o *DrainOptions) unreplicatedFilter(pod corev1.Pod) (bool, *warning, *fatal) {
	// any finished pod can be removed
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return true, nil, nil
	}

	controllerRef, err := o.getPodController(pod)
	if err != nil {
		// if we're forcing, remove orphaned pods with a warning
		if apierrors.IsNotFound(err) && o.Force {
			return true, &warning{err.Error()}, nil
		}
		return false, nil, &fatal{err.Error()}
	}
	if controllerRef != nil {
		return true, nil, nil
	}
	if !o.Force {
		return false, nil, &fatal{kUnmanagedFatal}
	}
	return true, &warning{kUnmanagedWarning}, nil
}

func (o *DrainOptions) daemonsetFilter(pod corev1.Pod) (bool, *warning, *fatal) {
	// Note that we return false in cases where the pod is DaemonSet managed,
	// regardless of flags.  We never delete them, the only question is whether
	// their presence constitutes an error.
	//
	// The exception is for pods that are orphaned (the referencing
	// management resource - including DaemonSet - is not found).
	// Such pods will be deleted if --force is used.
	controllerRef, err := o.getPodController(pod)
	if err != nil {
		// if we're forcing, remove orphaned pods with a warning
		if apierrors.IsNotFound(err) && o.Force {
			return true, &warning{err.Error()}, nil
		}
		return false, nil, &fatal{err.Error()}
	}
	if controllerRef == nil || controllerRef.Kind != "DaemonSet" {
		return true, nil, nil
	}
	if _, err := o.client.Extensions().DaemonSets(pod.Namespace).Get(controllerRef.Name, metav1.GetOptions{}); err != nil {
		return false, nil, &fatal{err.Error()}
	}
	if !o.IgnoreDaemonsets {
		return false, nil, &fatal{kDaemonsetFatal}
	}
	return false, &warning{kDaemonsetWarning}, nil
}

func mirrorPodFilter(pod corev1.Pod) (bool, *warning, *fatal) {
	if _, found := pod.ObjectMeta.Annotations[corev1.MirrorPodAnnotationKey]; found {
		return false, nil, nil
	}
	return true, nil, nil
}

func hasLocalStorage(pod corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return true
		}
	}

	return false
}

func (o *DrainOptions) localStorageFilter(pod corev1.Pod) (bool, *warning, *fatal) {
	if !hasLocalStorage(pod) {
		return true, nil, nil
	}
	if !o.DeleteLocalData {
		return false, nil, &fatal{kLocalStorageFatal}
	}
	return true, &warning{kLocalStorageWarning}, nil
}

// Map of status message to a list of pod names having that status.
type podStatuses map[string][]string

func (ps podStatuses) Message() string {
	msgs := []string{}

	for key, pods := range ps {
		msgs = append(msgs, fmt.Sprintf("%s: %s", key, strings.Join(pods, ", ")))
	}
	return strings.Join(msgs, "; ")
}

// getPodsForDeletion receives resource info for a node, and returns all the pods from the given node that we
// are planning on deleting. If there are any pods preventing us from deleting, we return that list in an error.
func (o *DrainOptions) getPodsForDeletion(nodeInfo *resource.Info) (pods []corev1.Pod, err error) {
	podList, err := o.client.Core().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeInfo.Name}).String()})
	if err != nil {
		return pods, err
	}

	ws := podStatuses{}
	fs := podStatuses{}

	for _, pod := range podList.Items {
		podOk := true
		for _, filt := range []podFilter{mirrorPodFilter, o.localStorageFilter, o.unreplicatedFilter, o.daemonsetFilter} {
			filterOk, w, f := filt(pod)

			podOk = podOk && filterOk
			if w != nil {
				ws[w.string] = append(ws[w.string], pod.Name)
			}
			if f != nil {
				fs[f.string] = append(fs[f.string], pod.Name)
			}
		}
		if podOk {
			pods = append(pods, pod)
		}
	}

	if len(fs) > 0 {
		return []corev1.Pod{}, errors.New(fs.Message())
	}
	if len(ws) > 0 {
		fmt.Fprintf(o.ErrOut, "WARNING: %s\n", ws.Message())
	}
	return pods, nil
}

func (o *DrainOptions) deletePod(pod corev1.Pod) error {
	deleteOptions := &metav1.DeleteOptions{}
	if o.GracePeriodSeconds >= 0 {
		gracePeriodSeconds := int64(o.GracePeriodSeconds)
		deleteOptions.GracePeriodSeconds = &gracePeriodSeconds
	}
	return o.client.Core().Pods(pod.Namespace).Delete(pod.Name, deleteOptions)
}

func (o *DrainOptions) evictPod(pod corev1.Pod, policyGroupVersion string) error {
	deleteOptions := &metav1.DeleteOptions{}
	if o.GracePeriodSeconds >= 0 {
		gracePeriodSeconds := int64(o.GracePeriodSeconds)
		deleteOptions.GracePeriodSeconds = &gracePeriodSeconds
	}
	eviction := &policyv1beta1.Eviction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyGroupVersion,
			Kind:       EvictionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: deleteOptions,
	}
	// Remember to change change the URL manipulation func when Evction's version change
	return o.client.Policy().Evictions(eviction.Namespace).Evict(eviction)
}

// deleteOrEvictPods deletes or evicts the pods on the api server
func (o *DrainOptions) deleteOrEvictPods(pods []corev1.Pod) error {
	if len(pods) == 0 {
		return nil
	}

	policyGroupVersion, err := SupportEviction(o.client)
	if err != nil {
		return err
	}

	getPodFn := func(namespace, name string) (*corev1.Pod, error) {
		return o.client.Core().Pods(namespace).Get(name, metav1.GetOptions{})
	}

	if len(policyGroupVersion) > 0 {
		return o.evictPods(pods, policyGroupVersion, getPodFn)
	} else {
		return o.deletePods(pods, getPodFn)
	}
}

func (o *DrainOptions) evictPods(pods []corev1.Pod, policyGroupVersion string, getPodFn func(namespace, name string) (*corev1.Pod, error)) error {
	doneCh := make(chan bool, len(pods))
	errCh := make(chan error, 1)

	for _, pod := range pods {
		go func(pod corev1.Pod, doneCh chan bool, errCh chan error) {
			var err error
			for {
				err = o.evictPod(pod, policyGroupVersion)
				if err == nil {
					break
				} else if apierrors.IsNotFound(err) {
					doneCh <- true
					return
				} else if apierrors.IsTooManyRequests(err) {
					time.Sleep(5 * time.Second)
				} else {
					errCh <- fmt.Errorf("error when evicting pod %q: %v", pod.Name, err)
					return
				}
			}
			podArray := []corev1.Pod{pod}
			_, err = o.waitForDelete(podArray, kubectl.Interval, time.Duration(math.MaxInt64), true, getPodFn)
			if err == nil {
				doneCh <- true
			} else {
				errCh <- fmt.Errorf("error when waiting for pod %q terminating: %v", pod.Name, err)
			}
		}(pod, doneCh, errCh)
	}

	doneCount := 0
	// 0 timeout means infinite, we use MaxInt64 to represent it.
	var globalTimeout time.Duration
	if o.Timeout == 0 {
		globalTimeout = time.Duration(math.MaxInt64)
	} else {
		globalTimeout = o.Timeout
	}
	for {
		select {
		case err := <-errCh:
			return err
		case <-doneCh:
			doneCount++
			if doneCount == len(pods) {
				return nil
			}
		case <-time.After(globalTimeout):
			return fmt.Errorf("Drain did not complete within %v", globalTimeout)
		}
	}
}

func (o *DrainOptions) deletePods(pods []corev1.Pod, getPodFn func(namespace, name string) (*corev1.Pod, error)) error {
	// 0 timeout means infinite, we use MaxInt64 to represent it.
	var globalTimeout time.Duration
	if o.Timeout == 0 {
		globalTimeout = time.Duration(math.MaxInt64)
	} else {
		globalTimeout = o.Timeout
	}
	for _, pod := range pods {
		err := o.deletePod(pod)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	_, err := o.waitForDelete(pods, kubectl.Interval, globalTimeout, false, getPodFn)
	return err
}

func (o *DrainOptions) waitForDelete(pods []corev1.Pod, interval, timeout time.Duration, usingEviction bool, getPodFn func(string, string) (*corev1.Pod, error)) ([]corev1.Pod, error) {
	var verbStr string
	if usingEviction {
		verbStr = "evicted"
	} else {
		verbStr = "deleted"
	}
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		pendingPods := []corev1.Pod{}
		for i, pod := range pods {
			p, err := getPodFn(pod.Namespace, pod.Name)
			if apierrors.IsNotFound(err) || (p != nil && p.ObjectMeta.UID != pod.ObjectMeta.UID) {
				o.Factory.PrintSuccess(o.mapper, false, o.Out, "pod", pod.Name, false, verbStr)
				continue
			} else if err != nil {
				return false, err
			} else {
				pendingPods = append(pendingPods, pods[i])
			}
		}
		pods = pendingPods
		if len(pendingPods) > 0 {
			return false, nil
		}
		return true, nil
	})
	return pods, err
}

// SupportEviction uses Discovery API to find out if the server support eviction subresource
// If support, it will return its groupVersion; Otherwise, it will return ""
func SupportEviction(clientset kubernetes.Interface) (string, error) {
	discoveryClient := clientset.Discovery()
	groupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return "", err
	}
	foundPolicyGroup := false
	var policyGroupVersion string
	for _, group := range groupList.Groups {
		if group.Name == "policy" {
			foundPolicyGroup = true
			policyGroupVersion = group.PreferredVersion.GroupVersion
			break
		}
	}
	if !foundPolicyGroup {
		return "", nil
	}
	resourceList, err := discoveryClient.ServerResourcesForGroupVersion("v1")
	if err != nil {
		return "", err
	}
	for _, resource := range resourceList.APIResources {
		if resource.Name == EvictionSubresource && resource.Kind == EvictionKind {
			return policyGroupVersion, nil
		}
	}
	return "", nil
}

// RunCordonOrUncordon runs either Cordon or Uncordon.  The desired value for
// "Unschedulable" is passed as the first arg.
func (o *DrainOptions) RunCordonOrUncordon(desired bool) error {
	cmdNamespace, _, err := o.Factory.DefaultNamespace()
	if err != nil {
		return err
	}

	cordonOrUncordon := "cordon"
	if !desired {
		cordonOrUncordon = "un" + cordonOrUncordon
	}

	for _, nodeInfo := range o.nodeInfos {
		if nodeInfo.Mapping.GroupVersionKind.Kind == "Node" {
			obj, err := nodeInfo.Mapping.ConvertToVersion(nodeInfo.Object, nodeInfo.Mapping.GroupVersionKind.GroupVersion())
			if err != nil {
				fmt.Printf("error: unable to %s node %q: %v", cordonOrUncordon, nodeInfo.Name, err)
				continue
			}
			oldData, err := json.Marshal(obj)
			if err != nil {
				fmt.Printf("error: unable to %s node %q: %v", cordonOrUncordon, nodeInfo.Name, err)
				continue
			}
			node, ok := obj.(*corev1.Node)
			if !ok {
				fmt.Fprintf(o.ErrOut, "error: unable to %s node %q: unexpected Type%T, expected Node", cordonOrUncordon, nodeInfo.Name, obj)
				continue
			}
			unsched := node.Spec.Unschedulable
			if unsched == desired {
				o.Factory.PrintSuccess(o.mapper, false, o.Out, nodeInfo.Mapping.Resource, nodeInfo.Name, o.DryRun, already(desired))
			} else {
				if !o.DryRun {
					helper := resource.NewHelper(o.restClient, nodeInfo.Mapping)
					node.Spec.Unschedulable = desired
					newData, err := json.Marshal(obj)
					if err != nil {
						fmt.Fprintf(o.ErrOut, "error: unable to %s node %q: %v", cordonOrUncordon, nodeInfo.Name, err)
						continue
					}
					patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, obj)
					if err != nil {
						fmt.Printf("error: unable to %s node %q: %v", cordonOrUncordon, nodeInfo.Name, err)
						continue
					}
					_, err = helper.Patch(cmdNamespace, nodeInfo.Name, types.StrategicMergePatchType, patchBytes)
					if err != nil {
						fmt.Printf("error: unable to %s node %q: %v", cordonOrUncordon, nodeInfo.Name, err)
						continue
					}
				}
				o.Factory.PrintSuccess(o.mapper, false, o.Out, nodeInfo.Mapping.Resource, nodeInfo.Name, o.DryRun, changed(desired))
			}
		} else {
			o.Factory.PrintSuccess(o.mapper, false, o.Out, nodeInfo.Mapping.Resource, nodeInfo.Name, o.DryRun, "skipped")
		}
	}

	return nil
}

// already() and changed() return suitable strings for {un,}cordoning

func already(desired bool) string {
	if desired {
		return "already cordoned"
	}
	return "already uncordoned"
}

func changed(desired bool) string {
	if desired {
		return "cordoned"
	}
	return "uncordoned"
}
