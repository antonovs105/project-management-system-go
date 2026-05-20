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

const maxRedirects = 5

var (
	ErrUnsafeURL        = errors.New("unsafe remote url")
	ErrTooManyRedirects = errors.New("too many remote redirects")
)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

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
			return ValidateURL(req.URL)
		},
	}
}

func ValidateRemoteURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid url", ErrUnsafeURL)
	}
	if err := ValidateURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func ValidateURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: absolute http url required", ErrUnsafeURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrUnsafeURL, parsed.Scheme)
	}
	return ValidateHostname(parsed.Hostname())
}

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

func isBlockedPrefix(addr netip.Addr) bool {
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
