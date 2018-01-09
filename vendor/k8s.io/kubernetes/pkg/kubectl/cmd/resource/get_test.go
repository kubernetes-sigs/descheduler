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

package resource

import (
	"bytes"
	encjson "encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
	restclientwatch "k8s.io/client-go/rest/watch"
	"k8s.io/kube-openapi/pkg/util/proto"
	"k8s.io/kubernetes/pkg/api/testapi"
	apitesting "k8s.io/kubernetes/pkg/api/testing"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/v1"

	cmdtesting "k8s.io/kubernetes/pkg/kubectl/cmd/testing"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/cmd/util/openapi"
	openapitesting "k8s.io/kubernetes/pkg/kubectl/cmd/util/openapi/testing"
	"k8s.io/kubernetes/pkg/kubectl/scheme"
)

var openapiSchemaPath = filepath.Join("..", "..", "..", "..", "api", "openapi-spec", "swagger.json")

// This init should be removed after switching this command and its tests to user external types.
func init() {
	api.AddToScheme(scheme.Scheme)
	scheme.Scheme.AddConversionFuncs(v1.Convert_core_PodSpec_To_v1_PodSpec)
	scheme.Scheme.AddConversionFuncs(v1.Convert_v1_PodSecurityContext_To_core_PodSecurityContext)
}

var unstructuredSerializer = dynamic.ContentConfig().NegotiatedSerializer

func defaultHeader() http.Header {
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}

func defaultClientConfig() *restclient.Config {
	return &restclient.Config{
		APIPath: "/api",
		ContentConfig: restclient.ContentConfig{
			NegotiatedSerializer: scheme.Codecs,
			ContentType:          runtime.ContentTypeJSON,
			GroupVersion:         &scheme.Registry.GroupOrDie(api.GroupName).GroupVersion,
		},
	}
}

func defaultClientConfigForVersion(version *schema.GroupVersion) *restclient.Config {
	return &restclient.Config{
		APIPath: "/api",
		ContentConfig: restclient.ContentConfig{
			NegotiatedSerializer: scheme.Codecs,
			ContentType:          runtime.ContentTypeJSON,
			GroupVersion:         version,
		},
	}
}

type testPrinter struct {
	Objects        []runtime.Object
	Err            error
	GenericPrinter bool
}

func (t *testPrinter) PrintObj(obj runtime.Object, out io.Writer) error {
	t.Objects = append(t.Objects, obj)
	fmt.Fprintf(out, "%#v", obj)
	return t.Err
}

// TODO: implement HandledResources()
func (t *testPrinter) HandledResources() []string {
	return []string{}
}

func (t *testPrinter) AfterPrint(output io.Writer, res string) error {
	return nil
}

func (t *testPrinter) IsGeneric() bool {
	return t.GenericPrinter
}

func objBody(codec runtime.Codec, obj runtime.Object) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, obj))))
}

func stringBody(body string) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(body)))
}

func initTestErrorHandler(t *testing.T) {
	cmdutil.BehaviorOnFatal(func(str string, code int) {
		t.Errorf("Error running command (exit code %d): %s", code, str)
	})
}

func testData() (*api.PodList, *api.ServiceList, *api.ReplicationControllerList) {
	pods := &api.PodList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "15",
		},
		Items: []api.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test", ResourceVersion: "10"},
				Spec:       apitesting.DeepEqualSafePodSpec(),
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "test", ResourceVersion: "11"},
				Spec:       apitesting.DeepEqualSafePodSpec(),
			},
		},
	}
	svc := &api.ServiceList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "16",
		},
		Items: []api.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "baz", Namespace: "test", ResourceVersion: "12"},
				Spec: api.ServiceSpec{
					SessionAffinity: "None",
					Type:            api.ServiceTypeClusterIP,
				},
			},
		},
	}
	rc := &api.ReplicationControllerList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "17",
		},
		Items: []api.ReplicationController{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "rc1", Namespace: "test", ResourceVersion: "18"},
				Spec: api.ReplicationControllerSpec{
					Replicas: 1,
				},
			},
		},
	}
	return pods, svc, rc
}

