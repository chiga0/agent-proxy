package trace

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

type Hop struct {
	Num  int
	Host string
	IP   string
	RTT  []time.Duration
}

type TraceResult struct {
	From    string
	To      string
	Hops    []Hop
	Loss    float64
	AvgRTT  time.Duration
	Error   string
}

// LocalToECS traces from local machine to the proxy ECS.
func LocalToECS(cfg *config.Config) TraceResult {
	return traceroute("local", cfg.Proxy.Host)
}

// ECSToTarget traces from ECS to a target domain (via SSH).
func ECSToTarget(cfg *config.Config, target string) TraceResult {
	// Resolve target to IP first
	ips, err := net.LookupHost(target)
	if err != nil || len(ips) == 0 {
		return TraceResult{From: cfg.Proxy.Host, To: target, Error: "DNS resolution failed"}
	}

	// Run traceroute on ECS via SSH
	cmd := fmt.Sprintf("traceroute -n -m 15 -w 2 %s 2>/dev/null || tracepath -m 15 %s 2>/dev/null", ips[0], ips[0])
	out, err := sshExec(cfg, cmd)
	if err != nil {
		return TraceResult{From: cfg.Proxy.Host, To: target, Error: err.Error()}
	}

	hops := parseTraceroute(out)
	return TraceResult{
		From: cfg.Proxy.Host,
		To:   fmt.Sprintf("%s (%s)", target, ips[0]),
		Hops: hops,
	}
}

// DNSInfo returns DNS resolution details for a domain.
func DNSInfo(domain string) (ip string, dur time.Duration, err error) {
	start := time.Now()
	ips, err := net.LookupHost(domain)
	dur = time.Since(start)
	if err != nil {
		return "", dur, err
	}
	if len(ips) > 0 {
		return ips[0], dur, nil
	}
	return "", dur, fmt.Errorf("no results")
}

func traceroute(from, target string) TraceResult {
	// Use system traceroute
	out, err := exec.Command("traceroute", "-n", "-m", "15", "-w", "2", target).CombinedOutput()
	if err != nil {
		// Fallback to ping-based check
		out2, err2 := exec.Command("ping", "-c", "3", "-W", "2000", target).CombinedOutput()
		if err2 != nil {
			return TraceResult{From: from, To: target, Error: "traceroute and ping both failed"}
		}
		return TraceResult{From: from, To: target, Hops: nil, Error: string(out2)}
	}

	hops := parseTraceroute(string(out))
	return TraceResult{From: from, To: target, Hops: hops}
}

func parseTraceroute(output string) []Hop {
	var hops []Hop
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "traceroute") || strings.HasPrefix(line, "tracepath") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hop := Hop{}
		fmt.Sscanf(fields[0], "%d", &hop.Num)
		if hop.Num == 0 {
			continue
		}

		// Parse IP and RTTs.
		// Standard traceroute emits "IP  RTT ms  RTT ms  RTT ms" where the
		// numeric RTT value and the "ms" unit are separate whitespace-delimited
		// tokens. Some implementations emit them joined ("1.234ms"), so we
		// handle both forms.
		var pendingRttVal float64
		var hasPendingRtt bool
		for _, f := range fields[1:] {
			if f == "*" {
				hasPendingRtt = false
				continue
			}
			if f == "ms" && hasPendingRtt {
				hop.RTT = append(hop.RTT, time.Duration(pendingRttVal*float64(time.Millisecond)))
				hasPendingRtt = false
				continue
			}
			if net.ParseIP(f) != nil {
				hop.IP = f
				hop.Host = f
				hasPendingRtt = false
			} else if strings.HasSuffix(f, "ms") {
				var ms float64
				fmt.Sscanf(f, "%fms", &ms)
				hop.RTT = append(hop.RTT, time.Duration(ms*float64(time.Millisecond)))
				hasPendingRtt = false
			} else {
				var ms float64
				if n, _ := fmt.Sscanf(f, "%f", &ms); n == 1 {
					pendingRttVal = ms
					hasPendingRtt = true
				} else {
					hasPendingRtt = false
				}
			}
		}
		if hop.IP != "" || len(hop.RTT) > 0 {
			hops = append(hops, hop)
		}
	}
	return hops
}

func sshExec(cfg *config.Config, cmd string) (string, error) {
	args := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10"}
	if cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	args = append(args, fmt.Sprintf("%s@%s", user, cfg.Proxy.Host), cmd)
	out, err := exec.Command("ssh", args...).CombinedOutput()
	return string(out), err
}

func PrintTrace(r TraceResult) {
	fmt.Printf("\n  %s → %s\n", r.From, r.To)
	fmt.Printf("  %s\n", strings.Repeat("-", 60))

	if r.Error != "" && len(r.Hops) == 0 {
		fmt.Printf("  Error: %s\n", r.Error)
		return
	}

	for _, h := range r.Hops {
		rtts := ""
		for _, rtt := range h.RTT {
			rtts += fmt.Sprintf(" %5.1fms", float64(rtt.Microseconds())/1000.0)
		}
		if rtts == "" {
			rtts = "     *    *    *"
		}
		host := h.Host
		if host == "" {
			host = "* * *"
		}
		fmt.Printf("  %2d  %-20s %s\n", h.Num, host, rtts)
	}
}
