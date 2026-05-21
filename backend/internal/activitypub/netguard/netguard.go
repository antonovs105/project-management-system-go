package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// maxRedirects limits remote fetch redirects before the client rejects the request.
const maxRedirects = 5

var (
	// ErrUnsafeURL reports a remote URL that could target private or unsafe network ranges.
	ErrUnsafeURL = errors.New("unsafe remote url")
	// ErrTooManyRedirects reports that a remote fetch exceeded the redirect limit.
	ErrTooManyRedirects = errors.New("too many remote redirects")
)

// URLPolicy configures outbound federation URL validation.
type URLPolicy struct {
	requireHTTPS bool
}

// URLPolicyOption changes outbound federation URL validation behavior.
type URLPolicyOption func(*URLPolicy)

// RequireHTTPS rejects plain HTTP URLs and HTTP redirects.
func RequireHTTPS() URLPolicyOption {
	return func(policy *URLPolicy) {
		policy.requireHTTPS = true
	}
}

// blockedPrefixes lists globally routed ranges that are still unsafe federation targets.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// NewHTTPClient creates an HTTP client that blocks unsafe federation destinations.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return NewHTTPClientWithPolicy(timeout)
}

// NewHTTPClientWithPolicy creates an HTTP client with explicit URL validation policy.
func NewHTTPClientWithPolicy(timeout time.Duration, opts ...URLPolicyOption) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	policy := urlPolicy(opts...)

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialContext(ctx, dialer, network, address)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return ErrTooManyRedirects
			}
			return ValidateURL(req.URL, policy.options()...)
		},
	}
}

// ValidateRemoteURL parses and validates an outbound federation URL.
func ValidateRemoteURL(raw string, opts ...URLPolicyOption) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid url", ErrUnsafeURL)
	}
	if err := ValidateURL(parsed, opts...); err != nil {
		return nil, err
	}
	return parsed, nil
}

// ValidateURL validates an already parsed outbound federation URL.
func ValidateURL(parsed *url.URL, opts ...URLPolicyOption) error {
	policy := urlPolicy(opts...)
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: absolute http url required", ErrUnsafeURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrUnsafeURL, parsed.Scheme)
	}
	if policy.requireHTTPS && scheme != "https" {
		return fmt.Errorf("%w: https required", ErrUnsafeURL)
	}
	return ValidateHostname(parsed.Hostname())
}

// urlPolicy applies URL policy options to their default values.
func urlPolicy(opts ...URLPolicyOption) URLPolicy {
	var policy URLPolicy
	for _, opt := range opts {
		if opt != nil {
			opt(&policy)
		}
	}
	return policy
}

// options converts a URL policy back into option functions.
func (p URLPolicy) options() []URLPolicyOption {
	if p.requireHTTPS {
		return []URLPolicyOption{RequireHTTPS()}
	}
	return nil
}

// ValidateHostname rejects hostnames or IP literals unsafe for outbound federation.
func ValidateHostname(hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return fmt.Errorf("%w: host required", ErrUnsafeURL)
	}
	if strings.Contains(hostname, "%") {
		return fmt.Errorf("%w: scoped host is not allowed", ErrUnsafeURL)
	}

	addr, err := netip.ParseAddr(hostname)
	if err == nil {
		return validateAddr(addr)
	}
	return nil
}

// dialContext resolves and validates a network destination before opening a socket.
func dialContext(ctx context.Context, dialer *net.Dialer, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err := ValidateHostname(host); err != nil {
		return nil, err
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
	}

	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("%w: host did not resolve", ErrUnsafeURL)
	}

	var selected netip.Addr
	for _, item := range resolved {
		addr, ok := netip.AddrFromSlice(item.IP)
		if !ok {
			return nil, fmt.Errorf("%w: invalid resolved address", ErrUnsafeURL)
		}
		addr = addr.Unmap()
		if err := validateAddr(addr); err != nil {
			return nil, err
		}
		if !selected.IsValid() {
			selected = addr
		}
	}

	return dialer.DialContext(ctx, network, net.JoinHostPort(selected.String(), port))
}

// validateAddr rejects loopback, private, multicast, and reserved addresses.
func validateAddr(addr netip.Addr) error {
	addr = addr.Unmap()
	if !addr.IsValid() ||
		!addr.IsGlobalUnicast() ||
		addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		isBlockedPrefix(addr) {
		return fmt.Errorf("%w: blocked address %s", ErrUnsafeURL, addr.String())
	}
	return nil
}

// isBlockedPrefix reports whether an address is in a reserved documentation range.
func isBlockedPrefix(addr netip.Addr) bool {
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
