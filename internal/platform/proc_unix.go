//go:build darwin || linux

package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

// FindPIDsByPattern returns PIDs of processes whose command line matches pattern.
func FindPIDsByPattern(pattern string) []int {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if pid, err := strconv.Atoi(line); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// GetProcessArgs returns the full command line of a process.
func GetProcessArgs(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetProcessName returns the executable name of a process.
func GetProcessName(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// IsProcessAlive checks whether a process with the given PID exists.
func IsProcessAlive(pid int) bool {
	return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
}
