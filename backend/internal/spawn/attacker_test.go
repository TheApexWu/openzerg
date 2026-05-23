package spawn

import (
	"strings"
	"testing"
)

func TestBuildAttackerPodRequiresName(t *testing.T) {
	_, err := BuildAttackerPod(AttackerPodOptions{
		TargetURL: "https://x", Genome: map[string]any{"v": 1}, RateLimitRPS: 10,
	})
	if err == nil {
		t.Fatal("expected error when Name is empty")
	}
}

func TestBuildAttackerPodInjectsGenomeAndEnvFromSecret(t *testing.T) {
	pod, err := BuildAttackerPod(AttackerPodOptions{
		Name:         "openzerg-attacker-r1-p0",
		TargetURL:    "https://juice-shop-production-d0c5.up.railway.app",
		Genome:       map[string]any{"vector": "sqli_login", "category": "injection"},
		RunID:        "r1",
		PodID:        "r1-p0",
		Generation:   1,
		RateLimitRPS: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pod.Spec.Containers[0].Image != DefaultAttackerImage {
		t.Fatalf("image=%q want default", pod.Spec.Containers[0].Image)
	}
	if len(pod.Spec.ImagePullSecrets) != 1 || pod.Spec.ImagePullSecrets[0].Name != DefaultImagePullSecretName {
		t.Fatalf("imagePullSecrets=%v want %q", pod.Spec.ImagePullSecrets, DefaultImagePullSecretName)
	}
	if len(pod.Spec.Containers[0].EnvFrom) != 1 ||
		pod.Spec.Containers[0].EnvFrom[0].SecretRef == nil ||
		pod.Spec.Containers[0].EnvFrom[0].SecretRef.Name != DefaultKeysSecretName {
		t.Fatalf("envFrom secret should be %q, got %+v", DefaultKeysSecretName, pod.Spec.Containers[0].EnvFrom)
	}
	gotEnv := map[string]string{}
	for _, e := range pod.Spec.Containers[0].Env {
		gotEnv[e.Name] = e.Value
	}
	if !strings.Contains(gotEnv["GENOME"], `"vector":"sqli_login"`) {
		t.Fatalf("GENOME env did not marshal genome: %q", gotEnv["GENOME"])
	}
	if gotEnv["TARGET_URL"] == "" {
		t.Fatal("TARGET_URL env missing")
	}
	if gotEnv["RATE_LIMIT_RPS"] != "10" {
		t.Fatalf("RATE_LIMIT_RPS=%q want 10", gotEnv["RATE_LIMIT_RPS"])
	}
}

func TestBuildAttackerPodOmitsImagePullSecretWhenDash(t *testing.T) {
	pod, err := BuildAttackerPod(AttackerPodOptions{
		Name:                "p",
		TargetURL:           "https://x",
		Genome:              map[string]any{"v": 1},
		RateLimitRPS:        10,
		ImagePullSecretName: "-",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Spec.ImagePullSecrets) != 0 {
		t.Fatalf("expected no imagePullSecrets, got %v", pod.Spec.ImagePullSecrets)
	}
}
