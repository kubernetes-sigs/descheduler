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

package internalversion

import (
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/apis/autoscaling"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/apis/certificates"
	"k8s.io/kubernetes/pkg/apis/coordination"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/discovery"
	"k8s.io/kubernetes/pkg/apis/networking"
	nodeapi "k8s.io/kubernetes/pkg/apis/node"
	"k8s.io/kubernetes/pkg/apis/policy"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/apis/scheduling"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/printers"
	utilpointer "k8s.io/utils/pointer"
)

func TestFormatResourceName(t *testing.T) {
	tests := []struct {
		kind schema.GroupKind
		name string
		want string
	}{
		{schema.GroupKind{}, "", ""},
		{schema.GroupKind{}, "name", "name"},
		{schema.GroupKind{Kind: "Kind"}, "", "kind/"}, // should not happen in practice
		{schema.GroupKind{Kind: "Kind"}, "name", "kind/name"},
		{schema.GroupKind{Group: "group", Kind: "Kind"}, "name", "kind.group/name"},
	}
	for _, tt := range tests {
		if got := formatResourceName(tt.kind, tt.name, true); got != tt.want {
			t.Errorf("formatResourceName(%q, %q) = %q, want %q", tt.kind, tt.name, got, tt.want)
		}
	}
}

type TestPrintHandler struct {
	numCalls int
}

func (t *TestPrintHandler) TableHandler(columnDefinitions []metav1beta1.TableColumnDefinition, printFunc interface{}) error {
	t.numCalls++
	return nil
}

func (t *TestPrintHandler) getNumCalls() int {
	return t.numCalls
}

func TestAllHandlers(t *testing.T) {
	h := &TestPrintHandler{numCalls: 0}
	AddHandlers(h)
	if h.getNumCalls() == 0 {
		t.Error("TableHandler not called in AddHandlers")
	}
}

func TestPrintEvent(t *testing.T) {
	tests := []struct {
		event    api.Event
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Basic event; no generate options
		{
			event: api.Event{
				Source: api.EventSource{Component: "kubelet"},
				InvolvedObject: api.ObjectReference{
					Kind:      "Pod",
					Name:      "Pod Name",
					FieldPath: "spec.containers{foo}",
				},
				Reason:         "Event Reason",
				Message:        "Message Data",
				FirstTimestamp: metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -3)},
				LastTimestamp:  metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -2)},
				Count:          6,
				Type:           api.EventTypeNormal,
				ObjectMeta:     metav1.ObjectMeta{Name: "event1"},
			},
			options: printers.GenerateOptions{},
			// Columns: Last Seen, Type, Reason, Object, Message
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"2d", "Normal", "Event Reason", "pod/Pod Name", "Message Data"}}},
		},
		// Basic event; generate options=Wide
		{
			event: api.Event{
				Source: api.EventSource{
					Component: "kubelet",
					Host:      "Node1",
				},
				InvolvedObject: api.ObjectReference{
					Kind:      "Deployment",
					Name:      "Deployment Name",
					FieldPath: "spec.containers{foo}",
				},
				Reason:         "Event Reason",
				Message:        "Message Data",
				FirstTimestamp: metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -3)},
				LastTimestamp:  metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -2)},
				Count:          6,
				Type:           api.EventTypeWarning,
				ObjectMeta:     metav1.ObjectMeta{Name: "event2"},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Last Seen, Type, Reason, Object, Subobject, Message, First Seen, Count, Name
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"2d", "Warning", "Event Reason", "deployment/Deployment Name", "spec.containers{foo}", "kubelet, Node1", "Message Data", "3d", int64(6), "event2"}}},
		},
	}

	for i, test := range tests {
		rows, err := printEvent(&test.event, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintEventsResultSorted(t *testing.T) {

	eventList := api.EventList{
		Items: []api.Event{
			{
				Source:         api.EventSource{Component: "kubelet"},
				Message:        "Item 1",
				FirstTimestamp: metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)),
				LastTimestamp:  metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)),
				Count:          1,
				Type:           api.EventTypeNormal,
			},
			{
				Source:         api.EventSource{Component: "scheduler"},
				Message:        "Item 2",
				FirstTimestamp: metav1.NewTime(time.Date(1987, time.June, 17, 0, 0, 0, 0, time.UTC)),
				LastTimestamp:  metav1.NewTime(time.Date(1987, time.June, 17, 0, 0, 0, 0, time.UTC)),
				Count:          1,
				Type:           api.EventTypeNormal,
			},
			{
				Source:         api.EventSource{Component: "kubelet"},
				Message:        "Item 3",
				FirstTimestamp: metav1.NewTime(time.Date(2002, time.December, 25, 0, 0, 0, 0, time.UTC)),
				LastTimestamp:  metav1.NewTime(time.Date(2002, time.December, 25, 0, 0, 0, 0, time.UTC)),
				Count:          1,
				Type:           api.EventTypeNormal,
			},
		},
	}

	rows, err := printEventList(&eventList, printers.GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Errorf("Generate Event List: Wrong number of table rows returned. Expected 3, got (%d)", len(rows))
	}
	// Verify the watch event dates are in order.
	firstRow := rows[0]
	message1 := firstRow.Cells[4]
	if message1.(string) != "Item 1" {
		t.Errorf("Wrong event ordering: expecting (Item 1), got (%s)", message1)
	}
	secondRow := rows[1]
	message2 := secondRow.Cells[4]
	if message2 != "Item 2" {
		t.Errorf("Wrong event ordering: expecting (Item 2), got (%s)", message2)
	}
	thirdRow := rows[2]
	message3 := thirdRow.Cells[4]
	if message3 != "Item 3" {
		t.Errorf("Wrong event ordering: expecting (Item 3), got (%s)", message3)
	}
}

func TestPrintNamespace(t *testing.T) {
	tests := []struct {
		namespace api.Namespace
		expected  []metav1beta1.TableRow
	}{
		// Basic namespace with status and age.
		{
			namespace: api.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "namespace1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Status: api.NamespaceStatus{
					Phase: "FooStatus",
				},
			},
			// Columns: Name, Status, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"namespace1", "FooStatus", "0s"}}},
		},
		// Basic namespace without status or age.
		{
			namespace: api.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "namespace2",
				},
			},
			// Columns: Name, Status, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"namespace2", "", "<unknown>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printNamespace(&test.namespace, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintSecret(t *testing.T) {
	tests := []struct {
		secret   api.Secret
		expected []metav1beta1.TableRow
	}{
		// Basic namespace with type, data, and age.
		{
			secret: api.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "secret1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Type: "kubernetes.io/service-account-token",
				Data: map[string][]byte{
					"token": []byte("secret data"),
				},
			},
			// Columns: Name, Type, Data, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"secret1", "kubernetes.io/service-account-token", int64(1), "0s"}}},
		},
		// Basic namespace with type and age; no data.
		{
			secret: api.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "secret1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Type: "kubernetes.io/service-account-token",
			},
			// Columns: Name, Type, Data, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"secret1", "kubernetes.io/service-account-token", int64(0), "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printSecret(&test.secret, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintServiceAccount(t *testing.T) {
	tests := []struct {
		serviceAccount api.ServiceAccount
		expected       []metav1beta1.TableRow
	}{
		// Basic service account without secrets
		{
			serviceAccount: api.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sa1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Secrets: []api.ObjectReference{},
			},
			// Columns: Name, (Num) Secrets, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sa1", int64(0), "0s"}}},
		},
		// Basic service account with two secrets.
		{
			serviceAccount: api.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sa1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Secrets: []api.ObjectReference{
					{Name: "Secret1"},
					{Name: "Secret2"},
				},
			},
			// Columns: Name, (Num) Secrets, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sa1", int64(2), "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printServiceAccount(&test.serviceAccount, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeStatus(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: api.NodeReady, Status: api.ConditionTrue}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo1", "Ready", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Spec:       api.NodeSpec{Unschedulable: true},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: api.NodeReady, Status: api.ConditionTrue}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo2", "Ready,SchedulingDisabled", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo3"},
				Status: api.NodeStatus{Conditions: []api.NodeCondition{
					{Type: api.NodeReady, Status: api.ConditionTrue},
					{Type: api.NodeReady, Status: api.ConditionTrue}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo3", "Ready", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo4"},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: api.NodeReady, Status: api.ConditionFalse}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo4", "NotReady", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo5"},
				Spec:       api.NodeSpec{Unschedulable: true},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: api.NodeReady, Status: api.ConditionFalse}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo5", "NotReady,SchedulingDisabled", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo6"},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: "InvalidValue", Status: api.ConditionTrue}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo6", "Unknown", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo7"},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo7", "Unknown", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo8"},
				Spec:       api.NodeSpec{Unschedulable: true},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{Type: "InvalidValue", Status: api.ConditionTrue}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo8", "Unknown,SchedulingDisabled", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo9"},
				Spec:       api.NodeSpec{Unschedulable: true},
				Status:     api.NodeStatus{Conditions: []api.NodeCondition{{}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo9", "Unknown,SchedulingDisabled", "<none>", "<unknown>", ""}}},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{})
		if err != nil {
			t.Fatalf("Error generating table rows for Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeRole(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo9"},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo9", "Unknown", "<none>", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "foo10",
					Labels: map[string]string{"node-role.kubernetes.io/master": "", "node-role.kubernetes.io/proxy": "", "kubernetes.io/role": "node"},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo10", "Unknown", "master,node,proxy", "<unknown>", ""}}},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "foo11",
					Labels: map[string]string{"kubernetes.io/role": "node"},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"foo11", "Unknown", "node", "<unknown>", ""}}},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{})
		if err != nil {
			t.Fatalf("An error occurred generating table rows for Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeOSImage(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{OSImage: "fake-os-image"},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo1", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "fake-os-image", "<unknown>", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{KernelVersion: "fake-kernel-version"},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo2", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "fake-kernel-version", "<unknown>"},
				},
			},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatalf("An error occurred generating table for Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeKernelVersion(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{KernelVersion: "fake-kernel-version"},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo1", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "fake-kernel-version", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{OSImage: "fake-os-image"},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo2", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "fake-os-image", "<unknown>", "<unknown>"},
				},
			},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatalf("An error occurred generating table rows Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeContainerRuntimeVersion(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{ContainerRuntimeVersion: "foo://1.2.3"},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo1", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "<unknown>", "foo://1.2.3"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Status: api.NodeStatus{
					NodeInfo:  api.NodeSystemInfo{},
					Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}},
				},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo2", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatalf("An error occurred generating table rows Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeName(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "127.0.0.1"},
				Status:     api.NodeStatus{},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"127.0.0.1", "Unknown", "<none>", "<unknown>", ""}}}},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: ""},
				Status:     api.NodeStatus{},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"", "Unknown", "<none>", "<unknown>", ""}}},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{})
		if err != nil {
			t.Fatalf("An error occurred generating table rows Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeExternalIP(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status:     api.NodeStatus{Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo1", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Status:     api.NodeStatus{Addresses: []api.NodeAddress{{Type: api.NodeInternalIP, Address: "1.1.1.1"}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo2", "Unknown", "<none>", "<unknown>", "", "1.1.1.1", "<none>", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo3"},
				Status: api.NodeStatus{Addresses: []api.NodeAddress{
					{Type: api.NodeExternalIP, Address: "2.2.2.2"},
					{Type: api.NodeInternalIP, Address: "3.3.3.3"},
					{Type: api.NodeExternalIP, Address: "4.4.4.4"},
				}},
			},
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo3", "Unknown", "<none>", "<unknown>", "", "3.3.3.3", "2.2.2.2", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatalf("An error occurred generating table rows Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNodeInternalIP(t *testing.T) {

	table := []struct {
		node     api.Node
		expected []metav1beta1.TableRow
	}{
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo1"},
				Status:     api.NodeStatus{Addresses: []api.NodeAddress{{Type: api.NodeInternalIP, Address: "1.1.1.1"}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo1", "Unknown", "<none>", "<unknown>", "", "1.1.1.1", "<none>", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo2"},
				Status:     api.NodeStatus{Addresses: []api.NodeAddress{{Type: api.NodeExternalIP, Address: "1.1.1.1"}}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo2", "Unknown", "<none>", "<unknown>", "", "<none>", "1.1.1.1", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
		{
			node: api.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "foo3"},
				Status: api.NodeStatus{Addresses: []api.NodeAddress{
					{Type: api.NodeInternalIP, Address: "2.2.2.2"},
					{Type: api.NodeExternalIP, Address: "3.3.3.3"},
					{Type: api.NodeInternalIP, Address: "4.4.4.4"},
				}},
			},
			// Columns: Name, Status, Roles, Age, KubeletVersion, NodeInternalIP, NodeExternalIP, OSImage, KernelVersion, ContainerRuntimeVersion
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"foo3", "Unknown", "<none>", "<unknown>", "", "2.2.2.2", "3.3.3.3", "<unknown>", "<unknown>", "<unknown>"},
				},
			},
		},
	}

	for i, test := range table {
		rows, err := printNode(&test.node, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatalf("An error occurred generating table rows Node: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintIngress(t *testing.T) {
	ingress := networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test1",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(-10, 0, 0)},
		},
		Spec: networking.IngressSpec{
			Backend: &networking.IngressBackend{
				ServiceName: "svc",
				ServicePort: intstr.FromInt(93),
			},
		},
		Status: networking.IngressStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					{
						IP:       "2.3.4.5",
						Hostname: "localhost.localdomain",
					},
				},
			},
		},
	}
	// Columns: Name, Hosts, Address, Ports, Age
	expected := []metav1beta1.TableRow{{Cells: []interface{}{"test1", "*", "2.3.4.5", "80", "10y"}}}

	rows, err := printIngress(&ingress, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error generating table rows for Ingress: %#v", err)
	}
	rows[0].Object.Object = nil
	if !reflect.DeepEqual(expected, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expected, rows))
	}
}

