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

// Package app implements a Server object for running the rescheduler.
package app

import (
	"fmt"

	"github.com/aveshagarwal/rescheduler/cmd/rescheduler/app/options"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/client"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/node"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewReschedulerCommand creates a *cobra.Command object with default parameters
func NewReschedulerCommand() *cobra.Command {
	s := options.NewReschedulerServer()
	s.AddFlags(pflag.CommandLine)
	cmd := &cobra.Command{
		Use:   "rescheduler",
		Short: "reschdeduler",
		Long:  `The rescheduler evicts pods which may be bound to less desired nodes`,
		Run: func(cmd *cobra.Command, args []string) {
			err := Run(s)
			if err != nil {
				fmt.Println(err)
			}

		},
	}

	return cmd
}

func Run(rs *options.ReschedulerServer) error {
	rsclient, err := client.CreateClient(rs.KubeconfigFile)
	if err != nil {
		return err
	}
	rs.Client = rsclient
	stopChannel := make(chan struct{})
	nodes, err := node.ReadyNodes(rs.Client, stopChannel)
	if err != nil {
		return err
	}

	fmt.Printf("\nnodes = %#v\n", nodes)
	return nil
}
