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

package app

import (
	"fmt"
	"github.com/spf13/cobra"
	"runtime"
	"strings"
)

var (
	// gitCommit is a constant representing the source version that
	// generated this build. It should be set during build via -ldflags.
	gitCommit string
	// version is a constant representing the version tag that
	// generated this build. It should be set during build via -ldflags.
	version string
	// buildDate in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	//It should be set during build via -ldflags.
	buildDate string
)

// Info holds the information related to descheduler app version.
type Info struct {
	Major      string `json:"major"`
	Minor      string `json:"minor"`
	GitCommit  string `json:"gitCommit"`
	GitVersion string `json:"gitVersion"`
	BuildDate  string `json:"buildDate"`
	GoVersion  string `json:"goVersion"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	majorVersion, minorVersion := splitVersion(version)
	return Info{
		Major:      majorVersion,
		Minor:      minorVersion,
		GitCommit:  gitCommit,
		GitVersion: version,
		BuildDate:  buildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func NewVersionCommand() *cobra.Command {
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Version of descheduler",
		Long:  `Prints the version of descheduler.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Descheduler version %+v\n", Get())
		},
	}
	return versionCmd
}

// splitVersion splits the git version to generate major and minor versions needed.
func splitVersion(version string) (string, string) {
	if version == "" {
		return "", ""
	}
	// A sample version would be of form v0.1.0-7-ge884046, so split at first '.' and
	// then return 0 and 1+(+ appended to follow semver convention) for major and minor versions.
	return strings.Trim(strings.Split(version, ".")[0], "v"), strings.Split(version, ".")[1] + "+"
}
