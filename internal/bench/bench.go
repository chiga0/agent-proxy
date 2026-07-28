package bench

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
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
	Domain   string
	Mode     string
	Runs     int
	Success  int
	LastErr  string
	DNS      [3]time.Duration // min, avg, max
	TCP      [3]time.Duration
	TLS      [3]time.Duration
	TTFB     [3]time.Duration
	Total    [3]time.Duration
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

	var proxyFunc func(*http.Request) (*url.URL, error)
	if mode == "proxy" {
		proxyURL, _ := url.Parse(cfg.ProxyURL())
		proxyFunc = http.ProxyURL(proxyURL)
	}

	transport := &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var dnsStart, tcpStart, tlsStart, firstByte time.Time
	var dnsEnd, tcpEnd, tlsEnd time.Time

	ct := &httptrace.ClientTrace{
		DNSStart:             func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(_ httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart:         func(_, _ string) { tcpStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { tcpEnd = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { tlsEnd = time.Now() },
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}

	req, err := http.NewRequest("GET", "https://"+domain, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), ct))

	start := time.Now()
	resp, err := client.Do(req)
	r.Total = time.Since(start)

	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	r.StatusCode = resp.StatusCode

	if !dnsEnd.IsZero() {
		r.DNS = dnsEnd.Sub(dnsStart)
	}
	if !tcpEnd.IsZero() {
		r.TCP = tcpEnd.Sub(tcpStart)
	}
	if !tlsEnd.IsZero() {
		r.TLS = tlsEnd.Sub(tlsStart)
	}
	if !firstByte.IsZero() {
		r.TTFB = firstByte.Sub(start)
	}

	return r
}

func summarize(domain, mode string, results []Result) Summary {
	s := Summary{Domain: domain, Mode: mode, Runs: len(results)}
	if len(results) == 0 {
		return s
	}

	var dns, tcp, tlsD, ttfbs, totals []time.Duration
	for _, r := range results {
		if r.Error != "" {
			s.LastErr = r.Error
			continue
		}
		s.Success++
		totals = append(totals, r.Total)
		ttfbs = append(ttfbs, r.TTFB)
		if r.DNS > 0 {
			dns = append(dns, r.DNS)
		}
		if r.TCP > 0 {
			tcp = append(tcp, r.TCP)
		}
		if r.TLS > 0 {
			tlsD = append(tlsD, r.TLS)
		}
	}

	if len(totals) > 0 {
		s.Total = minAvgMax(totals)
		s.TTFB = minAvgMax(ttfbs)
		s.DNS = minAvgMax(dns)
		s.TCP = minAvgMax(tcp)
		s.TLS = minAvgMax(tlsD)
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
	fmt.Printf("\n  %-25s %-8s %8s %8s %8s %8s %8s\n", "Domain", "Mode", "DNS", "TTFB", "Total", "OK/Run", "Status")
	fmt.Printf("  %s\n", strings.Repeat("-", 83))

	for _, s := range summaries {
		status := "✓"
		if s.Success == 0 {
			status = "✗"
		} else if s.Success < s.Runs {
			status = "△"
		}
		fmt.Printf("  %-25s %-8s %8s %8s %8s %5d/%-2d %8s\n",
			truncate(s.Domain, 25),
			s.Mode,
			formatDur(s.DNS[1]),
			formatDur(s.TTFB[1]),
			formatDur(s.Total[1]),
			s.Success, s.Runs,
			status,
		)
		if s.LastErr != "" && s.Success < s.Runs {
			fmt.Printf("  %-25s          err: %s\n", "", truncate(s.LastErr, 60))
		}
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