func testComponentStatusData() *api.ComponentStatusList {
	good := api.ComponentStatus{
		Conditions: []api.ComponentCondition{
			{Type: api.ComponentHealthy, Status: api.ConditionTrue, Message: "ok"},
		},
		ObjectMeta: metav1.ObjectMeta{Name: "servergood"},
	}

	bad := api.ComponentStatus{
		Conditions: []api.ComponentCondition{
			{Type: api.ComponentHealthy, Status: api.ConditionFalse, Message: "", Error: "bad status: 500"},
		},
		ObjectMeta: metav1.ObjectMeta{Name: "serverbad"},
	}

	unknown := api.ComponentStatus{
		Conditions: []api.ComponentCondition{
			{Type: api.ComponentHealthy, Status: api.ConditionUnknown, Message: "", Error: "fizzbuzz error"},
		},
		ObjectMeta: metav1.ObjectMeta{Name: "serverunknown"},
	}

	return &api.ComponentStatusList{
		Items: []api.ComponentStatus{good, bad, unknown},
	}
}

// Verifies that schemas that are not in the master tree of Kubernetes can be retrieved via Get.
func TestGetUnknownSchemaObject(t *testing.T) {
	f, tf, _, _ := cmdtesting.NewAPIFactory()
	tf.WithCustomScheme()
	_, _, codec, _ := cmdtesting.NewTestFactory()
	tf.Printer = &testPrinter{}
	tf.OpenAPISchemaFunc = openapitesting.CreateOpenAPISchemaFunc(openapiSchemaPath)

	obj := &cmdtesting.ExternalType{
		Kind:       "Type",
		APIVersion: "apitest/unlikelyversion",
		Name:       "foo",
	}

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp: &http.Response{
			StatusCode: 200, Header: defaultHeader(),
			Body: objBody(codec, obj),
		},
	}
	tf.Namespace = "test"
	tf.ClientConfig = defaultClientConfig()

	mapper, _ := f.Object()
	m, err := mapper.RESTMapping(schema.GroupKind{Group: "apitest", Kind: "Type"})
	if err != nil {
		t.Fatal(err)
	}
	convertedObj, err := m.ConvertToVersion(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Type",
			"apiVersion": "apitest/unlikelyversion",
			"name":       "foo",
		},
	}, schema.GroupVersion{Group: "apitest", Version: "unlikelyversion"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(convertedObj, obj) {
		t.Fatalf("unexpected conversion of unstructured object to structured: %s", diff.ObjectReflectDiff(convertedObj, obj))
	}

	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Run(cmd, []string{"type", "foo"})

	expected := []runtime.Object{cmdtesting.NewInternalType("", "", "foo")}
	actual := tf.Printer.(*testPrinter).Objects
	if len(actual) != len(expected) {
		t.Fatalf("expected: %#v, but actual: %#v", expected, actual)
	}
	t.Logf("actual: %#v", actual[0])
	for i, obj := range actual {
		expectedJSON := runtime.EncodeOrDie(codec, expected[i])
		expectedMap := map[string]interface{}{}
		if err := encjson.Unmarshal([]byte(expectedJSON), &expectedMap); err != nil {
			t.Fatal(err)
		}

		actualJSON := runtime.EncodeOrDie(codec, obj)
		actualMap := map[string]interface{}{}
		if err := encjson.Unmarshal([]byte(actualJSON), &actualMap); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(expectedMap, actualMap) {
			t.Errorf("expectedMap: %#v, but actualMap: %#v", expectedMap, actualMap)
		}
	}
}

// Verifies that schemas that are not in the master tree of Kubernetes can be retrieved via Get.
func TestGetSchemaObject(t *testing.T) {
	f, tf, _, _ := cmdtesting.NewAPIFactory()
	tf.Mapper = testapi.Default.RESTMapper()
	tf.Typer = scheme.Scheme
	codec := testapi.Default.Codec()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &api.ReplicationController{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})},
	}
	tf.Namespace = "test"
	tf.ClientConfig = defaultClientConfig()
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.Run(cmd, []string{"replicationcontrollers", "foo"})

	if !strings.Contains(buf.String(), "\"foo\"") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestGetObjectsWithOpenAPIOutputFormatPresent(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	// overide the openAPISchema function to return custom output
	// for Pod type.
	tf.OpenAPISchemaFunc = testOpenAPISchemaData
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods.Items[0])},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Set(useOpenAPIPrintColumnFlagLabel, "true")
	cmd.Run(cmd, []string{"pods", "foo"})

	expected := []runtime.Object{&pods.Items[0]}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

type FakeResources struct {
	resources map[schema.GroupVersionKind]proto.Schema
}

func (f FakeResources) LookupResource(s schema.GroupVersionKind) proto.Schema {
	return f.resources[s]
}

