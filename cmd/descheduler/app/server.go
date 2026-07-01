/*
Copyright 2017 The Kubernetes Authors.

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

// Package app implements a Server object for running the descheduler.
package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	deschedulerclient "sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/cycle"
	"sigs.k8s.io/descheduler/pkg/tracing"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/apiserver/pkg/server/mux"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

const triggerAPIPath = "/api/v1/descheduler/run"

// NewDeschedulerCommand creates a *cobra.Command object with default parameters
func NewDeschedulerCommand(out io.Writer) *cobra.Command {
	s, err := options.NewDeschedulerServer()
	if err != nil {
		klog.ErrorS(err, "unable to initialize server")
	}

	featureGate := featuregate.NewFeatureGate()
	logConfig := logsapi.NewLoggingConfiguration()

	cmd := &cobra.Command{
		Use:   "descheduler",
		Short: "descheduler",
		Long:  "The descheduler evicts pods which may be bound to less desired nodes",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			logs.InitLogs()
			if logsapi.ValidateAndApply(logConfig, featureGate); err != nil {
				return err
			}
			descheduler.SetupPlugins()
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err = s.Apply(); err != nil {
				klog.ErrorS(err, "failed to apply")
				return err
			}

			if err = Run(cmd.Context(), s); err != nil {
				klog.ErrorS(err, "failed to run descheduler server")
				return err
			}

			return nil
		},
	}
	cmd.SetOut(out)
	flags := cmd.Flags()
	s.AddFlags(flags)

	runtime.Must(logsapi.AddFeatureGates(featureGate))
	logsapi.AddFlags(logConfig, flags)

	return cmd
}

func Run(rootCtx context.Context, rs *options.DeschedulerServer) error {
	ctx, done := signal.NotifyContext(rootCtx, syscall.SIGINT, syscall.SIGTERM)

	pathRecorderMux := mux.NewPathRecorderMux("descheduler")
	if !rs.DisableMetrics {
		pathRecorderMux.Handle("/metrics", legacyregistry.HandlerWithReset())
	}

	healthz.InstallHandler(pathRecorderMux, healthz.NamedCheck("Descheduler", healthz.PingHealthz.Check))

	if rs.EnableTriggerAPI {
		if rs.TriggerAuthClient == nil {
			clientConnection := rs.ClientConnection
			if rs.KubeconfigFile != "" && clientConnection.Kubeconfig == "" {
				clientConnection.Kubeconfig = rs.KubeconfigFile
			}
			triggerClient, err := deschedulerclient.CreateClient(clientConnection, "descheduler-trigger-api")
			if err != nil {
				return err
			}
			rs.TriggerAuthClient = triggerClient
		}
		pathRecorderMux.Handle(triggerAPIPath, newTriggerHandler(rs))
		klog.V(1).Infof("Trigger API enabled at %s", triggerAPIPath)
	}

	var stoppedCh <-chan struct{}
	var err error
	if rs.SecureServingInfo != nil {
		stoppedCh, _, err = rs.SecureServingInfo.Serve(pathRecorderMux, 0, ctx.Done())
		if err != nil {
			klog.Fatalf("failed to start secure server: %v", err)
			return err
		}
	}

	err = tracing.NewTracerProvider(ctx, rs.Tracing.CollectorEndpoint, rs.Tracing.TransportCert, rs.Tracing.ServiceName, rs.Tracing.ServiceNamespace, rs.Tracing.SampleRate, rs.Tracing.FallbackToNoOpProviderOnError)
	if err != nil {
		klog.ErrorS(err, "failed to create tracer provider")
	}
	defer func() {
		// we give the tracing.Shutdown() its own context as the
		// original context may have been cancelled already. we
		// have arbitrarily chosen the timeout duration.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		tracing.Shutdown(ctx)
	}()

	// increase the fake watch channel so the dry-run mode can be run
	// over a cluster with thousands of pods
	watch.DefaultChanSize = 100000
	err = descheduler.Run(ctx, rs)
	if err != nil {
		return err
	}

	done()
	if stoppedCh != nil {
		// wait for metrics server to close
		<-stoppedCh
	}

	return nil
}

func newTriggerHandler(rs *options.DeschedulerServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"status":  "error",
				"message": "method not allowed, use POST",
			})
			return
		}

		if err := authorizeTriggerRequest(r.Context(), rs.TriggerAuthClient, r); err != nil {
			writeJSON(w, err.statusCode, map[string]string{
				"status":  "error",
				"message": err.message,
			})
			return
		}

		if rs.CycleManager == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":  "error",
				"message": "descheduling cycle manager is not ready",
			})
			return
		}

		if err := rs.CycleManager.TryStart(); err != nil {
			switch err {
			case cycle.ErrAlreadyRunning:
				writeJSON(w, http.StatusConflict, map[string]string{
					"status":  "error",
					"message": "descheduling cycle already running",
				})
			case cycle.ErrRunnerNotReady:
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status":  "error",
					"message": "descheduling cycle runner is not ready",
				})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"status":  "error",
					"message": err.Error(),
				})
			}
			return
		}

		klog.V(1).Info("Descheduling cycle triggered via API")
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "accepted",
			"message": "descheduling cycle started",
		})
	})
}

type triggerAuthError struct {
	statusCode int
	message    string
}

func authorizeTriggerRequest(ctx context.Context, client clientset.Interface, r *http.Request) *triggerAuthError {
	if client == nil {
		return &triggerAuthError{statusCode: http.StatusServiceUnavailable, message: "kubernetes client is not ready"}
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return &triggerAuthError{statusCode: http.StatusUnauthorized, message: "missing bearer token"}
	}

	tokenReview, err := client.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{Token: token},
	}, metav1.CreateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to authenticate trigger API request")
		return &triggerAuthError{statusCode: http.StatusInternalServerError, message: "failed to authenticate request"}
	}
	if !tokenReview.Status.Authenticated {
		return &triggerAuthError{statusCode: http.StatusUnauthorized, message: "invalid bearer token"}
	}

	subjectAccessReview, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   tokenReview.Status.User.Username,
			UID:    tokenReview.Status.User.UID,
			Groups: tokenReview.Status.User.Groups,
			Extra:  authorizationExtra(tokenReview.Status.User.Extra),
			NonResourceAttributes: &authorizationv1.NonResourceAttributes{
				Path: triggerAPIPath,
				Verb: "post",
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to authorize trigger API request")
		return &triggerAuthError{statusCode: http.StatusInternalServerError, message: "failed to authorize request"}
	}
	if !subjectAccessReview.Status.Allowed {
		return &triggerAuthError{statusCode: http.StatusForbidden, message: "not allowed to trigger descheduler"}
	}

	return nil
}

func bearerToken(header string) string {
	authType, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(authType, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func authorizationExtra(extra map[string]authenticationv1.ExtraValue) map[string]authorizationv1.ExtraValue {
	if len(extra) == 0 {
		return nil
	}
	converted := make(map[string]authorizationv1.ExtraValue, len(extra))
	for key, values := range extra {
		converted[key] = authorizationv1.ExtraValue(values)
	}
	return converted
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}
