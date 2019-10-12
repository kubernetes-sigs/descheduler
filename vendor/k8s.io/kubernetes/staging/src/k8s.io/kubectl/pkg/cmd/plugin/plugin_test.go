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

package plugin

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestPluginPathsAreUnaltered(t *testing.T) {
	tempDir, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tempDir2, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cleanup
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
		if err := os.RemoveAll(tempDir2); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
	}()

	ioStreams, _, _, errOut := genericclioptions.NewTestIOStreams()
	verifier := newFakePluginPathVerifier()
	pluginPaths := []string{tempDir, tempDir2}
	o := &PluginListOptions{
		Verifier:  verifier,
		IOStreams: ioStreams,

		PluginPaths: pluginPaths,
	}

	// write at least one valid plugin file
	if _, err := ioutil.TempFile(tempDir, "kubectl-"); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := ioutil.TempFile(tempDir2, "kubectl-"); err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if err := o.Run(); err != nil {
		t.Fatalf("unexpected error %v - %v", err, errOut.String())
	}

	// ensure original paths remain unaltered
	if len(verifier.seenUnsorted) != len(pluginPaths) {
		t.Fatalf("saw unexpected plugin paths. Expecting %v, got %v", pluginPaths, verifier.seenUnsorted)
	}
	for actual := range verifier.seenUnsorted {
		if !strings.HasPrefix(verifier.seenUnsorted[actual], pluginPaths[actual]) {
			t.Fatalf("expected PATH slice to be unaltered. Expecting %v, but got %v", pluginPaths[actual], verifier.seenUnsorted[actual])
		}
	}
}

func TestPluginPathsAreValid(t *testing.T) {
	tempDir, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cleanup
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
	}()

	tc := []struct {
		name               string
		pluginPaths        []string
		pluginFile         func() (*os.File, error)
		verifier           *fakePluginPathVerifier
		expectVerifyErrors []error
		expectErr          string
	}{
		{
			name:        "ensure no plugins found if no files begin with kubectl- prefix",
			pluginPaths: []string{tempDir},
			verifier:    newFakePluginPathVerifier(),
			pluginFile: func() (*os.File, error) {
				return ioutil.TempFile(tempDir, "notkubectl-")
			},
			expectErr: "unable to find any kubectl plugins in your PATH",
		},
		{
			name:        "ensure de-duplicated plugin-paths slice",
			pluginPaths: []string{tempDir, tempDir},
			verifier:    newFakePluginPathVerifier(),
			pluginFile: func() (*os.File, error) {
				return ioutil.TempFile(tempDir, "kubectl-")
			},
		},
	}

	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			ioStreams, _, _, errOut := genericclioptions.NewTestIOStreams()
			o := &PluginListOptions{
				Verifier:  test.verifier,
				IOStreams: ioStreams,

				PluginPaths: test.pluginPaths,
			}

			// create files
			if test.pluginFile != nil {
				if _, err := test.pluginFile(); err != nil {
					t.Fatalf("unexpected error creating plugin file: %v", err)
				}
			}

			for _, expected := range test.expectVerifyErrors {
				for _, actual := range test.verifier.errors {
					if expected != actual {
						t.Fatalf("unexpected error: expected %v, but got %v", expected, actual)
					}
				}
			}

			err := o.Run()
			if err == nil && len(test.expectErr) > 0 {
				t.Fatalf("unexpected non-error: expecting %v", test.expectErr)
			}
			if err != nil && len(test.expectErr) == 0 {
				t.Fatalf("unexpected error: %v - %v", err, errOut.String())
			}
			if err == nil {
				return
			}

			allErrs := bytes.NewBuffer(errOut.Bytes())
			if _, err := allErrs.WriteString(err.Error()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(test.expectErr) > 0 {
				if !strings.Contains(allErrs.String(), test.expectErr) {
					t.Fatalf("unexpected error: expected %v, but got %v", test.expectErr, allErrs.String())
				}
			}
		})
	}
}

type duplicatePathError struct {
	path string
}

func (d *duplicatePathError) Error() string {
	return fmt.Sprintf("path %q already visited", d.path)
}

type fakePluginPathVerifier struct {
	errors       []error
	seen         map[string]bool
	seenUnsorted []string
}

func (f *fakePluginPathVerifier) Verify(path string) []error {
	if f.seen[path] {
		err := &duplicatePathError{path}
		f.errors = append(f.errors, err)
		return []error{err}
	}
	f.seen[path] = true
	f.seenUnsorted = append(f.seenUnsorted, path)
	return nil
}

func newFakePluginPathVerifier() *fakePluginPathVerifier {
	return &fakePluginPathVerifier{seen: make(map[string]bool)}
}
