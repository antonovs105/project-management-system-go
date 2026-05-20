package domainblock

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var ErrInvalidActorID = errors.New("invalid actor id")

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

func FromActorID(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", ErrInvalidActorID
	}
	return Normalize(parsed.Hostname()), nil
}

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
