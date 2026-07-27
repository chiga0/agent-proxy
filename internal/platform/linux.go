//go:build linux

package platform

import (
	"fmt"
	"os/exec"
)

func DetectNetworkService() (string, error) {
	return "", nil
}

func SetAutoProxy(service, url string) error {
	// Try GNOME/gsettings first
	if err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "auto").Run(); err == nil {
		exec.Command("gsettings", "set", "org.gnome.system.proxy", "autoconfig-url", url).Run()
		return nil
	}
	// Fallback: env vars are the primary mechanism on Linux
	return nil
}

func ClearAutoProxy(service string) error {
	exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
	return nil
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "autoconfig-url").Output()
	if err != nil {
		return "", false, fmt.Errorf("gsettings not available")
	}
	url = string(out)
	modeOut, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	enabled = string(modeOut) == "'auto'\n"
	return url, enabled, nil
}
