package spawn

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRunPodRejectsNilClientset(t *testing.T) {
	if _, err := RunPod(context.Background(), nil, &corev1.Pod{}); err == nil {
		t.Fatal("expected error for nil clientset")
	}
}

func TestRunPodRejectsNilPod(t *testing.T) {
	cs := fake.NewSimpleClientset()
	if _, err := RunPod(context.Background(), cs, nil); err == nil {
		t.Fatal("expected error for nil pod")
	}
}

func TestRunPodsRejectsNilClientset(t *testing.T) {
	if _, err := RunPods(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error for nil clientset")
	}
}

func TestRunPodsEmptyInputReturnsEmptySlice(t *testing.T) {
	cs := fake.NewSimpleClientset()
	out, err := RunPods(context.Background(), cs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty outcomes, got %d", len(out))
	}
}
