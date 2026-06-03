package providers

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// ExecRequest controls the subprocess execution configuration.
type ExecRequest struct {
	Bin     string
	Args    []string
	WorkDir string
	Stdin   string
	Env     []string      // Environment variables, e.g. API keys.
	Timeout time.Duration
}

// ExecChunk represents a chunk of bytes outputted by the execution process.
type ExecChunk struct {
	Stream string // "stdout" or "stderr".
	Data   []byte
}

// ExecHandle manages the running execution process.
type ExecHandle interface {
	Chunks() <-chan ExecChunk // Stream output channel.
	Wait() (exitCode int, err error)
}

// CommandExecutor abstracts the process runner (local host vs container sandboxed).
type CommandExecutor interface {
	LookPath(bin string) (string, error)
	Start(ctx context.Context, req ExecRequest) (ExecHandle, error)
}

// HostExecutor implements CommandExecutor running processes on the local host.
type HostExecutor struct{}

// NewHostExecutor creates a new HostExecutor.
func NewHostExecutor() *HostExecutor {
	return &HostExecutor{}
}

// LookPath searches for an executable binary in the host's PATH.
func (e *HostExecutor) LookPath(bin string) (string, error) {
	return exec.LookPath(bin)
}

// Start spawns a command locally and streams output.
func (e *HostExecutor) Start(ctx context.Context, req ExecRequest) (ExecHandle, error) {
	binPath, err := exec.LookPath(req.Bin)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binPath, req.Args...)
	cmd.Dir = req.WorkDir
	cmd.Env = req.Env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}

	chunksChan := make(chan ExecChunk, 100)
	handle := &hostExecHandle{
		cmd:        cmd,
		chunksChan: chunksChan,
	}

	handle.wg.Add(2)

	// Stream stdout.
	go func() {
		defer handle.wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				chunksChan <- ExecChunk{Stream: "stdout", Data: data}
			}
			if err != nil {
				break
			}
		}
	}()

	// Stream stderr.
	go func() {
		defer handle.wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				chunksChan <- ExecChunk{Stream: "stderr", Data: data}
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait for process and close channel in background.
	go func() {
		_, _ = handle.Wait()
	}()

	return handle, nil
}

type hostExecHandle struct {
	cmd        *exec.Cmd
	chunksChan chan ExecChunk
	wg         sync.WaitGroup
	waitErr    error
	waitOnce   sync.Once
}

// Chunks returns the channel where stdout and stderr are pushed.
func (h *hostExecHandle) Chunks() <-chan ExecChunk {
	return h.chunksChan
}

// Wait waits for completion of reading streams and process termination.
func (h *hostExecHandle) Wait() (int, error) {
	h.wg.Wait()
	h.waitOnce.Do(func() {
		h.waitErr = h.cmd.Wait()
		close(h.chunksChan)
	})
	if h.waitErr != nil {
		if exitErr, ok := h.waitErr.(*exec.ExitError); ok {
			return exitErr.ExitCode(), h.waitErr
		}
		return -1, h.waitErr
	}
	return 0, nil
}
