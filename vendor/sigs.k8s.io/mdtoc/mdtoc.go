/*
Copyright 2020 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"sigs.k8s.io/mdtoc/pkg/mdtoc"
)

type utilityOptions struct {
	mdtoc.Options
	Inplace bool
}

var defaultOptions utilityOptions

func init() {
	flag.BoolVar(&defaultOptions.Dryrun, "dryrun", false, "Whether to check for changes to TOC, rather than overwriting. Requires --inplace flag.")
	flag.BoolVar(&defaultOptions.Inplace, "inplace", false, "Whether to edit the file in-place, or output to STDOUT. Requires toc tags to be present.")
	flag.BoolVar(&defaultOptions.SkipPrefix, "skip-prefix", true, "Whether to ignore any headers before the opening toc tag.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTIONS] [FILE]...\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Generate a table of contents for a markdown file (github flavor).\n")
		fmt.Fprintf(flag.CommandLine.Output(), "TOC may be wrapped in a pair of tags to allow in-place updates: <!-- toc --><!-- /toc -->\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if err := validateArgs(defaultOptions, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	switch defaultOptions.Inplace {
	case true:
		hadError := false
		for _, file := range flag.Args() {
			err := mdtoc.WriteTOC(file, defaultOptions.Options)
			if err != nil {
				log.Printf("%s: %v", file, err)
				hadError = true
			}
		}
		if hadError {
			os.Exit(1)
		}
	case false:
		toc, err := mdtoc.GetTOC(flag.Args()[0], defaultOptions.Options)
		if err != nil {
			os.Exit(1)
		}
		fmt.Println(toc)
	}
}

func validateArgs(opts utilityOptions, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("must specify at least 1 file")
	}
	if !opts.Inplace && len(args) > 1 {
		return fmt.Errorf("non-inplace updates require exactly 1 file")
	}
	if opts.Dryrun && !opts.Inplace {
		return fmt.Errorf("--dryrun requires --inplace")
	}
	return nil
}
