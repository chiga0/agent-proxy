package health

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/tunnel"
)

const (
	checkInterval    = 30 * time.Second
	failureThreshold = 3
	probeURL         = "http://www.google.com/generate_204"
	probeTimeout     = 10 * time.Second
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

// probeClient is reused across checks to avoid per-interval Transport allocation.
var probeClient = &http.Client{Timeout: probeTimeout}

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
			// Back off after failed recovery to avoid infinite SSH retry loop
			if rf := recoveryFailures.Load(); rf > 0 {
				backoff = checkInterval * time.Duration(rf)
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
	probeClient.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	resp, err := probeClient.Get(probeURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 204
}

func recoverTunnel(cfg *config.Config) {
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
	} else {
		log.Printf("[health] post-recovery check still failing — may need manual intervention")
	}
}
