/*
Copyright 2019 The Kubernetes Authors.

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

package apimachinery

import (
	"fmt"
	"strconv"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = SIGDescribe("client-go should negotiate", func() {
	f := framework.NewDefaultFramework("protocol")
	f.SkipNamespaceCreation = true

	for _, s := range []string{
		"application/json",
		"application/vnd.kubernetes.protobuf",
		"application/vnd.kubernetes.protobuf,application/json",
		"application/json,application/vnd.kubernetes.protobuf",
	} {
		accept := s
		g.It(fmt.Sprintf("watch and report errors with accept %q", accept), func() {
			cfg, err := framework.LoadConfig()
			framework.ExpectNoError(err)

			cfg.AcceptContentTypes = accept

			c := kubernetes.NewForConfigOrDie(cfg)
			svcs, err := c.CoreV1().Services("default").Get("kubernetes", metav1.GetOptions{})
			framework.ExpectNoError(err)
			rv, err := strconv.Atoi(svcs.ResourceVersion)
			framework.ExpectNoError(err)
			w, err := c.CoreV1().Services("default").Watch(metav1.ListOptions{ResourceVersion: strconv.Itoa(rv - 1)})
			framework.ExpectNoError(err)
			defer w.Stop()

			evt, ok := <-w.ResultChan()
			o.Expect(ok).To(o.BeTrue())
			switch evt.Type {
			case watch.Added, watch.Modified:
				// this is allowed
			case watch.Error:
				err := errors.FromObject(evt.Object)
				if errors.IsGone(err) {
					// this is allowed, since the kubernetes object could be very old
					break
				}
				if errors.IsUnexpectedObjectError(err) {
					g.Fail(fmt.Sprintf("unexpected object, wanted v1.Status: %#v", evt.Object))
				}
				g.Fail(fmt.Sprintf("unexpected error: %#v", evt.Object))
			default:
				g.Fail(fmt.Sprintf("unexpected type %s: %#v", evt.Type, evt.Object))
			}
		})
	}
})
