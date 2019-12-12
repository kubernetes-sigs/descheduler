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
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/upgrade"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
)

const (
	defaultImagePullTimeout = 15 * time.Minute
)

// applyFlags holds the information about the flags that can be passed to apply
type applyFlags struct {
	*applyPlanFlags

	nonInteractiveMode bool
	force              bool
	dryRun             bool
	etcdUpgrade        bool
	renewCerts         bool
	imagePullTimeout   time.Duration
	kustomizeDir       string
}

// sessionIsInteractive returns true if the session is of an interactive type (the default, can be opted out of with -y, -f or --dry-run)
func (f *applyFlags) sessionIsInteractive() bool {
	return !(f.nonInteractiveMode || f.dryRun || f.force)
}

// NewCmdApply returns the cobra command for `kubeadm upgrade apply`
func NewCmdApply(apf *applyPlanFlags) *cobra.Command {
	flags := &applyFlags{
		applyPlanFlags:   apf,
		imagePullTimeout: defaultImagePullTimeout,
		etcdUpgrade:      true,
		renewCerts:       true,
	}

	cmd := &cobra.Command{
		Use:                   "apply [version]",
		DisableFlagsInUseLine: true,
		Short:                 "Upgrade your Kubernetes cluster to the specified version",
		RunE: func(cmd *cobra.Command, args []string) error {
			userVersion, err := getK8sVersionFromUserInput(flags.applyPlanFlags, args, true)
			if err != nil {
				return err
			}

			return runApply(flags, userVersion)
		},
	}

	// Register the common flags for apply and plan
	addApplyPlanFlags(cmd.Flags(), flags.applyPlanFlags)
	// Specify the valid flags specific for apply
	cmd.Flags().BoolVarP(&flags.nonInteractiveMode, "yes", "y", flags.nonInteractiveMode, "Perform the upgrade and do not prompt for confirmation (non-interactive mode).")
	cmd.Flags().BoolVarP(&flags.force, "force", "f", flags.force, "Force upgrading although some requirements might not be met. This also implies non-interactive mode.")
	cmd.Flags().BoolVar(&flags.dryRun, options.DryRun, flags.dryRun, "Do not change any state, just output what actions would be performed.")
	cmd.Flags().BoolVar(&flags.etcdUpgrade, "etcd-upgrade", flags.etcdUpgrade, "Perform the upgrade of etcd.")
	cmd.Flags().BoolVar(&flags.renewCerts, options.CertificateRenewal, flags.renewCerts, "Perform the renewal of certificates used by component changed during upgrades.")
	cmd.Flags().DurationVar(&flags.imagePullTimeout, "image-pull-timeout", flags.imagePullTimeout, "The maximum amount of time to wait for the control plane pods to be downloaded.")
	options.AddKustomizePodsFlag(cmd.Flags(), &flags.kustomizeDir)

	return cmd
}

