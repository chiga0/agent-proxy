package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/bench"
	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/ecs"
	"github.com/chiga0/agent-proxy/internal/pac"
	"github.com/chiga0/agent-proxy/internal/proxy"
	"github.com/chiga0/agent-proxy/internal/trace"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	root := &cobra.Command{
		Use:   "agent-proxy",
		Short: "Domain-based selective proxy routing via overseas ECS",
		Long: `agent-proxy routes whitelisted domains through an overseas proxy server
while keeping all other traffic direct.

Built-in presets (ai, dev, search, cloud) are enabled by default —
zero configuration needed for common use cases.

Quick start:
  agent-proxy init          Interactive setup
  agent-proxy on            Enable proxy
  agent-proxy status        Check health
  agent-proxy doctor        Full diagnostics`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			skip := map[string]bool{"version": true, "init": true}
			if skip[cmd.Name()] {
				return nil
			}
			var err error
			cfg, err = config.Load()
			return err
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		cmdOn(), cmdOff(), cmdStatus(), cmdDoctor(),
		cmdInit(), cmdWhitelist(), cmdPreset(),
		cmdSetup(), cmdIP(), cmdBench(), cmdTrace(),
		cmdVersion(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdOn() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Enable proxy (PAC + CLI env vars)",
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
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run full diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print("=== agent-proxy doctor ===\n\n")
			fmt.Printf("Config:     %s\n", config.ConfigPath())
			fmt.Printf("Presets:    %v\n", cfg.Presets)
			wl := cfg.EffectiveWhitelist()
			fmt.Printf("Whitelist:  %d domains (%d from presets, %d custom)\n",
				len(wl), len(wl)-len(cfg.CustomDomains), len(cfg.CustomDomains))
			fmt.Printf("No-proxy:   %d entries\n", len(cfg.NoProxy))
			fmt.Printf("Proxy:      %s:%d\n\n", cfg.Proxy.Host, cfg.Proxy.Port)
			fmt.Println("--- Connectivity ---")
			results := proxy.Status(cfg)
			proxy.PrintStatus(results)
			allOK := true
			for _, r := range results {
				if !r.OK {
					allOK = false
				}
			}
			fmt.Println()
			if allOK {
				fmt.Println("✓ Everything looks good!")
			} else {
				fmt.Println("✗ Some checks failed. Run 'agent-proxy on' or check your config.")
				os.Exit(1)
			}
			return nil
		},
	}
}

func cmdInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive first-time setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("=== agent-proxy init ===\n\n")
			cfg = config.DefaultConfig()

			fmt.Print("ECS host (IP or hostname): ")
			host, _ := reader.ReadString('\n')
			cfg.Proxy.Host = strings.TrimSpace(host)

			fmt.Printf("Proxy port [%d]: ", cfg.Proxy.Port)
			port, _ := reader.ReadString('\n')
			if p := strings.TrimSpace(port); p != "" {
				fmt.Sscanf(p, "%d", &cfg.Proxy.Port)
			}

			fmt.Print("Proxy username: ")
			user, _ := reader.ReadString('\n')
			cfg.Proxy.User = strings.TrimSpace(user)

			fmt.Print("Proxy password: ")
			pass, _ := reader.ReadString('\n')
			cfg.Proxy.Password = strings.TrimSpace(pass)

			fmt.Print("SSH key path (optional): ")
			sshKey, _ := reader.ReadString('\n')
			cfg.Proxy.SSHKey = strings.TrimSpace(sshKey)

			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("\n✓ Config saved to %s\n", config.ConfigPath())
			fmt.Printf("  Presets: %v (%d domains)\n", cfg.Presets, len(cfg.EffectiveWhitelist()))
			fmt.Print("\nNext: agent-proxy setup → agent-proxy on → agent-proxy doctor")
			return nil
		},
	}
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
					cfg.Save()
					pac.Write(cfg)
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
					cfg.Save()
					pac.Write(cfg)
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
					cfg.Save()
					pac.Write(cfg)
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
					cfg.Save()
					pac.Write(cfg)
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
		Use: "ip", Short: "Refresh Squid IP whitelist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ecs.RefreshIP(cfg)
		},
	}
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("agent-proxy v0.3.0")
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
