package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/bench"
	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/ecs"
	"github.com/chiga0/agent-proxy/internal/pac"
	"github.com/chiga0/agent-proxy/internal/proxy"
	"github.com/chiga0/agent-proxy/internal/trace"
	"github.com/chiga0/agent-proxy/internal/tunnel"
	"github.com/spf13/cobra"
)

var cfg *config.Config
var version = "dev"
var verbose bool

func main() {
	root := &cobra.Command{
		Use:   "agent-proxy",
		Short: "Domain-based selective proxy routing via overseas ECS",
		Long: `agent-proxy routes whitelisted domains through an overseas proxy server
while keeping all other traffic direct.

Built-in presets (ai, dev, search, cloud, media) are enabled by default —
zero configuration needed for common use cases.

Quick start:
  agent-proxy init          One-command interactive setup
  agent-proxy on            Enable proxy
  agent-proxy status        Check health
  agent-proxy doctor        Full diagnostics`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			skip := map[string]bool{"version": true, "init": true, "serve-pac": true}
			if skip[cmd.Name()] {
				return nil
			}
			var err error
			cfg, err = config.Load()
			if err != nil {
				// Emergency off: allow cleanup even with corrupted config
				if cmd.Name() == "off" {
					fmt.Fprintf(os.Stderr, "Warning: config load failed (%v), proceeding with best-effort cleanup\n", err)
					cfg = config.DefaultConfig()
					return nil
				}
				return err
			}
			// Mutating commands require strict validation (off is excluded —
			// it must work even with broken config for emergency cleanup)
			mutating := map[string]bool{
				"on": true, "setup": true, "ip": true,
				"add": true, "rm": true, "remove": true, "del": true,
				"enable": true, "disable": true,
			}
			if mutating[cmd.Name()] {
				if err := cfg.Validate(); err != nil {
					return fmt.Errorf("config validation: %w", err)
				}
			}
			// SSH commands require the host to be in project known_hosts
			// (trust-host is excluded — it's the command that creates the entry)
			sshCmds := map[string]bool{"on": true, "setup": true, "ip": true, "trace": true}
			if sshCmds[cmd.Name()] && cfg.Proxy.Host != "" {
				if data, err := os.ReadFile(config.KnownHostsPath()); err != nil || !strings.Contains(string(data), cfg.Proxy.Host) {
					return fmt.Errorf("host %s not in project known_hosts — run: agent-proxy trust-host", cfg.Proxy.Host)
				}
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	root.AddCommand(
		cmdOn(), cmdOff(), cmdStatus(), cmdDoctor(),
		cmdInit(), cmdWhitelist(), cmdPreset(),
		cmdSetup(), cmdIP(), cmdBench(), cmdTrace(),
		cmdVersion(), cmdServePAC(), cmdUpdate(), cmdTrustHost(),
		cmdStats(),
		cmdConfigValidate(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdOn() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Enable proxy (PAC + CLI env vars + SSH tunnel)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return proxy.On(cfg)
		},
	}
}

func cmdOff() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Disable proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return proxy.Off(cfg)
		},
	}
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check proxy health",
		RunE: func(cmd *cobra.Command, args []string) error {
			results := proxy.Status(cfg)
			proxy.PrintStatus(results)
			for _, r := range results {
				if !r.OK {
					os.Exit(1)
				}
			}
			return nil
		},
	}
}

