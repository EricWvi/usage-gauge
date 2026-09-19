package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// codex serializes RPC exchanges. A failed exchange discards the process so a
// late response cannot be mistaken for the next HTTP request's response.
type codex struct {
	binary  string
	dir     string
	gate    chan struct{}
	process *codexProcess
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type incoming struct {
	response rpcResponse
	err      error
}

type codexProcess struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	messages chan incoming
	done     chan struct{}
	nextID   int
}

func newCodex(binary, dir string) *codex {
	return &codex{binary: binary, dir: dir, gate: make(chan struct{}, 1)}
}

func (c *codex) read(ctx context.Context) (json.RawMessage, error) {
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.process == nil {
		p, err := startCodex(ctx, c.binary, c.dir)
		if err != nil {
			return nil, err
		}
		c.process = p
	}
	result, err := c.process.call(ctx, "account/rateLimits/read", nil)
	if err != nil {
		c.process.close()
		c.process = nil
	}
	return result, err
}

func (c *codex) close() {
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	if c.process != nil {
		c.process.close()
		c.process = nil
	}
}

func startCodex(ctx context.Context, binary, dir string) (*codexProcess, error) {
	cmd := exec.Command(binary, "app-server", "--listen", "stdio://")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("start codex: %w", err)
	}
	p := &codexProcess{cmd: cmd, stdin: stdin, stdout: stdout, messages: make(chan incoming), done: make(chan struct{})}
	go func() {
		decoder := json.NewDecoder(stdout)
		for {
			var msg rpcResponse
			err := decoder.Decode(&msg)
			select {
			case p.messages <- incoming{msg, err}:
			case <-p.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	_, err = p.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "usage-gauge", "version": "0.1.0"},
	})
	if err == nil {
		err = json.NewEncoder(stdin).Encode(map[string]any{"method": "initialized"})
	}
	if err != nil {
		p.close()
		return nil, fmt.Errorf("initialize codex: %w", err)
	}
	return p, nil
}

func (p *codexProcess) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	p.nextID++
	request := map[string]any{"id": p.nextID, "method": method}
	if params != nil {
		request["params"] = params
	}
	if err := json.NewEncoder(p.stdin).Encode(request); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg := <-p.messages:
			if msg.err != nil {
				return nil, fmt.Errorf("read codex: %w", msg.err)
			}
			if msg.response.ID != p.nextID {
				continue
			} // Ignore notifications.
			if msg.response.Error != nil {
				return nil, fmt.Errorf("codex RPC %d: %s", msg.response.Error.Code, msg.response.Error.Message)
			}
			if len(msg.response.Result) == 0 {
				return nil, fmt.Errorf("codex returned no result")
			}
			return msg.response.Result, nil
		}
	}
}

func (p *codexProcess) close() {
	close(p.done)
	_ = p.stdin.Close()
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
	_ = p.stdout.Close()
}
