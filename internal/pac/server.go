package pac

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

func ServerRunning() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", config.PACPort), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func StartServer() error {
	if ServerRunning() {
		return nil
	}

	dir := config.DataDir()
	cmd := exec.Command("python3", "-m", "http.server",
		strconv.Itoa(config.PACPort), "--bind", "127.0.0.1")
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start PAC HTTP server: %w", err)
	}

	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if ServerRunning() {
			return nil
		}
	}
	return fmt.Errorf("PAC HTTP server did not start within 1s")
}

func StopServer() {
	out, err := exec.Command("pgrep", "-f",
		fmt.Sprintf("python3.*http.server.*%d", config.PACPort)).Output()
	if err != nil {
		return
	}
	for _, pid := range strings.Fields(string(out)) {
		exec.Command("kill", pid).Run()
	}
}

func ServeOnce(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(config.PACPath())
		if err != nil {
			http.Error(w, "PAC not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Write(data)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", config.PACPort),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
