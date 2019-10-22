// +build windows

/*
Copyright 2018 The Kubernetes Authors.

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

package util

import (
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		endpoint         string
		expectError      bool
		expectedProtocol string
		expectedAddr     string
	}{
		{
			endpoint:         "unix:///tmp/s1.sock",
			expectedProtocol: "unix",
			expectError:      true,
		},
		{
			endpoint:         "tcp://localhost:15880",
			expectedProtocol: "tcp",
			expectedAddr:     "localhost:15880",
		},
		{
			endpoint:         "npipe://./pipe/mypipe",
			expectedProtocol: "npipe",
			expectedAddr:     "//./pipe/mypipe",
		},
		{
			endpoint:         "npipe:////./pipe/mypipe2",
			expectedProtocol: "npipe",
			expectedAddr:     "//./pipe/mypipe2",
		},
		{
			endpoint:         "npipe:/pipe/mypipe3",
			expectedProtocol: "npipe",
			expectedAddr:     "//./pipe/mypipe3",
		},
		{
			endpoint:         "npipe:\\\\.\\pipe\\mypipe4",
			expectedProtocol: "npipe",
			expectedAddr:     "//./pipe/mypipe4",
		},
		{
			endpoint:         "npipe:\\pipe\\mypipe5",
			expectedProtocol: "npipe",
			expectedAddr:     "//./pipe/mypipe5",
		},
		{
			endpoint:         "tcp1://abc",
			expectedProtocol: "tcp1",
			expectError:      true,
		},
		{
			endpoint:    "a b c",
			expectError: true,
		},
	}

	for _, test := range tests {
		protocol, addr, err := parseEndpoint(test.endpoint)
		assert.Equal(t, test.expectedProtocol, protocol)
		if test.expectError {
			assert.NotNil(t, err, "Expect error during parsing %q", test.endpoint)
			continue
		}
		require.Nil(t, err, "Expect no error during parsing %q", test.endpoint)
		assert.Equal(t, test.expectedAddr, addr)
	}

}

func testPipe(t *testing.T, label string) {
	generatePipeName := func(suffixlen int) string {
		rand.Seed(time.Now().UnixNano())
		letter := []rune("abcdef0123456789")
		b := make([]rune, suffixlen)
		for i := range b {
			b[i] = letter[rand.Intn(len(letter))]
		}
		return "\\\\.\\pipe\\test-pipe" + string(b)
	}
	testFile := generatePipeName(4)
	pipeln, err := winio.ListenPipe(testFile, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)"})
	defer pipeln.Close()

	require.NoErrorf(t, err, "Failed to listen on named pipe for test purposes: %v while setting up: %s", err, label)
	result, err := IsUnixDomainSocket(testFile)
	assert.Nil(t, err, "Unexpected error: %v from IsUnixDomainSocket for %s", err, label)
	assert.False(t, result, "Unexpected result: true from IsUnixDomainSocket: %v for %s", result, label)
}

func testRegularFile(t *testing.T, label string, exists bool) {
	f, err := ioutil.TempFile("", "test-file")
	require.NoErrorf(t, err, "Failed to create file for test purposes: %v while setting up: %s", err, label)
	testFile := f.Name()
	if !exists {
		testFile = testFile + ".absent"
	}
	f.Close()
	result, err := IsUnixDomainSocket(testFile)
	os.Remove(f.Name())
	assert.Nil(t, err, "Unexpected error: %v from IsUnixDomainSocket for %s", err, label)
	assert.False(t, result, "Unexpected result: true from IsUnixDomainSocket: %v for %s", result, label)
}

func testUnixDomainSocket(t *testing.T, label string) {
	f, err := ioutil.TempFile("", "test-domain-socket")
	require.NoErrorf(t, err, "Failed to create file for test purposes: %v while setting up: %s", err, label)
	testFile := f.Name()
	f.Close()
	os.Remove(testFile)
	ta, err := net.ResolveUnixAddr("unix", testFile)
	require.NoErrorf(t, err, "Failed to ResolveUnixAddr: %v while setting up: %s", err, label)
	unixln, err := net.ListenUnix("unix", ta)
	require.NoErrorf(t, err, "Failed to ListenUnix: %v while setting up: %s", err, label)
	result, err := IsUnixDomainSocket(testFile)
	unixln.Close()
	assert.Nil(t, err, "Unexpected error: %v from IsUnixDomainSocket for %s", err, label)
	assert.True(t, result, "Unexpected result: false from IsUnixDomainSocket: %v for %s", result, label)
}

func TestIsUnixDomainSocket(t *testing.T) {
	testPipe(t, "Named Pipe")
	testRegularFile(t, "Regular File that Exists", true)
	testRegularFile(t, "Regular File that Does Not Exist", false)
	testUnixDomainSocket(t, "Unix Domain Socket File")
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		originalpath   string
		normalizedPath string
	}{
		{
			originalpath:   "\\path\\to\\file",
			normalizedPath: "c:\\path\\to\\file",
		},
		{
			originalpath:   "/path/to/file",
			normalizedPath: "c:\\path\\to\\file",
		},
		{
			originalpath:   "/path/to/dir/",
			normalizedPath: "c:\\path\\to\\dir\\",
		},
		{
			originalpath:   "\\path\\to\\dir\\",
			normalizedPath: "c:\\path\\to\\dir\\",
		},
		{
			originalpath:   "/file",
			normalizedPath: "c:\\file",
		},
		{
			originalpath:   "\\file",
			normalizedPath: "c:\\file",
		},
		{
			originalpath:   "fileonly",
			normalizedPath: "fileonly",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.normalizedPath, NormalizePath(test.originalpath))
	}
}
