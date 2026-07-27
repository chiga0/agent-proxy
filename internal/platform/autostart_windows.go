//go:build windows

package platform

import "github.com/chiga0/agent-proxy/internal/config"

func InstallAutoStart(cfg *config.Config) error {
	// TODO: Windows Task Scheduler integration
	return nil
}

func UninstallAutoStart() {
	// TODO: Windows Task Scheduler cleanup
}
