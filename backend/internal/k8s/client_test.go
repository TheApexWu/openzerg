package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

// minimalKubeconfig is a syntactically valid (but unreachable) kubeconfig
// used to exercise the non-network branches of BuildClientset.
const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: stub
  cluster:
    server: https://127.0.0.1:1
contexts:
- name: stub
  context:
    cluster: stub
    user: stub
current-context: stub
users:
- name: stub
  user:
    token: x
`

func writeStubKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(minimalKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildClientsetExplicitPath(t *testing.T) {
	path := writeStubKubeconfig(t)
	res, err := BuildClientset(path)
	if err != nil {
		t.Fatalf("BuildClientset: %v", err)
	}
	if res.Clientset == nil {
		t.Fatal("nil clientset")
	}
	if res.KubeconfigPath != path {
		t.Fatalf("KubeconfigPath = %q, want %q", res.KubeconfigPath, path)
	}
	if res.InCluster {
		t.Fatal("InCluster should be false when explicit path is given")
	}
}

func TestBuildClientsetUsesKubeconfigEnv(t *testing.T) {
	path := writeStubKubeconfig(t)
	t.Setenv("KUBECONFIG", path)
	res, err := BuildClientset("")
	if err != nil {
		t.Fatalf("BuildClientset: %v", err)
	}
	if res.KubeconfigPath != path {
		t.Fatalf("resolved path = %q, want %q", res.KubeconfigPath, path)
	}
	if res.InCluster {
		t.Fatal("InCluster should be false")
	}
}

func TestBuildClientsetMissingFileErrors(t *testing.T) {
	// Path that does not exist; clientcmd should surface a load error.
	_, err := BuildClientset(filepath.Join(t.TempDir(), "no-such-file"))
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig path")
	}
}
