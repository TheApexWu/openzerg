package k8s

import (
	"context"
	"testing"

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
