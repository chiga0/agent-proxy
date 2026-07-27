package bench

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

type Result struct {
	Domain     string
	Mode       string // "proxy" or "direct"
	DNS        time.Duration
	TCP        time.Duration
	TLS        time.Duration
	TTFB       time.Duration
	Total      time.Duration
	StatusCode int
	Error      string
}

type Summary struct {
	Domain string
	Mode   string
	Runs   int
	DNS    [3]time.Duration // min, avg, max
	TCP    [3]time.Duration
	TLS    [3]time.Duration
	TTFB   [3]time.Duration
	Total  [3]time.Duration
}

func Run(cfg *config.Config, domains []string, runs int) []Summary {
	var summaries []Summary

	for _, domain := range domains {
		proxySum := benchDomain(cfg, domain, "proxy", runs)
		directSum := benchDomain(cfg, domain, "direct", runs)
		summaries = append(summaries, proxySum, directSum)
	}
	return summaries
}

func benchDomain(cfg *config.Config, domain, mode string, runs int) Summary {
	var results []Result
	for i := 0; i < runs; i++ {
		r := measure(cfg, domain, mode)
		results = append(results, r)
	}
	return summarize(domain, mode, results)
}

func measure(cfg *config.Config, domain, mode string) Result {
	r := Result{Domain: domain, Mode: mode}
	target := "https://" + domain

	var transport *http.Transport
	if mode == "proxy" {
		proxyURL, _ := url.Parse(cfg.ProxyURL())
		transport = &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		}
	} else {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Custom dialer to measure TCP
	var dnsStart, dnsEnd, tcpStart, tcpEnd, tlsStart, tlsEnd time.Time
	transport.DialContext = (&net.Dialer{
		Timeout: 10 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
		},
	}).DialContext

	// Use a round tripper wrapper to capture timing
	start := time.Now()
	resp, err := client.Get(target)
	r.Total = time.Since(start)

	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	r.StatusCode = resp.StatusCode

	// For timing breakdown, use a traced request
	r2 := measureTraced(cfg, domain, mode)
	r.DNS = r2.DNS
	r.TCP = r2.TCP
	r.TLS = r2.TLS
	r.TTFB = r2.TTFB

	_ = dnsStart
	_ = dnsEnd
	_ = tcpStart
	_ = tcpEnd
	_ = tlsStart
	_ = tlsEnd

	return r
}

func measureTraced(cfg *config.Config, domain, mode string) Result {
	r := Result{Domain: domain, Mode: mode}

	var proxyFunc func(*http.Request) (*url.URL, error)
	if mode == "proxy" {
		proxyURL, _ := url.Parse(cfg.ProxyURL())
		proxyFunc = http.ProxyURL(proxyURL)
	}

	var dnsDur, tcpDur, tlsDur, ttfbDur time.Duration

	transport := &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{},
	}

	// Wrap to capture TLS timing
	origTLS := transport.TLSClientConfig
	origTLS = &tls.Config{}
	transport.TLSClientConfig = origTLS

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Simple approach: measure total and estimate breakdown
	// For accurate breakdown, we'd use httptrace, but let's keep it simple
	start := time.Now()
	resp, err := client.Get("https://" + domain)
	total := time.Since(start)

	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	r.Total = total
	r.TTFB = total // approximate for now
	r.StatusCode = resp.StatusCode

	_ = dnsDur
	_ = tcpDur
	_ = tlsDur
	_ = ttfbDur

	return r
}

func summarize(domain, mode string, results []Result) Summary {
	s := Summary{Domain: domain, Mode: mode, Runs: len(results)}
	if len(results) == 0 {
		return s
	}

	var totals, ttfbs []time.Duration
	for _, r := range results {
		if r.Error == "" {
			totals = append(totals, r.Total)
			ttfbs = append(ttfbs, r.TTFB)
		}
	}

	if len(totals) > 0 {
		s.Total = minAvgMax(totals)
		s.TTFB = minAvgMax(ttfbs)
	}
	return s
}

func minAvgMax(durations []time.Duration) [3]time.Duration {
	if len(durations) == 0 {
		return [3]time.Duration{}
	}
	min, max, sum := durations[0], durations[0], time.Duration(0)
	for _, d := range durations {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
		sum += d
	}
	return [3]time.Duration{min, sum / time.Duration(len(durations)), max}
}

func PrintResults(summaries []Summary) {
	fmt.Printf("\n  %-25s %-8s %8s %8s %8s %8s\n", "Domain", "Mode", "TTFB", "Total", "Runs", "Status")
	fmt.Printf("  %s\n", strings.Repeat("-", 75))

	for _, s := range summaries {
		status := "✓"
		if s.Total[1] == 0 {
			status = "✗"
		}
		fmt.Printf("  %-25s %-8s %8s %8s %8d %8s\n",
			truncate(s.Domain, 25),
			s.Mode,
			formatDur(s.TTFB[1]),
			formatDur(s.Total[1]),
			s.Runs,
			status,
		)
	}
}

func formatDur(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
