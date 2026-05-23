// Package k8s — client-go construction for the control plane.
//
// BuildClientset is the single entrypoint used by M2's pod orchestration code
// (CreatePod / StreamLogs / DeletePod) to talk to the live cluster. It honors
// both out-of-cluster and in-cluster modes per PRD.json's
// integrations.kubernetes.client contract:
//
//   - If kubeconfigPath is non-empty, load that file (out-of-cluster).
//   - Else if KUBECONFIG env is set, use it.
//   - Else fall back to in-cluster service account.
//
// The function is intentionally narrow: it returns a *kubernetes.Clientset and
// the resolved kubeconfig path (for diagnostics). Higher-level helpers in this
// package will consume the clientset; main.go does not need to touch
// client-go directly.
package k8s

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientResult bundles the constructed clientset with the resolved
// configuration path so callers (and `openzerg doctor`) can report what
// was actually used.
type ClientResult struct {
	Clientset      *kubernetes.Clientset
	KubeconfigPath string // empty when in-cluster mode was used
	InCluster      bool
}

// BuildClientset constructs a Kubernetes clientset.
//
// Resolution order:
//  1. explicit kubeconfigPath argument (when non-empty)
//  2. $KUBECONFIG env var
//  3. in-cluster config (service-account token + apiserver URL from env)
//
// On success the *rest.Config is configured with sane timeouts so M2's
// streaming log calls don't hang the control plane on a partitioned node.
func BuildClientset(kubeconfigPath string) (*ClientResult, error) {
	cfg, resolvedPath, inCluster, err := buildRestConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: NewForConfig: %w", err)
	}
	return &ClientResult{
		Clientset:      cs,
		KubeconfigPath: resolvedPath,
		InCluster:      inCluster,
	}, nil
}

// buildRestConfig is the resolution-only half of BuildClientset, factored out
// so it can be unit-tested without contacting any apiserver.
func buildRestConfig(kubeconfigPath string) (*rest.Config, string, bool, error) {
	// 1. explicit path
	if kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, kubeconfigPath, false, fmt.Errorf("k8s: load kubeconfig %q: %w", kubeconfigPath, err)
		}
		return cfg, kubeconfigPath, false, nil
	}
	// 2. KUBECONFIG env
	if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", envPath)
		if err != nil {
			return nil, envPath, false, fmt.Errorf("k8s: load $KUBECONFIG %q: %w", envPath, err)
		}
		return cfg, envPath, false, nil
	}
	// 3. in-cluster
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", false, fmt.Errorf("k8s: no kubeconfig and not in-cluster: %w", err)
	}
	return cfg, "", true, nil
}
