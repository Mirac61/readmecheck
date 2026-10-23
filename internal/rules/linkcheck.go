package rules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

const (
	DefaultLinkWorkers   = 8
	DefaultLinkTimeout   = 5 * time.Second
	DefaultMaxLinkChecks = 50
)

type LinkCheck struct {
	URL        string
	OK         bool
	StatusCode int
	Err        string
}

type LinkChecker interface {
	Check(ctx context.Context, urls []string) []LinkCheck
}

type HTTPLinkChecker struct {
	Client    *http.Client
	Workers   int
	Timeout   time.Duration
	MaxLinks  int
	UserAgent string
}

func NewHTTPLinkChecker() *HTTPLinkChecker {
	return &HTTPLinkChecker{
		Client:    newSafeHTTPClient(),
		Workers:   DefaultLinkWorkers,
		Timeout:   DefaultLinkTimeout,
		MaxLinks:  DefaultMaxLinkChecks,
		UserAgent: "readmecheck/1.0 (+https://github.com; link checker)",
	}
}

func newSafeHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = safeDialContext
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

func isBlockedIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// safeDialContext resolves the host itself and refuses private addresses, so
// a crafted README link cannot reach loopback services or the LAN.
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, address := range addresses {
		if isBlockedIP(address.IP) {
			lastErr = fmt.Errorf("link checker refuses private address %s", address.IP)
			continue
		}
		dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable address for %s", host)
	}
	return nil, lastErr
}

func (c *HTTPLinkChecker) Check(ctx context.Context, urls []string) []LinkCheck {
	urls = uniqueStrings(urls)
	if c.MaxLinks > 0 && len(urls) > c.MaxLinks {
		urls = urls[:c.MaxLinks]
	}
	if len(urls) == 0 {
		return nil
	}

	workers := c.Workers
	if workers <= 0 {
		workers = DefaultLinkWorkers
	}
	if workers > len(urls) {
		workers = len(urls)
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	results := make([]LinkCheck, len(urls))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					results[index] = LinkCheck{URL: urls[index], Err: ctx.Err().Error()}
					continue
				}
				results[index] = c.checkOne(ctx, client, urls[index])
			}
		}()
	}
	for index := range urls {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func (c *HTTPLinkChecker) checkOne(ctx context.Context, client *http.Client, target string) LinkCheck {
	if _, err := url.ParseRequestURI(target); err != nil {
		return LinkCheck{URL: target, Err: "invalid URL"}
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultLinkTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	check := func(method string) LinkCheck {
		req, err := http.NewRequestWithContext(requestCtx, method, target, nil)
		if err != nil {
			return LinkCheck{URL: target, Err: "invalid URL"}
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		resp, err := client.Do(req)
		if err != nil {
			return LinkCheck{URL: target, Err: cleanLinkError(err, requestCtx)}
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return LinkCheck{
			URL:        target,
			OK:         reachableStatus(resp.StatusCode),
			StatusCode: resp.StatusCode,
		}
	}

	result := check(http.MethodHead)
	// Some hosts reject HEAD, and bot protection often returns 400/404/500 for
	// HEAD while serving the page to GET. Retry any unsuccessful HEAD once.
	if result.Err == "" && !result.OK && !isIndeterminateStatus(result.StatusCode) {
		result = check(http.MethodGet)
	}
	return result
}

func isIndeterminateStatus(status int) bool {
	// These codes already prove the host answered; treating them as reachable
	// is intentional and a GET retry would not add information.
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func reachableStatus(status int) bool {
	if status >= 200 && status < 400 {
		return true
	}
	// Authentication walls, bot protection, and rate limits prove the host
	// answered; they are not evidence that a link is dead.
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func cleanLinkError(err error, ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return "timed out"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (l LinkCheck) String() string {
	if l.Err != "" {
		return fmt.Sprintf("%s: %s", l.URL, l.Err)
	}
	if l.StatusCode != 0 {
		return fmt.Sprintf("%s: HTTP %d", l.URL, l.StatusCode)
	}
	return l.URL
}
