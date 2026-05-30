package config

import "testing"

func TestResolveTargetDNS_Valid(t *testing.T) {
    // example.com should resolve in most environments
    if err := ResolveTargetDNS("https://example.com"); err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
}

func TestResolveTargetDNS_Invalid(t *testing.T) {
    if err := ResolveTargetDNS("https://nonexistent.invalid"); err == nil {
        t.Fatalf("expected error for invalid host, got nil")
    }
}
