package domainblock

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrInvalidActorID reports an ActivityPub actor ID without a parseable host.
var ErrInvalidActorID = errors.New("invalid actor id")

// Normalize canonicalizes a domain, host:port, or URL for block matching.
func Normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		value = parsed.Hostname()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "[]")
	if value == "" || strings.ContainsAny(value, " \t\r\n/") {
		return ""
	}
	return value
}

// FromActorID extracts and normalizes the host from an ActivityPub actor ID.
func FromActorID(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", ErrInvalidActorID
	}
	return Normalize(parsed.Hostname()), nil
}

// Candidates returns the domain and parent domains that could match a block.
func Candidates(domain string) []string {
	domain = Normalize(domain)
	if domain == "" {
		return nil
	}
	candidates := []string{domain}
	for {
		dot := strings.IndexByte(domain, '.')
		if dot < 0 {
			return candidates
		}
		domain = domain[dot+1:]
		candidates = append(candidates, domain)
	}
}

// Contains reports whether domain or one of its parents is blocked.
func Contains(blocked map[string]struct{}, domain string) bool {
	if len(blocked) == 0 {
		return false
	}
	for _, candidate := range Candidates(domain) {
		if _, ok := blocked[candidate]; ok {
			return true
		}
	}
	return false
}