func TestPrintServiceLoadBalancer(t *testing.T) {
	tests := []struct {
		service  api.Service
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Test load balancer service with multiple external IP's
		{
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "service1"},
				Spec: api.ServiceSpec{
					ClusterIP: "1.2.3.4",
					Type:      "LoadBalancer",
					Ports:     []api.ServicePort{{Port: 80, Protocol: "TCP"}},
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{{IP: "2.3.4.5"}, {IP: "3.4.5.6"}}},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"service1", "LoadBalancer", "1.2.3.4", "2.3.4.5,3.4.5.6", "80/TCP", "<unknown>"}}},
		},
		// Test load balancer service with pending external IP.
		{
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "service2"},
				Spec: api.ServiceSpec{
					ClusterIP: "1.3.4.5",
					Type:      "LoadBalancer",
					Ports:     []api.ServicePort{{Port: 80, Protocol: "TCP"}, {Port: 8090, Protocol: "UDP"}, {Port: 8000, Protocol: "TCP"}, {Port: 7777, Protocol: "SCTP"}},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"service2", "LoadBalancer", "1.3.4.5", "<pending>", "80/TCP,8090/UDP,8000/TCP,7777/SCTP", "<unknown>"}}},
		},
		// Test load balancer service with multiple ports.
		{
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "service3"},
				Spec: api.ServiceSpec{
					ClusterIP: "1.4.5.6",
					Type:      "LoadBalancer",
					Ports:     []api.ServicePort{{Port: 80, Protocol: "TCP"}, {Port: 8090, Protocol: "UDP"}, {Port: 8000, Protocol: "TCP"}},
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{{IP: "2.3.4.5"}}},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"service3", "LoadBalancer", "1.4.5.6", "2.3.4.5", "80/TCP,8090/UDP,8000/TCP", "<unknown>"}}},
		},
		// Long external IP's list gets elided.
		{
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "service4"},
				Spec: api.ServiceSpec{
					ClusterIP: "1.5.6.7",
					Type:      "LoadBalancer",
					Ports:     []api.ServicePort{{Port: 80, Protocol: "TCP"}, {Port: 8090, Protocol: "UDP"}, {Port: 8000, Protocol: "TCP"}},
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{{IP: "2.3.4.5"}, {IP: "3.4.5.6"}, {IP: "5.6.7.8", Hostname: "host5678"}}},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"service4", "LoadBalancer", "1.5.6.7", "2.3.4.5,3.4.5...", "80/TCP,8090/UDP,8000/TCP", "<unknown>"}}},
		},
		// Generate options: Wide, includes selectors.
		{
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "service4"},
				Spec: api.ServiceSpec{
					ClusterIP: "1.5.6.7",
					Type:      "LoadBalancer",
					Ports:     []api.ServicePort{{Port: 80, Protocol: "TCP"}, {Port: 8090, Protocol: "UDP"}, {Port: 8000, Protocol: "TCP"}},
					Selector:  map[string]string{"foo": "bar"},
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{{IP: "2.3.4.5"}, {IP: "3.4.5.6"}, {IP: "5.6.7.8", Hostname: "host5678"}}},
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"service4", "LoadBalancer", "1.5.6.7", "2.3.4.5,3.4.5.6,5.6.7.8", "80/TCP,8090/UDP,8000/TCP", "<unknown>", "foo=bar"}}},
		},
	}

	for i, test := range tests {
		rows, err := printService(&test.service, test.options)
		if err != nil {
			t.Fatalf("Error printing table rows for Service: %#v", err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPod(t *testing.T) {
	tests := []struct {
		pod    api.Pod
		expect []metav1beta1.TableRow
	}{
		{
			// Test name, num of containers, restarts, container ready status
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test1", "1/2", "podPhase", int64(6), "<unknown>"}}},
		},
		{
			// Test container error overwrites pod phase
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test2"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{State: api.ContainerState{Waiting: &api.ContainerStateWaiting{Reason: "ContainerWaitingReason"}}, RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test2", "1/2", "ContainerWaitingReason", int64(6), "<unknown>"}}},
		},
		{
			// Test the same as the above but with Terminated state and the first container overwrites the rest
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test3"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{State: api.ContainerState{Waiting: &api.ContainerStateWaiting{Reason: "ContainerWaitingReason"}}, RestartCount: 3},
						{State: api.ContainerState{Terminated: &api.ContainerStateTerminated{Reason: "ContainerTerminatedReason"}}, RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test3", "0/2", "ContainerWaitingReason", int64(6), "<unknown>"}}},
		},
		{
			// Test ready is not enough for reporting running
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test4"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{Ready: true, RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test4", "1/2", "podPhase", int64(6), "<unknown>"}}},
		},
		{
			// Test ready is not enough for reporting running
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test5"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Reason: "podReason",
					Phase:  "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{Ready: true, RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test5", "1/2", "podReason", int64(6), "<unknown>"}}},
		},
		{
			// Test pod has 2 containers, one is running and the other is completed.
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test6"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase:  "Running",
					Reason: "",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Terminated: &api.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}}},
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test6", "1/2", "Running", int64(6), "<unknown>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPod(&test.pod, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expect, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expect, rows))
		}
	}
}