// runApply takes care of the actual upgrade functionality
// It does the following things:
// - Checks if the cluster is healthy
// - Gets the configuration from the kubeadm-config ConfigMap in the cluster
// - Enforces all version skew policies
// - Asks the user if they really want to upgrade
// - Makes sure the control plane images are available locally on the control-plane(s)
// - Upgrades the control plane components
// - Applies the other resources that'd be created with kubeadm init as well, like
//   - Creating the RBAC rules for the bootstrap tokens and the cluster-info ConfigMap
//   - Applying new kube-dns and kube-proxy manifests
//   - Uploads the newly used configuration to the cluster ConfigMap
func runApply(flags *applyFlags, userVersion string) error {

	// Start with the basics, verify that the cluster is healthy and get the configuration from the cluster (using the ConfigMap)
	klog.V(1).Infoln("[upgrade/apply] verifying health of cluster")
	klog.V(1).Infoln("[upgrade/apply] retrieving configuration from cluster")
	client, versionGetter, cfg, err := enforceRequirements(flags.applyPlanFlags, flags.dryRun, userVersion)
	if err != nil {
		return err
	}

	// Validate requested and validate actual version
	klog.V(1).Infoln("[upgrade/apply] validating requested and actual version")
	if err := configutil.NormalizeKubernetesVersion(&cfg.ClusterConfiguration); err != nil {
		return err
	}

	// Use normalized version string in all following code.
	newK8sVersion, err := version.ParseSemantic(cfg.KubernetesVersion)
	if err != nil {
		return errors.Errorf("unable to parse normalized version %q as a semantic version", cfg.KubernetesVersion)
	}

	if err := features.ValidateVersion(features.InitFeatureGates, cfg.FeatureGates, cfg.KubernetesVersion); err != nil {
		return err
	}

	// Enforce the version skew policies
	klog.V(1).Infoln("[upgrade/version] enforcing version skew policies")
	if err := EnforceVersionPolicies(cfg.KubernetesVersion, newK8sVersion, flags, versionGetter); err != nil {
		return errors.Wrap(err, "[upgrade/version] FATAL")
	}

	// If the current session is interactive, ask the user whether they really want to upgrade.
	if flags.sessionIsInteractive() {
		if err := InteractivelyConfirmUpgrade("Are you sure you want to proceed with the upgrade?"); err != nil {
			return err
		}
	}

	waiter := getWaiter(flags.dryRun, client)

	// Use a prepuller implementation based on creating DaemonSets
	// and block until all DaemonSets are ready; then we know for sure that all control plane images are cached locally
	klog.V(1).Infoln("[upgrade/apply] creating prepuller")
	prepuller := upgrade.NewDaemonSetPrepuller(client, waiter, &cfg.ClusterConfiguration)
	componentsToPrepull := constants.ControlPlaneComponents
	if cfg.Etcd.External == nil && flags.etcdUpgrade {
		componentsToPrepull = append(componentsToPrepull, constants.Etcd)
	}
	if err := upgrade.PrepullImagesInParallel(prepuller, flags.imagePullTimeout, componentsToPrepull); err != nil {
		return errors.Wrap(err, "[upgrade/prepull] Failed prepulled the images for the control plane components error")
	}

	// Now; perform the upgrade procedure
	klog.V(1).Infoln("[upgrade/apply] performing upgrade")
	if err := PerformControlPlaneUpgrade(flags, client, waiter, cfg); err != nil {
		return errors.Wrap(err, "[upgrade/apply] FATAL")
	}

	// Upgrade RBAC rules and addons.
	klog.V(1).Infoln("[upgrade/postupgrade] upgrading RBAC rules and addons")
	if err := upgrade.PerformPostUpgradeTasks(client, cfg, newK8sVersion, flags.dryRun); err != nil {
		return errors.Wrap(err, "[upgrade/postupgrade] FATAL post-upgrade error")
	}

	if flags.dryRun {
		fmt.Println("[dryrun] Finished dryrunning successfully!")
		return nil
	}

	fmt.Println("")
	fmt.Printf("[upgrade/successful] SUCCESS! Your cluster was upgraded to %q. Enjoy!\n", cfg.KubernetesVersion)
	fmt.Println("")
	fmt.Println("[upgrade/kubelet] Now that your control plane is upgraded, please proceed with upgrading your kubelets if you haven't already done so.")

	return nil
}

// EnforceVersionPolicies makes sure that the version the user specified is valid to upgrade to
// There are both fatal and skippable (with --force) errors
func EnforceVersionPolicies(newK8sVersionStr string, newK8sVersion *version.Version, flags *applyFlags, versionGetter upgrade.VersionGetter) error {
	fmt.Printf("[upgrade/version] You have chosen to change the cluster version to %q\n", newK8sVersionStr)

	versionSkewErrs := upgrade.EnforceVersionPolicies(versionGetter, newK8sVersionStr, newK8sVersion, flags.allowExperimentalUpgrades, flags.allowRCUpgrades)
	if versionSkewErrs != nil {

		if len(versionSkewErrs.Mandatory) > 0 {
			return errors.Errorf("the --version argument is invalid due to these fatal errors:\n\n%v\nPlease fix the misalignments highlighted above and try upgrading again",
				kubeadmutil.FormatErrMsg(versionSkewErrs.Mandatory))
		}

		if len(versionSkewErrs.Skippable) > 0 {
			// Return the error if the user hasn't specified the --force flag
			if !flags.force {
				return errors.Errorf("the --version argument is invalid due to these errors:\n\n%v\nCan be bypassed if you pass the --force flag",
					kubeadmutil.FormatErrMsg(versionSkewErrs.Skippable))
			}
			// Soft errors found, but --force was specified
			fmt.Printf("[upgrade/version] Found %d potential version compatibility errors but skipping since the --force flag is set: \n\n%v", len(versionSkewErrs.Skippable), kubeadmutil.FormatErrMsg(versionSkewErrs.Skippable))
		}
	}
	return nil
}

// PerformControlPlaneUpgrade actually performs the upgrade procedure for the cluster of your type (self-hosted or static-pod-hosted)
func PerformControlPlaneUpgrade(flags *applyFlags, client clientset.Interface, waiter apiclient.Waiter, internalcfg *kubeadmapi.InitConfiguration) error {

	// OK, the cluster is hosted using static pods. Upgrade a static-pod hosted cluster
	fmt.Printf("[upgrade/apply] Upgrading your Static Pod-hosted control plane to version %q...\n", internalcfg.KubernetesVersion)

	if flags.dryRun {
		return upgrade.DryRunStaticPodUpgrade(flags.kustomizeDir, internalcfg)
	}

	// Don't save etcd backup directory if etcd is HA, as this could cause corruption
	return upgrade.PerformStaticPodUpgrade(client, waiter, internalcfg, flags.etcdUpgrade, flags.renewCerts, flags.kustomizeDir)
}
