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

package rescheduler

import (
	"encoding/json"
	//"fmt"
	"io/ioutil"
	//"os"

	"github.com/aveshagarwal/rescheduler/cmd/rescheduler/app/options"
	"github.com/aveshagarwal/rescheduler/pkg/api/v1alpha1"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/client"
	eutils "github.com/aveshagarwal/rescheduler/pkg/rescheduler/evictions/utils"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/strategies"
)

func Run(rs *options.ReschedulerServer) error {
	rsclient, err := client.CreateClient(rs.KubeconfigFile)
	if err != nil {
		return err
	}
	rs.Client = rsclient

	reschedulerPolicy := v1alpha1.ReschedulerPolicy{}
	if len(rs.PolicyConfigFile) > 0 {
		data, err := ioutil.ReadFile(rs.PolicyConfigFile)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &reschedulerPolicy); err != nil {
			return err
		}
	}

	policyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(policyGroupVersion) == 0 {
		return err
	}

	strategies.RemoveDuplicatePods(rs.Client, policyGroupVersion)
	/*stopChannel := make(chan struct{})
	  nodes, err := node.ReadyNodes(rs.Client, stopChannel)
	  if err != nil {
	          return err
	  }

	  for _, n := range nodes {
	          fmt.Printf("\nnode = %#v\n", n)
	  }

	  for _, node := range nodes {
	          pods, err := pod.ListPodsOnANode(rs.Client, node)
	          if err != nil {
	                  return err
	          }

	          for _, p := range pods {
	                  fmt.Printf("\npod = %#v\n", p)
	          }
	  }*/
	return nil
}
