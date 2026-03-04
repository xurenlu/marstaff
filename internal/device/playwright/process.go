package playwright

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultIdleTimeout = 5 * time.Minute
)

// Process manages the Playwright Node.js sidecar lifecycle.
type Process struct {
	sidecarDir string
	nodePath   string
	idleTimeout time.Duration

	mu       sync.Mutex
	cmd      *exec.Cmd
	client   *Client
	lastUsed time.Time
	stopIdle chan struct{}
}

// NewProcess creates a new sidecar process manager.
func NewProcess(sidecarDir, nodePath string) *Process {
	if sidecarDir == "" {
		sidecarDir = "playwright-sidecar"
	}
	if nodePath == "" {
		nodePath = "node"
	}
	return &Process{
		sidecarDir:  sidecarDir,
		nodePath:    nodePath,
		idleTimeout: defaultIdleTimeout,
	}
}

// SetIdleTimeout sets how long to wait before killing the sidecar when idle.
func (p *Process) SetIdleTimeout(d time.Duration) {
	p.idleTimeout = d
}

// Client returns a connected JSON-RPC client. Starts the sidecar if needed.
func (p *Process) Client(ctx context.Context) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		// Process still running if ProcessState is nil (not yet exited)
		if p.cmd != nil && p.cmd.Process != nil && p.cmd.ProcessState == nil {
			p.lastUsed = time.Now()
			p.resetIdleTimer()
			return p.client, nil
		}
		p.cleanupLocked()
	}

	absDir, err := filepath.Abs(p.sidecarDir)
	if err != nil {
		return nil, fmt.Errorf("resolve sidecar dir: %w", err)
	}
	serverPath := filepath.Join(absDir, "server.js")
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("playwright sidecar not found at %s (run: cd playwright-sidecar && npm install)", serverPath)
	}

	cmd := exec.CommandContext(ctx, p.nodePath, "server.js")
	cmd.Dir = absDir
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sidecar: %w", err)
	}

	client := NewClient(stdin, stdout)
	p.cmd = cmd
	p.client = client
	p.lastUsed = time.Now()
	p.resetIdleTimer()

	log.Info().Str("dir", absDir).Msg("playwright sidecar started")

	return client, nil
}

func (p *Process) resetIdleTimer() {
	if p.stopIdle != nil {
		select {
		case p.stopIdle <- struct{}{}:
		default:
		}
	}
	p.stopIdle = make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-p.stopIdle:
				return
			case <-ticker.C:
				p.mu.Lock()
				if p.client != nil && time.Since(p.lastUsed) > p.idleTimeout {
					log.Info().Msg("playwright sidecar idle timeout, shutting down")
					p.cleanupLocked()
					p.mu.Unlock()
					return
				}
				p.mu.Unlock()
			}
		}
	}()
}

func (p *Process) cleanupLocked() {
	if p.stopIdle != nil {
		close(p.stopIdle)
		p.stopIdle = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		p.cmd = nil
	}
	p.client = nil
}

// Stop kills the sidecar process.
func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupLocked()
}
