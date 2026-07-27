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
	// Set IE/Edge system proxy via registry
	reg := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	exec.Command("reg", "add", reg, "/v", "AutoConfigURL", "/t", "REG_SZ", "/d", url, "/f").Run()
	exec.Command("reg", "add", reg, "/v", "AutoDetect", "/t", "REG_DWORD", "/d", "0", "/f").Run()
	return nil
}

func ClearAutoProxy(service string) error {
	reg := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	exec.Command("reg", "delete", reg, "/v", "AutoConfigURL", "/f").Run()
	return nil
}

func GetAutoProxy(service string) (url string, enabled bool, err error) {
	out, err := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "AutoConfigURL").Output()
	if err != nil {
		return "", false, fmt.Errorf("registry query failed")
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