func cmdDoctor() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run full diagnostics with actionable suggestions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print("=== agent-proxy doctor ===\n\n")
			fmt.Printf("Config:     %s\n", config.ConfigPath())
			fmt.Printf("Presets:    %v\n", cfg.Presets)
			wl := cfg.EffectiveWhitelist()
			fmt.Printf("Whitelist:  %d domains (%d from presets, %d custom)\n",
				len(wl), len(wl)-len(cfg.CustomDomains), len(cfg.CustomDomains))
			fmt.Printf("No-proxy:   %d entries\n", len(cfg.NoProxy))
			fmt.Printf("Proxy:      %s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
			if cfg.Proxy.Tunnel {
				fmt.Printf(" (SSH tunnel enabled)")
			}
			fmt.Printf("\n\n")

			fmt.Println("--- Connectivity ---")
			results := proxy.Status(cfg)
			proxy.PrintStatus(results)

			fmt.Println("\n--- Diagnosis ---")
			hasFailure := false
			for _, r := range results {
				if !r.OK {
					hasFailure = true
					printFix(r)
				}
			}

			// SNI detection (only relevant without tunnel)
			if !cfg.Proxy.Tunnel && proxy.DetectSNIBlock(cfg) {
				hasFailure = true
				fmt.Println("  ⚠ TLS connections reset after CONNECT — GFW SNI filtering detected")
				fmt.Println("    Fix: enable SSH tunnel to encrypt proxy traffic")
				fmt.Println("    Run: edit config.yaml → proxy.tunnel: true → agent-proxy setup → agent-proxy on")
			}

			// ECS Squid listen mode check (tunnel mode should be loopback-only)
			if cfg.Proxy.Tunnel && cfg.Proxy.Host != "" {
				loopback, detail, err := ecs.CheckSquidListenMode(cfg)
				if err != nil {
					hasFailure = true
					fmt.Printf("  ⚠ Cannot verify ECS Squid mode: %v\n", err)
					fmt.Println("    Security status: UNKNOWN — run agent-proxy doctor after fixing SSH")
				} else if !loopback {
					hasFailure = true
					fmt.Printf("  ⚠ ECS Squid is NOT loopback-only: %s\n", detail)
					fmt.Println("    Your Squid may still be listening on all interfaces from a previous deployment.")
					fmt.Println("    Fix: agent-proxy setup   (rewrites Squid config for tunnel mode)")
				}
			}

			// no_proxy coverage check: detect Chinese domains going through proxy
			if cfg.Proxy.Host != "" {
				fmt.Print("  → no_proxy coverage... ")
				logText, err := ecs.FetchRecentLogs(cfg, 500)
				if err != nil {
					fmt.Printf("⚠ (cannot fetch logs: %v)\n", err)
				} else {
					entries := ecs.ParseLogLines(logText)
					noProxySet := make(map[string]bool)
					for _, d := range cfg.NoProxy {
						noProxySet[strings.ToLower(d)] = true
					}
					var flagged []string
					seen := make(map[string]bool)
					for _, e := range entries {
						if e.Domain == "" || seen[e.Domain] {
							continue
						}
						seen[e.Domain] = true
						if ecs.LooksChinese(e.Domain) {
							// Check if already covered by no_proxy
							covered := false
							d := strings.ToLower(e.Domain)
							for np := range noProxySet {
								if strings.HasSuffix(d, np) || d == strings.TrimPrefix(np, ".") {
									covered = true
									break
								}
							}
							if !covered {
								flagged = append(flagged, e.Domain)
							}
						}
					}
					if len(flagged) > 0 {
						fmt.Printf("⚠ %d Chinese domain(s) routing through proxy:\n", len(flagged))
						for _, d := range flagged {
							fmt.Printf("    • %s\n", d)
						}
						if fix {
							// Auto-add flagged domains to no_proxy
							for _, d := range flagged {
								cfg.NoProxy = append(cfg.NoProxy, d)
							}
							if err := cfg.Save(); err != nil {
								fmt.Printf("    ✗ Failed to save config: %v\n", err)
							} else {
								cfg.WriteEnvFile()
								fmt.Printf("    ✓ Added %d domain(s) to no_proxy and regenerated env.sh\n", len(flagged))
							}
						} else {
							fmt.Println("    Fix: agent-proxy doctor --fix   (auto-add to no_proxy)")
							fmt.Println("    Or:  edit config.yaml → no_proxy → add these domains → agent-proxy on")
							hasFailure = true
						}
					} else {
						fmt.Println("✓")
					}
				}
			}

			fmt.Println()
			if !hasFailure {
				fmt.Println("✓ Everything looks good!")
			} else {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Auto-fix no_proxy issues by adding flagged domains to config")
	return cmd
}

func printFix(r proxy.CheckResult) {
	switch {
	case strings.Contains(r.Name, "SSH") && !r.OK:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
		fmt.Println("    Fix: check server is running, SSH key path is correct")
		fmt.Printf("    Run: ssh -i %s %s@%s\n", cfg.Proxy.SSHKey, cfg.Proxy.SSHUser, cfg.Proxy.Host)
	case strings.Contains(r.Name, "Proxy port") && !r.OK:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
		fmt.Println("    Fix: deploy Squid on your server")
		fmt.Println("    Run: agent-proxy setup")
	case strings.Contains(r.Name, "PAC HTTP") && !r.OK:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
		fmt.Println("    Fix: restart proxy services")
		fmt.Println("    Run: agent-proxy on")
	case strings.Contains(r.Name, "System PAC") && !r.OK:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
		fmt.Println("    Run: agent-proxy on")
	case strings.Contains(r.Name, "SSH tunnel") && !r.OK:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
		fmt.Println("    Fix: restart the SSH tunnel")
		fmt.Println("    Run: agent-proxy on")
	default:
		fmt.Printf("  ✗ %s: %s\n", r.Name, r.Detail)
	}
}

func cmdInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "One-command interactive setup",
		Long: `Interactive first-time setup. Walks you through:
  1. Server connection (IP + SSH key)
  2. Squid deployment on server
  3. SSH tunnel setup (for China users)
  4. Local proxy activation (PAC + env vars)
  5. Auto-start registration
  6. Connectivity verification`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print(`
  ┌─────────────────────────────────┐
  │   agent-proxy 首次配置向导       │
  └─────────────────────────────────┘

`)
			cfg = config.DefaultConfig()

			// --- Step 1: Server info ---
			fmt.Print("服务器 IP: ")
			host, _ := reader.ReadString('\n')
			cfg.Proxy.Host = strings.TrimSpace(host)
			if cfg.Proxy.Host == "" {
				return fmt.Errorf("server IP is required")
			}
			// Basic validation: reject URLs and spaces
			if strings.Contains(cfg.Proxy.Host, "://") || strings.Contains(cfg.Proxy.Host, " ") {
				return fmt.Errorf("invalid server address %q — enter IP or hostname only (no http:// prefix)", cfg.Proxy.Host)
			}

			fmt.Printf("SSH 用户 [root]: ")
			sshUser, _ := reader.ReadString('\n')
			if u := strings.TrimSpace(sshUser); u != "" {
				cfg.Proxy.SSHUser = u
			} else {
				cfg.Proxy.SSHUser = "root"
			}

			fmt.Printf("SSH 密钥路径 [~/.ssh/id_rsa]: ")
			sshKey, _ := reader.ReadString('\n')
			sshKey = strings.TrimSpace(sshKey)
			if sshKey == "" {
				home, _ := os.UserHomeDir()
				sshKey = home + "/.ssh/id_rsa"
			}
			sshKey = expandHome(sshKey)

			// Validate SSH key (may return a new path if auto-copied)
			finalKey, err := validateSSHKey(sshKey)
			if err != nil {
				return err
			}
			cfg.Proxy.SSHKey = finalKey

			fmt.Printf("代理端口 [%d]: ", cfg.Proxy.Port)
			port, _ := reader.ReadString('\n')
			if p := strings.TrimSpace(port); p != "" {
				var portNum int
				if _, err := fmt.Sscanf(p, "%d", &portNum); err != nil || portNum < 1 || portNum > 65535 {
					return fmt.Errorf("invalid port %q — must be a number between 1 and 65535", p)
				}
				cfg.Proxy.Port = portNum
			}

			// --- Step 2: SSH tunnel choice (BEFORE deploy so Squid is configured correctly) ---
			fmt.Printf("\n─── SSH 隧道 ───\n")
			fmt.Print("  启用 SSH 加密隧道? (中国用户推荐) [Y/n]: ")
			tunnelAns, _ := reader.ReadString('\n')
			tunnelAns = strings.TrimSpace(strings.ToLower(tunnelAns))
			if tunnelAns == "" || tunnelAns == "y" || tunnelAns == "yes" {
				cfg.Proxy.Tunnel = true
			} else {
				cfg.Proxy.Tunnel = false
				fmt.Println("  ⚠ 直连模式: Squid 将监听公网，仅靠 IP 白名单保护，无代理认证。")
				fmt.Println("    建议确保 ECS 安全组限制 Squid 端口访问。")
			}

			// --- Step 3: SSH host key verification ---
			fmt.Printf("\n─── 主机验证 ───\n")
			if err := verifyHostKey(cfg, reader); err != nil {
				return err
			}

			// --- Step 4: SSH connectivity check ---
			fmt.Printf("\n─── 连接检查 ───\n")
			fmt.Print("  → SSH 连接... ")
			if err := ecs.CheckSSH(cfg); err != nil {
				fmt.Println("✗")
				return fmt.Errorf("SSH connection failed: %w\n  Fix: check IP, user, and key path", err)
			}
			fmt.Println("✓")

			// --- Step 4: Deploy Squid (with tunnel choice already set) ---
			fmt.Printf("\n─── 服务器部署 ───\n")
			if err := ecs.Deploy(cfg); err != nil {
				return fmt.Errorf("deploy failed: %w", err)
			}

			// --- Step 5: Start SSH tunnel if enabled ---
			if cfg.Proxy.Tunnel {
				fmt.Printf("\n─── SSH 隧道 ───\n")
				fmt.Print("  → 建立 SSH 隧道... ")
				if _, err := tunnel.Start(cfg); err != nil {
					fmt.Println("✗")
					fmt.Printf("    Warning: %v\n", err)
					fmt.Println("    You can retry later: agent-proxy on")
				} else {
					fmt.Println("✓")
				}
			}

			// --- Step 6: Validate and save config ---
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("config validation: %w", err)
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("\n  ✓ 配置已保存: %s\n", config.ConfigPath())

			// --- Step 7: Enable proxy ---
			fmt.Printf("\n─── 本地启用 ───\n")
			if err := proxy.On(cfg); err != nil {
				return fmt.Errorf("enable proxy: %w", err)
			}

			// --- Step 8: Verify ---
			fmt.Printf("\n─── 连通性验证 ───\n")
			testDomains := []string{"google.com", "github.com", "youtube.com"}
			for _, d := range testDomains {
				fmt.Printf("  → %-20s ", d)
				ok, detail := quickTest(cfg, d)
				if ok {
					fmt.Printf("✓ %s\n", detail)
				} else {
					fmt.Printf("✗ %s\n", detail)
				}
			}

			fmt.Printf(`
  🎉 配置完成！
     将以下 source 语句加入 shell profile (~/.zshrc 或 ~/.bashrc) 后，新终端自动生效:
     [ -f "%s" ] && source "%s"
     当前终端执行: source %s
`, config.EnvPath(), config.EnvPath(), config.EnvPath())
			return nil
		},
	}
}

