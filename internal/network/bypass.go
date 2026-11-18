package network

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type BypassMode string

const (
	ModeAuto      BypassMode = "auto"
	ModeForceDoH  BypassMode = "doh"
	ModeDirectIP  BypassMode = "direct"
	ModeSystemDNS BypassMode = "system"
)

type ConnectionMeta struct {
	ClientIP     string `json:"client_ip"`
	ASN          string `json:"asn"`
	Colo         string `json:"colo"`
	Country      string `json:"country"`
	City         string `json:"city"`
	TLSVersion   string `json:"tls_version"`
	BypassMethod string `json:"bypass_method"`
	EdgeIP       string `json:"edge_ip"`
}

func (m ConnectionMeta) MaskedIP() string {
	if m.ClientIP == "" {
		return "Unknown"
	}
	parts := strings.Split(m.ClientIP, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.xxx.xxx", parts[0], parts[1])
	}
	if strings.Contains(m.ClientIP, ":") {
		segs := strings.Split(m.ClientIP, ":")
		if len(segs) > 2 {
			return segs[0] + ":" + segs[1] + "::xxxx"
		}
	}
	return m.ClientIP
}

func (m ConnectionMeta) LocationSummary() string {
	var parts []string
	if m.Colo != "" {
		parts = append(parts, m.Colo+" Edge")
	}
	if m.City != "" && m.Country != "" {
		parts = append(parts, fmt.Sprintf("%s, %s", m.City, m.Country))
	} else if m.Country != "" {
		parts = append(parts, m.Country)
	}
	if len(parts) == 0 {
		return "Cloudflare Anycast Edge"
	}
	return strings.Join(parts, " • ")
}

// Verified Cloudflare Anycast edge IPs that serve speed.cloudflare.com directly over TLS/443
var fallbackCloudflareEdgeIPs = []string{
	"172.66.0.218",
	"162.159.140.220",
}

// Encrypted DoH endpoints queried directly by IP address (no local DNS lookup required)
var dohEndpoints = []struct {
	Name string
	URL  string
	Host string
}{
	{Name: "Cloudflare DoH (1.1.1.1)", URL: "https://1.1.1.1/dns-query", Host: "cloudflare-dns.com"},
	{Name: "Cloudflare DoH (1.0.0.1)", URL: "https://1.0.0.1/dns-query", Host: "cloudflare-dns.com"},
	{Name: "Google DoH (8.8.8.8)", URL: "https://8.8.8.8/resolve", Host: "dns.google"},
}

type dnsCacheEntry struct {
	ips       []string
	method    string
	expiresAt time.Time
}

var (
	resolverCacheMu sync.RWMutex
	resolverCache   = make(map[string]dnsCacheEntry)
)

type dohResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func isValidPublicIPv4(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4.IsLoopback() || ip4.IsUnspecified() || ip4.IsPrivate() || ip4.IsLinkLocalUnicast() {
		return false
	}
	return true
}

// ResolveHostResilient resolves a hostname using DoH, System DNS, and Anycast Edge fallback
// depending on the selected BypassMode, preventing DNS poisoning or UDP:53 blocks.
func ResolveHostResilient(ctx context.Context, host string, mode BypassMode) ([]string, string) {
	if net.ParseIP(host) != nil {
		return []string{host}, "Direct IP"
	}

	cacheKey := string(mode) + ":" + host
	resolverCacheMu.RLock()
	if entry, ok := resolverCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		resolverCacheMu.RUnlock()
		return entry.ips, entry.method
	}
	resolverCacheMu.RUnlock()

	cacheStore := func(ips []string, method string) ([]string, string) {
		resolverCacheMu.Lock()
		resolverCache[cacheKey] = dnsCacheEntry{
			ips:       ips,
			method:    method,
			expiresAt: time.Now().Add(5 * time.Minute),
		}
		resolverCacheMu.Unlock()
		return ips, method
	}

	if mode == ModeDirectIP && strings.EqualFold(host, "speed.cloudflare.com") {
		return cacheStore(fallbackCloudflareEdgeIPs, "Direct Anycast IP (DNS Bypassed)")
	}

	if mode == ModeSystemDNS {
		if ips := resolveViaSystem(ctx, host); len(ips) > 0 {
			return cacheStore(ips, "System DNS + TCP/443")
		}
	}

	if mode == ModeForceDoH || mode == ModeAuto {
		if ips, provider := resolveViaDoH(ctx, host); len(ips) > 0 {
			return cacheStore(ips, provider)
		}
	}

	if mode == ModeAuto {
		if ips := resolveViaSystem(ctx, host); len(ips) > 0 {
			return cacheStore(ips, "System DNS + TCP/443")
		}
	}

	if strings.EqualFold(host, "speed.cloudflare.com") {
		return cacheStore(fallbackCloudflareEdgeIPs, "Direct Anycast Edge IP (Fallback)")
	}

	return nil, "Resolution Failed"
}

func resolveViaDoH(ctx context.Context, host string) ([]string, string) {
	dialer := &net.Dialer{Timeout: 2500 * time.Millisecond}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   2800 * time.Millisecond,
	}

	for _, ep := range dohEndpoints {
		reqCtx, cancel := context.WithTimeout(ctx, 2200*time.Millisecond)
		reqURL := fmt.Sprintf("%s?name=%s&type=A", ep.URL, url.QueryEscape(host))
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, reqURL, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("Accept", "application/dns-json")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}

		var parsed dohResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		cancel()

		if decodeErr != nil || parsed.Status != 0 {
			continue
		}

		var validIPs []string
		for _, ans := range parsed.Answer {
			if ans.Type == 1 && isValidPublicIPv4(ans.Data) {
				validIPs = append(validIPs, ans.Data)
			}
		}
		if len(validIPs) > 0 {
			return validIPs, ep.Name
		}
	}

	return nil, ""
}

