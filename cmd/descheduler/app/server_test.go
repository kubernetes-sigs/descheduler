package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/descheduler/cycle"
)

func TestTriggerHandlerMethodNotAllowed(t *testing.T) {
	rs := newTriggerTestServer(t, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, triggerAPIPath, nil)

	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestTriggerHandlerRequiresBearerToken(t *testing.T) {
	rs := newTriggerTestServer(t, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, triggerAPIPath, nil)

	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestTriggerHandlerRejectsInvalidToken(t *testing.T) {
	rs := newTriggerTestServer(t, false)
	recorder := httptest.NewRecorder()
	request := triggerRequest()

	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestTriggerHandlerRejectsForbiddenUser(t *testing.T) {
	rs := newTriggerTestServer(t, true)
	addSubjectAccessReviewReactor(rs.TriggerAuthClient.(*fakeclientset.Clientset), false)
	recorder := httptest.NewRecorder()
	request := triggerRequest()

	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestTriggerHandlerStartsCycle(t *testing.T) {
	rs := newTriggerTestServer(t, true)
	addSubjectAccessReviewReactor(rs.TriggerAuthClient.(*fakeclientset.Clientset), true)
	started := make(chan struct{})
	rs.CycleManager.SetRunner(func() error {
		close(started)
		return nil
	})
	recorder := httptest.NewRecorder()
	request := triggerRequest()

	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", recorder.Code)
	}
	<-started
	assertJSONStatus(t, recorder, "accepted")
}

func TestTriggerHandlerRejectsRunningCycle(t *testing.T) {
	rs := newTriggerTestServer(t, true)
	addSubjectAccessReviewReactor(rs.TriggerAuthClient.(*fakeclientset.Clientset), true)
	release := make(chan struct{})
	rs.CycleManager.SetRunner(func() error {
		<-release
		return nil
	})
	if err := rs.CycleManager.TryStart(); err != nil {
		t.Fatalf("expected initial cycle to start, got %v", err)
	}
	defer close(release)

	recorder := httptest.NewRecorder()
	request := triggerRequest()
	newTriggerHandler(rs).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", recorder.Code)
	}
}

func newTriggerTestServer(t *testing.T, authenticated bool) *options.DeschedulerServer {
	t.Helper()
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action core.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.TokenReview{
			Status: authenticationv1.TokenReviewStatus{
				Authenticated: authenticated,
				User: authenticationv1.UserInfo{
					Username: "system:serviceaccount:kube-system:descheduler-trigger",
					UID:      "test-uid",
					Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:kube-system"},
				},
			},
		}, nil
	})

	return &options.DeschedulerServer{
		TriggerAuthClient: client,
		CycleManager:      cycle.NewManager(),
	}
}

func addSubjectAccessReviewReactor(client *fakeclientset.Clientset, allowed bool) {
	client.PrependReactor("create", "subjectaccessreviews", func(action core.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: allowed,
			},
		}, nil
	})
}

func triggerRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, triggerAPIPath, nil)
	request.Header.Set("Authorization", "Bearer test-token")
	return request
}

func assertJSONStatus(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if response["status"] != expected {
		t.Fatalf("expected response status %q, got %q", expected, response["status"])
	}
}

func TestAuthorizeTriggerRequestUsesNonResourceAttributes(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action core.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.TokenReview{
			Status: authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User: authenticationv1.UserInfo{
					Username: "test-user",
					Groups:   []string{"test-group"},
				},
			},
		}, nil
	})
	client.PrependReactor("create", "subjectaccessreviews", func(action core.Action) (bool, runtime.Object, error) {
		createAction := action.(core.CreateAction)
		review := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
		if review.Spec.NonResourceAttributes == nil {
			t.Fatalf("expected non-resource attributes")
		}
		if review.Spec.NonResourceAttributes.Path != triggerAPIPath {
			t.Fatalf("expected path %q, got %q", triggerAPIPath, review.Spec.NonResourceAttributes.Path)
		}
		if review.Spec.NonResourceAttributes.Verb != "post" {
			t.Fatalf("expected verb post, got %q", review.Spec.NonResourceAttributes.Verb)
		}
		return true, &authorizationv1.SubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
		}, nil
	})

	request := triggerRequest()
	if err := authorizeTriggerRequest(context.Background(), client, request); err != nil {
		t.Fatalf("expected authorization to succeed, got %v", err)
	}
}
