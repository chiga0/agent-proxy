package proxy

import (
	"crypto/tls"
	"encoding/base64"
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
	"github.com/chiga0/agent-proxy/internal/tunnel"
)

type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

func Status(cfg *config.Config) []CheckResult {
	results := []CheckResult{
		checkSSH(cfg),
	}

	if cfg.Proxy.Tunnel {
		results = append(results, checkTunnel(cfg))
	} else {
		results = append(results, checkPort(cfg))
	}

	results = append(results,
		checkForwarding(cfg),
		checkPAC(cfg),
		checkPACFile(cfg),
		checkPACServer(),
	)

	return results
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

func checkTunnel(cfg *config.Config) CheckResult {
	if tunnel.Running(cfg) {
		return CheckResult{"SSH tunnel", true, fmt.Sprintf("127.0.0.1:%d → %s", cfg.Proxy.Port, cfg.Proxy.Host)}
	}
	return CheckResult{"SSH tunnel", false, "not running — run: agent-proxy on"}
}

func checkForwarding(cfg *config.Config) CheckResult {
	proxyURL, _ := url.Parse(cfg.ProxyURL())
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		detail := err.Error()
		if strings.Contains(detail, "connection reset") {
			detail = "TLS reset — possible SNI filtering (try: proxy.tunnel: true)"
		}
		return CheckResult{"Proxy forwarding", false, detail}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 407 {
		return CheckResult{"Proxy forwarding", false, "407 auth required — run: agent-proxy setup"}
	}
	if resp.StatusCode != 200 {
		return CheckResult{"Proxy forwarding", false, fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		IP string `json:"ip"`
	}
	json.Unmarshal(body, &info)
	return CheckResult{"Proxy forwarding", true, "exit IP: " + info.IP}
}

// DetectSNIBlock tests whether TLS connections are being reset by GFW SNI filtering.
func DetectSNIBlock(cfg *config.Config) bool {
	if cfg.Proxy.Tunnel {
		return false // tunnel encrypts SNI
	}

	proxyHost := cfg.Proxy.EffectiveHost()
	addr := fmt.Sprintf("%s:%d", proxyHost, cfg.Proxy.Port)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	connectReq := fmt.Sprintf("CONNECT google.com:443 HTTP/1.1\r\nHost: google.com:443\r\n")
	if cfg.HasAuth() {
		auth := base64.StdEncoding.EncodeToString([]byte(cfg.Proxy.User + ":" + cfg.Proxy.Password))
		connectReq += "Proxy-Authorization: Basic " + auth + "\r\n"
	}
	connectReq += "\r\n"
	conn.Write([]byte(connectReq))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         "google.com",
		InsecureSkipVerify: true,
	})
	tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
	err = tlsConn.Handshake()
	if err != nil && strings.Contains(err.Error(), "reset") {
		return true
	}
	return false
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
