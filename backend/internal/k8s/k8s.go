// Package k8s wraps client-go for the control plane: pod create, log stream,
// wait-for-completion, and delete.
//
// M1 ships only the read-only probe used by `openzerg doctor`: parse the
// kubeconfig file and report file existence and the current-context value.
// Real client construction (CreatePod / StreamLogs / DeletePod) lands in M2,
// at which point this package will pull in k8s.io/client-go.
package k8s

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// KubeconfigStatus is the machine-friendly result of a doctor probe.
type KubeconfigStatus struct {
	Path           string
	Exists         bool
	CurrentContext string
	// ParseError is non-nil when the file exists but we could not extract
	// a current-context line. It is intentionally non-fatal so doctor can
	// still print something useful.
	ParseError error
}

// ProbeKubeconfig opens path (no error if absent) and extracts the
// `current-context:` field with a small line scan. We deliberately avoid
// pulling in a YAML parser or client-go here — doctor needs to be cheap and
// safe even on a malformed kubeconfig.
func ProbeKubeconfig(path string) KubeconfigStatus {
	st := KubeconfigStatus{Path: path}
	if path == "" {
		return st
	}
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			st.ParseError = err
		}
		return st
	}
	defer f.Close()
	st.Exists = true

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		// We want a top-level key at column 0; ignore indented occurrences
		// (e.g. inside a context entry's name field).
		if !strings.HasPrefix(line, "current-context:") {
			continue
		}
		_ = trim // avoid unused-var if logic shifts later
		val := strings.TrimSpace(strings.TrimPrefix(line, "current-context:"))
		val = strings.Trim(val, "\"'")
		st.CurrentContext = val
		break
	}
	if err := sc.Err(); err != nil {
		st.ParseError = err
	}
	return st
}