func TestPrintPodwide(t *testing.T) {
	condition1 := "condition1"
	condition2 := "condition2"
	condition3 := "condition3"
	tests := []struct {
		pod    api.Pod
		expect []metav1beta1.TableRow
	}{
		{
			// Test when the NodeName and PodIP are not none
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec: api.PodSpec{
					Containers: make([]api.Container, 2),
					NodeName:   "test1",
					ReadinessGates: []api.PodReadinessGate{
						{
							ConditionType: api.PodConditionType(condition1),
						},
						{
							ConditionType: api.PodConditionType(condition2),
						},
						{
							ConditionType: api.PodConditionType(condition3),
						},
					},
				},
				Status: api.PodStatus{
					Conditions: []api.PodCondition{
						{
							Type:   api.PodConditionType(condition1),
							Status: api.ConditionFalse,
						},
						{
							Type:   api.PodConditionType(condition2),
							Status: api.ConditionTrue,
						},
					},
					Phase:  "podPhase",
					PodIPs: []api.PodIP{{IP: "1.1.1.1"}},
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
					NominatedNodeName: "node1",
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test1", "1/2", "podPhase", int64(6), "<unknown>", "1.1.1.1", "test1", "node1", "1/3"}}},
		},
		{
			// Test when the NodeName and PodIP are not none
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec: api.PodSpec{
					Containers: make([]api.Container, 2),
					NodeName:   "test1",
					ReadinessGates: []api.PodReadinessGate{
						{
							ConditionType: api.PodConditionType(condition1),
						},
						{
							ConditionType: api.PodConditionType(condition2),
						},
						{
							ConditionType: api.PodConditionType(condition3),
						},
					},
				},
				Status: api.PodStatus{
					Conditions: []api.PodCondition{
						{
							Type:   api.PodConditionType(condition1),
							Status: api.ConditionFalse,
						},
						{
							Type:   api.PodConditionType(condition2),
							Status: api.ConditionTrue,
						},
					},
					Phase:  "podPhase",
					PodIPs: []api.PodIP{{IP: "1.1.1.1"}, {IP: "2001:db8::"}},
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
					NominatedNodeName: "node1",
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test1", "1/2", "podPhase", int64(6), "<unknown>", "1.1.1.1", "test1", "node1", "1/3"}}},
		},
		{
			// Test when the NodeName and PodIP are none
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test2"},
				Spec: api.PodSpec{
					Containers: make([]api.Container, 2),
					NodeName:   "",
				},
				Status: api.PodStatus{
					Phase: "podPhase",
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{State: api.ContainerState{Waiting: &api.ContainerStateWaiting{Reason: "ContainerWaitingReason"}}, RestartCount: 3},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test2", "1/2", "ContainerWaitingReason", int64(6), "<unknown>", "<none>", "<none>", "<none>", "<none>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPod(&test.pod, printers.GenerateOptions{Wide: true})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expect, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expect, rows))
		}
	}
}

func TestPrintPodConditions(t *testing.T) {
	runningPod := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test1", Labels: map[string]string{"a": "1", "b": "2"}},
		Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
		Status: api.PodStatus{
			Phase: "Running",
			ContainerStatuses: []api.ContainerStatus{
				{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
				{RestartCount: 3},
			},
		},
	}
	succeededPod := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test1", Labels: map[string]string{"a": "1", "b": "2"}},
		Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
		Status: api.PodStatus{
			Phase: "Succeeded",
			ContainerStatuses: []api.ContainerStatus{
				{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
				{RestartCount: 3},
			},
		},
	}
	failedPod := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test2", Labels: map[string]string{"b": "2"}},
		Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
		Status: api.PodStatus{
			Phase: "Failed",
			ContainerStatuses: []api.ContainerStatus{
				{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
				{RestartCount: 3},
			},
		},
	}
	tests := []struct {
		pod    *api.Pod
		expect []metav1beta1.TableRow
	}{
		// Should not have TableRowCondition
		{
			pod: runningPod,
			// Columns: Name, Ready, Reason, Restarts, Age
			expect: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "1/2", "Running", int64(6), "<unknown>"}}},
		},
		// Should have TableRowCondition: podSuccessConditions
		{
			pod: succeededPod,
			expect: []metav1beta1.TableRow{
				{
					// Columns: Name, Ready, Reason, Restarts, Age
					Cells:      []interface{}{"test1", "1/2", "Succeeded", int64(6), "<unknown>"},
					Conditions: podSuccessConditions,
				},
			},
		},
		// Should have TableRowCondition: podFailedCondition
		{
			pod: failedPod,
			expect: []metav1beta1.TableRow{
				{
					// Columns: Name, Ready, Reason, Restarts, Age
					Cells:      []interface{}{"test2", "1/2", "Failed", int64(6), "<unknown>"},
					Conditions: podFailedConditions,
				},
			},
		},
	}

	for i, test := range tests {
		rows, err := printPod(test.pod, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expect, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expect, rows))
		}
	}
}

func TestPrintPodList(t *testing.T) {
	tests := []struct {
		pods   api.PodList
		expect []metav1beta1.TableRow
	}{
		// Test podList's pod: name, num of containers, restarts, container ready status
		{
			api.PodList{
				Items: []api.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test1"},
						Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
						Status: api.PodStatus{
							Phase: "podPhase",
							ContainerStatuses: []api.ContainerStatus{
								{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
								{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test2"},
						Spec:       api.PodSpec{Containers: make([]api.Container, 1)},
						Status: api.PodStatus{
							Phase: "podPhase",
							ContainerStatuses: []api.ContainerStatus{
								{Ready: true, RestartCount: 1, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
							},
						},
					},
				},
			},
			[]metav1beta1.TableRow{{Cells: []interface{}{"test1", "2/2", "podPhase", int64(6), "<unknown>"}}, {Cells: []interface{}{"test2", "1/1", "podPhase", int64(1), "<unknown>"}}},
		},
	}

	for _, test := range tests {
		rows, err := printPodList(&test.pods, printers.GenerateOptions{})

		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expect, rows) {
			t.Errorf("mismatch: %s", diff.ObjectReflectDiff(test.expect, rows))
		}
	}
}

func TestPrintNonTerminatedPod(t *testing.T) {
	tests := []struct {
		pod    api.Pod
		expect []metav1beta1.TableRow
	}{
		{
			// Test pod phase Running should be printed
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: api.PodRunning,
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
				},
			},
			// Columns: Name, Ready, Reason, Restarts, Age
			[]metav1beta1.TableRow{{Cells: []interface{}{"test1", "1/2", "Running", int64(6), "<unknown>"}}},
		},
		{
			// Test pod phase Pending should be printed
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test2"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: api.PodPending,
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
				},
			},
			// Columns: Name, Ready, Reason, Restarts, Age
			[]metav1beta1.TableRow{{Cells: []interface{}{"test2", "1/2", "Pending", int64(6), "<unknown>"}}},
		},
		{
			// Test pod phase Unknown should be printed
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test3"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: api.PodUnknown,
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
				},
			},
			// Columns: Name, Ready, Reason, Restarts, Age
			[]metav1beta1.TableRow{{Cells: []interface{}{"test3", "1/2", "Unknown", int64(6), "<unknown>"}}},
		},
		{
			// Test pod phase Succeeded should be printed
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test4"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: api.PodSucceeded,
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{RestartCount: 3},
					},
				},
			},
			// Columns: Name, Ready, Reason, Restarts, Age
			[]metav1beta1.TableRow{
				{
					Cells:      []interface{}{"test4", "1/2", "Succeeded", int64(6), "<unknown>"},
					Conditions: podSuccessConditions,
				},
			},
		},
		{
			// Test pod phase Failed shouldn't be printed
			api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test5"},
				Spec:       api.PodSpec{Containers: make([]api.Container, 2)},
				Status: api.PodStatus{
					Phase: api.PodFailed,
					ContainerStatuses: []api.ContainerStatus{
						{Ready: true, RestartCount: 3, State: api.ContainerState{Running: &api.ContainerStateRunning{}}},
						{Ready: true, RestartCount: 3},
					},
				},
			},
			// Columns: Name, Ready, Reason, Restarts, Age
			[]metav1beta1.TableRow{
				{
					Cells:      []interface{}{"test5", "1/2", "Failed", int64(6), "<unknown>"},
					Conditions: podFailedConditions,
				},
			},
		},
	}

	for i, test := range tests {
		rows, err := printPod(&test.pod, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expect, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expect, rows))
		}
	}
}

func TestPrintPodTemplate(t *testing.T) {
	tests := []struct {
		podTemplate api.PodTemplate
		options     printers.GenerateOptions
		expected    []metav1beta1.TableRow
	}{
		// Test basic pod template with no containers.
		{
			podTemplate: api.PodTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-template-1"},
				Template: api.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-template-1"},
					Spec: api.PodSpec{
						Containers: []api.Container{},
					},
				},
			},

			options: printers.GenerateOptions{},
			// Columns: Name, Containers, Images, Pod Labels
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pod-template-1", "", "", "<none>"}}},
		},
		// Test basic pod template with two containers.
		{
			podTemplate: api.PodTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-template-2"},
				Template: api.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-template-2"},
					Spec: api.PodSpec{
						Containers: []api.Container{
							{
								Name:  "fake-container1",
								Image: "fake-image1",
							},
							{
								Name:  "fake-container2",
								Image: "fake-image2",
							},
						},
					},
				},
			},

			options: printers.GenerateOptions{},
			// Columns: Name, Containers, Images, Pod Labels
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pod-template-2", "fake-container1,fake-container2", "fake-image1,fake-image2", "<none>"}}},
		},
		// Test basic pod template with pod labels
		{
			podTemplate: api.PodTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-template-3"},
				Template: api.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "pod-template-3",
						Labels: map[string]string{"foo": "bar"},
					},
					Spec: api.PodSpec{
						Containers: []api.Container{},
					},
				},
			},

			options: printers.GenerateOptions{},
			// Columns: Name, Containers, Images, Pod Labels
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pod-template-3", "", "", "foo=bar"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPodTemplate(&test.podTemplate, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPodTemplateList(t *testing.T) {

	templateList := api.PodTemplateList{
		Items: []api.PodTemplate{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-template-1"},
				Template: api.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "pod-template-2",
						Labels: map[string]string{"foo": "bar"},
					},
					Spec: api.PodSpec{
						Containers: []api.Container{},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-template-2"},
				Template: api.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "pod-template-2",
						Labels: map[string]string{"a": "b"},
					},
					Spec: api.PodSpec{
						Containers: []api.Container{},
					},
				},
			},
		},
	}

	// Columns: Name, Containers, Images, Pod Labels
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"pod-template-1", "", "", "foo=bar"}},
		{Cells: []interface{}{"pod-template-2", "", "", "a=b"}},
	}

	rows, err := printPodTemplateList(&templateList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing pod template list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

type stringTestList []struct {
	name, got, exp string
}

func TestTranslateTimestampSince(t *testing.T) {
	tl := stringTestList{
		{"a while from now", translateTimestampSince(metav1.Time{Time: time.Now().Add(2.1e9)}), "<invalid>"},
		{"almost now", translateTimestampSince(metav1.Time{Time: time.Now().Add(1.9e9)}), "0s"},
		{"now", translateTimestampSince(metav1.Time{Time: time.Now()}), "0s"},
		{"unknown", translateTimestampSince(metav1.Time{}), "<unknown>"},
		{"30 seconds ago", translateTimestampSince(metav1.Time{Time: time.Now().Add(-3e10)}), "30s"},
		{"5 minutes ago", translateTimestampSince(metav1.Time{Time: time.Now().Add(-3e11)}), "5m"},
		{"an hour ago", translateTimestampSince(metav1.Time{Time: time.Now().Add(-6e12)}), "100m"},
		{"2 days ago", translateTimestampSince(metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -2)}), "2d"},
		{"months ago", translateTimestampSince(metav1.Time{Time: time.Now().UTC().AddDate(0, 0, -90)}), "90d"},
		{"10 years ago", translateTimestampSince(metav1.Time{Time: time.Now().UTC().AddDate(-10, 0, 0)}), "10y"},
	}
	for _, test := range tl {
		if test.got != test.exp {
			t.Errorf("On %v, expected '%v', but got '%v'",
				test.name, test.exp, test.got)
		}
	}
}