func quickTest(cfg *config.Config, domain string) (bool, string) {
	proxyURL := cfg.ProxyURL()
	cmd := exec.Command("curl", "-s", "--max-time", "8",
		"--proxy", proxyURL,
		"-o", "/dev/null",
		"-w", "%{http_code} %{time_total}s",
		"https://"+domain)
	out, err := cmd.Output()
	if err != nil {
		return false, "timeout or connection failed"
	}
	result := strings.TrimSpace(string(out))
	parts := strings.Fields(result)
	if len(parts) >= 1 && parts[0] == "200" {
		return true, result
	}
	if len(parts) >= 1 && (parts[0] == "403" || parts[0] == "301" || parts[0] == "302") {
		return true, result + " (reachable)"
	}
	return false, result
}

func validateSSHKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("SSH key not found: %s\n  Fix: check the path or generate a key: ssh-keygen -t ed25519", path)
	}

	// Check permissions
	if info.Mode().Perm()&0077 != 0 {
		fmt.Printf("  → 修复密钥权限 (0%o → 0600)... ", info.Mode().Perm())
		if err := os.Chmod(path, 0600); err != nil {
			fmt.Println("✗")
			return "", fmt.Errorf("cannot fix key permissions: %w", err)
		}
		fmt.Println("✓")
	}

	// Warn about TCC-protected directories on macOS
	home, _ := os.UserHomeDir()
	protected := []string{
		home + "/Documents/",
		home + "/Desktop/",
		home + "/Downloads/",
	}
	for _, dir := range protected {
		if strings.HasPrefix(path, dir) {
			dest := home + "/.ssh/" + info.Name()
			fmt.Printf("\n  ⚠ 密钥在 macOS 受保护目录 (%s)\n", dir)
			fmt.Printf("    后台服务 (LaunchAgent) 无法访问此目录。\n")
			fmt.Printf("    建议复制到: %s\n", dest)
			fmt.Printf("    自动复制? [Y/n]: ")
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
				fmt.Printf("    ✓ 已复制到 %s\n", dest)
				return dest, nil
			}
			break
		}
	}
	return path, nil
}

