package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/pac"
	"github.com/chiga0/agent-proxy/internal/platform"
)

type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

func Status(cfg *config.Config) []CheckResult {
	return []CheckResult{
		checkSSH(cfg),
		checkPort(cfg),
		checkAuth(cfg),
		checkNoAuth(cfg),
		checkPAC(cfg),
		checkPACFile(cfg),
		checkPACServer(),
	}
}

func checkSSH(cfg *config.Config) CheckResult {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:22", cfg.Proxy.Host), 5*time.Second)
	if err != nil {
		return CheckResult{"SSH (22)", false, "unreachable"}
	}
	conn.Close()
	return CheckResult{"SSH (22)", true, ""}
}

func checkPort(cfg *config.Config) CheckResult {
	addr := net.JoinHostPort(cfg.Proxy.Host, fmt.Sprintf("%d", cfg.Proxy.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return CheckResult{fmt.Sprintf("Proxy port (%d)", cfg.Proxy.Port), false, "blocked?"}
	}
	conn.Close()
	return CheckResult{fmt.Sprintf("Proxy port (%d)", cfg.Proxy.Port), true, ""}
}

func checkAuth(cfg *config.Config) CheckResult {
	proxyURL, _ := url.Parse(cfg.ProxyURL())
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return CheckResult{"Auth + forwarding", false, err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return CheckResult{"Auth + forwarding", false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	body, _ := io.ReadAll(resp.Body)
	var info struct{ IP string `json:"ip"` }
	json.Unmarshal(body, &info)
	return CheckResult{"Auth + forwarding", true, "exit IP: " + info.IP}
}

func checkNoAuth(cfg *config.Config) CheckResult {
	proxyURL, _ := url.Parse(cfg.ProxyURLNoAuth())
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return CheckResult{"No-auth rejection", true, "connection refused"}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 407 {
		return CheckResult{"No-auth rejection", true, "407"}
	}
	return CheckResult{"No-auth rejection", false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func checkPAC(cfg *config.Config) CheckResult {
	service, err := platform.DetectNetworkService()
	if err != nil {
		return CheckResult{"System PAC", false, "cannot detect network"}
	}
	pacURL, enabled, err := platform.GetAutoProxy(service)
	if err != nil {
		return CheckResult{"System PAC", false, err.Error()}
	}
	if !enabled {
		return CheckResult{"System PAC", false, "disabled"}
	}
	return CheckResult{"System PAC", true, pacURL}
}

func checkPACFile(cfg *config.Config) CheckResult {
	data, err := exec.Command("grep", "-c", "dnsDomainIs", config.PACPath()).Output()
	if err != nil {
		return CheckResult{"PAC file", false, "not found"}
	}
	return CheckResult{"PAC file", true, strings.TrimSpace(string(data)) + " domains"}
}

func checkPACServer() CheckResult {
	if pac.ServerRunning() {
		return CheckResult{"PAC HTTP server", true, fmt.Sprintf("127.0.0.1:%d", config.PACPort)}
	}
	return CheckResult{"PAC HTTP server", false, "not running"}
}

func PrintStatus(results []CheckResult) {
	pass, fail := 0, 0
	for _, r := range results {
		icon := "✓"
		if !r.OK {
			icon = "✗"
			fail++
		} else {
			pass++
		}
		detail := ""
		if r.Detail != "" {
			detail = " (" + r.Detail + ")"
		}
		fmt.Printf("  %s %s%s\n", icon, r.Name, detail)
	}
	fmt.Printf("\n  Result: %d passed, %d failed\n", pass, fail)
}
