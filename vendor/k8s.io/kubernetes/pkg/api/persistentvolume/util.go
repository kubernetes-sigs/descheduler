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

package persistentvolume

import (
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/features"
)

// DropDisabledFields removes disabled fields from the pv spec.
// This should be called from PrepareForCreate/PrepareForUpdate for all resources containing a pv spec.
func DropDisabledFields(pvSpec *api.PersistentVolumeSpec, oldPVSpec *api.PersistentVolumeSpec) {
	if !utilfeature.DefaultFeatureGate.Enabled(features.BlockVolume) && !volumeModeInUse(oldPVSpec) {
		pvSpec.VolumeMode = nil
	}

	if !utilfeature.DefaultFeatureGate.Enabled(features.ExpandCSIVolumes) && !hasExpansionSecrets(oldPVSpec) {
		if pvSpec.CSI != nil {
			pvSpec.CSI.ControllerExpandSecretRef = nil
		}
	}
}

func hasExpansionSecrets(oldPVSpec *api.PersistentVolumeSpec) bool {
	if oldPVSpec == nil || oldPVSpec.CSI == nil {
		return false
	}

	if oldPVSpec.CSI.ControllerExpandSecretRef != nil {
		return true
	}
	return false
}

func volumeModeInUse(oldPVSpec *api.PersistentVolumeSpec) bool {
	if oldPVSpec == nil {
		return false
	}
	if oldPVSpec.VolumeMode != nil {
		return true
	}
	return false
}