func TestTranslateTimestampUntil(t *testing.T) {
	// Since this method compares the time with time.Now() internally,
	// small buffers of 0.1 seconds are added on comparing times to consider method call overhead.
	// Otherwise, the output strings become shorter than expected.
	const buf = 1e8
	tl := stringTestList{
		{"a while ago", translateTimestampUntil(metav1.Time{Time: time.Now().Add(-2.1e9)}), "<invalid>"},
		{"almost now", translateTimestampUntil(metav1.Time{Time: time.Now().Add(-1.9e9)}), "0s"},
		{"now", translateTimestampUntil(metav1.Time{Time: time.Now()}), "0s"},
		{"unknown", translateTimestampUntil(metav1.Time{}), "<unknown>"},
		{"in 30 seconds", translateTimestampUntil(metav1.Time{Time: time.Now().Add(3e10 + buf)}), "30s"},
		{"in 5 minutes", translateTimestampUntil(metav1.Time{Time: time.Now().Add(3e11 + buf)}), "5m"},
		{"in an hour", translateTimestampUntil(metav1.Time{Time: time.Now().Add(6e12 + buf)}), "100m"},
		{"in 2 days", translateTimestampUntil(metav1.Time{Time: time.Now().UTC().AddDate(0, 0, 2).Add(buf)}), "2d"},
		{"in months", translateTimestampUntil(metav1.Time{Time: time.Now().UTC().AddDate(0, 0, 90).Add(buf)}), "90d"},
		{"in 10 years", translateTimestampUntil(metav1.Time{Time: time.Now().UTC().AddDate(10, 0, 0).Add(buf)}), "10y"},
	}
	for _, test := range tl {
		if test.got != test.exp {
			t.Errorf("On %v, expected '%v', but got '%v'",
				test.name, test.exp, test.got)
		}
	}
}

