/*
Copyright 2026 The Kubernetes Authors.

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

package descheduler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestPromClientControllerSync_InvalidConfig(t *testing.T) {
	testCases := []struct {
		name             string
		objects          []runtime.Object
		prometheusConfig *api.Prometheus
		expectedErr      error
	}{
		{
			name:             "empty prometheus config",
			prometheusConfig: nil,
			expectedErr:      fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
		{
			name: "missing prometheus config",
			prometheusConfig: &api.Prometheus{
				URL: prometheusURL,
			},
			expectedErr: fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
		{
			name: "missing auth token config",
			prometheusConfig: &api.Prometheus{
				URL:       prometheusURL,
				AuthToken: nil,
			},
			expectedErr: fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
		{
			name: "missing secret reference",
			prometheusConfig: &api.Prometheus{
				URL: prometheusURL,
				AuthToken: &api.AuthToken{
					SecretReference: nil,
				},
			},
			expectedErr: fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
		{
			name: "missing secret reference name",
			prometheusConfig: &api.Prometheus{
				URL: prometheusURL,
				AuthToken: &api.AuthToken{
					SecretReference: &api.SecretReference{
						Namespace: "kube-system",
					},
				},
			},
			expectedErr: fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
		{
			name: "missing secret reference namespace",
			prometheusConfig: &api.Prometheus{
				URL: prometheusURL,
				AuthToken: &api.AuthToken{
					SecretReference: &api.SecretReference{
						Name: "prom-token",
					},
				},
			},
			expectedErr: fmt.Errorf("prometheus metrics source configuration is missing authentication token secret"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			_, err := setupPromClientControllerTest(ctx, t, tc.objects, tc.prometheusConfig, false)

			// Verify error expectations
			if tc.expectedErr != nil {
				if err == nil {
					t.Errorf("Expected error %q but got none", tc.expectedErr)
				} else if err.Error() != tc.expectedErr.Error() {
					t.Errorf("Expected error %q but got %q", tc.expectedErr, err.Error())
				}
			} else {
				t.Errorf("Expected an error, got none")
			}
		})
	}
}

func TestPromClientControllerSync_InvalidSecret(t *testing.T) {
	testCases := []struct {
		name             string
		objects          []runtime.Object
		prometheusConfig *api.Prometheus
		expectedErr      error
	}{
		{
			name:             "secret exists but empty token",
			objects:          []runtime.Object{newPrometheusAuthSecret(withToken(""))},
			prometheusConfig: newPrometheusConfig(),
			expectedErr:      fmt.Errorf("prometheus authentication token secret missing \"prometheusAuthToken\" data or empty"),
		},
		{
			name: "secret exists but missing token key",
			objects: []runtime.Object{newPrometheusAuthSecret(func(s *v1.Secret) {
				s.Data = map[string][]byte{}
			})},
			prometheusConfig: newPrometheusConfig(),
			expectedErr:      fmt.Errorf("prometheus authentication token secret missing \"prometheusAuthToken\" data or empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			setup, err := setupPromClientControllerTest(ctx, t, tc.objects, tc.prometheusConfig, true)
			if err != nil {
				t.Fatal(err)
			}

			// Call sync
			err = setup.ctrl.sync()

			// Verify error expectations
			if tc.expectedErr != nil {
				if err == nil {
					t.Errorf("Expected error %q but got none", tc.expectedErr)
				} else if err.Error() != tc.expectedErr.Error() {
					t.Errorf("Expected error %q but got %q", tc.expectedErr, err.Error())
				}
			} else {
				t.Errorf("Expected an error, got none")
			}
		})
	}
}

func TestPromClientControllerSync_ClientCreation(t *testing.T) {
	testCases := []struct {
		name                           string
		objects                        []runtime.Object
		currentAuthToken               string
		createPrometheusClientFunc     func(url, token string) (promapi.Client, *http.Transport, error)
		expectedErr                    error
		expectClientCreated            bool
		expectCurrentTokenCleared      bool
		expectPreviousTransportCleared bool
	}{
		{
			name:             "secret not found",
			currentAuthToken: "old-token",
			expectedErr:      fmt.Errorf("unable to get kube-system/prom-token secret"),
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				t.Fatalf("unexpected create client invocation")
				return nil, nil, fmt.Errorf("unexpected create client invocation")
			},
			expectCurrentTokenCleared:      true,
			expectPreviousTransportCleared: true,
		},
		{
			name:             "token unchanged - no client creation",
			objects:          []runtime.Object{newPrometheusAuthSecret(withToken("same-token"))},
			currentAuthToken: "same-token",
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				t.Fatalf("unexpected create client invocation")
				return nil, nil, fmt.Errorf("unexpected create client invocation")
			},
			expectClientCreated: false,
		},
		{
			name:             "token changed - client created successfully",
			objects:          []runtime.Object{newPrometheusAuthSecret(withToken("new-token"))},
			currentAuthToken: "old-token",
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				return &mockPrometheusClient{name: "new-client"}, &http.Transport{}, nil
			},
			expectClientCreated: true,
		},
		{
			name:             "token changed - client creation fails",
			objects:          []runtime.Object{newPrometheusAuthSecret(withToken("new-token"))},
			currentAuthToken: "old-token",
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				return nil, nil, fmt.Errorf("failed to create client")
			},
			expectedErr:         fmt.Errorf("unable to create a prometheus client: failed to create client"),
			expectClientCreated: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, setupMode := range []struct {
				name    string
				setupFn func(context.Context, *testing.T, []runtime.Object) *secretBasedPromClientController
			}{
				{
					name: "running with prom reconciler directly",
					setupFn: func(ctx context.Context, t *testing.T, objects []runtime.Object) *secretBasedPromClientController {
						setup, err := setupPromClientControllerTest(ctx, t, objects, newPrometheusConfig(), true)
						if err != nil {
							t.Fatal(err)
						}
						return setup.ctrl
					},
				},
				{
					name: "running with full descheduler",
					setupFn: func(ctx context.Context, t *testing.T, objects []runtime.Object) *secretBasedPromClientController {
						deschedulerPolicy := &api.DeschedulerPolicy{
							MetricsProviders: []api.MetricsProvider{
								{
									Source:     api.PrometheusMetrics,
									Prometheus: newPrometheusConfig(),
								},
							},
						}
						_, descheduler, _, _ := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, false, objects...)
						return descheduler.secretBasedPromClientCtrl
					},
				},
			} {
				t.Run(setupMode.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.TODO())
					defer cancel()

					ctrl := setupMode.setupFn(ctx, t, tc.objects)

					// Set additional test-specific fields
					ctrl.currentPrometheusAuthToken = tc.currentAuthToken
					if tc.currentAuthToken != "" {
						ctrl.previousPrometheusClientTransport = &http.Transport{}
					}

					// Mock createPrometheusClient
					clientCreated := false
					if tc.createPrometheusClientFunc != nil {
						ctrl.createPrometheusClient = func(url, token string) (promapi.Client, *http.Transport, error) {
							client, transport, err := tc.createPrometheusClientFunc(url, token)
							if err == nil {
								clientCreated = true
							}
							return client, transport, err
						}
					}

					// Call sync
					err := ctrl.sync()

					// Verify error expectations
					if tc.expectedErr != nil {
						if err == nil {
							t.Errorf("Expected error %q but got none", tc.expectedErr)
						} else if err.Error() != tc.expectedErr.Error() {
							t.Errorf("Expected error %q but got %q", tc.expectedErr, err.Error())
						}
					} else {
						if err != nil {
							t.Errorf("Expected no error but got: %v", err)
						}
					}

					// Verify client creation expectations
					if tc.expectClientCreated && !clientCreated {
						t.Errorf("Expected prometheus client to be created but it wasn't")
					}
					if !tc.expectClientCreated && clientCreated {
						t.Errorf("Expected prometheus client not to be created but it was")
					}

					// Verify token cleared expectations
					if tc.expectCurrentTokenCleared && ctrl.currentPrometheusAuthToken != "" {
						t.Errorf("Expected current auth token to be cleared but it wasn't")
					}

					// Verify previous transport cleared expectations
					if tc.expectPreviousTransportCleared && ctrl.previousPrometheusClientTransport != nil {
						t.Errorf("Expected previous transport to be cleared but it wasn't")
					}

					// Verify promClient cleared when secret not found
					if tc.expectPreviousTransportCleared && ctrl.prometheusClient() != nil {
						t.Errorf("Expected promClient to be cleared but it wasn't")
					}

					// Verify token updated when client created
					if tc.expectClientCreated && len(tc.objects) > 0 {
						if secret, ok := tc.objects[0].(*v1.Secret); ok && secret.Data != nil {
							expectedToken := string(secret.Data[prometheusAuthTokenSecretKey])
							if ctrl.currentPrometheusAuthToken != expectedToken {
								t.Errorf("Expected current auth token to be %q but got %q", expectedToken, ctrl.currentPrometheusAuthToken)
							}
						}
					}
				})
			}
		})
	}
}

func TestPromClientControllerSync_EventHandler(t *testing.T) {
	testCases := []struct {
		name                              string
		operation                         func(ctx context.Context, fakeClient *fakeclientset.Clientset) error
		processItem                       bool
		expectedPromClientSet             bool
		expectedCreatedClientsCount       int
		expectedCurrentToken              string
		expectedPreviousTransportCleared  bool
		expectDifferentClients            bool
		expectCreatePrometheusClientError bool
	}{
		// Check initial conditions
		{
			name:                        "no secret initially",
			operation:                   func(ctx context.Context, fakeClient *fakeclientset.Clientset) error { return nil },
			processItem:                 false,
			expectedPromClientSet:       false,
			expectedCreatedClientsCount: 0,
			expectedCurrentToken:        "",
		},
		// Change conditions
		{
			name: "add secret",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-1"))
				_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
				return err
			},
			processItem:                 true,
			expectedPromClientSet:       true,
			expectedCreatedClientsCount: 1,
			expectedCurrentToken:        "token-1",
		},
		{
			name: "update secret",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-2"))
				_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
				return err
			},
			processItem:                 true,
			expectedPromClientSet:       true,
			expectedCreatedClientsCount: 2,
			expectedCurrentToken:        "token-2",
			expectDifferentClients:      true,
		},
		{
			name: "update secret with invalid data",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-3"))
				secret.Data[prometheusAuthTokenSecretKey] = []byte{}
				_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
				return err
			},
			processItem:                 true,
			expectedPromClientSet:       false,
			expectedCreatedClientsCount: 2,
			expectedCurrentToken:        "",
			expectDifferentClients:      true,
		},
		{
			name: "update secret with valid data",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-4"))
				_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
				return err
			},
			processItem:                 true,
			expectedPromClientSet:       true,
			expectedCreatedClientsCount: 3,
			expectedCurrentToken:        "token-4",
			expectDifferentClients:      true,
		},
		{
			name: "update secret with valid data but createPrometheusClient failing",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-5"))
				_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
				return err
			},
			processItem:                       true,
			expectedPromClientSet:             false,
			expectedCreatedClientsCount:       3,
			expectedCurrentToken:              "",
			expectDifferentClients:            true,
			expectCreatePrometheusClientError: true,
		},
		{
			name: "delete secret",
			operation: func(ctx context.Context, fakeClient *fakeclientset.Clientset) error {
				secret := newPrometheusAuthSecret(withToken("token-5"))
				return fakeClient.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
			},
			processItem:                      true,
			expectedPromClientSet:            false,
			expectedCreatedClientsCount:      3,
			expectedCurrentToken:             "",
			expectedPreviousTransportCleared: true,
		},
	}

	for _, setupMode := range []struct {
		name string
		init func(t *testing.T, ctx context.Context) (ctrl *secretBasedPromClientController, fakeClient *fakeclientset.Clientset)
	}{
		{
			name: "running with prom reconciler directly",
			init: func(t *testing.T, ctx context.Context) (ctrl *secretBasedPromClientController, fakeClient *fakeclientset.Clientset) {
				setup, err := setupPromClientControllerTest(ctx, t, nil, newPrometheusConfig(), true)
				if err != nil {
					t.Fatal(err)
				}

				// Start the reconciler to process queue items
				go setup.ctrl.runAuthenticationSecretReconciler(ctx)

				return setup.ctrl, setup.fakeClient
			},
		},
		{
			name: "running with full descheduler",
			init: func(t *testing.T, ctx context.Context) (ctrl *secretBasedPromClientController, fakeClient *fakeclientset.Clientset) {
				deschedulerPolicy := &api.DeschedulerPolicy{
					MetricsProviders: []api.MetricsProvider{
						{
							Source:     api.PrometheusMetrics,
							Prometheus: newPrometheusConfig(),
						},
					},
				}

				_, descheduler, _, client := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, false)
				// The reconciler is already started by initDescheduler via bootstrapDescheduler

				return descheduler.secretBasedPromClientCtrl, client
			},
		},
	} {
		t.Run(setupMode.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctrl, fakeClient := setupMode.init(t, ctx)

			// Track created clients to verify different instances
			var createdClients []promapi.Client
			var createdClientsMu sync.Mutex

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					ctrl.mu.Lock()
					ctrl.createPrometheusClient = func(url, token string) (promapi.Client, *http.Transport, error) {
						if tc.expectCreatePrometheusClientError {
							return nil, &http.Transport{}, fmt.Errorf("error creating a prometheus client")
						}
						client := &mockPrometheusClient{name: "client-" + token}
						createdClientsMu.Lock()
						createdClients = append(createdClients, client)
						createdClientsMu.Unlock()
						return client, &http.Transport{}, nil
					}
					ctrl.mu.Unlock()

					if err := tc.operation(ctx, fakeClient); err != nil {
						t.Fatalf("Failed to execute operation: %v", err)
					}

					if tc.processItem {
						// Wait for event to be processed by the reconciler
						err := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 2*time.Second, true, func(ctx context.Context) (bool, error) {
							// Check if all expected conditions are met (with mutex protection)
							ctrl.mu.RLock()
							promClient := ctrl.promClient
							currentToken := ctrl.currentPrometheusAuthToken
							previousTransport := ctrl.previousPrometheusClientTransport
							ctrl.mu.RUnlock()

							t.Logf("promClient: %v\n", promClient)
							if tc.expectedPromClientSet {
								if promClient == nil {
									return false, nil
								}
							} else {
								if promClient != nil {
									return false, nil
								}
							}

							createdClientsMu.Lock()
							createdClientsLen := len(createdClients)
							createdClientsMu.Unlock()
							t.Logf("createdClientsLen: %v\n", createdClientsLen)
							if createdClientsLen != tc.expectedCreatedClientsCount {
								return false, nil
							}
							t.Logf("currentToken: %v\n", currentToken)
							if currentToken != tc.expectedCurrentToken {
								return false, nil
							}
							t.Logf("previousTransport: %v\n", previousTransport)
							if tc.expectedPreviousTransportCleared {
								if previousTransport != nil {
									return false, nil
								}
							}

							return true, nil
						})
						if err != nil {
							t.Fatalf("Timed out waiting for expected conditions: %v", err)
						}

						// Log all expected conditions that were met
						t.Logf("All expected conditions met: promClientSet=%v, createdClientsCount=%d, currentToken=%q, previousTransportCleared=%v",
							tc.expectedPromClientSet, tc.expectedCreatedClientsCount, tc.expectedCurrentToken, tc.expectedPreviousTransportCleared)
					}

					// Validate post-conditions
					if tc.expectedPromClientSet {
						if ctrl.prometheusClient() == nil {
							t.Error("Expected prometheus client to be set, but it was nil")
						}
					} else {
						promClient := ctrl.prometheusClient()
						if promClient != nil {
							t.Errorf("Expected prometheus client to be nil, but got: %v", promClient)
						}
					}

					createdClientsMu.Lock()
					createdClientsLen := len(createdClients)
					createdClientsMu.Unlock()
					if createdClientsLen != tc.expectedCreatedClientsCount {
						t.Errorf("Expected %d clients created, but got %d", tc.expectedCreatedClientsCount, len(createdClients))
					}

					if ctrl.currentPrometheusAuthToken != tc.expectedCurrentToken {
						t.Errorf("Expected current token to be %q, got %q", tc.expectedCurrentToken, ctrl.currentPrometheusAuthToken)
					}

					if tc.expectedPreviousTransportCleared {
						if ctrl.previousPrometheusClientTransport != nil {
							t.Error("Expected previous transport to be cleared, but it was set")
						}
					}

					if tc.expectDifferentClients && len(createdClients) >= 2 {
						createdClientsMu.Lock()
						defer createdClientsMu.Unlock()
						if createdClients[0] == createdClients[1] {
							t.Error("Expected different client instances")
						}
					}
				})
			}
		})
	}
}

func TestReconcileInClusterSAToken(t *testing.T) {
	testCases := []struct {
		name                           string
		currentAuthToken               string
		inClusterConfigFunc            func() (*rest.Config, error)
		createPrometheusClientFunc     func(url, token string) (promapi.Client, *http.Transport, error)
		expectedErr                    error
		expectClientCreated            bool
		expectCurrentToken             string
		expectPreviousTransportCleared bool
		expectPromClientCleared        bool
	}{
		{
			name:             "token unchanged - no client creation",
			currentAuthToken: "same-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return &rest.Config{BearerToken: "same-token"}, nil
			},
			expectClientCreated: false,
			expectCurrentToken:  "same-token",
		},
		{
			name:             "token changed - client created successfully",
			currentAuthToken: "old-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return &rest.Config{BearerToken: "new-token"}, nil
			},
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				if token != "new-token" {
					t.Errorf("Expected token to be %q, got %q", "new-token", token)
				}
				return &mockPrometheusClient{name: "new-client"}, &http.Transport{}, nil
			},
			expectClientCreated: true,
			expectCurrentToken:  "new-token",
		},
		{
			name:             "token changed - client creation fails",
			currentAuthToken: "old-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return &rest.Config{BearerToken: "new-token"}, nil
			},
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				return nil, nil, fmt.Errorf("failed to create client")
			},
			expectedErr:                    fmt.Errorf("unable to create a prometheus client: failed to create client"),
			expectClientCreated:            false,
			expectCurrentToken:             "",
			expectPreviousTransportCleared: false,
			expectPromClientCleared:        false,
		},
		{
			name:             "not in cluster - no error",
			currentAuthToken: "current-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return nil, rest.ErrNotInCluster
			},
			expectClientCreated: false,
			expectCurrentToken:  "current-token",
		},
		{
			name:             "unexpected error",
			currentAuthToken: "current-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return nil, fmt.Errorf("unexpected error")
			},
			expectedErr:         fmt.Errorf("unexpected error when reading in cluster config: unexpected error"),
			expectClientCreated: false,
			expectCurrentToken:  "current-token",
		},
		{
			name:             "first token - client created successfully",
			currentAuthToken: "",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return &rest.Config{BearerToken: "first-token"}, nil
			},
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				return &mockPrometheusClient{name: "first-client"}, &http.Transport{}, nil
			},
			expectClientCreated: true,
			expectCurrentToken:  "first-token",
		},
		{
			name:             "token changed with previous transport - clears previous transport",
			currentAuthToken: "old-token",
			inClusterConfigFunc: func() (*rest.Config, error) {
				return &rest.Config{BearerToken: "new-token"}, nil
			},
			createPrometheusClientFunc: func(url, token string) (promapi.Client, *http.Transport, error) {
				return &mockPrometheusClient{name: "new-client"}, &http.Transport{}, nil
			},
			expectClientCreated:            true,
			expectCurrentToken:             "new-token",
			expectPreviousTransportCleared: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, setupMode := range []struct {
				name string
				init func(t *testing.T) *inClusterPromClientController
			}{
				{
					name: "running with prom reconciler directly",
					init: func(t *testing.T) *inClusterPromClientController {
						return &inClusterPromClientController{
							currentPrometheusAuthToken: tc.currentAuthToken,
							prometheusConfig: &api.Prometheus{
								URL: prometheusURL,
							},
							inClusterConfig: tc.inClusterConfigFunc,
						}
					},
				},
				{
					name: "running with full descheduler",
					init: func(t *testing.T) *inClusterPromClientController {
						ctx := context.Background()
						prometheusConfig := &api.Prometheus{
							URL: prometheusURL,
						}
						deschedulerPolicy := &api.DeschedulerPolicy{
							MetricsProviders: []api.MetricsProvider{
								{
									Source:     api.PrometheusMetrics,
									Prometheus: prometheusConfig,
								},
							},
						}
						_, descheduler, _, _ := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, false)

						// Override the fields needed for this test (no need for a mutex
						// since reconcileInClusterSAToken gets to run later)
						descheduler.inClusterPromClientCtrl.currentPrometheusAuthToken = tc.currentAuthToken
						descheduler.inClusterPromClientCtrl.inClusterConfig = tc.inClusterConfigFunc
						return descheduler.inClusterPromClientCtrl
					},
				},
			} {
				t.Run(setupMode.name, func(t *testing.T) {
					ctrl := setupMode.init(t)

					// Set previous transport and client if test expects them to be cleared
					if tc.expectPreviousTransportCleared {
						ctrl.previousPrometheusClientTransport = &http.Transport{}
					}
					if tc.expectPromClientCleared {
						ctrl.promClient = &mockPrometheusClient{name: "old-client"}
					}

					// Mock createPrometheusClient
					clientCreated := false
					if tc.createPrometheusClientFunc != nil {
						ctrl.createPrometheusClient = func(url, token string) (promapi.Client, *http.Transport, error) {
							client, transport, err := tc.createPrometheusClientFunc(url, token)
							if err == nil {
								clientCreated = true
							}
							return client, transport, err
						}
					}

					// Call reconcileInClusterSAToken
					err := ctrl.reconcileInClusterSAToken()

					// Verify error expectations
					if tc.expectedErr != nil {
						if err == nil {
							t.Errorf("Expected error %q but got none", tc.expectedErr)
						} else if err.Error() != tc.expectedErr.Error() {
							t.Errorf("Expected error %q but got %q", tc.expectedErr, err.Error())
						}
					} else {
						if err != nil {
							t.Errorf("Expected no error but got: %v", err)
						}
					}

					// Verify client creation expectations
					if tc.expectClientCreated && !clientCreated {
						t.Errorf("Expected prometheus client to be created but it wasn't")
					}
					if !tc.expectClientCreated && clientCreated {
						t.Errorf("Expected prometheus client not to be created but it was")
					}

					// Verify token expectations
					if ctrl.currentPrometheusAuthToken != tc.expectCurrentToken {
						t.Errorf("Expected current token to be %q but got %q", tc.expectCurrentToken, ctrl.currentPrometheusAuthToken)
					}

					// Verify previous transport cleared when expected
					if tc.expectPreviousTransportCleared {
						if tc.expectClientCreated {
							// Success case: new transport should be set
							if ctrl.previousPrometheusClientTransport == nil {
								t.Error("Expected previous transport to be set to new transport, but it was nil")
							}
						} else if tc.expectedErr != nil {
							// Failure case: transport should be nil
							if ctrl.previousPrometheusClientTransport != nil {
								t.Error("Expected previous transport to be cleared on error, but it was set")
							}
						}
					}

					// Verify promClient cleared when expected
					if tc.expectPromClientCleared {
						if ctrl.promClient != nil {
							t.Error("Expected promClient to be cleared, but it was set")
						}
					}
				})
			}
		})
	}
}