// verifyHostKey checks if the ECS host key is already trusted. If not,
// it fetches the key via ssh-keyscan, computes and displays the SHA256
// fingerprint, and requires explicit user confirmation before adding it
// to the project known_hosts.
func verifyHostKey(cfg *config.Config, reader *bufio.Reader) error {
	knownHosts := config.KnownHostsPath()
	os.MkdirAll(config.DataDir(), 0700)

	// Check if host is already in project known_hosts
	if data, err := os.ReadFile(knownHosts); err == nil {
		if strings.Contains(string(data), cfg.Proxy.Host) {
			fmt.Printf("  ✓ 主机 %s 已在 known_hosts 中\n", cfg.Proxy.Host)
			return nil
		}
	}

	// Check system known_hosts for migration
	home, _ := os.UserHomeDir()
	systemKH := filepath.Join(home, ".ssh", "known_hosts")
	if data, err := os.ReadFile(systemKH); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, cfg.Proxy.Host+" ") || strings.HasPrefix(line, cfg.Proxy.Host+",") {
				fmt.Printf("  → 在系统 known_hosts 中发现 %s 的密钥\n", cfg.Proxy.Host)
				fmt.Printf("  迁移到项目专用 known_hosts? [Y/n]: ")
				ans, _ := reader.ReadString('\n')
				ans = strings.TrimSpace(strings.ToLower(ans))
				if ans == "" || ans == "y" || ans == "yes" {
					f, err := os.OpenFile(knownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
					if err != nil {
						return fmt.Errorf("open known_hosts: %w", err)
					}
					fmt.Fprintf(f, "%s\n", line)
					f.Close()
					fmt.Printf("  ✓ 已迁移到 %s\n", knownHosts)
					return nil
				}
				break
			}
		}
	}

	// Fetch host key via ssh-keyscan
	fmt.Printf("  → 获取 %s 主机密钥... ", cfg.Proxy.Host)
	out, err := exec.Command("ssh-keyscan", "-T", "5", cfg.Proxy.Host).Output()
	if err != nil || len(out) == 0 {
		fmt.Println("✗")
		return fmt.Errorf("cannot fetch host key from %s — check network connectivity", cfg.Proxy.Host)
	}
	fmt.Println("✓")

	// Write to temp file and compute fingerprints
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

	fmt.Println("\n  主机密钥指纹:")
	for _, line := range strings.Split(strings.TrimSpace(string(fpOut)), "\n") {
		fmt.Printf("    %s\n", strings.TrimSpace(line))
	}

	fmt.Printf("\n  请核对以上 SHA256 指纹与 ECS 控制台一致。\n")
	fmt.Printf("  输入 yes 确认: ")
	ans, _ := reader.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans != "yes" {
		return fmt.Errorf("host key verification aborted — you must type 'yes' to accept")
	}

	// Append to project known_hosts
	f, err := os.OpenFile(knownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}
	fmt.Printf("  ✓ 已保存到 %s\n", knownHosts)
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