func TestPrintDeployment(t *testing.T) {

	testDeployment := apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test1",
			CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
		},
		Spec: apps.DeploymentSpec{
			Replicas: 5,
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  "fake-container1",
							Image: "fake-image1",
						},
						{
							Name:  "fake-container2",
							Image: "fake-image2",
						},
					},
				},
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
		},
		Status: apps.DeploymentStatus{
			Replicas:            10,
			UpdatedReplicas:     2,
			AvailableReplicas:   1,
			UnavailableReplicas: 4,
		},
	}

	tests := []struct {
		deployment apps.Deployment
		options    printers.GenerateOptions
		expected   []metav1beta1.TableRow
	}{
		// Test Deployment with no generate options.
		{
			deployment: testDeployment,
			options:    printers.GenerateOptions{},
			// Columns: Name, ReadyReplicas, UpdatedReplicas, AvailableReplicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "0/5", int64(2), int64(1), "0s"}}},
		},
		// Test generate options: Wide.
		{
			deployment: testDeployment,
			options:    printers.GenerateOptions{Wide: true},
			// Columns: Name, ReadyReplicas, UpdatedReplicas, AvailableReplicas, Age, Containers, Images, Selectors
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "0/5", int64(2), int64(1), "0s", "fake-container1,fake-container2", "fake-image1,fake-image2", "foo=bar"}}},
		},
	}

	for i, test := range tests {
		rows, err := printDeployment(&test.deployment, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintDaemonSet(t *testing.T) {

	testDaemonSet := apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test1",
			CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
		},
		Spec: apps.DaemonSetSpec{
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  "fake-container1",
							Image: "fake-image1",
						},
						{
							Name:  "fake-container2",
							Image: "fake-image2",
						},
					},
				},
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
		},
		Status: apps.DaemonSetStatus{
			CurrentNumberScheduled: 2,
			DesiredNumberScheduled: 3,
			NumberReady:            1,
			UpdatedNumberScheduled: 2,
			NumberAvailable:        0,
		},
	}

	tests := []struct {
		daemonSet apps.DaemonSet
		options   printers.GenerateOptions
		expected  []metav1beta1.TableRow
	}{
		// Test generate daemon set with no generate options.
		{
			daemonSet: testDaemonSet,
			options:   printers.GenerateOptions{},
			// Columns: Name, Num Desired, Num Current, Num Ready, Num Updated, Num Available, Selectors, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", int64(3), int64(2), int64(1), int64(2), int64(0), "<none>", "0s"}}},
		},
		// Test generate daemon set with "Wide" generate options.
		{
			daemonSet: testDaemonSet,
			options:   printers.GenerateOptions{Wide: true},
			// Columns: Name, Num Desired, Num Current, Num Ready, Num Updated, Num Available, Node Selectors, Age, Containers, Images, Labels
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", int64(3), int64(2), int64(1), int64(2), int64(0), "<none>", "0s", "fake-container1,fake-container2", "fake-image1,fake-image2", "foo=bar"}}},
		},
	}

	for i, test := range tests {
		rows, err := printDaemonSet(&test.daemonSet, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintDaemonSetList(t *testing.T) {

	daemonSetList := apps.DaemonSetList{
		Items: []apps.DaemonSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "daemonset1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.DaemonSetSpec{
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
				},
				Status: apps.DaemonSetStatus{
					CurrentNumberScheduled: 2,
					DesiredNumberScheduled: 3,
					NumberReady:            1,
					UpdatedNumberScheduled: 2,
					NumberAvailable:        0,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "daemonset2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.DaemonSetSpec{
					Template: api.PodTemplateSpec{},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
				Status: apps.DaemonSetStatus{
					CurrentNumberScheduled: 4,
					DesiredNumberScheduled: 2,
					NumberReady:            9,
					UpdatedNumberScheduled: 3,
					NumberAvailable:        3,
				},
			},
		},
	}

	// Columns: Name, Num Desired, Num Current, Num Ready, Num Updated, Num Available, Selectors, Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"daemonset1", int64(3), int64(2), int64(1), int64(2), int64(0), "<none>", "0s"}},
		{Cells: []interface{}{"daemonset2", int64(2), int64(4), int64(9), int64(3), int64(3), "<none>", "0s"}},
	}

	rows, err := printDaemonSetList(&daemonSetList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing daemon set list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintJob(t *testing.T) {
	now := time.Now()
	completions := int32(2)
	tests := []struct {
		job      batch.Job
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		{
			// Generate table rows for Job with no generate options.
			job: batch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.JobSpec{
					Completions: &completions,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-job-container1",
									Image: "fake-job-image1",
								},
								{
									Name:  "fake-job-container2",
									Image: "fake-job-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job-label": "job-lable-value"}},
				},
				Status: batch.JobStatus{
					Succeeded: 1,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Completions, Duration, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"job1", "1/2", "", "0s"}}},
		},
		// Generate table rows for Job with generate options "Wide".
		{
			job: batch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.JobSpec{
					Completions: &completions,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-job-container1",
									Image: "fake-job-image1",
								},
								{
									Name:  "fake-job-container2",
									Image: "fake-job-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job-label": "job-label-value"}},
				},
				Status: batch.JobStatus{
					Succeeded: 1,
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Completions, Duration, Age, Containers, Images, Selectors
			expected: []metav1beta1.TableRow{
				{
					Cells: []interface{}{"job1", "1/2", "", "0s", "fake-job-container1,fake-job-container2", "fake-job-image1,fake-job-image2", "job-label=job-label-value"},
				},
			},
		},
		// Job with ten-year age.
		{
			job: batch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job2",
					CreationTimestamp: metav1.Time{Time: time.Now().AddDate(-10, 0, 0)},
				},
				Spec: batch.JobSpec{
					Completions: nil,
				},
				Status: batch.JobStatus{
					Succeeded: 0,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Completions, Duration, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"job2", "0/1", "", "10y"}}},
		},
		// Job with duration.
		{
			job: batch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job3",
					CreationTimestamp: metav1.Time{Time: time.Now().AddDate(-10, 0, 0)},
				},
				Spec: batch.JobSpec{
					Completions: nil,
				},
				Status: batch.JobStatus{
					Succeeded:      0,
					StartTime:      &metav1.Time{Time: now.Add(time.Minute)},
					CompletionTime: &metav1.Time{Time: now.Add(31 * time.Minute)},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Completions, Duration, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"job3", "0/1", "30m", "10y"}}},
		},
		{
			job: batch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job4",
					CreationTimestamp: metav1.Time{Time: time.Now().AddDate(-10, 0, 0)},
				},
				Spec: batch.JobSpec{
					Completions: nil,
				},
				Status: batch.JobStatus{
					Succeeded: 0,
					StartTime: &metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Completions, Duration, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"job4", "0/1", "20m", "10y"}}},
		},
	}

	for i, test := range tests {
		rows, err := printJob(&test.job, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintJobList(t *testing.T) {
	completions := int32(2)
	jobList := batch.JobList{
		Items: []batch.Job{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.JobSpec{
					Completions: &completions,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-job-container1",
									Image: "fake-job-image1",
								},
								{
									Name:  "fake-job-container2",
									Image: "fake-job-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job-label": "job-lable-value"}},
				},
				Status: batch.JobStatus{
					Succeeded: 1,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.JobSpec{
					Completions: &completions,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-job-container1",
									Image: "fake-job-image1",
								},
								{
									Name:  "fake-job-container2",
									Image: "fake-job-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"job-label": "job-lable-value"}},
				},
				Status: batch.JobStatus{
					Succeeded: 2,
					StartTime: &metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
				},
			},
		},
	}

	// Columns: Name, Completions, Duration, Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"job1", "1/2", "", "0s"}},
		{Cells: []interface{}{"job2", "2/2", "20m", "0s"}},
	}

	rows, err := printJobList(&jobList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing job list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintHPA(t *testing.T) {
	minReplicasVal := int32(2)
	targetUtilizationVal := int32(80)
	currentUtilizationVal := int32(50)
	metricLabelSelector, err := metav1.ParseToLabelSelector("label=value")
	if err != nil {
		t.Errorf("unable to parse label selector: %v", err)
	}
	tests := []struct {
		hpa      autoscaling.HorizontalPodAutoscaler
		expected []metav1beta1.TableRow
	}{
		// minReplicas unset
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MaxReplicas: 10,
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<none>", "<unset>", int64(10), int64(4), "<unknown>"}}},
		},
		// external source type, target average value (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name:     "some-external-metric",
									Selector: metricLabelSelector,
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/100m (avg)", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// external source type, target average value
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name:     "some-external-metric",
									Selector: metricLabelSelector,
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricStatus{
								Metric: autoscaling.MetricIdentifier{
									Name:     "some-external-metric",
									Selector: metricLabelSelector,
								},
								Current: autoscaling.MetricValueStatus{
									AverageValue: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m (avg)", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// external source type, target value (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name:     "some-service-metric",
									Selector: metricLabelSelector,
								},
								Target: autoscaling.MetricTarget{
									Type:  autoscaling.ValueMetricType,
									Value: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// external source type, target value
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name:     "some-external-metric",
									Selector: metricLabelSelector,
								},
								Target: autoscaling.MetricTarget{
									Type:  autoscaling.ValueMetricType,
									Value: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.ExternalMetricSourceType,
							External: &autoscaling.ExternalMetricStatus{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-external-metric",
								},
								Current: autoscaling.MetricValueStatus{
									Value: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// pods source type (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-pods-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// pods source type
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-pods-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricStatus{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-pods-metric",
								},
								Current: autoscaling.MetricValueStatus{
									AverageValue: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// object source type (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ObjectMetricSourceType,
							Object: &autoscaling.ObjectMetricSource{
								DescribedObject: autoscaling.CrossVersionObjectReference{
									Name: "some-service",
									Kind: "Service",
								},
								Metric: autoscaling.MetricIdentifier{
									Name: "some-service-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:  autoscaling.ValueMetricType,
									Value: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// object source type
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ObjectMetricSourceType,
							Object: &autoscaling.ObjectMetricSource{
								DescribedObject: autoscaling.CrossVersionObjectReference{
									Name: "some-service",
									Kind: "Service",
								},
								Metric: autoscaling.MetricIdentifier{
									Name: "some-service-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:  autoscaling.ValueMetricType,
									Value: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.ObjectMetricSourceType,
							Object: &autoscaling.ObjectMetricStatus{
								DescribedObject: autoscaling.CrossVersionObjectReference{
									Name: "some-service",
									Kind: "Service",
								},
								Metric: autoscaling.MetricIdentifier{
									Name: "some-service-metric",
								},
								Current: autoscaling.MetricValueStatus{
									Value: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// resource source type, targetVal (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricSource{
								Name: api.ResourceCPU,
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// resource source type, targetVal
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricSource{
								Name: api.ResourceCPU,
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricStatus{
								Name: api.ResourceCPU,
								Current: autoscaling.MetricValueStatus{
									AverageValue: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// resource source type, targetUtil (no current)
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricSource{
								Name: api.ResourceCPU,
								Target: autoscaling.MetricTarget{
									Type:               autoscaling.UtilizationMetricType,
									AverageUtilization: &targetUtilizationVal,
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "<unknown>/80%", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// resource source type, targetUtil
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricSource{
								Name: api.ResourceCPU,
								Target: autoscaling.MetricTarget{
									Type:               autoscaling.UtilizationMetricType,
									AverageUtilization: &targetUtilizationVal,
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricStatus{
								Name: api.ResourceCPU,
								Current: autoscaling.MetricValueStatus{
									AverageUtilization: &currentUtilizationVal,
									AverageValue:       resource.NewMilliQuantity(40, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50%/80%", "2", int64(10), int64(4), "<unknown>"}}},
		},
		// multiple specs
		{
			hpa: autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "some-hpa"},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscaling.CrossVersionObjectReference{
						Name: "some-rc",
						Kind: "ReplicationController",
					},
					MinReplicas: &minReplicasVal,
					MaxReplicas: 10,
					Metrics: []autoscaling.MetricSpec{
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-pods-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(100, resource.DecimalSI),
								},
							},
						},
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricSource{
								Name: api.ResourceCPU,
								Target: autoscaling.MetricTarget{
									Type:               autoscaling.UtilizationMetricType,
									AverageUtilization: &targetUtilizationVal,
								},
							},
						},
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricSource{
								Metric: autoscaling.MetricIdentifier{
									Name: "other-pods-metric",
								},
								Target: autoscaling.MetricTarget{
									Type:         autoscaling.AverageValueMetricType,
									AverageValue: resource.NewMilliQuantity(400, resource.DecimalSI),
								},
							},
						},
					},
				},
				Status: autoscaling.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 4,
					DesiredReplicas: 5,
					CurrentMetrics: []autoscaling.MetricStatus{
						{
							Type: autoscaling.PodsMetricSourceType,
							Pods: &autoscaling.PodsMetricStatus{
								Metric: autoscaling.MetricIdentifier{
									Name: "some-pods-metric",
								},
								Current: autoscaling.MetricValueStatus{
									AverageValue: resource.NewMilliQuantity(50, resource.DecimalSI),
								},
							},
						},
						{
							Type: autoscaling.ResourceMetricSourceType,
							Resource: &autoscaling.ResourceMetricStatus{
								Name: api.ResourceCPU,
								Current: autoscaling.MetricValueStatus{
									AverageUtilization: &currentUtilizationVal,
									AverageValue:       resource.NewMilliQuantity(40, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
			// Columns: Name, Reference, Targets, MinPods, MaxPods, Replicas, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"some-hpa", "ReplicationController/some-rc", "50m/100m, 50%/80% + 1 more...", "2", int64(10), int64(4), "<unknown>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printHorizontalPodAutoscaler(&test.hpa, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintService(t *testing.T) {
	singleExternalIP := []string{"80.11.12.10"}
	mulExternalIP := []string{"80.11.12.10", "80.11.12.11"}
	tests := []struct {
		service  api.Service
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		{
			// Test name, cluster ip, port with protocol
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeClusterIP,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     2233,
						},
					},
					ClusterIP: "10.9.8.7",
					Selector:  map[string]string{"foo": "bar"}, // Does NOT get printed.
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "ClusterIP", "10.9.8.7", "<none>", "2233/tcp", "<unknown>"}}},
		},
		{
			// Test generate options: Wide includes selectors.
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test1"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeClusterIP,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     2233,
						},
					},
					ClusterIP: "10.9.8.7",
					Selector:  map[string]string{"foo": "bar"},
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age, Selector
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "ClusterIP", "10.9.8.7", "<none>", "2233/tcp", "<unknown>", "foo=bar"}}},
		},
		{
			// Test NodePort service
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test2"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeNodePort,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     8888,
							NodePort: 9999,
						},
					},
					ClusterIP: "10.9.8.7",
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test2", "NodePort", "10.9.8.7", "<none>", "8888:9999/tcp", "<unknown>"}}},
		},
		{
			// Test LoadBalancer service
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test3"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeLoadBalancer,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     8888,
						},
					},
					ClusterIP: "10.9.8.7",
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test3", "LoadBalancer", "10.9.8.7", "<pending>", "8888/tcp", "<unknown>"}}},
		},
		{
			// Test LoadBalancer service with single ExternalIP and no LoadBalancerStatus
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test4"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeLoadBalancer,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     8888,
						},
					},
					ClusterIP:   "10.9.8.7",
					ExternalIPs: singleExternalIP,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test4", "LoadBalancer", "10.9.8.7", "80.11.12.10", "8888/tcp", "<unknown>"}}},
		},
		{
			// Test LoadBalancer service with single ExternalIP
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test5"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeLoadBalancer,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     8888,
						},
					},
					ClusterIP:   "10.9.8.7",
					ExternalIPs: singleExternalIP,
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{
							{
								IP:       "3.4.5.6",
								Hostname: "test.cluster.com",
							},
						},
					},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test5", "LoadBalancer", "10.9.8.7", "3.4.5.6,80.11.12.10", "8888/tcp", "<unknown>"}}},
		},
		{
			// Test LoadBalancer service with mul ExternalIPs
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test6"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeLoadBalancer,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     8888,
						},
					},
					ClusterIP:   "10.9.8.7",
					ExternalIPs: mulExternalIP,
				},
				Status: api.ServiceStatus{
					LoadBalancer: api.LoadBalancerStatus{
						Ingress: []api.LoadBalancerIngress{
							{
								IP:       "2.3.4.5",
								Hostname: "test.cluster.local",
							},
							{
								IP:       "3.4.5.6",
								Hostname: "test.cluster.com",
							},
						},
					},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test6", "LoadBalancer", "10.9.8.7", "2.3.4.5,3.4.5.6,80.11.12.10,80.11.12.11", "8888/tcp", "<unknown>"}}},
		},
		{
			// Test ExternalName service
			service: api.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "test7"},
				Spec: api.ServiceSpec{
					Type:         api.ServiceTypeExternalName,
					ExternalName: "my.database.example.com",
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test7", "ExternalName", "<none>", "my.database.example.com", "<none>", "<unknown>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printService(&test.service, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintServiceList(t *testing.T) {
	serviceList := api.ServiceList{
		Items: []api.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "service1"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeClusterIP,
					Ports: []api.ServicePort{
						{
							Protocol: "tcp",
							Port:     2233,
						},
					},
					ClusterIP: "10.9.8.7",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "service2"},
				Spec: api.ServiceSpec{
					Type: api.ServiceTypeNodePort,
					Ports: []api.ServicePort{
						{
							Protocol: "udp",
							Port:     5566,
						},
					},
					ClusterIP: "1.2.3.4",
				},
			},
		},
	}

	// Columns: Name, Type, Cluster-IP, External-IP, Port(s), Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"service1", "ClusterIP", "10.9.8.7", "<none>", "2233/tcp", "<unknown>"}},
		{Cells: []interface{}{"service2", "NodePort", "1.2.3.4", "<none>", "5566/udp", "<unknown>"}},
	}

	rows, err := printServiceList(&serviceList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing service list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintPodDisruptionBudget(t *testing.T) {
	minAvailable := intstr.FromInt(22)
	maxUnavailable := intstr.FromInt(11)
	tests := []struct {
		pdb      policy.PodDisruptionBudget
		expected []metav1beta1.TableRow
	}{
		// Min Available set, no Max Available.
		{
			pdb: policy.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "pdb1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: policy.PodDisruptionBudgetSpec{
					MinAvailable: &minAvailable,
				},
				Status: policy.PodDisruptionBudgetStatus{
					PodDisruptionsAllowed: 5,
				},
			},
			// Columns: Name, Min Available, Max Available, Allowed Disruptions, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pdb1", "22", "N/A", int64(5), "0s"}}},
		},
		// Max Available set, no Min Available.
		{
			pdb: policy.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns2",
					Name:              "pdb2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: policy.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
				},
				Status: policy.PodDisruptionBudgetStatus{
					PodDisruptionsAllowed: 5,
				},
			},
			// Columns: Name, Min Available, Max Available, Allowed Disruptions, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pdb2", "N/A", "11", int64(5), "0s"}}},
		}}

	for i, test := range tests {
		rows, err := printPodDisruptionBudget(&test.pdb, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPodDisruptionBudgetList(t *testing.T) {
	minAvailable := intstr.FromInt(22)
	maxUnavailable := intstr.FromInt(11)

	pdbList := policy.PodDisruptionBudgetList{
		Items: []policy.PodDisruptionBudget{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "pdb1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: policy.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
				},
				Status: policy.PodDisruptionBudgetStatus{
					PodDisruptionsAllowed: 5,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns2",
					Name:              "pdb2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: policy.PodDisruptionBudgetSpec{
					MinAvailable: &minAvailable,
				},
				Status: policy.PodDisruptionBudgetStatus{
					PodDisruptionsAllowed: 3,
				},
			},
		},
	}

	// Columns: Name, Min Available, Max Available, Allowed Disruptions, Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"pdb1", "N/A", "11", int64(5), "0s"}},
		{Cells: []interface{}{"pdb2", "22", "N/A", int64(3), "0s"}},
	}

	rows, err := printPodDisruptionBudgetList(&pdbList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing pod template list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintControllerRevision(t *testing.T) {
	tests := []struct {
		history  apps.ControllerRevision
		expected []metav1beta1.TableRow
	}{
		{
			history: apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
					OwnerReferences: []metav1.OwnerReference{
						{
							Controller: boolP(true),
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
							Name:       "foo",
						},
					},
				},
				Revision: 1,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "daemonset.apps/foo", int64(1), "0s"}}},
		},
		{
			history: apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
					OwnerReferences: []metav1.OwnerReference{
						{
							Controller: boolP(false),
							Kind:       "ABC",
							Name:       "foo",
						},
					},
				},
				Revision: 2,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test2", "<none>", int64(2), "0s"}}},
		},
		{
			history: apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
					OwnerReferences:   []metav1.OwnerReference{},
				},
				Revision: 3,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test3", "<none>", int64(3), "0s"}}},
		},
		{
			history: apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test4",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
					OwnerReferences:   nil,
				},
				Revision: 4,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test4", "<none>", int64(4), "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printControllerRevision(&test.history, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func boolP(b bool) *bool {
	return &b
}

func TestPrintConfigMap(t *testing.T) {
	tests := []struct {
		configMap api.ConfigMap
		expected  []metav1beta1.TableRow
	}{
		// Basic config map with no data.
		{
			configMap: api.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "configmap1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
			},
			// Columns: Name, Data, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"configmap1", int64(0), "0s"}}},
		},
		// Basic config map with one data entry
		{
			configMap: api.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "configmap2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			// Columns: Name, (Num) Data, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"configmap2", int64(1), "0s"}}},
		},
		// Basic config map with one data and one binary data entry.
		{
			configMap: api.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "configmap3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Data: map[string]string{
					"foo": "bar",
				},
				BinaryData: map[string][]byte{
					"bin": []byte("binary data"),
				},
			},
			// Columns: Name, (Num) Data, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"configmap3", int64(2), "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printConfigMap(&test.configMap, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintNetworkPolicy(t *testing.T) {
	tests := []struct {
		policy   networking.NetworkPolicy
		expected []metav1beta1.TableRow
	}{
		// Basic network policy with empty spec.
		{
			policy: networking.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "policy1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: networking.NetworkPolicySpec{},
			},
			// Columns: Name, Pod-Selector, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"policy1", "<none>", "0s"}}},
		},
		// Basic network policy with pod selector.
		{
			policy: networking.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "policy2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: networking.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			// Columns: Name, Pod-Selector, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"policy2", "foo=bar", "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printNetworkPolicy(&test.policy, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintRoleBinding(t *testing.T) {
	tests := []struct {
		binding  rbac.RoleBinding
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Basic role binding
		{
			binding: rbac.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "binding1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subjects: []rbac.Subject{
					{
						Kind: "User",
						Name: "system:kube-controller-manager",
					},
				},
				RoleRef: rbac.RoleRef{
					Kind: "Role",
					Name: "extension-apiserver-authentication-reader",
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"binding1", "0s"}}},
		},
		// Generate options=Wide; print subject and roles.
		{
			binding: rbac.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "binding2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subjects: []rbac.Subject{
					{
						Kind: "User",
						Name: "user-name",
					},
					{
						Kind: "Group",
						Name: "group-name",
					},
					{
						Kind:      "ServiceAccount",
						Name:      "service-account-name",
						Namespace: "service-account-namespace",
					},
				},
				RoleRef: rbac.RoleRef{
					Kind: "Role",
					Name: "role-name",
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Age, Role, Users, Groups, ServiceAccounts
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"binding2", "0s", "Role/role-name", "user-name", "group-name", "service-account-namespace/service-account-name"}}},
		},
	}

	for i, test := range tests {
		rows, err := printRoleBinding(&test.binding, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintClusterRoleBinding(t *testing.T) {
	tests := []struct {
		binding  rbac.ClusterRoleBinding
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Basic cluster role binding
		{
			binding: rbac.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "binding1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subjects: []rbac.Subject{
					{
						Kind: "User",
						Name: "system:kube-controller-manager",
					},
				},
				RoleRef: rbac.RoleRef{
					Kind: "Role",
					Name: "extension-apiserver-authentication-reader",
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"binding1", "0s"}}},
		},
		// Generate options=Wide; print subject and roles.
		{
			binding: rbac.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "binding2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subjects: []rbac.Subject{
					{
						Kind: "User",
						Name: "user-name",
					},
					{
						Kind: "Group",
						Name: "group-name",
					},
					{
						Kind:      "ServiceAccount",
						Name:      "service-account-name",
						Namespace: "service-account-namespace",
					},
				},
				RoleRef: rbac.RoleRef{
					Kind: "Role",
					Name: "role-name",
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Age, Role, Users, Groups, ServiceAccounts
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"binding2", "0s", "Role/role-name", "user-name", "group-name", "service-account-namespace/service-account-name"}}},
		},
	}

	for i, test := range tests {
		rows, err := printClusterRoleBinding(&test.binding, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}
func TestPrintCertificateSigningRequest(t *testing.T) {
	tests := []struct {
		csr      certificates.CertificateSigningRequest
		expected []metav1beta1.TableRow
	}{
		// Basic CSR with no spec or status; defaults to status: Pending.
		{
			csr: certificates.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "csr1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec:   certificates.CertificateSigningRequestSpec{},
				Status: certificates.CertificateSigningRequestStatus{},
			},
			// Columns: Name, Age, Requestor, Condition
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"csr1", "0s", "", "Pending"}}},
		},
		// Basic CSR with Spec and Status=Approved.
		{
			csr: certificates.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "csr2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: certificates.CertificateSigningRequestSpec{
					Username: "CSR Requestor",
				},
				Status: certificates.CertificateSigningRequestStatus{
					Conditions: []certificates.CertificateSigningRequestCondition{
						{
							Type: certificates.CertificateApproved,
						},
					},
				},
			},
			// Columns: Name, Age, Requestor, Condition
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"csr2", "0s", "CSR Requestor", "Approved"}}},
		},
		// Basic CSR with Spec and Status=Approved; certificate issued.
		{
			csr: certificates.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "csr2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: certificates.CertificateSigningRequestSpec{
					Username: "CSR Requestor",
				},
				Status: certificates.CertificateSigningRequestStatus{
					Conditions: []certificates.CertificateSigningRequestCondition{
						{
							Type: certificates.CertificateApproved,
						},
					},
					Certificate: []byte("cert data"),
				},
			},
			// Columns: Name, Age, Requestor, Condition
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"csr2", "0s", "CSR Requestor", "Approved,Issued"}}},
		},
		// Basic CSR with Spec and Status=Denied.
		{
			csr: certificates.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "csr3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: certificates.CertificateSigningRequestSpec{
					Username: "CSR Requestor",
				},
				Status: certificates.CertificateSigningRequestStatus{
					Conditions: []certificates.CertificateSigningRequestCondition{
						{
							Type: certificates.CertificateDenied,
						},
					},
				},
			},
			// Columns: Name, Age, Requestor, Condition
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"csr3", "0s", "CSR Requestor", "Denied"}}},
		},
	}

	for i, test := range tests {
		rows, err := printCertificateSigningRequest(&test.csr, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintReplicationController(t *testing.T) {
	tests := []struct {
		rc       api.ReplicationController
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Basic print replication controller without replicas or status.
		{
			rc: api.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rc1",
					Namespace: "test-namespace",
				},
				Spec: api.ReplicationControllerSpec{
					Selector: map[string]string{"a": "b"},
					Template: &api.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"a": "b"},
						},
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:                     "test",
									Image:                    "test_image",
									ImagePullPolicy:          api.PullIfNotPresent,
									TerminationMessagePolicy: api.TerminationMessageReadFile,
								},
							},
							RestartPolicy: api.RestartPolicyAlways,
							DNSPolicy:     api.DNSClusterFirst,
						},
					},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Desired, Current, Ready, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"rc1", int64(0), int64(0), int64(0), "<unknown>"}}},
		},
		// Basic print replication controller with replicas; does not print containers or labels
		{
			rc: api.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rc1",
					Namespace: "test-namespace",
				},
				Spec: api.ReplicationControllerSpec{
					Replicas: 5,
					Selector: map[string]string{"a": "b"},
					Template: &api.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"a": "b"},
						},
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:                     "test",
									Image:                    "test_image",
									ImagePullPolicy:          api.PullIfNotPresent,
									TerminationMessagePolicy: api.TerminationMessageReadFile,
								},
							},
							RestartPolicy: api.RestartPolicyAlways,
							DNSPolicy:     api.DNSClusterFirst,
						},
					},
				},
				Status: api.ReplicationControllerStatus{
					Replicas:      3,
					ReadyReplicas: 1,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Desired, Current, Ready, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"rc1", int64(5), int64(3), int64(1), "<unknown>"}}},
		},
		// Generate options: Wide; print containers and labels.
		{
			rc: api.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rc1",
				},
				Spec: api.ReplicationControllerSpec{
					Replicas: 5,
					Selector: map[string]string{"a": "b"},
					Template: &api.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"a": "b"},
						},
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:                     "test",
									Image:                    "test_image",
									ImagePullPolicy:          api.PullIfNotPresent,
									TerminationMessagePolicy: api.TerminationMessageReadFile,
								},
							},
							RestartPolicy: api.RestartPolicyAlways,
							DNSPolicy:     api.DNSClusterFirst,
						},
					},
				},
				Status: api.ReplicationControllerStatus{
					Replicas:      3,
					ReadyReplicas: 1,
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Desired, Current, Ready, Age, Containers, Images, Selector
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"rc1", int64(5), int64(3), int64(1), "<unknown>", "test", "test_image", "a=b"}}},
		},
	}

	for i, test := range tests {
		rows, err := printReplicationController(&test.rc, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintReplicaSet(t *testing.T) {
	tests := []struct {
		replicaSet apps.ReplicaSet
		options    printers.GenerateOptions
		expected   []metav1beta1.TableRow
	}{
		// Generate options empty
		{
			replicaSet: apps.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.ReplicaSetSpec{
					Replicas: 5,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
				Status: apps.ReplicaSetStatus{
					Replicas:      5,
					ReadyReplicas: 2,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Desired, Current, Ready, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", int64(5), int64(5), int64(2), "0s"}}},
		},
		// Generate options "Wide"
		{
			replicaSet: apps.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.ReplicaSetSpec{
					Replicas: 5,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
				Status: apps.ReplicaSetStatus{
					Replicas:      5,
					ReadyReplicas: 2,
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Desired, Current, Ready, Age, Containers, Images, Selector
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", int64(5), int64(5), int64(2), "0s", "fake-container1,fake-container2", "fake-image1,fake-image2", "foo=bar"}}},
		},
	}

	for i, test := range tests {
		rows, err := printReplicaSet(&test.replicaSet, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintReplicaSetList(t *testing.T) {

	replicaSetList := apps.ReplicaSetList{
		Items: []apps.ReplicaSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "replicaset1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.ReplicaSetSpec{
					Replicas: 5,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
				},
				Status: apps.ReplicaSetStatus{
					Replicas:      5,
					ReadyReplicas: 2,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "replicaset2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.ReplicaSetSpec{
					Replicas: 4,
					Template: api.PodTemplateSpec{},
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
				Status: apps.ReplicaSetStatus{
					Replicas:      3,
					ReadyReplicas: 1,
				},
			},
		},
	}

	// Columns: Name, Desired, Current, Ready, Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"replicaset1", int64(5), int64(5), int64(2), "0s"}},
		{Cells: []interface{}{"replicaset2", int64(4), int64(3), int64(1), "0s"}},
	}

	rows, err := printReplicaSetList(&replicaSetList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing replica set list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintStatefulSet(t *testing.T) {
	tests := []struct {
		statefulSet apps.StatefulSet
		options     printers.GenerateOptions
		expected    []metav1beta1.TableRow
	}{
		// Basic stateful set; no generate options.
		{
			statefulSet: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.StatefulSetSpec{
					Replicas: 5,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
				},
				Status: apps.StatefulSetStatus{
					Replicas:      5,
					ReadyReplicas: 2,
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Ready, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "2/5", "0s"}}},
		},
		// Generate options "Wide"; includes containers and images.
		{
			statefulSet: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: apps.StatefulSetSpec{
					Replicas: 5,
					Template: api.PodTemplateSpec{
						Spec: api.PodSpec{
							Containers: []api.Container{
								{
									Name:  "fake-container1",
									Image: "fake-image1",
								},
								{
									Name:  "fake-container2",
									Image: "fake-image2",
								},
							},
						},
					},
				},
				Status: apps.StatefulSetStatus{
					Replicas:      5,
					ReadyReplicas: 2,
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Ready, Age, Containers, Images
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "2/5", "0s", "fake-container1,fake-container2", "fake-image1,fake-image2"}}},
		},
	}

	for i, test := range tests {
		rows, err := printStatefulSet(&test.statefulSet, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPersistentVolume(t *testing.T) {
	myScn := "my-scn"

	claimRef := api.ObjectReference{
		Name:      "test",
		Namespace: "default",
	}
	tests := []struct {
		pv       api.PersistentVolume
		expected []metav1beta1.TableRow
	}{
		{
			// Test bound
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:    &claimRef,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadOnlyMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("4Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumeBound,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "4Gi", "ROX", "", "Bound", "default/test", "", "", "<unknown>", "<unset>"}}},
		},
		{
			// Test failed
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:    &claimRef,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadOnlyMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("4Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumeFailed,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test2", "4Gi", "ROX", "", "Failed", "default/test", "", "", "<unknown>", "<unset>"}}},
		},
		{
			// Test pending
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test3",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:    &claimRef,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadWriteMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumePending,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test3", "10Gi", "RWX", "", "Pending", "default/test", "", "", "<unknown>", "<unset>"}}},
		},
		{
			// Test pending, storageClass
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test4",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:         &claimRef,
					StorageClassName: myScn,
					AccessModes:      []api.PersistentVolumeAccessMode{api.ReadWriteOnce},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumePending,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test4", "10Gi", "RWO", "", "Pending", "default/test", "my-scn", "", "<unknown>", "<unset>"}}},
		},
		{
			// Test available
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test5",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:         &claimRef,
					StorageClassName: myScn,
					AccessModes:      []api.PersistentVolumeAccessMode{api.ReadWriteOnce},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumeAvailable,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test5", "10Gi", "RWO", "", "Available", "default/test", "my-scn", "", "<unknown>", "<unset>"}}},
		},
		{
			// Test released
			pv: api.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test6",
				},
				Spec: api.PersistentVolumeSpec{
					ClaimRef:         &claimRef,
					StorageClassName: myScn,
					AccessModes:      []api.PersistentVolumeAccessMode{api.ReadWriteOnce},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
				Status: api.PersistentVolumeStatus{
					Phase: api.VolumeReleased,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test6", "10Gi", "RWO", "", "Released", "default/test", "my-scn", "", "<unknown>", "<unset>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPersistentVolume(&test.pv, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPersistentVolumeClaim(t *testing.T) {
	volumeMode := api.PersistentVolumeFilesystem
	myScn := "my-scn"
	tests := []struct {
		pvc      api.PersistentVolumeClaim
		expected []metav1beta1.TableRow
	}{
		{
			// Test name, num of containers, restarts, container ready status
			pvc: api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
				},
				Spec: api.PersistentVolumeClaimSpec{
					VolumeName: "my-volume",
					VolumeMode: &volumeMode,
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase:       api.ClaimBound,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadOnlyMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("4Gi"),
					},
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test1", "Bound", "my-volume", "4Gi", "ROX", "", "<unknown>", "Filesystem"}}},
		},
		{
			// Test name, num of containers, restarts, container ready status
			pvc: api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: api.PersistentVolumeClaimSpec{
					VolumeMode: &volumeMode,
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase:       api.ClaimLost,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadOnlyMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("4Gi"),
					},
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test2", "Lost", "", "", "", "", "<unknown>", "Filesystem"}}},
		},
		{
			// Test name, num of containers, restarts, container ready status
			pvc: api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test3",
				},
				Spec: api.PersistentVolumeClaimSpec{
					VolumeName: "my-volume",
					VolumeMode: &volumeMode,
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase:       api.ClaimPending,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadWriteMany},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test3", "Pending", "my-volume", "10Gi", "RWX", "", "<unknown>", "Filesystem"}}},
		},
		{
			// Test name, num of containers, restarts, container ready status
			pvc: api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test4",
				},
				Spec: api.PersistentVolumeClaimSpec{
					VolumeName:       "my-volume",
					StorageClassName: &myScn,
					VolumeMode:       &volumeMode,
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase:       api.ClaimPending,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadWriteOnce},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test4", "Pending", "my-volume", "10Gi", "RWO", "my-scn", "<unknown>", "Filesystem"}}},
		},
		{
			// Test name, num of containers, restarts, container ready status
			pvc: api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test5",
				},
				Spec: api.PersistentVolumeClaimSpec{
					VolumeName:       "my-volume",
					StorageClassName: &myScn,
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase:       api.ClaimPending,
					AccessModes: []api.PersistentVolumeAccessMode{api.ReadWriteOnce},
					Capacity: map[api.ResourceName]resource.Quantity{
						api.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"test5", "Pending", "my-volume", "10Gi", "RWO", "my-scn", "<unknown>", "<unset>"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPersistentVolumeClaim(&test.pvc, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintComponentStatus(t *testing.T) {
	tests := []struct {
		componentStatus api.ComponentStatus
		expected        []metav1beta1.TableRow
	}{
		// Basic component status without conditions
		{
			componentStatus: api.ComponentStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cs1",
				},
				Conditions: []api.ComponentCondition{},
			},
			// Columns: Name, Status, Message, Error
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cs1", "Unknown", "", ""}}},
		},
		// Basic component status with healthy condition.
		{
			componentStatus: api.ComponentStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cs2",
				},
				Conditions: []api.ComponentCondition{
					{
						Type:    "Healthy",
						Status:  api.ConditionTrue,
						Message: "test message",
						Error:   "test error",
					},
				},
			},
			// Columns: Name, Status, Message, Error
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cs2", "Healthy", "test message", "test error"}}},
		},
		// Basic component status with healthy condition.
		{
			componentStatus: api.ComponentStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cs3",
				},
				Conditions: []api.ComponentCondition{
					{
						Type:    "Healthy",
						Status:  api.ConditionFalse,
						Message: "test message",
						Error:   "test error",
					},
				},
			},
			// Columns: Name, Status, Message, Error
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cs3", "Unhealthy", "test message", "test error"}}},
		},
	}

	for i, test := range tests {
		rows, err := printComponentStatus(&test.componentStatus, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintCronJob(t *testing.T) {
	completions := int32(2)
	suspend := false
	tests := []struct {
		cronjob  batch.CronJob
		options  printers.GenerateOptions
		expected []metav1beta1.TableRow
	}{
		// Basic cron job; does not print containers, images, or labels.
		{
			cronjob: batch.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "0/5 * * * ?",
					Suspend:  &suspend,
					JobTemplate: batch.JobTemplateSpec{
						Spec: batch.JobSpec{
							Completions: &completions,
							Template: api.PodTemplateSpec{
								Spec: api.PodSpec{
									Containers: []api.Container{
										{
											Name:  "fake-job-container1",
											Image: "fake-job-image1",
										},
										{
											Name:  "fake-job-container2",
											Image: "fake-job-image2",
										},
									},
								},
							},
							Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
						},
					},
				},
				Status: batch.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Now().Add(1.9e9)},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Schedule, Suspend, Active, Last Schedule, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cronjob1", "0/5 * * * ?", "False", int64(0), "0s", "0s"}}},
		},
		// Generate options: Wide; prints containers, images, and labels.
		{
			cronjob: batch.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "0/5 * * * ?",
					Suspend:  &suspend,
					JobTemplate: batch.JobTemplateSpec{
						Spec: batch.JobSpec{
							Completions: &completions,
							Template: api.PodTemplateSpec{
								Spec: api.PodSpec{
									Containers: []api.Container{
										{
											Name:  "fake-job-container1",
											Image: "fake-job-image1",
										},
										{
											Name:  "fake-job-container2",
											Image: "fake-job-image2",
										},
									},
								},
							},
							Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
						},
					},
				},
				Status: batch.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Now().Add(1.9e9)},
				},
			},
			options: printers.GenerateOptions{Wide: true},
			// Columns: Name, Schedule, Suspend, Active, Last Schedule, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cronjob1", "0/5 * * * ?", "False", int64(0), "0s", "0s", "fake-job-container1,fake-job-container2", "fake-job-image1,fake-job-image2", "a=b"}}},
		},
		// CronJob with Last Schedule and Age
		{
			cronjob: batch.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "0/5 * * * ?",
					Suspend:  &suspend,
				},
				Status: batch.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Now().Add(-3e10)},
				},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Schedule, Suspend, Active, Last Schedule, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cronjob2", "0/5 * * * ?", "False", int64(0), "30s", "5m"}}},
		},
		// CronJob without Last Schedule
		{
			cronjob: batch.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "0/5 * * * ?",
					Suspend:  &suspend,
				},
				Status: batch.CronJobStatus{},
			},
			options: printers.GenerateOptions{},
			// Columns: Name, Schedule, Suspend, Active, Last Schedule, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"cronjob3", "0/5 * * * ?", "False", int64(0), "<none>", "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printCronJob(&test.cronjob, test.options)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintCronJobList(t *testing.T) {
	completions := int32(2)
	suspend := false

	cronJobList := batch.CronJobList{
		Items: []batch.CronJob{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "0/5 * * * ?",
					Suspend:  &suspend,
					JobTemplate: batch.JobTemplateSpec{
						Spec: batch.JobSpec{
							Completions: &completions,
							Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
						},
					},
				},
				Status: batch.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Now().Add(1.9e9)},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cronjob2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: batch.CronJobSpec{
					Schedule: "4/5 1 1 1 ?",
					Suspend:  &suspend,
					JobTemplate: batch.JobTemplateSpec{
						Spec: batch.JobSpec{
							Completions: &completions,
							Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
						},
					},
				},
				Status: batch.CronJobStatus{
					LastScheduleTime: &metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
				},
			},
		},
	}

	// Columns: Name, Schedule, Suspend, Active, Last Schedule, Age
	expectedRows := []metav1beta1.TableRow{
		{Cells: []interface{}{"cronjob1", "0/5 * * * ?", "False", int64(0), "0s", "0s"}},
		{Cells: []interface{}{"cronjob2", "4/5 1 1 1 ?", "False", int64(0), "20m", "0s"}},
	}

	rows, err := printCronJobList(&cronJobList, printers.GenerateOptions{})
	if err != nil {
		t.Fatalf("Error printing job list: %#v", err)
	}
	for i := range rows {
		rows[i].Object.Object = nil
	}
	if !reflect.DeepEqual(expectedRows, rows) {
		t.Errorf("mismatch: %s", diff.ObjectReflectDiff(expectedRows, rows))
	}
}