var _ openapi.Resources = &FakeResources{}

func testOpenAPISchemaData() (openapi.Resources, error) {
	return &FakeResources{
		resources: map[schema.GroupVersionKind]proto.Schema{
			{
				Version: "v1",
				Kind:    "Pod",
			}: &proto.Primitive{
				BaseSchema: proto.BaseSchema{
					Extensions: map[string]interface{}{
						"x-kubernetes-print-columns": "custom-columns=NAME:.metadata.name,RSRC:.metadata.resourceVersion",
					},
				},
			},
		},
	}, nil
}

func TestGetObjects(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods.Items[0])},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Run(cmd, []string{"pods", "foo"})

	expected := []runtime.Object{&pods.Items[0]}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetObjectsFiltered(t *testing.T) {
	initTestErrorHandler(t)

	pods, _, _ := testData()
	pods.Items[0].Status.Phase = api.PodFailed
	first := &pods.Items[0]
	second := &pods.Items[1]

	testCases := []struct {
		args           []string
		resp           runtime.Object
		flags          map[string]string
		expect         []runtime.Object
		genericPrinter bool
	}{
		{args: []string{"pods", "foo"}, resp: first, expect: []runtime.Object{first}, genericPrinter: true},
		{args: []string{"pods", "foo"}, flags: map[string]string{"show-all": "false"}, resp: first, expect: []runtime.Object{first}, genericPrinter: true},
		{args: []string{"pods"}, flags: map[string]string{"show-all": "true"}, resp: pods, expect: []runtime.Object{first, second}},
		{args: []string{"pods/foo"}, resp: first, expect: []runtime.Object{first}, genericPrinter: true},
		{args: []string{"pods"}, flags: map[string]string{"output": "yaml"}, resp: pods, expect: []runtime.Object{second}},
		{args: []string{}, flags: map[string]string{"filename": "../../../../examples/storage/cassandra/cassandra-controller.yaml"}, resp: pods, expect: []runtime.Object{first, second}},

		{args: []string{"pods"}, resp: pods, expect: []runtime.Object{second}},
		{args: []string{"pods"}, flags: map[string]string{"show-all": "true", "output": "yaml"}, resp: pods, expect: []runtime.Object{first, second}},
		{args: []string{"pods"}, flags: map[string]string{"show-all": "false"}, resp: pods, expect: []runtime.Object{second}},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			f, tf, codec, _ := cmdtesting.NewAPIFactory()
			tf.WithLegacyScheme()
			tf.Printer = &testPrinter{GenericPrinter: test.genericPrinter}
			tf.UnstructuredClient = &fake.RESTClient{
				GroupVersion:         schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: unstructuredSerializer,
				Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, test.resp)},
			}
			tf.Namespace = "test"
			buf := bytes.NewBuffer([]byte{})
			errBuf := bytes.NewBuffer([]byte{})

			cmd := NewCmdGet(f, buf, errBuf)
			cmd.SetOutput(buf)
			for k, v := range test.flags {
				cmd.Flags().Lookup(k).Value.Set(v)
			}
			cmd.Run(cmd, test.args)

			verifyObjects(t, test.expect, tf.Printer.(*testPrinter).Objects)

			if len(buf.String()) == 0 {
				t.Errorf("%d: unexpected empty output", i)
			}
		})
	}
}