func cmdWhitelist() *cobra.Command {
	wl := &cobra.Command{Use: "whitelist", Aliases: []string{"wl"}, Short: "Manage custom domains"}
	wl.AddCommand(
		&cobra.Command{
			Use: "add <domain> [domain...]", Short: "Add custom domain(s)", Args: cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				added := 0
				for _, d := range args {
					if cfg.AddDomain(d) {
						added++
						fmt.Printf("  + %s\n", d)
					} else {
						fmt.Printf("  = %s (already exists)\n", d)
					}
				}
				if added > 0 {
					if err := cfg.Save(); err != nil {
						return fmt.Errorf("save config: %w", err)
					}
					if err := pac.Write(cfg); err != nil {
						return fmt.Errorf("regenerate PAC: %w", err)
					}
					fmt.Printf("\n✓ %d added, PAC regenerated\n", added)
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "rm <domain> [domain...]", Aliases: []string{"remove", "del"}, Short: "Remove custom domain(s)", Args: cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				removed := 0
				for _, d := range args {
					if cfg.RemoveDomain(d) {
						removed++
						fmt.Printf("  - %s\n", d)
					} else {
						fmt.Printf("  ? %s (not in custom; may be in a preset)\n", d)
					}
				}
				if removed > 0 {
					if err := cfg.Save(); err != nil {
						return fmt.Errorf("save config: %w", err)
					}
					if err := pac.Write(cfg); err != nil {
						return fmt.Errorf("regenerate PAC: %w", err)
					}
					fmt.Printf("\n✓ %d removed, PAC regenerated\n", removed)
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "ls", Aliases: []string{"list"}, Short: "List all effective domains",
			RunE: func(cmd *cobra.Command, args []string) error {
				wl := cfg.EffectiveWhitelist()
				fmt.Printf("Effective whitelist: %d domains\n\n", len(wl))
				for _, name := range config.PresetOrder {
					enabled := false
					for _, p := range cfg.Presets {
						if p == name {
							enabled = true
							break
						}
					}
					info := config.Presets[name]
					icon := "✓"
					if !enabled {
						icon = "✗"
					}
					fmt.Printf("[%s] %-10s %s (%d domains)\n", icon, name, info.Description, len(info.Domains))
				}
				if len(cfg.CustomDomains) > 0 {
					fmt.Printf("\n[✓] custom     User-added (%d):\n", len(cfg.CustomDomains))
					for _, d := range cfg.CustomDomains {
						fmt.Printf("             %s\n", d)
					}
				}
				return nil
			},
		},
	)
	return wl
}

func cmdPreset() *cobra.Command {
	p := &cobra.Command{Use: "preset", Short: "Manage domain presets"}
	p.AddCommand(
		&cobra.Command{
			Use: "ls", Aliases: []string{"list"}, Short: "List presets with domains",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Print("Available presets:\n\n")
				for _, name := range config.PresetOrder {
					info := config.Presets[name]
					enabled := false
					if cfg != nil {
						for _, p := range cfg.Presets {
							if p == name {
								enabled = true
								break
							}
						}
					}
					tag := "enabled"
					if !enabled {
						tag = "disabled"
					}
					fmt.Printf("  %-10s [%s] %s\n", name, tag, info.Description)
					for _, d := range info.Domains {
						fmt.Printf("             %s\n", d)
					}
					fmt.Println()
				}
			},
		},
		&cobra.Command{
			Use: "enable <name>", Short: "Enable a preset", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if cfg.EnablePreset(args[0]) {
					if err := cfg.Save(); err != nil {
						return err
					}
					if err := pac.Write(cfg); err != nil {
						return err
					}
					fmt.Printf("✓ Preset '%s' enabled\n", args[0])
				} else {
					fmt.Printf("Already enabled or unknown: %s\n", args[0])
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "disable <name>", Aliases: []string{"rm"}, Short: "Disable a preset", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if cfg.DisablePreset(args[0]) {
					if err := cfg.Save(); err != nil {
						return err
					}
					if err := pac.Write(cfg); err != nil {
						return err
					}
					fmt.Printf("✓ Preset '%s' disabled\n", args[0])
				} else {
					fmt.Printf("Not enabled: %s\n", args[0])
				}
				return nil
			},
		},
	)
	return p
}

func cmdSetup() *cobra.Command {
	return &cobra.Command{
		Use: "setup", Short: "Deploy Squid on ECS (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Proxy.Host == "" {
				return fmt.Errorf("proxy.host not set. Run 'agent-proxy init' first")
			}
			fmt.Printf("Deploying to %s:%d...\n\n", cfg.Proxy.Host, cfg.Proxy.Port)
			if err := ecs.Deploy(cfg); err != nil {
				return err
			}
			fmt.Print("\n✓ Setup complete. Next: agent-proxy on\n")
			return nil
		},
	}
}