func TestPrintStorageClass(t *testing.T) {
	policyDelte := api.PersistentVolumeReclaimDelete
	policyRetain := api.PersistentVolumeReclaimRetain
	bindModeImmediate := storage.VolumeBindingImmediate
	bindModeWait := storage.VolumeBindingWaitForFirstConsumer
	tests := []struct {
		sc       storage.StorageClass
		expected []metav1beta1.TableRow
	}{
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Provisioner: "kubernetes.io/glusterfs",
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc1", "kubernetes.io/glusterfs", "Delete",
				"Immediate", false, "0s"}}},
		},
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Provisioner: "kubernetes.io/nfs",
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc2", "kubernetes.io/nfs", "Delete",
				"Immediate", false, "5m"}}},
		},
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Provisioner:   "kubernetes.io/nfs",
				ReclaimPolicy: &policyDelte,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc3", "kubernetes.io/nfs", "Delete",
				"Immediate", false, "5m"}}},
		},
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc4",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Provisioner:       "kubernetes.io/nfs",
				ReclaimPolicy:     &policyRetain,
				VolumeBindingMode: &bindModeImmediate,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc4", "kubernetes.io/nfs", "Retain",
				"Immediate", false, "5m"}}},
		},
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc5",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Provisioner:       "kubernetes.io/nfs",
				ReclaimPolicy:     &policyRetain,
				VolumeBindingMode: &bindModeWait,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc5", "kubernetes.io/nfs", "Retain",
				"WaitForFirstConsumer", false, "5m"}}},
		},
		{
			sc: storage.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sc6",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Provisioner:          "kubernetes.io/nfs",
				ReclaimPolicy:        &policyRetain,
				AllowVolumeExpansion: boolP(true),
				VolumeBindingMode:    &bindModeWait,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"sc6", "kubernetes.io/nfs", "Retain",
				"WaitForFirstConsumer", true, "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printStorageClass(&test.sc, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintLease(t *testing.T) {
	holder1 := "holder1"
	holder2 := "holder2"
	tests := []struct {
		lease    coordination.Lease
		expected []metav1beta1.TableRow
	}{
		{
			lease: coordination.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "lease1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Spec: coordination.LeaseSpec{
					HolderIdentity: &holder1,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"lease1", "holder1", "0s"}}},
		},
		{
			lease: coordination.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "lease2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Spec: coordination.LeaseSpec{
					HolderIdentity: &holder2,
				},
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"lease2", "holder2", "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printLease(&test.lease, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintPriorityClass(t *testing.T) {
	tests := []struct {
		pc       scheduling.PriorityClass
		expected []metav1beta1.TableRow
	}{
		{
			pc: scheduling.PriorityClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pc1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Value: 1,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pc1", int64(1), bool(false), "0s"}}},
		},
		{
			pc: scheduling.PriorityClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pc2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Value:         1000000000,
				GlobalDefault: true,
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"pc2", int64(1000000000), bool(true), "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printPriorityClass(&test.pc, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintRuntimeClass(t *testing.T) {
	tests := []struct {
		rc       nodeapi.RuntimeClass
		expected []metav1beta1.TableRow
	}{
		{
			rc: nodeapi.RuntimeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rc1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Handler: "h1",
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"rc1", "h1", "0s"}}},
		},
		{
			rc: nodeapi.RuntimeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rc2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Handler: "h2",
			},
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"rc2", "h2", "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printRuntimeClass(&test.rc, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}