func TestGetObjectIgnoreNotFound(t *testing.T) {
	initTestErrorHandler(t)

	ns := &api.NamespaceList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "1",
		},
		Items: []api.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "testns", Namespace: "test", ResourceVersion: "11"},
				Spec:       api.NamespaceSpec{},
			},
		},
	}

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{GenericPrinter: true}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/test/pods/nonexistentpod" && m == "GET":
				return &http.Response{StatusCode: 404, Header: defaultHeader(), Body: stringBody("")}, nil
			case p == "/api/v1/namespaces/test" && m == "GET":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &ns.Items[0])}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Set("ignore-not-found", "true")
	cmd.Flags().Set("output", "yaml")
	cmd.Run(cmd, []string{"pods", "nonexistentpod"})

	if buf.String() != "" {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestGetSortedObjects(t *testing.T) {
	pods := &api.PodList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "15",
		},
		Items: []api.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "test", ResourceVersion: "10"},
				Spec:       apitesting.DeepEqualSafePodSpec(),
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "test", ResourceVersion: "11"},
				Spec:       apitesting.DeepEqualSafePodSpec(),
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "test", ResourceVersion: "9"},
				Spec:       apitesting.DeepEqualSafePodSpec(),
			},
		},
	}

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}
	tf.Namespace = "test"
	tf.ClientConfig = &restclient.Config{ContentConfig: restclient.ContentConfig{GroupVersion: &schema.GroupVersion{Version: "v1"}}}

	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	// sorting with metedata.name
	cmd.Flags().Set("sort-by", ".metadata.name")
	cmd.Run(cmd, []string{"pods"})

	// expect sorted: a,b,c
	expected := []runtime.Object{&pods.Items[2], &pods.Items[1], &pods.Items[0]}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func verifyObjects(t *testing.T, expected, actual []runtime.Object) {
	var actualObj runtime.Object
	var err error

	if len(actual) != len(expected) {
		t.Fatalf("expected %d, but actual %d", len(expected), len(actual))
	}
	for i, obj := range actual {
		switch obj.(type) {
		case runtime.Unstructured, *runtime.Unknown:
			actualObj, err = runtime.Decode(
				scheme.Codecs.UniversalDecoder(),
				[]byte(runtime.EncodeOrDie(scheme.Codecs.LegacyCodec(), obj)))
		default:
			actualObj = obj
			err = nil
		}

		if err != nil {
			t.Fatal(err)
		}
		if !apiequality.Semantic.DeepEqual(expected[i], actualObj) {
			t.Errorf("expected object: %#v, but actualObj:%#v\n", expected[i], actualObj)
		}
	}
}

func TestGetObjectsIdentifiedByFile(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{GenericPrinter: true}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods.Items[0])},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Set("filename", "../../../../examples/storage/cassandra/cassandra-controller.yaml")
	cmd.Run(cmd, []string{})

	expected := []runtime.Object{&pods.Items[0]}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetListObjects(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Run(cmd, []string{"pods"})

	expected, err := extractResourceList([]runtime.Object{pods})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func extractResourceList(objs []runtime.Object) ([]runtime.Object, error) {
	finalObjs := []runtime.Object{}
	for _, obj := range objs {
		items, err := meta.ExtractList(obj)
		if err != nil {
			return nil, err
		}
		finalObjs = append(finalObjs, items...)
	}
	return finalObjs, nil
}

func TestGetAllListObjects(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Set("show-all", "true")
	cmd.Run(cmd, []string{"pods"})

	expected, err := extractResourceList([]runtime.Object{pods})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetListComponentStatus(t *testing.T) {
	statuses := testComponentStatusData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, statuses)},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Run(cmd, []string{"componentstatuses"})

	expected, err := extractResourceList([]runtime.Object{statuses})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetMixedGenericObjects(t *testing.T) {
	initTestErrorHandler(t)

	// ensure that a runtime.Object without
	// an ObjectMeta field is handled properly
	structuredObj := &metav1.Status{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Status",
			APIVersion: "v1",
		},
		Status:  "Success",
		Message: "",
		Reason:  "",
		Code:    0,
	}

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{GenericPrinter: true}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, structuredObj)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	tf.ClientConfig = defaultClientConfig()
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Set("output", "json")
	cmd.Run(cmd, []string{"pods"})

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}

	actual := tf.Printer.(*testPrinter).Objects
	fn := func(obj runtime.Object) unstructured.Unstructured {
		data, err := runtime.Encode(scheme.Codecs.LegacyCodec(schema.GroupVersion{Version: "v1"}), obj)
		if err != nil {
			panic(err)
		}
		out := &unstructured.Unstructured{Object: make(map[string]interface{})}
		if err := encjson.Unmarshal(data, &out.Object); err != nil {
			panic(err)
		}
		return *out
	}

	expected := &unstructured.UnstructuredList{
		Object: map[string]interface{}{"kind": "List", "apiVersion": "v1", "metadata": map[string]interface{}{"selfLink": "", "resourceVersion": ""}},
		Items: []unstructured.Unstructured{
			fn(structuredObj),
		},
	}
	actualBytes, err := encjson.Marshal(actual[0])
	if err != nil {
		t.Fatal(err)
	}
	expectedBytes, err := encjson.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualBytes) != string(expectedBytes) {
		t.Errorf("expectedBytes: %s,but actualBytes: %s", expectedBytes, actualBytes)
	}
}

