package utils

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// IsPVCLocal returns true if the pod has claimed a Persistent Volume
// that is a Local or HostPath type
func IsPVCLocal(ctx context.Context, client clientset.Interface, pvc *v1.PersistentVolumeClaim) (isPVCLocal bool, err error) {
	pv, err := GetPV(ctx, client, pvc.Spec.VolumeName)
	if pv != nil {
		// consider both Local and HostPast as "local"
		if pv.Spec.Local != nil {
			return true, nil
		}
		if pv.Spec.HostPath != nil {
			return true, nil
		}
	}
	if err != nil {
		klog.ErrorS(err, "Failed to get PV for PVC")
		return false, err
	}
	return false, nil
}

// GetPVC returns a PersistentVolumeClaim matching provided name string in namespace
func GetPVC(ctx context.Context, client clientset.Interface, name string, namespace string) (pv *v1.PersistentVolumeClaim, err error) {
	if name != "" {
		pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return pvc, nil
	}
	return nil, nil
}

// GetPV returns a PersistentVolume matching provided name string
func GetPV(ctx context.Context, client clientset.Interface, name string) (pv *v1.PersistentVolume, err error) {
	if name != "" {
		pv, err := client.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return pv, nil
	}
	return nil, nil
}
