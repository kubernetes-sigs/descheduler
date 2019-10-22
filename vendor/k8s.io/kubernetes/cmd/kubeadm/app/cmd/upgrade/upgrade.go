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

package upgrade

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

// applyPlanFlags holds the values for the common flags in `kubeadm upgrade apply` and `kubeadm upgrade plan`
type applyPlanFlags struct {
	kubeConfigPath            string
	cfgPath                   string
	featureGatesString        string
	allowExperimentalUpgrades bool
	allowRCUpgrades           bool
	printConfig               bool
	ignorePreflightErrors     []string
	out                       io.Writer
}

// NewCmdUpgrade returns the cobra command for `kubeadm upgrade`
func NewCmdUpgrade(out io.Writer) *cobra.Command {
	flags := &applyPlanFlags{
		kubeConfigPath:            kubeadmconstants.GetAdminKubeConfigPath(),
		cfgPath:                   "",
		featureGatesString:        "",
		allowExperimentalUpgrades: false,
		allowRCUpgrades:           false,
		printConfig:               false,
		out:                       out,
	}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade your cluster smoothly to a newer version with this command",
		RunE:  cmdutil.SubCmdRunE("upgrade"),
	}

	cmd.AddCommand(NewCmdApply(flags))
	cmd.AddCommand(NewCmdPlan(flags))
	cmd.AddCommand(NewCmdDiff(out))
	cmd.AddCommand(NewCmdNode())
	return cmd
}

func addApplyPlanFlags(fs *pflag.FlagSet, flags *applyPlanFlags) {
	options.AddKubeConfigFlag(fs, &flags.kubeConfigPath)
	options.AddConfigFlag(fs, &flags.cfgPath)

	fs.BoolVar(&flags.allowExperimentalUpgrades, "allow-experimental-upgrades", flags.allowExperimentalUpgrades, "Show unstable versions of Kubernetes as an upgrade alternative and allow upgrading to an alpha/beta/release candidate versions of Kubernetes.")
	fs.BoolVar(&flags.allowRCUpgrades, "allow-release-candidate-upgrades", flags.allowRCUpgrades, "Show release candidate versions of Kubernetes as an upgrade alternative and allow upgrading to a release candidate versions of Kubernetes.")
	fs.BoolVar(&flags.printConfig, "print-config", flags.printConfig, "Specifies whether the configuration file that will be used in the upgrade should be printed or not.")
	options.AddFeatureGatesStringFlag(fs, &flags.featureGatesString)
	options.AddIgnorePreflightErrorsFlag(fs, &flags.ignorePreflightErrors)
}