func resolveViaSystem(ctx context.Context, host string) []string {
	resCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIP(resCtx, "ip4", host)
	if err != nil {
		return nil
	}
	var valid []string
	for _, ip := range addrs {
		s := ip.String()
		if isValidPublicIPv4(s) {
			valid = append(valid, s)
		}
	}
	return valid
}

// NewResilientClient constructs an HTTP client optimized for high-throughput bandwidth testing
// and anti-censorship firewall traversal over TCP/443.
func NewResilientClient(mode BypassMode, timeout time.Duration) (*http.Client, func() (string, string)) {
	var (
		mu         sync.RWMutex
		lastMethod string
		lastIP     string
	)

	dialer := &net.Dialer{
		Timeout:   4 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   16,
		MaxConnsPerHost:       16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   4 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		WriteBufferSize:       64 * 1024,
		ReadBufferSize:        64 * 1024,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return dialer.DialContext(ctx, "tcp4", addr)
			}

			ips, method := ResolveHostResilient(ctx, host, mode)
			var candidates []string
			candidates = append(candidates, ips...)
			if strings.EqualFold(host, "speed.cloudflare.com") {
				for _, fb := range fallbackCloudflareEdgeIPs {
					found := false
					for _, c := range candidates {
						if c == fb {
							found = true
							break
						}
					}
					if !found {
						candidates = append(candidates, fb)
					}
				}
			}

			if len(candidates) == 0 {
				conn, err := dialer.DialContext(ctx, "tcp4", addr)
				if err == nil {
					if tcpConn, ok := conn.(*net.TCPConn); ok {
						_ = tcpConn.SetNoDelay(true)
					}
				}
				return conn, err
			}

			var lastErr error
			for _, ip := range candidates {
				target := net.JoinHostPort(ip, port)
				conn, err := dialer.DialContext(ctx, "tcp4", target)
				if err == nil {
					if tcpConn, ok := conn.(*net.TCPConn); ok {
						_ = tcpConn.SetNoDelay(true)
					}
					mu.Lock()
					lastMethod = method
					lastIP = ip
					mu.Unlock()
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	getInfo := func() (string, string) {
		mu.RLock()
		defer mu.RUnlock()
		return lastMethod, lastIP
	}

	return client, getInfo
}

// FetchConnectionMeta discovers edge PoP, ASN, client IP, and active bypass route.
func FetchConnectionMeta(ctx context.Context, mode BypassMode) ConnectionMeta {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, getInfo := NewResilientClient(mode, 5*time.Second)
	meta := ConnectionMeta{
		TLSVersion:   "TLS 1.3",
		BypassMethod: "DoH + TCP/443",
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://speed.cloudflare.com/__down?bytes=64", nil)
	if err == nil {
		req.Header.Set("User-Agent", "NetProbe-Diagnostic/2.0")
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			method, edgeIP := getInfo()
			if method != "" {
				meta.BypassMethod = method
			}
			meta.EdgeIP = edgeIP

			if ip := resp.Header.Get("cf-meta-ip"); ip != "" {
				meta.ClientIP = ip
			}
			if asn := resp.Header.Get("asn"); asn != "" {
				meta.ASN = "AS" + asn
			} else if asn := resp.Header.Get("cf-meta-asn"); asn != "" {
				meta.ASN = "AS" + asn
			}
			if colo := resp.Header.Get("colo"); colo != "" {
				meta.Colo = colo
			} else if colo := resp.Header.Get("cf-meta-colo"); colo != "" {
				meta.Colo = colo
			}
			if country := resp.Header.Get("country"); country != "" {
				meta.Country = country
			} else if country := resp.Header.Get("cf-meta-country"); country != "" {
				meta.Country = country
			}
			if city := resp.Header.Get("city"); city != "" {
				if decoded, err := url.QueryUnescape(city); err == nil {
					meta.City = decoded
				} else {
					meta.City = city
				}
			}
			if resp.TLS != nil {
				switch resp.TLS.Version {
				case tls.VersionTLS13:
					meta.TLSVersion = "TLS 1.3"
				case tls.VersionTLS12:
					meta.TLSVersion = "TLS 1.2"
				}
			}
			if meta.ClientIP != "" || meta.Colo != "" {
				return meta
			}
		}
	}

	// Fallback to 1.1.1.1/cdn-cgi/trace directly by IP
	traceReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err == nil {
		resp, err := client.Do(traceReq)
		if err == nil {
			defer resp.Body.Close()
			buf := make([]byte, 2048)
			n, _ := resp.Body.Read(buf)
			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(kv) != 2 {
					continue
				}
				switch kv[0] {
				case "ip":
					if meta.ClientIP == "" {
						meta.ClientIP = kv[1]
					}
				case "colo":
					if meta.Colo == "" {
						meta.Colo = kv[1]
					}
				case "loc":
					if meta.Country == "" {
						meta.Country = kv[1]
					}
				case "tls":
					meta.TLSVersion = kv[1]
				}
			}
		}
	}

	return meta
}
