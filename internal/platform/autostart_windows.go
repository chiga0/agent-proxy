//go:build windows

package platform

import "github.com/chiga0/agent-proxy/internal/config"

func InstallAutoStart(cfg *config.Config) {
	// TODO: Windows Task Scheduler integration
}

func UninstallAutoStart() {
	// TODO: Windows Task Scheduler cleanup
}
