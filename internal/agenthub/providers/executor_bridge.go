package providers

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/ZihengXiong/GenMult/internal/workspace/bridge"
	pb "github.com/ZihengXiong/GenMult/internal/workspace/bridgepb"
)

type BridgeExecutor struct {
	client *bridge.Client
}

func NewBridgeExecutor(client *bridge.Client) *BridgeExecutor {
	return &BridgeExecutor{client: client}
}

func (e *BridgeExecutor) LookPath(bin string) (string, error) {
	return bin, nil
}

func (e *BridgeExecutor) Start(ctx context.Context, req ExecRequest) (ExecHandle, error) {
	timeout := int32(0)
	if req.Timeout > 0 {
		timeout = int32(req.Timeout.Seconds())
	}

	command := req.Bin
	if len(req.Args) > 0 {
		var args []string
		for _, arg := range req.Args {
			args = append(args, "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'")
		}
		command += " " + strings.Join(args, " ")
	}

	env := append(os.Environ(), req.Env...)
	stream, err := e.client.ExecStream(ctx, command, req.WorkDir, timeout, env)
	if err != nil {
		return nil, err
	}

	if req.Stdin != "" {
		_ = stream.SendStdin([]byte(req.Stdin))
	}

	chunksChan := make(chan ExecChunk, 100)
	handle := &bridgeExecHandle{
		stream:     stream,
		chunksChan: chunksChan,
	}

	go handle.receiveLoop()

	return handle, nil
}

type bridgeExecHandle struct {
	stream     *bridge.ExecStream
	chunksChan chan ExecChunk
	exitCode   int
	err        error
	wg         sync.WaitGroup
}

func (h *bridgeExecHandle) receiveLoop() {
	h.wg.Add(1)
	defer h.wg.Done()
	defer close(h.chunksChan)

	for {
		out, err := h.stream.Recv()
		if err != nil {
			if err != io.EOF {
				h.err = err
			}
			break
		}

		switch out.Stream {
		case pb.ExecOutput_STDOUT:
			h.chunksChan <- ExecChunk{Stream: "stdout", Data: out.Data}
		case pb.ExecOutput_STDERR:
			h.chunksChan <- ExecChunk{Stream: "stderr", Data: out.Data}
		case pb.ExecOutput_EXIT:
			h.exitCode = int(out.ExitCode)
		}
	}
}

func (h *bridgeExecHandle) Chunks() <-chan ExecChunk {
	return h.chunksChan
}

func (h *bridgeExecHandle) Wait() (int, error) {
	h.wg.Wait()
	if h.err != nil {
		return h.exitCode, h.err
	}
	if h.exitCode != 0 {
		return h.exitCode, fmt.Errorf("exit status %d", h.exitCode)
	}
	return 0, nil
}
