// Package fetch retrieves subscription content from remote URLs with
// protections against server-side request forgery (SSRF).
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent is sent when the caller does not supply one.
const DefaultUserAgent = "SubFlow/0.1"

// Defaults for outbound requests.
const (
	DefaultTimeout  = 20 * time.Second
	DefaultMaxBytes = 8 << 20 // 8 MiB
	maxRedirects    = 5
)

// Options tunes a Fetcher.
type Options struct {
	// UserAgent overrides DefaultUserAgent when non-empty.
	UserAgent string
	// Timeout bounds the whole request. Defaults to DefaultTimeout.
	Timeout time.Duration
	// MaxBytes caps the response body size. Defaults to DefaultMaxBytes.
	MaxBytes int64
	// AllowPrivateHosts permits requests to private/loopback addresses.
	// It defaults to false; enable it only for self-hosted endpoints.
	AllowPrivateHosts bool
}

// Result is the outcome of a successful fetch.
type Result struct {
	Content     string `json:"content"`
	FinalURL    string `json:"finalUrl"`
	StatusCode  int    `json:"statusCode"`
	ContentType string `json:"contentType"`
	Bytes       int    `json:"bytes"`
}

// Fetcher performs SSRF-hardened HTTP GETs.
type Fetcher struct {
	client            *http.Client
	userAgent         string
	allowPrivateHosts bool
	maxBytes          int64
	cacheMu           sync.Mutex
	cache             map[string]cacheEntry
	inflight          map[string]*inflightFetch
}

// New builds a Fetcher from opts.
func New(opts Options) *Fetcher {
	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	f := &Fetcher{
		userAgent:         userAgent,
		allowPrivateHosts: opts.AllowPrivateHosts,
		maxBytes:          maxBytes,
		cache:             map[string]cacheEntry{},
	}

	// Per-address dial timeout. Several addresses may be tried for one host,
	// so keep each attempt short and let the overall request timeout bound the
	// total (see Options.Timeout).
	dialer := &net.Dialer{Timeout: 6 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil, // never honour ambient proxy settings
		DisableKeepAlives:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		// Reject connections to internal addresses at dial time. This also
		// defeats DNS rebinding because the check runs on the resolved IP
		// that is actually dialled. All public addresses returned by the
		// resolver are tried concurrently so a host whose first address is
		// unreachable (for example a blocked IPv4 behind a CDN) can still be
		// reached over another address family.
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", address, err)
			}
			ips, err := resolveAllSafe(ctx, host, f.allowPrivateHosts)
			if err != nil {
				return nil, err
			}
			return dialAny(ctx, dialer, ips, port)
		},
	}

	f.client = &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("stopped after too many redirects")
			}
			return validateURL(req.URL, f.allowPrivateHosts)
		},
	}
	return f
}

// Get fetches rawURL and returns its body. A non-empty userAgent overrides the
// fetcher default for this request only.
func (f *Fetcher) Get(ctx context.Context, rawURL, userAgent string) (Result, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Result{}, fmt.Errorf("invalid url: %w", err)
	}
	if err := validateURL(target, f.allowPrivateHosts); err != nil {
		return Result{}, err
	}
	if userAgent == "" {
		userAgent = f.userAgent
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, f.maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return Result{}, fmt.Errorf("response exceeds %d bytes", f.maxBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	finalURL := target.String()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return Result{
		Content:     string(body),
		FinalURL:    finalURL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Bytes:       len(body),
	}, nil
}

func validateURL(u *url.URL, allowPrivate bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q, only http and https are allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url is missing a host")
	}
	// A literal IP can be checked directly; a hostname is checked at dial time.
	if ip := net.ParseIP(host); ip != nil && !allowPrivate && isBlockedIP(ip) {
		return fmt.Errorf("refusing to connect to non-public address %s", ip)
	}
	return nil
}

// maxDialAddresses bounds how many addresses are dialled concurrently for a
// single host.
const maxDialAddresses = 4

// resolveAllSafe resolves host and returns the public addresses that are safe
// to dial, preserving resolver order and dropping duplicates.
func resolveAllSafe(ctx context.Context, host string, allowPrivate bool) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivate && isBlockedIP(ip) {
			return nil, fmt.Errorf("refusing to connect to non-public address %s", ip)
		}
		return []net.IP{ip}, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	seen := make(map[string]struct{}, len(addrs))
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if !allowPrivate && isBlockedIP(addr.IP) {
			continue
		}
		key := addr.IP.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ips = append(ips, addr.IP)
		if len(ips) >= maxDialAddresses {
			break
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("refusing to connect to non-public address for %s", host)
	}
	return ips, nil
}

// dialAny dials the given addresses concurrently and returns the first
// successful connection, cancelling the remaining attempts. It reports the
// first error when every attempt fails.
func dialAny(ctx context.Context, dialer *net.Dialer, ips []net.IP, port string) (net.Conn, error) {
	if len(ips) == 1 {
		return dialer.DialContext(ctx, "tcp", net.JoinHostPort(ips[0].String(), port))
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		conn net.Conn
		err  error
	}
	results := make(chan result, len(ips))
	for _, ip := range ips {
		go func(ip net.IP) {
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
			results <- result{conn: conn, err: err}
		}(ip)
	}

	var firstErr error
	for range ips {
		res := <-results
		if res.err == nil {
			return res.conn, nil
		}
		if firstErr == nil {
			firstErr = res.err
		}
	}
	return nil, firstErr
}

// isBlockedIP reports whether ip must not be reachable from the server.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return true
	}
	// Carrier-grade NAT (100.64.0.0/10) and IPv6 unique-local (fc00::/7) are
	// not covered by IsPrivate on every Go release, so check them explicitly.
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
	}
	return ip[0]&0xfe == 0xfc
}
