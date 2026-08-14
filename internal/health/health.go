package health

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/platform"
	"github.com/chiga0/agent-proxy/internal/tunnel"
)

const (
	checkInterval    = 5 * time.Second
	failureThreshold = 2
	probeURL         = "http://www.google.com/generate_204"
	probeTimeout     = 3 * time.Second
	maxBackoff       = 10 * time.Minute
)

// State exposes health status for metrics/dashboard.
var (
	consecutiveFailures atomic.Int64
	lastCheckOK         atomic.Bool
	lastRecovery        atomic.Int64 // unix timestamp of last recovery, 0 = never
	recoveryFailures    atomic.Int64 // consecutive failed recovery attempts
)

func ConsecutiveFailures() int64 { return consecutiveFailures.Load() }
func LastCheckOK() bool          { return lastCheckOK.Load() }
func LastRecovery() int64        { return lastRecovery.Load() }

// Watch runs the health check loop. Blocking — call in a goroutine.
// Stops when ctx is cancelled. Only active when tunnel mode is enabled.
func Watch(ctx context.Context) {
	backoff := checkInterval

	for {
		select {
		case <-ctx.Done():
			log.Printf("[health] stopped")
			return
		case <-time.After(backoff):
		}

		cfg, err := config.Load()
		if err != nil || !cfg.Proxy.Tunnel {
			backoff = checkInterval
			continue
		}

		if probe(cfg) {
			if consecutiveFailures.Load() > 0 {
				log.Printf("[health] proxy recovered after %d consecutive failures", consecutiveFailures.Load())
			}
			consecutiveFailures.Store(0)
			recoveryFailures.Store(0)
			lastCheckOK.Store(true)
			backoff = checkInterval
			continue
		}

		n := consecutiveFailures.Add(1)
		lastCheckOK.Store(false)
		log.Printf("[health] proxy check failed (%d/%d consecutive)", n, failureThreshold)

		if n >= failureThreshold {
			recoverTunnel(cfg)
			// Back off exponentially after failed recovery: churning ssh
			// reconnects against a hostile environment (e.g. endpoint-security
			// strict period) only extends the strict period.
			if rf := recoveryFailures.Load(); rf > 0 {
				shift := rf
				if shift > 11 {
					shift = 11
				}
				backoff = checkInterval << uint(shift)
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				log.Printf("[health] backing off %v after %d failed recovery attempts", backoff, rf)
			} else {
				backoff = checkInterval
			}
		}
	}
}

func probe(cfg *config.Config) bool {
	proxyURL, err := url.Parse(cfg.ProxyURL())
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout:   probeTimeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get(probeURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 204
}

func recoverTunnel(cfg *config.Config) {
	// When an OS supervisor (launchd / systemd / scheduled task) owns the
	// tunnel process, it respawns ssh on death. Starting our own instance
	// here would race that respawn and can produce split-brain listeners
	// (one ssh bound to IPv4, another to IPv6, neither fully functional).
	// Only clean stale sockets and kill the broken process; the supervisor
	// brings the tunnel back.
	if platform.IsAutoStartInstalled() {
		log.Printf("[health] killing broken tunnel (supervisor will respawn)...")
		tunnel.KillForRestart(cfg)
		consecutiveFailures.Store(0)
		lastRecovery.Store(time.Now().Unix())
		// Circuit breaker: if the respawned tunnel is still broken, the
		// environment is killing sessions faster than we recover; back off
		// instead of churning ssh connections.
		time.Sleep(5 * time.Second)
		if probe(cfg) {
			recoveryFailures.Store(0)
			log.Printf("[health] supervisor respawn healthy")
		} else {
			rf := recoveryFailures.Add(1)
			log.Printf("[health] respawn still broken (%d consecutive) — backing off", rf)
		}
		return
	}

	log.Printf("[health] attempting tunnel restart...")

	tunnel.Stop(cfg)
	time.Sleep(time.Second)

	started, err := tunnel.Start(cfg)
	if err != nil {
		n := recoveryFailures.Add(1)
		log.Printf("[health] tunnel restart failed (%d consecutive): %v", n, err)
		return
	}

	if started {
		log.Printf("[health] tunnel restarted successfully")
	} else {
		log.Printf("[health] tunnel was already running")
	}
	consecutiveFailures.Store(0)
	recoveryFailures.Store(0)
	lastRecovery.Store(time.Now().Unix())

	// Verify the recovery actually fixed things
	time.Sleep(2 * time.Second)
	if probe(cfg) {
		log.Printf("[health] post-recovery check passed")
		recoveryFailures.Store(0)
	} else {
		rf := recoveryFailures.Add(1)
		log.Printf("[health] post-recovery check still failing (%d consecutive) — backing off", rf)
	}
}
