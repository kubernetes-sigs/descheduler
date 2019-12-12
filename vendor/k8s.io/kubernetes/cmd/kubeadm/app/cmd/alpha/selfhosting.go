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

package alpha

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1beta2 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/selfhosting"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
)

var (
	selfhostingLongDesc = cmdutil.LongDesc(`
		Convert static Pod files for control plane components into self-hosted DaemonSets configured via the Kubernetes API.

		See the documentation for self-hosting limitations.

		` + cmdutil.AlphaDisclaimer)

	selfhostingExample = cmdutil.Examples(`
		# Convert a static Pod-hosted control plane into a self-hosted one.

		kubeadm alpha phase self-hosting convert-from-staticpods
		`)
)

// NewCmdSelfhosting returns the self-hosting Cobra command
func NewCmdSelfhosting(in io.Reader) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "selfhosting",
		Aliases: []string{"selfhosted", "self-hosting"},
		Short:   "Make a kubeadm cluster self-hosted",
		Long:    cmdutil.MacroCommandLongDescription,
	}

	cmd.AddCommand(getSelfhostingSubCommand(in))
	return cmd
}

// getSelfhostingSubCommand returns sub commands for Self-hosting phase
func getSelfhostingSubCommand(in io.Reader) *cobra.Command {

	cfg := &kubeadmapiv1beta2.ClusterConfiguration{}
	// Default values for the cobra help text
	kubeadmscheme.Scheme.Default(cfg)

	var cfgPath, featureGatesString, kubeConfigFile string
	forcePivot, certsInSecrets := false, false

	// Creates the UX Command
	cmd := &cobra.Command{
		Use:     "pivot",
		Aliases: []string{"from-staticpods"},
		Short:   "Convert a static Pod-hosted control plane into a self-hosted one",
		Long:    selfhostingLongDesc,
		Example: selfhostingExample,
		RunE: func(cmd *cobra.Command, args []string) error {

			var err error

			if !forcePivot {
				fmt.Println("WARNING: self-hosted clusters are not supported by kubeadm upgrade and by other kubeadm commands!")
				fmt.Print("[pivot] are you sure you want to proceed? [y/n]: ")
				s := bufio.NewScanner(in)
				s.Scan()

				if err = s.Err(); err != nil {
					return err
				}

				if strings.ToLower(s.Text()) != "y" {
					return errors.New("aborted pivot operation")
				}
			}

			fmt.Println("[pivot] pivoting cluster to self-hosted")

			if cfg.FeatureGates, err = features.NewFeatureGate(&features.InitFeatureGates, featureGatesString); err != nil {
				return err
			}

			if err := validation.ValidateMixedArguments(cmd.Flags()); err != nil {
				return err
			}

			// Gets the Kubernetes client
			kubeConfigFile = cmdutil.GetKubeConfigPath(kubeConfigFile)
			client, err := kubeconfigutil.ClientSetFromFile(kubeConfigFile)
			if err != nil {
				return err
			}

			// KubernetesVersion is not used, but we set it explicitly to avoid the lookup
			// of the version from the internet when executing LoadOrDefaultInitConfiguration
			phases.SetKubernetesVersion(cfg)

			// This call returns the ready-to-use configuration based on the configuration file that might or might not exist and the default cfg populated by flags
			internalcfg, err := configutil.LoadOrDefaultInitConfiguration(cfgPath, &kubeadmapiv1beta2.InitConfiguration{}, cfg)
			if err != nil {
				return err
			}

			// Converts the Static Pod-hosted control plane into a self-hosted one
			waiter := apiclient.NewKubeWaiter(client, 2*time.Minute, os.Stdout)
			return selfhosting.CreateSelfHostedControlPlane(constants.GetStaticPodDirectory(), constants.KubernetesDir, internalcfg, client, waiter, false, certsInSecrets)
		},
	}

	// Add flags to the command
	// flags bound to the configuration object
	cmd.Flags().StringVar(&cfg.CertificatesDir, "cert-dir", cfg.CertificatesDir, `The path where certificates are stored`)
	options.AddConfigFlag(cmd.Flags(), &cfgPath)

	cmd.Flags().BoolVarP(
		&certsInSecrets, "store-certs-in-secrets", "s",
		false, "Enable storing certs in secrets")

	cmd.Flags().BoolVarP(
		&forcePivot, "force", "f", false,
		"Pivot the cluster without prompting for confirmation",
	)

	// flags that are not bound to the configuration object
	// Note: All flags that are not bound to the cfg object should be whitelisted in cmd/kubeadm/app/apis/kubeadm/validation/validation.go
	options.AddKubeConfigFlag(cmd.Flags(), &kubeConfigFile)

	return cmd
}
