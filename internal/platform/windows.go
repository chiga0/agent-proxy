//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

const ieRegPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func DetectNetworkService() (string, error) {
	return "", nil
}

func SetAutoProxy(service, url string) error {
	if out, err := exec.Command("reg", "add", ieRegPath, "/v", "AutoConfigURL", "/t", "REG_SZ", "/d", url, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("set AutoConfigURL: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("reg", "add", ieRegPath, "/v", "AutoDetect", "/t", "REG_DWORD", "/d", "0", "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("set AutoDetect: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ClearAutoProxy(service string) error {
	out, err := exec.Command("reg", "delete", ieRegPath, "/v", "AutoConfigURL", "/f").CombinedOutput()
	if err != nil {
		// Only ignore "value not found" — propagate real errors
		output := strings.TrimSpace(string(out))
		if strings.Contains(output, "not found") || strings.Contains(output, "找不到") {
			return nil
		}
		return fmt.Errorf("delete AutoConfigURL: %s: %w", output, err)
	}
	return nil
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	out, err := exec.Command("reg", "query", ieRegPath, "/v", "AutoConfigURL").CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		// Distinguish "value not found" from real errors
		if strings.Contains(output, "not found") || strings.Contains(output, "找不到") || strings.Contains(output, "No values") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("registry query: %s: %w", output, err)
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

// CaptureExtraState saves the AutoDetect registry value and its presence.
func CaptureExtraState(service string) (map[string]string, error) {
	out, err := exec.Command("reg", "query", ieRegPath, "/v", "AutoDetect").CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if strings.Contains(output, "not found") || strings.Contains(output, "找不到") || strings.Contains(output, "No values") {
			// Value genuinely absent — record this so we can restore absence
			return map[string]string{"auto_detect_present": "false"}, nil
		}
		return nil, fmt.Errorf("query AutoDetect: %s: %w", output, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "AutoDetect") {
			parts := strings.SplitN(line, "REG_DWORD", 2)
			if len(parts) == 2 {
				return map[string]string{"auto_detect": strings.TrimSpace(parts[1]), "auto_detect_present": "true"}, nil
			}
		}
	}
	return map[string]string{"auto_detect_present": "false"}, nil
}

// RestoreExtraState restores the AutoDetect registry value or removes it
// if it was originally absent.
func RestoreExtraState(service string, data map[string]string) error {
	if data == nil {
		return nil
	}
	present := data["auto_detect_present"]
	if present == "false" {
		// Originally absent — delete the value we created
		exec.Command("reg", "delete", ieRegPath, "/v", "AutoDetect", "/f").Run()
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
