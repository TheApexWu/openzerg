// Attacker pod builder.
//
// BuildAttackerPod renders the real M3 attacker Pod spec: the pi-attacker
// image, the per-pod genome env, envFrom the openzerg-keys Secret, and the
// DigitalOcean Container Registry imagePullSecret. The shape mirrors
// BuildBusyboxPod (same safety knobs) but the command is the image
// entrypoint and the inputs flow in via env, not via a shell template.
package spawn

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultAttackerImage is the DO registry image the pi-attacker pod runs.
// Overridable via the --image / ATTACKER_IMAGE flag on the control plane.
const DefaultAttackerImage = "registry.digitalocean.com/openzerg/pi-attacker:latest"

// DefaultKeysSecretName is the k8s Secret the attacker pod reads its API
// keys from via envFrom. The secret is created out-of-band by the operator
// (see PRD: `kubectl create secret generic openzerg-keys ...`).
const DefaultKeysSecretName = "openzerg-keys"

// DefaultImagePullSecretName is the docker-registry Secret that lets the
// pod pull from DigitalOcean Container Registry. Created via
// `doctl registry kubernetes-manifest`.
const DefaultImagePullSecretName = "openzerg"

// AttackerNonRootUID matches the `node` user (uid 1000) baked into the
// pi-attacker base image (node:22-bookworm-slim). Pinning the numeric UID
// in the pod spec lets the RunAsNonRoot admission check pass even when the
// cluster has not introspected the image manifest yet.
const AttackerNonRootUID int64 = 1000

// AttackerPodOptions parameterizes BuildAttackerPod. Everything except
// ImagePullSecretName and KeysSecretName is required.
type AttackerPodOptions struct {
	// Name is the pod's metadata.name. Must be DNS-1123-compliant.
	Name string
	// Namespace places the pod. Defaults to "openzerg" when empty.
	Namespace string
	// Image is the container image to run. Defaults to DefaultAttackerImage
	// when empty.
	Image string
	// KeysSecretName is the Secret pulled in via envFrom. Defaults to
	// DefaultKeysSecretName ("openzerg-keys").
	KeysSecretName string
	// ImagePullSecretName is the docker-registry Secret used to pull the
	// image. Defaults to DefaultImagePullSecretName ("registry-openzerg").
	// Set to "-" to omit the imagePullSecrets list entirely (useful for
	// public images or when the cluster is registry-linked).
	ImagePullSecretName string
	// Genome is the attack genome for this pod. Marshalled to JSON and
	// injected as the GENOME env var the entrypoint script consumes.
	Genome any
	// RunID, PodID, Generation identify the pod within a run for the
	// result line's bookkeeping fields.
	RunID      string
	PodID      string
	Generation int
	// TargetURL is the URL the attacker probes. Required.
	TargetURL string
	// RateLimitRPS is the in-pod request-rate cap. Required.
	RateLimitRPS int
	// TimeoutSeconds is the HARD per-pod time budget. The entrypoint runs
	// pi under `timeout $TIMEOUT_SECONDS`, and we also set
	// activeDeadlineSeconds = TimeoutSeconds + 30 on the pod as a kubelet
	// backstop. 0 = unlimited. Going past this kills the model mid-call.
	TimeoutSeconds int
	// SoftTimeoutSeconds is the SOFT target the model aims for. The
	// attacker skill's time_check.sh helper returns WARN/EXPIRING as this
	// deadline approaches so the model wraps up gracefully and emits its
	// final JSON result line. Going past it is fine; nothing kills the
	// model. 0 = no soft pressure. Should be <= TimeoutSeconds.
	SoftTimeoutSeconds int
	// Labels are merged onto the pod. An "openzerg/role: attacker" label
	// is always added.
	Labels map[string]string
	// DisableNimble flips OPENZERG_DISABLE_NIMBLE=1 on the container so
	// the in-pod nimble_fetch.sh wrapper short-circuits and returns an
	// "ok:false" error to the model. Used by --disable-nimble.
	DisableNimble bool
}

