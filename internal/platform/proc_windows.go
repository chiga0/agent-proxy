//go:build windows

package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

// FindPIDsByPattern returns PIDs of processes whose command line matches pattern.
// Uses wmic on Windows (available on Windows 7+).
func FindPIDsByPattern(pattern string) []int {
	out, err := exec.Command("wmic", "process", "where",
		"CommandLine like '%"+pattern+"%'", "get", "ProcessId", "/value").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProcessId=") {
			pidStr := strings.TrimPrefix(line, "ProcessId=")
			if pid, err := strconv.Atoi(strings.TrimSpace(pidStr)); err == nil && pid > 0 {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

// GetProcessArgs returns the full command line of a process.
func GetProcessArgs(pid int) string {
	out, err := exec.Command("wmic", "process", "where",
		"ProcessId="+strconv.Itoa(pid), "get", "CommandLine", "/value").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "CommandLine=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "CommandLine="))
		}
	}
	return ""
}

// GetProcessName returns the executable name of a process.
func GetProcessName(pid int) string {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid),
		"/NH", "/FO", "CSV").Output()
	if err != nil {
		return ""
	}
	// Output: "name.exe","PID","Session","Session#","MemUsage"
	line := strings.TrimSpace(string(out))
	if idx := strings.Index(line, "\""); idx >= 0 {
		end := strings.Index(line[idx+1:], "\"")
		if end >= 0 {
			return line[idx+1 : idx+1+end]
		}
	}
	return ""
}

// IsProcessAlive checks whether a process with the given PID exists.
func IsProcessAlive(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid),
		"/NH", "/FO", "CSV").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}
