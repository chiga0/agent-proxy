//go:build linux

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
	if !gsettingsAvailable() {
		return fmt.Errorf("gsettings not available — system PAC not supported; use CLI env vars only")
	}
	if err := runGsettings("set", "org.gnome.system.proxy", "mode", "auto"); err != nil {
		return fmt.Errorf("set proxy mode: %w", err)
	}
	if err := runGsettings("set", "org.gnome.system.proxy", "autoconfig-url", url); err != nil {
		return fmt.Errorf("set autoconfig-url: %w", err)
	}
	return nil
}

func ClearAutoProxy(service string) error {
	if !gsettingsAvailable() {
		return nil
	}
	return runGsettings("set", "org.gnome.system.proxy", "mode", "none")
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	if !gsettingsAvailable() {
		return "", false, fmt.Errorf("gsettings not available")
	}
	urlOut, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "autoconfig-url").Output()
	if err != nil {
		return "", false, fmt.Errorf("gsettings get autoconfig-url: %w", err)
	}
	url = strings.Trim(strings.TrimSpace(string(urlOut)), "'")

	modeOut, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	if err != nil {
		return "", false, fmt.Errorf("gsettings get mode: %w", err)
	}
	enabled = strings.TrimSpace(string(modeOut)) == "'auto'"
	return url, enabled, nil
}

func gsettingsAvailable() bool {
	return exec.Command("gsettings", "--version").Run() == nil
}

func runGsettings(args ...string) error {
	out, err := exec.Command("gsettings", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CaptureExtraState saves the GNOME proxy mode so it can be restored later.
func CaptureExtraState(service string) map[string]string {
	if !gsettingsAvailable() {
		return nil
	}
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	if err != nil {
		return nil
	}
	return map[string]string{"mode": strings.Trim(strings.TrimSpace(string(out)), "'")}
}

// RestoreExtraState restores the GNOME proxy mode.
func RestoreExtraState(service string, data map[string]string) error {
	if data == nil {
		return nil
	}
	if mode, ok := data["mode"]; ok && mode != "" && gsettingsAvailable() {
		return runGsettings("set", "org.gnome.system.proxy", "mode", mode)
	}
	return nil
}
