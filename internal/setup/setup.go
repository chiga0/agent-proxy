package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

// ExpandHome expands a leading ~/ to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

// ValidateSSHKey checks the key file exists, fixes permissions, and warns
// about macOS TCC-protected directories. Returns the (possibly relocated) path.
func ValidateSSHKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("SSH key not found: %s\n  Fix: check the path or generate a key: ssh-keygen -t ed25519", path)
	}

	if info.Mode().Perm()&0077 != 0 {
		fmt.Printf("  → Fixing key permissions (0%o → 0600)... ", info.Mode().Perm())
		if err := os.Chmod(path, 0600); err != nil {
			fmt.Println("✗")
			return "", fmt.Errorf("cannot fix key permissions: %w", err)
		}
		fmt.Println("✓")
	}

	home, _ := os.UserHomeDir()
	protected := []string{
		home + "/Documents/",
		home + "/Desktop/",
		home + "/Downloads/",
	}
	for _, dir := range protected {
		if strings.HasPrefix(path, dir) {
			dest := home + "/.ssh/" + info.Name()
			fmt.Printf("\n  ⚠ Key is in a macOS protected directory (%s)\n", dir)
			fmt.Printf("    Background services (LaunchAgent) cannot access this directory.\n")
			fmt.Printf("    Recommended: copy to %s\n", dest)
			fmt.Printf("    Copy automatically? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			ans, _ := reader.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			if ans == "" || ans == "y" || ans == "yes" {
				os.MkdirAll(home+"/.ssh", 0700)
				data, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("read key: %w", err)
				}
				if err := os.WriteFile(dest, data, 0600); err != nil {
					return "", fmt.Errorf("copy key: %w", err)
				}
				fmt.Printf("    ✓ Copied to %s\n", dest)
				return dest, nil
			}
			break
		}
	}
	return path, nil
}

// VerifyHostKey checks if the ECS host key is already trusted. If not,
// it fetches the key via ssh-keyscan, displays the SHA256 fingerprint,
// and requires explicit user confirmation before adding it to known_hosts.
func VerifyHostKey(cfg *config.Config, reader *bufio.Reader) error {
	knownHosts := config.KnownHostsPath()
	os.MkdirAll(config.DataDir(), 0700)

	if data, err := os.ReadFile(knownHosts); err == nil {
		if strings.Contains(string(data), cfg.Proxy.Host) {
			fmt.Printf("  ✓ Host %s already in known_hosts\n", cfg.Proxy.Host)
			return nil
		}
	}

	fmt.Printf("  → Fetching host key from %s... ", cfg.Proxy.Host)
	out, err := exec.Command("ssh-keyscan", "-T", "5", cfg.Proxy.Host).Output()
	if err != nil || len(out) == 0 {
		fmt.Println("✗")
		return fmt.Errorf("cannot fetch host key from %s — check network connectivity", cfg.Proxy.Host)
	}
	fmt.Println("✓")

	tmpFile, err := os.CreateTemp("", "agent-proxy-keyscan-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	tmpFile.Write(out)
	tmpFile.Close()

	fpOut, err := exec.Command("ssh-keygen", "-lf", tmpPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("compute fingerprint: %s: %w", strings.TrimSpace(string(fpOut)), err)
	}

	fmt.Println("\n  Host key fingerprint:")
	for _, line := range strings.Split(strings.TrimSpace(string(fpOut)), "\n") {
		fmt.Printf("    %s\n", strings.TrimSpace(line))
	}

	fmt.Printf("\n  Verify the SHA256 fingerprint above matches your ECS console.\n")
	fmt.Printf("  Type yes to confirm: ")
	ans, _ := reader.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans != "yes" {
		return fmt.Errorf("host key verification aborted — you must type 'yes' to accept")
	}

	f, err := os.OpenFile(knownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}
	fmt.Printf("  ✓ Saved to %s\n", knownHosts)
	return nil
}
