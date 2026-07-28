//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

func DetectNetworkService() (string, error) {
	return "", nil
}

func SetAutoProxy(service, url string) error {
	reg := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	if out, err := exec.Command("reg", "add", reg, "/v", "AutoConfigURL", "/t", "REG_SZ", "/d", url, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("set AutoConfigURL: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("reg", "add", reg, "/v", "AutoDetect", "/t", "REG_DWORD", "/d", "0", "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("set AutoDetect: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ClearAutoProxy(service string) error {
	reg := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	// Ignore "value not found" errors — the key may already be absent
	exec.Command("reg", "delete", reg, "/v", "AutoConfigURL", "/f").Run()
	return nil
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	out, err := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "AutoConfigURL").Output()
	if err != nil {
		return "", false, nil // key not present = no PAC configured
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "AutoConfigURL") {
			parts := strings.SplitN(line, "REG_SZ", 2)
			if len(parts) == 2 {
				url = strings.TrimSpace(parts[1])
				enabled = url != ""
			}
		}
	}
	return url, enabled, nil
}

const ieRegPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// CaptureExtraState saves the AutoDetect registry value.
func CaptureExtraState(service string) map[string]string {
	out, err := exec.Command("reg", "query", ieRegPath, "/v", "AutoDetect").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "AutoDetect") {
			parts := strings.SplitN(line, "REG_DWORD", 2)
			if len(parts) == 2 {
				return map[string]string{"auto_detect": strings.TrimSpace(parts[1])}
			}
		}
	}
	return nil
}

// RestoreExtraState restores the AutoDetect registry value.
func RestoreExtraState(service string, data map[string]string) error {
	if data == nil {
		return nil
	}
	if v, ok := data["auto_detect"]; ok && v != "" {
		out, err := exec.Command("reg", "add", ieRegPath, "/v", "AutoDetect", "/t", "REG_DWORD", "/d", v, "/f").CombinedOutput()
		if err != nil {
			return fmt.Errorf("restore AutoDetect: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}
