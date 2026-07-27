package main

import (
	"fmt"
	"os"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/ecs"
	"github.com/chiga0/agent-proxy/internal/pac"
	"github.com/chiga0/agent-proxy/internal/proxy"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	root := &cobra.Command{
		Use:   "agent-proxy",
		Short: "Domain-based selective proxy routing via overseas ECS",
		Long: `agent-proxy routes whitelisted domains through an overseas proxy server
while keeping all other traffic direct. It uses PAC for browsers/desktop apps
and environment variables for CLI tools.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "version" {
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
		cmdOn(),
		cmdOff(),
		cmdStatus(),
		cmdWhitelist(),
		cmdSetup(),
		cmdIP(),
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

func cmdWhitelist() *cobra.Command {
	wl := &cobra.Command{
		Use:     "whitelist",
		Aliases: []string{"wl"},
		Short:   "Manage domain whitelist",
	}

	wl.AddCommand(
		&cobra.Command{
			Use:   "add <domain> [domain...]",
			Short: "Add domain(s) to whitelist",
			Args:  cobra.MinimumNArgs(1),
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
						return err
					}
					if err := pac.Write(cfg); err != nil {
						return err
					}
					fmt.Printf("\n✓ %d domain(s) added, PAC regenerated\n", added)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:     "rm <domain> [domain...]",
			Aliases: []string{"remove", "del", "delete"},
			Short:   "Remove domain(s) from whitelist",
			Args:    cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				removed := 0
				for _, d := range args {
					if cfg.RemoveDomain(d) {
						removed++
						fmt.Printf("  - %s\n", d)
					} else {
						fmt.Printf("  ? %s (not found)\n", d)
					}
				}
				if removed > 0 {
					if err := cfg.Save(); err != nil {
						return err
					}
					if err := pac.Write(cfg); err != nil {
						return err
					}
					fmt.Printf("\n✓ %d domain(s) removed, PAC regenerated\n", removed)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:     "ls",
			Aliases: []string{"list"},
			Short:   "List whitelisted domains",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("Whitelist (%d domains):\n", len(cfg.Whitelist))
				for _, d := range cfg.Whitelist {
					fmt.Printf("  %s\n", d)
				}
				return nil
			},
		},
	)

	return wl
}

func cmdSetup() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Deploy Squid proxy on ECS (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Proxy.Host == "" {
				return fmt.Errorf("proxy.host not configured. Edit %s first", config.ConfigPath())
			}
			fmt.Printf("Deploying to %s:%d...\n\n", cfg.Proxy.Host, cfg.Proxy.Port)
			if err := ecs.Deploy(cfg); err != nil {
				return err
			}
			fmt.Printf("\n✓ Setup complete\n")
			fmt.Printf("  Proxy: %s:%d\n", cfg.Proxy.Host, cfg.Proxy.Port)
			fmt.Printf("  User:  %s\n", cfg.Proxy.User)
			fmt.Printf("\nNext: agent-proxy on\n")
			return nil
		},
	}
}

func cmdIP() *cobra.Command {
	ip := &cobra.Command{
		Use:   "ip",
		Short: "Manage IP whitelist on Squid",
	}

	ip.AddCommand(&cobra.Command{
		Use:   "refresh",
		Short: "Update Squid trusted IP to current public IP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ecs.RefreshIP(cfg)
		},
	})

	return ip
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("agent-proxy v0.1.0")
		},
	}
}
