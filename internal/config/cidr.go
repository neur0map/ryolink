package config

import (
	"fmt"
	"net"
)

// validateCIDR accepts a CIDR prefix or a bare IPv4/IPv6 address (shorthand
// for a /32 or /128) and returns the normalized CIDR form.
func validateCIDR(s string) error {
	if s == "" {
		return fmt.Errorf("ryolink.yaml: empty CIDR entry")
	}
	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}
	if net.ParseIP(s) != nil {
		return nil
	}
	return fmt.Errorf("ryolink.yaml: %q is not a valid IP or CIDR", s)
}
