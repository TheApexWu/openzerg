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
