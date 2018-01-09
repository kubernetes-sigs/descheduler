/*
Copyright 2014 The Kubernetes Authors.

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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/kubectl/cmd/templates"
	"k8s.io/kubernetes/pkg/printers"
	printersinternal "k8s.io/kubernetes/pkg/printers/internalversion"

	"github.com/spf13/cobra"
)

// AddPrinterFlags adds printing related flags to a command (e.g. output format, no headers, template path)
func AddPrinterFlags(cmd *cobra.Command) {
	AddNonDeprecatedPrinterFlags(cmd)

	cmd.Flags().String("output-version", "", "DEPRECATED: To use a specific API version, fully-qualify the resource, version, and group (for example: 'jobs.v1.batch/myjob').")
	cmd.Flags().MarkDeprecated("output-version", "The resource is used exactly as fetched from the API. To get a specific API version, fully-qualify the resource, version, and group (for example: 'jobs.v1.batch/myjob').")
	cmd.Flags().MarkHidden("output-version")
}

// AddNonDeprecatedPrinterFlags supports the conversion case which must logically have output-version.  Once output-version
// is completely removed, this function can go away.
func AddNonDeprecatedPrinterFlags(cmd *cobra.Command) {
	AddOutputFlags(cmd)
	AddNoHeadersFlags(cmd)
	cmd.Flags().Bool("show-labels", false, "When printing, show all labels as the last column (default hide labels column)")
	cmd.Flags().String("template", "", "Template string or path to template file to use when -o=go-template, -o=go-template-file. The template format is golang templates [http://golang.org/pkg/text/template/#pkg-overview].")
	cmd.MarkFlagFilename("template")
	cmd.Flags().String("sort-by", "", "If non-empty, sort list types using this field specification.  The field specification is expressed as a JSONPath expression (e.g. '{.metadata.name}'). The field in the API resource specified by this JSONPath expression must be an integer or a string.")
	cmd.Flags().BoolP("show-all", "a", false, "When printing, show all resources (default hide terminated pods.)")
}

// AddOutputFlagsForMutation adds output related flags to a command. Used by mutations only.
func AddOutputFlagsForMutation(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", "Output mode. Use \"-o name\" for shorter output (resource/name).")
}

// AddOutputVarFlagsForMutation adds output related flags to a command. Used by mutations only.
func AddOutputVarFlagsForMutation(cmd *cobra.Command, output *string) {
	cmd.Flags().StringVarP(output, "output", "o", "", "Output mode. Use \"-o name\" for shorter output (resource/name).")
}

// AddOutputFlags adds output related flags to a command.
func AddOutputFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", "Output format. One of: json|yaml|wide|name|custom-columns=...|custom-columns-file=...|go-template=...|go-template-file=...|jsonpath=...|jsonpath-file=... See custom columns [http://kubernetes.io/docs/user-guide/kubectl-overview/#custom-columns], golang template [http://golang.org/pkg/text/template/#pkg-overview] and jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	cmd.Flags().Bool("allow-missing-template-keys", true, "If true, ignore any errors in templates when a field or map key is missing in the template. Only applies to golang and jsonpath output formats.")
}

// AddNoHeadersFlags adds no-headers flags to a command.
func AddNoHeadersFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("no-headers", false, "When using the default or custom-column output format, don't print headers (default print headers).")
}

// ValidateOutputArgs validates -o flag args for mutations
func ValidateOutputArgs(cmd *cobra.Command) error {
	outputMode := GetFlagString(cmd, "output")
	if outputMode != "" && outputMode != "name" {
		return UsageErrorf(cmd, "Unexpected -o output mode: %v. We only support '-o name'.", outputMode)
	}
	return nil
}

// PrinterForOptions returns the printer for the outputOptions (if given) or
// returns the default printer for the command. Requires that printer flags have
// been added to cmd (see AddPrinterFlags).
func PrinterForOptions(mapper meta.RESTMapper, typer runtime.ObjectTyper, encoder runtime.Encoder, decoders []runtime.Decoder, options *printers.PrintOptions) (printers.ResourcePrinter, error) {
	printer, err := printers.GetStandardPrinter(mapper, typer, encoder, decoders, *options)
	if err != nil {
		return nil, err
	}

	// we try to convert to HumanReadablePrinter, if return ok, it must be no generic
	// we execute AddHandlers() here before maybeWrapSortingPrinter so that we don't
	// need to convert to delegatePrinter again then invoke AddHandlers()
	if humanReadablePrinter, ok := printer.(*printers.HumanReadablePrinter); ok {
		printersinternal.AddHandlers(humanReadablePrinter)
	}

	return maybeWrapSortingPrinter(printer, *options), nil
}

// ExtractCmdPrintOptions parses printer specific commandline args and
// returns a PrintOptions object.
// Requires that printer flags have been added to cmd (see AddPrinterFlags)
func ExtractCmdPrintOptions(cmd *cobra.Command, withNamespace bool) *printers.PrintOptions {
	flags := cmd.Flags()

	columnLabel, err := flags.GetStringSlice("label-columns")
	if err != nil {
		columnLabel = []string{}
	}

	options := &printers.PrintOptions{
		NoHeaders:          GetFlagBool(cmd, "no-headers"),
		Wide:               GetWideFlag(cmd),
		ShowAll:            GetFlagBool(cmd, "show-all"),
		ShowLabels:         GetFlagBool(cmd, "show-labels"),
		AbsoluteTimestamps: isWatch(cmd),
		ColumnLabels:       columnLabel,
		WithNamespace:      withNamespace,
	}

	var outputFormat string
	if flags.Lookup("output") != nil {
		outputFormat = GetFlagString(cmd, "output")
	}

	if flags.Lookup("sort-by") != nil {
		options.SortBy = GetFlagString(cmd, "sort-by")
	}

	// templates are logically optional for specifying a format.
	// TODO once https://github.com/kubernetes/kubernetes/issues/12668 is fixed, this should fall back to GetFlagString
	var templateFile string
	if flag := flags.Lookup("template"); flag != nil {
		if flag.Value.Type() == "string" {
			templateFile = GetFlagString(cmd, "template")
		}
	}
	if len(outputFormat) == 0 && len(templateFile) != 0 {
		outputFormat = "template"
	}

	templateFormats := []string{
		"go-template=", "go-template-file=", "jsonpath=", "jsonpath-file=", "custom-columns=", "custom-columns-file=",
	}
	for _, format := range templateFormats {
		if strings.HasPrefix(outputFormat, format) {
			templateFile = outputFormat[len(format):]
			outputFormat = format[:len(format)-1]
		}
	}

	// this function may be invoked by a command that did not call AddPrinterFlags first, so we need
	// to be safe about how we access the allow-missing-template-keys flag
	if flags.Lookup("allow-missing-template-keys") != nil {
		options.AllowMissingKeys = GetFlagBool(cmd, "allow-missing-template-keys")
	}

	options.OutputFormatType = outputFormat
	options.OutputFormatArgument = templateFile

	return options
}

func maybeWrapSortingPrinter(printer printers.ResourcePrinter, printOpts printers.PrintOptions) printers.ResourcePrinter {
	if len(printOpts.SortBy) != 0 {
		return &kubectl.SortingPrinter{
			Delegate:  printer,
			SortField: fmt.Sprintf("{%s}", printOpts.SortBy),
		}
	}
	return printer
}

// ValidResourceTypeList returns a multi-line string containing the valid resources. May
// be called before the factory is initialized.
// TODO: This function implementation should be replaced with a real implementation from the
//   discovery service.
func ValidResourceTypeList(f ClientAccessFactory) string {
	// TODO: Should attempt to use the cached discovery list or fallback to a static list
	// that is calculated from code compiled into the factory.
	return templates.LongDesc(`Valid resource types include:
	
			* all
			* certificatesigningrequests (aka 'csr')
			* clusterrolebindings
			* clusterroles
			* componentstatuses (aka 'cs')
			* configmaps (aka 'cm')
			* controllerrevisions
			* cronjobs
			* customresourcedefinition (aka 'crd')
			* daemonsets (aka 'ds')
			* deployments (aka 'deploy')
			* endpoints (aka 'ep')
			* events (aka 'ev')
			* horizontalpodautoscalers (aka 'hpa')
			* ingresses (aka 'ing')
			* jobs
			* limitranges (aka 'limits')
			* namespaces (aka 'ns')
			* networkpolicies (aka 'netpol')
			* nodes (aka 'no')
			* persistentvolumeclaims (aka 'pvc')
			* persistentvolumes (aka 'pv')
			* poddisruptionbudgets (aka 'pdb')
			* podpreset
			* pods (aka 'po')
			* podsecuritypolicies (aka 'psp')
			* podtemplates
			* replicasets (aka 'rs')
			* replicationcontrollers (aka 'rc')
			* resourcequotas (aka 'quota')
			* rolebindings
			* roles
			* secrets
			* serviceaccounts (aka 'sa')
			* services (aka 'svc')
			* statefulsets (aka 'sts')
			* storageclasses (aka 'sc')
	
	`)
}
