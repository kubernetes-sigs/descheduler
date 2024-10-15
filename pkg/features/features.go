package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
	// Every feature gate should add method here following this template:
	//
	// // owner: @username
	// // kep: kep link
	// // alpha: v1.X
	// MyFeature featuregate.Feature = "MyFeature"
	//
	// Feature gates should be listed in alphabetical, case-sensitive
	// (upper before any lower case character) order. This reduces the risk
	// of code conflicts because changes are more likely to be scattered
	// across the file.

	// owner: @ingvagabund
	// kep: https://github.com/kubernetes-sigs/descheduler/issues/1397
	// alpha: v1.31
	//
	// Enable evictions in background so users can create their own eviction policies
	// as an alternative to immediate evictions.
	EvictionsInBackground featuregate.Feature = "EvictionsInBackground"
)

func init() {
	runtime.Must(DefaultMutableFeatureGate.Add(defaultDeschedulerFeatureGates))
}

// defaultDeschedulerFeatureGates consists of all known descheduler-specific feature keys.
// To add a new feature, define a key for it above and add it here. The features will be
// available throughout descheduler binary.
//
// Entries are separated from each other with blank lines to avoid sweeping gofmt changes
// when adding or removing one entry.
var defaultDeschedulerFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	EvictionsInBackground: {Default: false, PreRelease: featuregate.Alpha},
}

// DefaultMutableFeatureGate is a mutable version of DefaultFeatureGate.
// Only top-level commands/options setup and the k8s.io/component-base/featuregate/testing package should make use of this.
// Tests that need to modify feature gates for the duration of their test should use:
//
//	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.<FeatureName>, <value>)()
var DefaultMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()
