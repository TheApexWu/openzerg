// Package k8s — pod lifecycle helpers used by the M2 spawn pipeline.
//
// This file holds the small CreatePod / DeletePod wrappers around the typed
// CoreV1 client. Higher-level orchestration (StreamLogs / WaitForCompletion)
// lands in subsequent increments. The wrappers exist so that internal/spawn
// never has to import client-go directly.
package k8s

import (
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// defaultPollInterval is the polling cadence used by WaitForCompletion when
// the caller does not pass an explicit interval. Tests inject a much smaller
// value through WaitForCompletionWithInterval.
const defaultPollInterval = 2 * time.Second

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

// DeletePod removes a pod by namespace/name. NotFound is treated as success
// so that the function is idempotent and safe to call from a defer after a
// CreatePod failure or after the pod has already been garbage-collected.
//
// gracePeriodSeconds=0 is used to ensure the swarm cleans up promptly; for
// the busybox/PI attacker workloads we don't need graceful shutdown.
func DeletePod(ctx context.Context, cs kubernetes.Interface, namespace, name string) error {
	if cs == nil {
		return fmt.Errorf("k8s.DeletePod: nil clientset")
	}
	if namespace == "" {
		return fmt.Errorf("k8s.DeletePod: empty namespace")
	}
	if name == "" {
		return fmt.Errorf("k8s.DeletePod: empty name")
	}
	zero := int64(0)
	err := cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &zero,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("k8s.DeletePod: delete %s/%s: %w", namespace, name, err)
	}
	return nil
}

// WaitForCompletion polls the pod until it reaches a terminal phase
// (PodSucceeded or PodFailed) or the context is cancelled. The final pod
// object (after the terminal Get) is returned so callers can inspect
// container statuses, exit codes, and message strings without an extra
// round-trip.
//
// A NotFound during polling is treated as an error: the swarm should never
// race a foreign delete, and silently succeeding here would hide bugs.
//
// Callers wanting a non-default poll cadence (e.g. unit tests) should use
// WaitForCompletionWithInterval; this wrapper exists so that production call
// sites stay terse.
func WaitForCompletion(ctx context.Context, cs kubernetes.Interface, namespace, name string) (*corev1.Pod, error) {
	return WaitForCompletionWithInterval(ctx, cs, namespace, name, defaultPollInterval)
}

// WaitForCompletionWithInterval is WaitForCompletion with an injectable poll
// interval. interval must be > 0.
func WaitForCompletionWithInterval(ctx context.Context, cs kubernetes.Interface, namespace, name string, interval time.Duration) (*corev1.Pod, error) {
	if cs == nil {
		return nil, fmt.Errorf("k8s.WaitForCompletion: nil clientset")
	}
	if namespace == "" {
		return nil, fmt.Errorf("k8s.WaitForCompletion: empty namespace")
	}
	if name == "" {
		return nil, fmt.Errorf("k8s.WaitForCompletion: empty name")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("k8s.WaitForCompletion: non-positive interval %v", interval)
	}

	pods := cs.CoreV1().Pods(namespace)

	// Initial fetch covers the case where the pod is already terminal.
	pod, err := pods.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.WaitForCompletion: get %s/%s: %w", namespace, name, err)
	}
	if isTerminalPhase(pod.Status.Phase) {
		return pod, nil
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("k8s.WaitForCompletion: %s/%s: %w", namespace, name, ctx.Err())
		case <-ticker.C:
			pod, err := pods.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("k8s.WaitForCompletion: get %s/%s: %w", namespace, name, err)
			}
			if isTerminalPhase(pod.Status.Phase) {
				return pod, nil
			}
		}
	}
}

func isTerminalPhase(p corev1.PodPhase) bool {
	return p == corev1.PodSucceeded || p == corev1.PodFailed
}

// StreamLogs opens a follow=true log stream against the named pod's first
// container and returns the underlying io.ReadCloser. The caller MUST Close
// the returned reader; spawn will typically wrap it in a bufio.Scanner and
// close it from a defer.
//
// The function intentionally does not buffer or scan lines itself — that
// belongs to the consumer (internal/spawn parses the final JSON line). This
// keeps the k8s package free of evolve-layer concerns and lets the same
// helper serve doctor / debug commands that just tee logs to stdout.
//
// If containerName is empty the server-side default (first container) is
// used. Errors from the request are wrapped with the pod identity so log
// failures are obviously distinct from create/delete failures upstream.
func StreamLogs(ctx context.Context, cs kubernetes.Interface, namespace, name, containerName string) (io.ReadCloser, error) {
	if cs == nil {
		return nil, fmt.Errorf("k8s.StreamLogs: nil clientset")
	}
	if namespace == "" {
		return nil, fmt.Errorf("k8s.StreamLogs: empty namespace")
	}
	if name == "" {
		return nil, fmt.Errorf("k8s.StreamLogs: empty name")
	}
	opts := &corev1.PodLogOptions{Follow: true}
	if containerName != "" {
		opts.Container = containerName
	}
	req := cs.CoreV1().Pods(namespace).GetLogs(name, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("k8s.StreamLogs: stream %s/%s: %w", namespace, name, err)
	}
	return rc, nil
}
