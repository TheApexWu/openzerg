package spawn

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildBusyboxPodRequiresName(t *testing.T) {
	if _, err := BuildBusyboxPod(PodOptions{FinalJSON: `{"x":1}`}); err == nil {
		t.Fatal("expected error when Name is empty")
	}
}

func TestBuildBusyboxPodRequiresFinalJSON(t *testing.T) {
	if _, err := BuildBusyboxPod(PodOptions{Name: "p"}); err == nil {
		t.Fatal("expected error when FinalJSON is empty")
	}
}

func TestBuildBusyboxPodRendersExpectedSpec(t *testing.T) {
	p, err := BuildBusyboxPod(PodOptions{
		Name:      "attacker-g0-0",
		Namespace: "openzerg",
		FinalJSON: `{"type":"result","fitness":0.0}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "attacker-g0-0" {
		t.Fatalf("Name=%q", p.Name)
	}
	if p.Namespace != "openzerg" {
		t.Fatalf("Namespace=%q", p.Namespace)
	}
	if p.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("RestartPolicy=%v", p.Spec.RestartPolicy)
	}
	if p.Spec.ActiveDeadlineSeconds == nil || *p.Spec.ActiveDeadlineSeconds != 120 {
		t.Fatalf("ActiveDeadlineSeconds: want 120, got %v", p.Spec.ActiveDeadlineSeconds)
	}
	if p.Spec.AutomountServiceAccountToken == nil || *p.Spec.AutomountServiceAccountToken {
		t.Fatalf("AutomountServiceAccountToken should be false")
	}
	if got := p.Labels["openzerg/role"]; got != "attacker-stub" {
		t.Fatalf("missing role label, got %q", got)
	}
	if len(p.Spec.Containers) != 1 {
		t.Fatalf("want 1 container, got %d", len(p.Spec.Containers))
	}
	c := p.Spec.Containers[0]
	if c.Image != BusyboxImage {
		t.Fatalf("image=%q want %q", c.Image, BusyboxImage)
	}
	if len(c.Command) < 3 || c.Command[0] != "sh" || c.Command[1] != "-c" {
		t.Fatalf("command shape unexpected: %v", c.Command)
	}
	if !strings.Contains(c.Command[2], `{"type":"result","fitness":0.0}`) {
		t.Fatalf("final JSON not embedded in command: %q", c.Command[2])
	}
	if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil ||
		*c.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("AllowPrivilegeEscalation must be false")
	}
}

func TestBuildBusyboxPodDefaultNamespace(t *testing.T) {
	p, err := BuildBusyboxPod(PodOptions{
		Name:      "p",
		FinalJSON: `{"a":1}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Namespace != "openzerg" {
		t.Fatalf("default namespace want openzerg, got %q", p.Namespace)
	}
}

func TestShellSingleQuoteEscapesQuotes(t *testing.T) {
	got := shellSingleQuote(`it's a "test"`)
	want := `'it'\''s a "test"'`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