func TestPrintEndpoint(t *testing.T) {

	tests := []struct {
		endpoint api.Endpoints
		expected []metav1beta1.TableRow
	}{
		// Basic endpoint with no IP's
		{
			endpoint: api.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "endpoint1",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
			},
			// Columns: Name, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"endpoint1", "<none>", "0s"}}},
		},
		// Endpoint with no ports
		{
			endpoint: api.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "endpoint3",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: "1.2.3.4",
							},
							{
								IP: "5.6.7.8",
							},
						},
					},
				},
			},
			// Columns: Name, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"endpoint3", "1.2.3.4,5.6.7.8", "5m"}}},
		},
		// Basic endpoint with two IP's and one port
		{
			endpoint: api.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "endpoint2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: "1.2.3.4",
							},
							{
								IP: "5.6.7.8",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     8001,
								Protocol: "tcp",
							},
						},
					},
				},
			},
			// Columns: Name, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"endpoint2", "1.2.3.4:8001,5.6.7.8:8001", "0s"}}},
		},
		// Basic endpoint with greater than three IP's triggering "more" string
		{
			endpoint: api.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "endpoint2",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: "1.2.3.4",
							},
							{
								IP: "5.6.7.8",
							},
							{
								IP: "9.8.7.6",
							},
							{
								IP: "6.6.6.6",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     8001,
								Protocol: "tcp",
							},
						},
					},
				},
			},
			// Columns: Name, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"endpoint2", "1.2.3.4:8001,5.6.7.8:8001,9.8.7.6:8001 + 1 more...", "0s"}}},
		},
	}

	for i, test := range tests {
		rows, err := printEndpoints(&test.endpoint, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}

}