// BuildAttackerPod renders the real attacker Pod spec.
//
// Required envs on the container (mirrors data_contracts.attacker_pod_env
// in PRD.json):
//   - TARGET_URL, GENOME, GENERATION, RUN_ID, POD_ID
//   - RATE_LIMIT_RPS, TIMEOUT_SECONDS, SOFT_TIMEOUT_SECONDS
//   - OPENROUTER_API_KEY, NIMBLE_API_KEY (from openzerg-keys Secret via envFrom)
func BuildAttackerPod(opts AttackerPodOptions) (*corev1.Pod, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("spawn: AttackerPodOptions.Name is required")
	}
	if opts.TargetURL == "" {
		return nil, fmt.Errorf("spawn: AttackerPodOptions.TargetURL is required")
	}
	if opts.Genome == nil {
		return nil, fmt.Errorf("spawn: AttackerPodOptions.Genome is required")
	}
	if opts.RateLimitRPS <= 0 {
		return nil, fmt.Errorf("spawn: AttackerPodOptions.RateLimitRPS must be positive")
	}

	genomeJSON, err := json.Marshal(opts.Genome)
	if err != nil {
		return nil, fmt.Errorf("spawn: marshal genome: %w", err)
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = "openzerg"
	}
	image := opts.Image
	if image == "" {
		image = DefaultAttackerImage
	}
	keysSecret := opts.KeysSecretName
	if keysSecret == "" {
		keysSecret = DefaultKeysSecretName
	}
	timeoutSeconds := opts.TimeoutSeconds
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}
	softTimeoutSeconds := opts.SoftTimeoutSeconds
	if softTimeoutSeconds < 0 {
		softTimeoutSeconds = 0
	}
	// A soft deadline beyond the hard deadline is nonsensical: the model
	// would never be told to wrap up before the kernel killed it. Clamp.
	if timeoutSeconds > 0 && softTimeoutSeconds > timeoutSeconds {
		softTimeoutSeconds = timeoutSeconds
	}

	labels := map[string]string{"openzerg/role": "attacker"}
	for k, v := range opts.Labels {
		labels[k] = v
	}

	automountToken := false
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	runAsUID := AttackerNonRootUID

	// When TimeoutSeconds is 0 the caller wants the agent to run until it
	// finishes, so leave ActiveDeadlineSeconds unset (no kubelet kill).
	// Otherwise set a kubelet hard-stop slightly above the hard budget so
	// the `timeout` wrapper in entrypoint.sh always wins the race.
	var activeDeadlinePtr *int64
	if timeoutSeconds > 0 {
		deadline := int64(timeoutSeconds + 30)
		activeDeadlinePtr = &deadline
	}

	containerEnv := []corev1.EnvVar{
		{Name: "TARGET_URL", Value: opts.TargetURL},
		{Name: "GENOME", Value: string(genomeJSON)},
		{Name: "GENERATION", Value: fmt.Sprintf("%d", opts.Generation)},
		{Name: "RUN_ID", Value: opts.RunID},
		{Name: "POD_ID", Value: opts.PodID},
		{Name: "RATE_LIMIT_RPS", Value: fmt.Sprintf("%d", opts.RateLimitRPS)},
		{Name: "TIMEOUT_SECONDS", Value: fmt.Sprintf("%d", timeoutSeconds)},
		{Name: "SOFT_TIMEOUT_SECONDS", Value: fmt.Sprintf("%d", softTimeoutSeconds)},
	}
	if opts.DisableNimble {
		containerEnv = append(containerEnv,
			corev1.EnvVar{Name: "OPENZERG_DISABLE_NIMBLE", Value: "1"})
	}

	containerEnvFrom := []corev1.EnvFromSource{{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: keysSecret},
		},
	}}

	var imagePullSecrets []corev1.LocalObjectReference
	pullSecretName := opts.ImagePullSecretName
	if pullSecretName == "" {
		pullSecretName = DefaultImagePullSecretName
	}
	if pullSecretName != "-" {
		imagePullSecrets = []corev1.LocalObjectReference{{Name: pullSecretName}}
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:        activeDeadlinePtr,
			AutomountServiceAccountToken: &automountToken,
			ImagePullSecrets:             imagePullSecrets,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRoot,
				RunAsUser:    &runAsUID,
				RunAsGroup:   &runAsUID,
			},
			Containers: []corev1.Container{{
				Name:    "attacker",
				Image:   image,
				Env:     containerEnv,
				EnvFrom: containerEnvFrom,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &allowPrivilegeEscalation,
					RunAsNonRoot:             &runAsNonRoot,
					RunAsUser:                &runAsUID,
				},
			}},
		},
	}, nil
}
