//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

func DetectNetworkService() (string, error) {
	out, err := exec.Command("route", "get", "default").Output()
	if err != nil {
		return fallbackService()
	}

	var iface string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "interface:"))
			break
		}
	}

	if iface != "" {
		list, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
		if err == nil {
			lines := strings.Split(string(list), "\n")
			for i, line := range lines {
				if strings.Contains(line, "Device: "+iface+")") && i > 0 {
					svc := strings.TrimSpace(lines[i-1])
					svc = strings.TrimPrefix(svc, "("+extractNum(svc)+") ")
					if svc != "" {
						return svc, nil
					}
				}
			}
		}
	}

	return fallbackService()
}

func fallbackService() (string, error) {
	for _, name := range []string{"Wi-Fi", "Ethernet", "Thunderbolt Bridge"} {
		out, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
		if err == nil && strings.Contains(string(out), name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("cannot detect network service")
}

func extractNum(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] == '(' {
		end := strings.Index(s, ")")
		if end > 1 {
			return s[1:end]
		}
	}
	return "0"
}

func SetAutoProxy(service, url string) error {
	if err := run("networksetup", "-setautoproxyurl", service, url); err != nil {
		return err
	}
	return run("networksetup", "-setautoproxystate", service, "on")
}

func ClearAutoProxy(service string) error {
	return run("networksetup", "-setautoproxystate", service, "off")
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	out, err := exec.Command("networksetup", "-getautoproxyurl", service).Output()
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "URL:") {
			url = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
		}
		if strings.HasPrefix(line, "Enabled:") {
			enabled = strings.TrimSpace(strings.TrimPrefix(line, "Enabled:")) == "Yes"
		}
	}
	return url, enabled, nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %w", name, strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CaptureExtraState returns nil on macOS — URL + enabled is sufficient.
func CaptureExtraState(service string) map[string]string { return nil }

// RestoreExtraState is a no-op on macOS.
func RestoreExtraState(service string, data map[string]string) error { return nil }
