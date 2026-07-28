//go:build windows

package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

// FindPIDsByPattern returns PIDs of processes whose command line matches pattern.
// Uses PowerShell Get-CimInstance with regex matching (WMIC is deprecated).
func FindPIDsByPattern(pattern string) []int {
	// Escape the pattern for PowerShell single-quoted string, then use [regex]
	escaped := strings.ReplaceAll(pattern, "'", "''")
	// Exclude $PID (the PowerShell process itself) to avoid self-match
	script := "Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -and [regex]::IsMatch($_.CommandLine, '" + escaped + "') -and $_.ProcessId -ne $PID } | ForEach-Object { $_.ProcessId }"
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if pid, err := strconv.Atoi(line); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// GetProcessArgs returns the full command line of a process.
func GetProcessArgs(pid int) string {
	script := "(Get-CimInstance Win32_Process -Filter 'ProcessId=" + strconv.Itoa(pid) + "').CommandLine"
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