func TestGetMultipleTypeObjects(t *testing.T) {
	pods, svc, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)}, nil
			case "/namespaces/test/services":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, svc)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Run(cmd, []string{"pods,services"})

	expected, err := extractResourceList([]runtime.Object{pods, svc})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetMultipleTypeObjectsAsList(t *testing.T) {
	pods, svc, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{GenericPrinter: true}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)}, nil
			case "/namespaces/test/services":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, svc)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	tf.ClientConfig = defaultClientConfig()
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("output", "json")
	cmd.Run(cmd, []string{"pods,services"})

	actual := tf.Printer.(*testPrinter).Objects
	fn := func(obj runtime.Object) unstructured.Unstructured {
		data, err := runtime.Encode(scheme.Codecs.LegacyCodec(schema.GroupVersion{Version: "v1"}), obj)
		if err != nil {
			t.Fatal(err)
		}
		out := &unstructured.Unstructured{Object: make(map[string]interface{})}
		if err := encjson.Unmarshal(data, &out.Object); err != nil {
			t.Fatal(err)
		}
		return *out
	}

	expected := &unstructured.UnstructuredList{
		Object: map[string]interface{}{"kind": "List", "apiVersion": "v1", "metadata": map[string]interface{}{"selfLink": "", "resourceVersion": ""}},
		Items: []unstructured.Unstructured{
			fn(&pods.Items[0]),
			fn(&pods.Items[1]),
			fn(&svc.Items[0]),
		},
	}
	actualBytes, err := encjson.Marshal(actual[0])
	if err != nil {
		t.Fatal(err)
	}
	expectedBytes, err := encjson.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualBytes) != string(expectedBytes) {
		t.Errorf("expectedBytes: %s,but actualBytes: %s", expectedBytes, actualBytes)
	}
}

func TestGetMultipleTypeObjectsWithLabelSelector(t *testing.T) {
	pods, svc, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get(metav1.LabelSelectorQueryParam("v1")) != "a=b" {
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
			}
			switch req.URL.Path {
			case "/namespaces/test/pods":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)}, nil
			case "/namespaces/test/services":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, svc)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("selector", "a=b")
	cmd.Run(cmd, []string{"pods,services"})

	expected, err := extractResourceList([]runtime.Object{pods, svc})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetMultipleTypeObjectsWithFieldSelector(t *testing.T) {
	pods, svc, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get(metav1.FieldSelectorQueryParam("v1")) != "a=b" {
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
			}
			switch req.URL.Path {
			case "/namespaces/test/pods":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)}, nil
			case "/namespaces/test/services":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, svc)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("field-selector", "a=b")
	cmd.Run(cmd, []string{"pods,services"})

	expected, err := extractResourceList([]runtime.Object{pods, svc})
	if err != nil {
		t.Fatal(err)
	}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Errorf("unexpected empty output")
	}
}

func TestGetMultipleTypeObjectsWithDirectReference(t *testing.T) {
	_, svc, _ := testData()
	node := &api.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: api.NodeSpec{
			ExternalID: "ext",
		},
	}

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/nodes/foo":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, node)}, nil
			case "/namespaces/test/services/bar":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &svc.Items[0])}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Run(cmd, []string{"services/bar", "node/foo"})

	expected := []runtime.Object{&svc.Items[0], node}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestGetByFormatForcesFlag(t *testing.T) {
	pods, _, _ := testData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{GenericPrinter: true}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods.Items[0])},
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)
	cmd.Flags().Lookup("output").Value.Set("yaml")
	cmd.Run(cmd, []string{"pods"})

	showAllFlag, _ := cmd.Flags().GetBool("show-all")
	if showAllFlag {
		t.Error("expected showAll to not be true when getting resource")
	}
}

func watchTestData() ([]api.Pod, []watch.Event) {
	pods := []api.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "bar",
				Namespace:       "test",
				ResourceVersion: "9",
			},
			Spec: apitesting.DeepEqualSafePodSpec(),
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "foo",
				Namespace:       "test",
				ResourceVersion: "10",
			},
			Spec: apitesting.DeepEqualSafePodSpec(),
		},
	}
	events := []watch.Event{
		// current state events
		{
			Type: watch.Added,
			Object: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "test",
					ResourceVersion: "9",
				},
				Spec: apitesting.DeepEqualSafePodSpec(),
			},
		},
		{
			Type: watch.Added,
			Object: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "test",
					ResourceVersion: "10",
				},
				Spec: apitesting.DeepEqualSafePodSpec(),
			},
		},
		// resource events
		{
			Type: watch.Modified,
			Object: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "test",
					ResourceVersion: "11",
				},
				Spec: apitesting.DeepEqualSafePodSpec(),
			},
		},
		{
			Type: watch.Deleted,
			Object: &api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "test",
					ResourceVersion: "12",
				},
				Spec: apitesting.DeepEqualSafePodSpec(),
			},
		},
	}
	return pods, events
}

