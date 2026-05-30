package config

import (
    "fmt"
    "net"
    "net/url"
)

// ResolveTargetDNS checks that the hostname in the target URL resolves via DNS.
func ResolveTargetDNS(target string) error {
    u, err := url.Parse(target)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    host := u.Hostname()
    if host == "" {
        return fmt.Errorf("no hostname in URL")
    }
    _, err = net.LookupHost(host)
    if err != nil {
        return fmt.Errorf("dns lookup failed for %s: %w", host, err)
    }
    return nil
}
