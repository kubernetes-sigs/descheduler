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
	"flag"
	"io"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/descheduler"

	"github.com/spf13/cobra"

	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/mux"
	restclient "k8s.io/client-go/rest"
	aflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// NewDeschedulerCommand creates a *cobra.Command object with default parameters
func NewDeschedulerCommand(out io.Writer) *cobra.Command {
	s, err := options.NewDeschedulerServer()

	if err != nil {
		klog.ErrorS(err, "unable to initialize server")
	}

	cmd := &cobra.Command{
		Use:   "descheduler",
		Short: "descheduler",
		Long:  `The descheduler evicts pods which may be bound to less desired nodes`,
		Run: func(cmd *cobra.Command, args []string) {
			s.Logs.Config.Format = s.Logging.Format
			s.Logs.Apply()

			// LoopbackClientConfig is a config for a privileged loopback connection
			var LoopbackClientConfig *restclient.Config
			var SecureServing *apiserver.SecureServingInfo
			if err := s.SecureServing.ApplyTo(&SecureServing, &LoopbackClientConfig); err != nil {
				klog.ErrorS(err, "failed to apply secure server configuration")
				return
			}

			if err := s.Validate(); err != nil {
				klog.ErrorS(err, "failed to validate server configuration")
				return
			}

			if !s.DisableMetrics {
				ctx := context.TODO()
				pathRecorderMux := mux.NewPathRecorderMux("descheduler")
				pathRecorderMux.Handle("/metrics", legacyregistry.HandlerWithReset())

				if _, err := SecureServing.Serve(pathRecorderMux, 0, ctx.Done()); err != nil {
					klog.Fatalf("failed to start secure server: %v", err)
					return
				}
			}

			err := Run(s)
			if err != nil {
				klog.ErrorS(err, "descheduler server")
			}
		},
	}
	cmd.SetOut(out)
	flags := cmd.Flags()
	flags.SetNormalizeFunc(aflag.WordSepNormalizeFunc)
	flags.AddGoFlagSet(flag.CommandLine)
	s.AddFlags(flags)
	return cmd
}

func Run(rs *options.DeschedulerServer) error {
	return descheduler.Run(rs)
}