func cmdIP() *cobra.Command {
	return &cobra.Command{
		Use: "ip", Short: "Refresh Squid IP whitelist (direct mode only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Proxy.Tunnel {
				fmt.Println("Tunnel mode: Squid listens on 127.0.0.1 only — no IP whitelist needed.")
				fmt.Println("Your proxy traffic is encrypted via SSH; no public Squid port is exposed.")
				return nil
			}
			return ecs.RefreshIP(cfg)
		},
	}
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("agent-proxy %s\n", version)
		},
	}
}

func cmdUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update agent-proxy to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Updating agent-proxy...")
			// Use the install script from the current version's release tag,
			// not from the mutable main branch, to avoid executing unverified code.
			tag := version
			if tag == "dev" || tag == "" {
				tag = "main" // dev builds fall back to main
			} else if !strings.HasPrefix(tag, "v") {
				tag = "v" + tag // GoReleaser strips the v prefix; git tags have it
			}
			scriptURL := fmt.Sprintf(
				"https://raw.githubusercontent.com/chiga0/agent-proxy/%s/install.sh", tag)
			c := exec.Command("bash", "-c",
				fmt.Sprintf("curl -fsSL %s | bash", scriptURL))
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func cmdTrustHost() *cobra.Command {
	return &cobra.Command{
		Use:   "trust-host",
		Short: "Verify and trust the ECS host key (for upgrades or key rotation)",
		Long: `Fetches the ECS host key, displays its SHA256 fingerprint for
verification against the ECS console, and adds it to the project-specific
known_hosts file. Run this after upgrading from a version that used
~/.ssh/known_hosts, or after ECS host key rotation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			return verifyHostKey(cfg, reader)
		},
	}
}

func cmdStats() *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show proxy traffic statistics from Squid access logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Proxy.Host == "" {
				return fmt.Errorf("proxy.host not configured")
			}
			fmt.Printf("Fetching last %d log entries from %s...\n\n", lines, cfg.Proxy.Host)
			logText, err := ecs.FetchRecentLogs(cfg, lines)
			if err != nil {
				return fmt.Errorf("fetch logs: %w", err)
			}
			entries := ecs.ParseLogLines(logText)
			if len(entries) == 0 {
				fmt.Println("No log entries found.")
				return nil
			}

			// Summary
			var totalBytes int64
			for _, e := range entries {
				totalBytes += e.Bytes
			}
			fmt.Printf("  Requests: %d\n", len(entries))
			fmt.Printf("  Total traffic: %s\n\n", ecs.FormatBytes(totalBytes))

			// Top domains by traffic
			stats := ecs.AggregateByDomain(entries)
			top := 15
			if len(stats) < top {
				top = len(stats)
			}
			fmt.Printf("  Top %d domains by traffic:\n", top)
			fmt.Printf("  %-45s %8s %8s\n", "Domain", "Requests", "Traffic")
			fmt.Printf("  %-45s %8s %8s\n", strings.Repeat("-", 45), strings.Repeat("-", 8), strings.Repeat("-", 8))
			for i := 0; i < top; i++ {
				s := stats[i]
				marker := ""
				if ecs.LooksChinese(s.Domain) {
					marker = " 🇳"
				}
				fmt.Printf("  %-45s %8d %8s%s\n", s.Domain, s.Requests, ecs.FormatBytes(s.Bytes), marker)
			}

			// Chinese domains summary
			var cnBytes int64
			var cnReqs int
			for _, e := range entries {
				if ecs.LooksChinese(e.Domain) {
					cnBytes += e.Bytes
					cnReqs++
				}
			}
			if cnReqs > 0 {
				pct := float64(cnBytes) / float64(totalBytes) * 100
				fmt.Printf("\n  🇨 Chinese traffic: %d requests, %s (%.0f%% of total)\n", cnReqs, ecs.FormatBytes(cnBytes), pct)
				if pct > 10 {
					fmt.Println("  ⚠ High Chinese traffic through proxy — check no_proxy configuration")
					fmt.Println("    Run: agent-proxy doctor")
				}
			}

			return nil
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 1000, "Number of log lines to analyze")
	return cmd
}

func cmdConfigValidate() *cobra.Command {
	return &cobra.Command{
		Use:   "config-validate",
		Short: "Validate config file without starting proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config load: %w", err)
			}
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("config validation: %w", err)
			}
			fmt.Printf("✓ Config valid: %s\n", config.ConfigPath())
			fmt.Printf("  Host: %s:%d\n", cfg.Proxy.Host, cfg.Proxy.Port)
			fmt.Printf("  Tunnel: %v\n", cfg.Proxy.Tunnel)
			fmt.Printf("  Presets: %v\n", cfg.Presets)
			fmt.Printf("  Whitelist: %d domains\n", len(cfg.EffectiveWhitelist()))
			fmt.Printf("  No-proxy: %d entries\n", len(cfg.NoProxy))
			if cfg.Proxy.HasFallback() {
				fmt.Printf("  Fallback: %s\n", cfg.Proxy.FallbackHost)
			}
			return nil
		},
	}
}

func cmdServePAC() *cobra.Command {
	return &cobra.Command{
		Use:    "serve-pac",
		Hidden: true,
		Short:  "Run PAC HTTP server (used internally)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pac.ServeForeground()
		},
	}
}

func cmdBench() *cobra.Command {
	var runs int
	cmd := &cobra.Command{
		Use:   "bench [domain...]",
		Short: "Benchmark proxy latency (TTFB, total RT)",
		Long: `Measure request latency through the proxy vs direct connection.
If no domains specified, tests a default set of AI endpoints.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			domains := args
			if len(domains) == 0 {
				domains = []string{"chatgpt.com", "openai.com", "api.anthropic.com", "github.com"}
			}
			fmt.Printf("Benchmarking %d domains × %d runs (proxy vs direct)...\n", len(domains), runs)
			results := bench.Run(cfg, domains, runs)
			bench.PrintResults(results)
			return nil
		},
	}
	cmd.Flags().IntVarP(&runs, "runs", "n", 3, "Number of requests per domain per mode")
	return cmd
}