func TestWatchLabelSelector(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	podList := &api.PodList{
		Items: pods,
		ListMeta: metav1.ListMeta{
			ResourceVersion: "10",
		},
	}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get(metav1.LabelSelectorQueryParam("v1")) != "a=b" {
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
			}
			switch req.URL.Path {
			case "/namespaces/test/pods":
				if req.URL.Query().Get("watch") == "true" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[2:])}, nil
				}
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, podList)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch", "true")
	cmd.Flags().Set("selector", "a=b")
	cmd.Run(cmd, []string{"pods"})

	expected := []runtime.Object{&pods[0], &pods[1], events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestWatchFieldSelector(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	podList := &api.PodList{
		Items: pods,
		ListMeta: metav1.ListMeta{
			ResourceVersion: "10",
		},
	}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get(metav1.FieldSelectorQueryParam("v1")) != "a=b" {
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
			}
			switch req.URL.Path {
			case "/namespaces/test/pods":
				if req.URL.Query().Get("watch") == "true" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[2:])}, nil
				}
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, podList)}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch", "true")
	cmd.Flags().Set("field-selector", "a=b")
	cmd.Run(cmd, []string{"pods"})

	expected := []runtime.Object{&pods[0], &pods[1], events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Errorf("unexpected empty output")
	}
}

func TestWatchResource(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods/foo":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods[1])}, nil
			case "/namespaces/test/pods":
				if req.URL.Query().Get("watch") == "true" && req.URL.Query().Get("fieldSelector") == "metadata.name=foo" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[1:])}, nil
				}
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch", "true")
	cmd.Run(cmd, []string{"pods", "foo"})

	expected := []runtime.Object{&pods[1], events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestWatchResourceIdentifiedByFile(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/replicationcontrollers/cassandra":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods[1])}, nil
			case "/namespaces/test/replicationcontrollers":
				if req.URL.Query().Get("watch") == "true" && req.URL.Query().Get("fieldSelector") == "metadata.name=cassandra" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[1:])}, nil
				}
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch", "true")
	cmd.Flags().Set("filename", "../../../../examples/storage/cassandra/cassandra-controller.yaml")
	cmd.Run(cmd, []string{})

	expected := []runtime.Object{&pods[1], events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestWatchOnlyResource(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods/foo":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &pods[1])}, nil
			case "/namespaces/test/pods":
				if req.URL.Query().Get("watch") == "true" && req.URL.Query().Get("fieldSelector") == "metadata.name=foo" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[1:])}, nil
				}
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch-only", "true")
	cmd.Run(cmd, []string{"pods", "foo"})

	expected := []runtime.Object{events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func TestWatchOnlyList(t *testing.T) {
	pods, events := watchTestData()

	f, tf, codec, _ := cmdtesting.NewAPIFactory()
	tf.Printer = &testPrinter{}
	podList := &api.PodList{
		Items: pods,
		ListMeta: metav1.ListMeta{
			ResourceVersion: "10",
		},
	}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/namespaces/test/pods":
				if req.URL.Query().Get("watch") == "true" {
					return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: watchBody(codec, events[2:])}, nil
				}
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, podList)}, nil
			default:
				t.Fatalf("request url: %#v,and request: %#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Namespace = "test"
	buf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})

	cmd := NewCmdGet(f, buf, errBuf)
	cmd.SetOutput(buf)

	cmd.Flags().Set("watch-only", "true")
	cmd.Run(cmd, []string{"pods"})

	expected := []runtime.Object{events[2].Object, events[3].Object}
	verifyObjects(t, expected, tf.Printer.(*testPrinter).Objects)

	if len(buf.String()) == 0 {
		t.Error("unexpected empty output")
	}
}

func watchBody(codec runtime.Codec, events []watch.Event) io.ReadCloser {
	buf := bytes.NewBuffer([]byte{})
	enc := restclientwatch.NewEncoder(streaming.NewEncoder(buf, codec), codec)
	for i := range events {
		enc.Encode(&events[i])
	}
	return json.Framer.NewFrameReader(ioutil.NopCloser(buf))
}
