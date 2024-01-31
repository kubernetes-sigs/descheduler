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
	"io"
	"os/signal"
	"syscall"

	"k8s.io/apiserver/pkg/server/healthz"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/tracing"

	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/mux"
	restclient "k8s.io/client-go/rest"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

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
			// loopbackClientConfig is a config for a privileged loopback connection
			var loopbackClientConfig *restclient.Config
			var secureServing *apiserver.SecureServingInfo
			if err := s.SecureServing.ApplyTo(&secureServing, &loopbackClientConfig); err != nil {
				klog.ErrorS(err, "failed to apply secure server configuration")
				return err
			}

			secureServing.DisableHTTP2 = !s.EnableHTTP2

			ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

			pathRecorderMux := mux.NewPathRecorderMux("descheduler")
			if !s.DisableMetrics {
				pathRecorderMux.Handle("/metrics", legacyregistry.HandlerWithReset())
			}

			healthz.InstallHandler(pathRecorderMux, healthz.NamedCheck("Descheduler", healthz.PingHealthz.Check))

			stoppedCh, _, err := secureServing.Serve(pathRecorderMux, 0, ctx.Done())
			if err != nil {
				klog.Fatalf("failed to start secure server: %v", err)
				return err
			}

			if err = Run(ctx, s); err != nil {
				klog.ErrorS(err, "descheduler server")
				return err
			}

			done()
			// wait for metrics server to close
			<-stoppedCh

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

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	err := tracing.NewTracerProvider(ctx, rs.Tracing.CollectorEndpoint, rs.Tracing.TransportCert, rs.Tracing.ServiceName, rs.Tracing.ServiceNamespace, rs.Tracing.SampleRate, rs.Tracing.FallbackToNoOpProviderOnError)
	if err != nil {
		return err
	}
	defer tracing.Shutdown(ctx)
	// increase the fake watch channel so the dry-run mode can be run
	// over a cluster with thousands of pods
	watch.DefaultChanSize = 100000
	return descheduler.Run(ctx, rs)
}
