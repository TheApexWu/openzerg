package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeKubeconfigMissingFile(t *testing.T) {
	st := ProbeKubeconfig(filepath.Join(t.TempDir(), "no-such"))
	if st.Exists {
		t.Fatalf("expected Exists=false")
	}
	if st.ParseError != nil {
		t.Fatalf("missing file should not be a parse error, got %v", st.ParseError)
	}
	if st.CurrentContext != "" {
		t.Fatalf("expected empty CurrentContext, got %q", st.CurrentContext)
	}
}

func TestProbeKubeconfigExtractsCurrentContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "" +
		"apiVersion: v1\n" +
		"kind: Config\n" +
		"current-context: do-nyc1-k8s-1-36-0-do-0-nyc1-1779544226353\n" +
		"contexts:\n" +
		"- name: other\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	st := ProbeKubeconfig(path)
	if !st.Exists {
		t.Fatalf("expected Exists=true")
	}
	want := "do-nyc1-k8s-1-36-0-do-0-nyc1-1779544226353"
	if st.CurrentContext != want {
		t.Fatalf("CurrentContext = %q, want %q", st.CurrentContext, want)
	}
}
