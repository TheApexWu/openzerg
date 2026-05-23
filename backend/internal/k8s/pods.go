// Package k8s — pod lifecycle helpers used by the M2 spawn pipeline.
//
// This file adds the smallest possible CreatePod wrapper around the typed
// CoreV1 client. Higher-level orchestration (StreamLogs / WaitForCompletion
// / DeletePod) lands in subsequent increments. The wrapper exists so that
// internal/spawn never has to import client-go directly.
package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreatePod creates pod in its declared namespace using the supplied
// clientset. The created object (with server-populated fields like UID and
// resourceVersion) is returned on success.
//
// The pod argument must already have ObjectMeta.Namespace and
// ObjectMeta.Name set; spawn.BuildBusyboxPod produces objects that satisfy
// this. CreatePod uses metav1.CreateOptions{} (no field manager / dry-run);
// callers wanting those should call the underlying client directly.
func CreatePod(ctx context.Context, cs kubernetes.Interface, pod *corev1.Pod) (*corev1.Pod, error) {
	if cs == nil {
		return nil, fmt.Errorf("k8s.CreatePod: nil clientset")
	}
	if pod == nil {
		return nil, fmt.Errorf("k8s.CreatePod: nil pod")
	}
	if pod.Namespace == "" {
		return nil, fmt.Errorf("k8s.CreatePod: pod %q has empty namespace", pod.Name)
	}
	if pod.Name == "" {
		return nil, fmt.Errorf("k8s.CreatePod: pod has empty name")
	}
	created, err := cs.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.CreatePod: create %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return created, nil
}