func cmdTrace() *cobra.Command {
	return &cobra.Command{
		Use:   "trace [domain]",
		Short: "Network path trace: local → ECS → target",
		Long: `Trace the full network path from your machine to the ECS proxy
and from the ECS to the target domain. Useful for diagnosing
latency issues and identifying routing bottlenecks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "chatgpt.com"
			if len(args) > 0 {
				target = args[0]
			}

			fmt.Println("=== Network Trace ===")

			// 1. DNS resolution
			fmt.Printf("\n--- DNS Resolution ---\n")
			for _, d := range []string{cfg.Proxy.Host, target} {
				ip, dur, err := trace.DNSInfo(d)
				if err != nil {
					fmt.Printf("  %-25s ERROR: %v\n", d, err)
				} else {
					fmt.Printf("  %-25s → %-16s (%s)\n", d, ip, dur.Round(time.Millisecond))
				}
			}

			// 2. Local → ECS
			fmt.Printf("\n--- Local → ECS (%s) ---\n", cfg.Proxy.Host)
			r1 := trace.LocalToECS(cfg)
			trace.PrintTrace(r1)

			// 3. ECS → Target
			fmt.Printf("\n--- ECS → %s ---\n", target)
			r2 := trace.ECSToTarget(cfg, target)
			trace.PrintTrace(r2)

			fmt.Println()
			return nil
		},
	}
}