func TestPrintEndpointSlice(t *testing.T) {
	tcpProtocol := api.ProtocolTCP

	tests := []struct {
		endpointSlice discovery.EndpointSlice
		expected      []metav1beta1.TableRow
	}{
		{
			endpointSlice: discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "abcslice.123",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1.9e9)},
				},
				AddressType: discovery.AddressTypeIPv4,
				Ports: []discovery.EndpointPort{{
					Name:     utilpointer.StringPtr("http"),
					Port:     utilpointer.Int32Ptr(80),
					Protocol: &tcpProtocol,
				}},
				Endpoints: []discovery.Endpoint{{
					Addresses: []string{"10.1.2.3", "2001:db8::1234:5678"},
				}},
			},
			// Columns: Name, AddressType, Ports, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"abcslice.123", "IPv4", "80", "10.1.2.3,2001:db8::1234:5678", "0s"}}},
		}, {
			endpointSlice: discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "longerslicename.123",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				AddressType: discovery.AddressTypeIPv6,
				Ports: []discovery.EndpointPort{{
					Name:     utilpointer.StringPtr("http"),
					Port:     utilpointer.Int32Ptr(80),
					Protocol: &tcpProtocol,
				}, {
					Name:     utilpointer.StringPtr("https"),
					Port:     utilpointer.Int32Ptr(443),
					Protocol: &tcpProtocol,
				}},
				Endpoints: []discovery.Endpoint{{
					Addresses: []string{"10.1.2.3", "2001:db8::1234:5678"},
				}, {
					Addresses: []string{"10.2.3.4", "2001:db8::2345:6789"},
				}},
			},
			// Columns: Name, AddressType, Ports, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"longerslicename.123", "IPv6", "80,443", "10.1.2.3,2001:db8::1234:5678,10.2.3.4 + 1 more...", "5m"}}},
		}, {
			endpointSlice: discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "multiportslice.123",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-3e11)},
				},
				AddressType: discovery.AddressTypeIPv4,
				Ports: []discovery.EndpointPort{{
					Name:     utilpointer.StringPtr("http"),
					Port:     utilpointer.Int32Ptr(80),
					Protocol: &tcpProtocol,
				}, {
					Name:     utilpointer.StringPtr("https"),
					Port:     utilpointer.Int32Ptr(443),
					Protocol: &tcpProtocol,
				}, {
					Name:     utilpointer.StringPtr("extra1"),
					Port:     utilpointer.Int32Ptr(3000),
					Protocol: &tcpProtocol,
				}, {
					Name:     utilpointer.StringPtr("extra2"),
					Port:     utilpointer.Int32Ptr(3001),
					Protocol: &tcpProtocol,
				}},
				Endpoints: []discovery.Endpoint{{
					Addresses: []string{"10.1.2.3", "2001:db8::1234:5678"},
				}, {
					Addresses: []string{"10.2.3.4", "2001:db8::2345:6789"},
				}},
			},
			// Columns: Name, AddressType, Ports, Endpoints, Age
			expected: []metav1beta1.TableRow{{Cells: []interface{}{"multiportslice.123", "IPv4", "80,443,3000 + 1 more...", "10.1.2.3,2001:db8::1234:5678,10.2.3.4 + 1 more...", "5m"}}},
		},
	}

	for i, test := range tests {
		rows, err := printEndpointSlice(&test.endpointSlice, printers.GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			rows[i].Object.Object = nil
		}
		if !reflect.DeepEqual(test.expected, rows) {
			t.Errorf("%d mismatch: %s", i, diff.ObjectReflectDiff(test.expected, rows))
		}
	}
}
