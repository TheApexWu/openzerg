package config

import (
    "os"
    "testing"
)

func TestResolveKubeconfigPath_EnvSet(t *testing.T) {
    const envPath = "/tmp/custom/kubeconfig"
    // Save original env
    orig := os.Getenv("KUBECONFIG")
    defer func() { os.Setenv("KUBECONFIG", orig) }()
    os.Setenv("KUBECONFIG", envPath)
    got := ResolveKubeconfigPath()
    if got != envPath {
        t.Fatalf("expected %s, got %s", envPath, got)
    }
}

func TestResolveKubeconfigPath_NoEnv(t *testing.T) {
    // Save and clear env
    orig := os.Getenv("KUBECONFIG")
    defer func() { os.Setenv("KUBECONFIG", orig) }()
    os.Unsetenv("KUBECONFIG")
    // Expect default path under home
    home, err := os.UserHomeDir()
    if err != nil {
        t.Fatalf("UserHomeDir error: %v", err)
    }
    expected := home + "/.kube/config"
    got := ResolveKubeconfigPath()
    if got != expected {
        t.Fatalf("expected %s, got %s", expected, got)
    }
}
