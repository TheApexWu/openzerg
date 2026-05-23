// Package spawn is the high-level orchestrator that turns a Genome into a
// concrete k8s Pod, launches it, awaits completion, and parses the final
// result JSON line from stdout.
//
// M2 ships only the manifest-rendering step. Real cluster operations
// (CreatePod / StreamLogs / DeletePod) are wired through internal/k8s in a
// later increment.
package spawn

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// BusyboxImage is the placeholder image used in M2 to validate the
// pod-spawn / log-stream / parse pipeline before the real PI attacker image
// (M3) is available.
const BusyboxImage = "busybox:1.36"

// PodOptions parameterizes BuildBusyboxPod. All fields are required except
// Labels; if Labels is nil a default set is applied.
type PodOptions struct {
	// Name is the pod's metadata.name. Must be DNS-1123-compliant; the
	// caller is expected to keep it short and unique per generation.
	Name string
	// Namespace places the pod. Defaults to "openzerg" when empty.
	Namespace string
	// FinalJSON is the JSON object the pod should print as its last
	// stdout line. The control plane scans pod stdout from the end and
	// parses this line as the attacker_result_jsonl payload (see PRD).
	FinalJSON string
	// Labels are merged onto the pod. A "openzerg/role: attacker-stub"
	// label is always added so kubectl get pods -l openzerg/role=...
	// matches our test pods.
	Labels map[string]string
}

// BuildBusyboxPod renders an M2 placeholder Pod that prints two log lines
// and then a single JSON result line, then exits 0. The final line is the
// payload that internal/evolve.ParseLastJSONLine is expected to extract.
//
// The pod spec follows the safety knobs called out in PRD.json's
// kubernetes.pod_spec_essentials block:
//   - restartPolicy=Never (one-shot)
//   - activeDeadlineSeconds=120 (no runaway pods)
//   - automountServiceAccountToken=false (pods need no API access)
//   - non-root securityContext + no privilege escalation
//   - tight CPU/memory limits
func BuildBusyboxPod(opts PodOptions) (*corev1.Pod, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("spawn: PodOptions.Name is required")
	}
	if opts.FinalJSON == "" {
		return nil, fmt.Errorf("spawn: PodOptions.FinalJSON is required")
	}
	ns := opts.Namespace
	if ns == "" {
		ns = "openzerg"
	}

	labels := map[string]string{"openzerg/role": "attacker-stub"}
	for k, v := range opts.Labels {
		labels[k] = v
	}

	// The payload is single-quoted into the shell; protect against any
	// embedded single quotes in the caller-supplied JSON by closing,
	// escaping, and reopening the quote (standard sh trick).
	safeJSON := shellSingleQuote(opts.FinalJSON)
	cmd := fmt.Sprintf("echo log1; echo log2; sleep 1; echo %s", safeJSON)

	deadline := int64(120)
	automount := false
	nonRoot := true
	allowEscalation := false

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.Name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:        &deadline,
			AutomountServiceAccountToken: &automount,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &nonRoot,
			},
			Containers: []corev1.Container{{
				Name:    "attacker",
				Image:   BusyboxImage,
				Command: []string{"sh", "-c", cmd},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &allowEscalation,
					RunAsNonRoot:             &nonRoot,
				},
			}},
		},
	}, nil
}

// shellSingleQuote wraps s in POSIX single quotes, escaping any embedded
// single quote with the canonical '\'' dance. The result is safe to splice
// into a `sh -c` command.
func shellSingleQuote(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '\'')
	return string(out)
}
