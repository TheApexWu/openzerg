package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestPod(ns, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "c",
				Image:   "busybox:1.36",
				Command: []string{"sh", "-c", "echo hi"},
			}},
		},
	}
}

func TestCreatePodHappyPath(t *testing.T) {
	cs := fake.NewSimpleClientset()
	pod := newTestPod("openzerg", "attacker-1")
	got, err := CreatePod(context.Background(), cs, pod)
	if err != nil {
		t.Fatalf("CreatePod: %v", err)
	}
	if got.Name != "attacker-1" || got.Namespace != "openzerg" {
		t.Fatalf("returned pod = %s/%s, want openzerg/attacker-1", got.Namespace, got.Name)
	}
	// Ensure the fake actually stored it.
	fetched, err := cs.CoreV1().Pods("openzerg").Get(context.Background(), "attacker-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("post-create Get: %v", err)
	}
	if fetched.Spec.Containers[0].Image != "busybox:1.36" {
		t.Fatalf("stored pod has unexpected image %q", fetched.Spec.Containers[0].Image)
	}
}

func TestCreatePodValidatesArgs(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if _, err := CreatePod(context.Background(), nil, newTestPod("openzerg", "x")); err == nil {
		t.Fatal("expected error for nil clientset")
	}
	if _, err := CreatePod(context.Background(), cs, nil); err == nil {
		t.Fatal("expected error for nil pod")
	}
	if _, err := CreatePod(context.Background(), cs, newTestPod("", "x")); err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if _, err := CreatePod(context.Background(), cs, newTestPod("openzerg", "")); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreatePodDuplicateRejected(t *testing.T) {
	cs := fake.NewSimpleClientset()
	pod := newTestPod("openzerg", "dup")
	if _, err := CreatePod(context.Background(), cs, pod); err != nil {
		t.Fatalf("first CreatePod: %v", err)
	}
	if _, err := CreatePod(context.Background(), cs, pod); err == nil {
		t.Fatal("expected AlreadyExists error on second create")
	}
}

func TestDeletePodHappyPath(t *testing.T) {
	cs := fake.NewSimpleClientset()
	pod := newTestPod("openzerg", "kill-me")
	if _, err := CreatePod(context.Background(), cs, pod); err != nil {
		t.Fatalf("seed CreatePod: %v", err)
	}
	if err := DeletePod(context.Background(), cs, "openzerg", "kill-me"); err != nil {
		t.Fatalf("DeletePod: %v", err)
	}
	if _, err := cs.CoreV1().Pods("openzerg").Get(context.Background(), "kill-me", metav1.GetOptions{}); err == nil {
		t.Fatal("expected pod to be gone after DeletePod")
	}
}

func TestDeletePodNotFoundIsIdempotent(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if err := DeletePod(context.Background(), cs, "openzerg", "ghost"); err != nil {
		t.Fatalf("DeletePod on missing pod returned error: %v", err)
	}
}

func TestDeletePodValidatesArgs(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if err := DeletePod(context.Background(), nil, "openzerg", "x"); err == nil {
		t.Fatal("expected error for nil clientset")
	}
	if err := DeletePod(context.Background(), cs, "", "x"); err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if err := DeletePod(context.Background(), cs, "openzerg", ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestWaitForCompletionAlreadySucceeded(t *testing.T) {
	pod := newTestPod("openzerg", "done")
	pod.Status.Phase = corev1.PodSucceeded
	cs := fake.NewSimpleClientset(pod)

	got, err := WaitForCompletionWithInterval(context.Background(), cs, "openzerg", "done", 5*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForCompletion: %v", err)
	}
	if got.Status.Phase != corev1.PodSucceeded {
		t.Fatalf("phase = %q, want Succeeded", got.Status.Phase)
	}
}

func TestWaitForCompletionTransitionsThenFails(t *testing.T) {
	pod := newTestPod("openzerg", "flip")
	pod.Status.Phase = corev1.PodPending
	cs := fake.NewSimpleClientset(pod)

	// Background flip: after a few ticks, mark the pod Failed.
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(15 * time.Millisecond)
		p, err := cs.CoreV1().Pods("openzerg").Get(context.Background(), "flip", metav1.GetOptions{})
		if err != nil {
			t.Errorf("background get: %v", err)
			return
		}
		p.Status.Phase = corev1.PodFailed
		if _, err := cs.CoreV1().Pods("openzerg").UpdateStatus(context.Background(), p, metav1.UpdateOptions{}); err != nil {
			t.Errorf("background update: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got, err := WaitForCompletionWithInterval(ctx, cs, "openzerg", "flip", 5*time.Millisecond)
	<-done
	if err != nil {
		t.Fatalf("WaitForCompletion: %v", err)
	}
	if got.Status.Phase != corev1.PodFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
}

func TestWaitForCompletionContextCancelled(t *testing.T) {
	pod := newTestPod("openzerg", "stuck")
	pod.Status.Phase = corev1.PodRunning
	cs := fake.NewSimpleClientset(pod)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := WaitForCompletionWithInterval(ctx, cs, "openzerg", "stuck", 5*time.Millisecond); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestWaitForCompletionValidatesArgs(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if _, err := WaitForCompletionWithInterval(context.Background(), nil, "openzerg", "x", time.Millisecond); err == nil {
		t.Fatal("expected error for nil clientset")
	}
	if _, err := WaitForCompletionWithInterval(context.Background(), cs, "", "x", time.Millisecond); err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if _, err := WaitForCompletionWithInterval(context.Background(), cs, "openzerg", "", time.Millisecond); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := WaitForCompletionWithInterval(context.Background(), cs, "openzerg", "x", 0); err == nil {
		t.Fatal("expected error for non-positive interval")
	}
}
