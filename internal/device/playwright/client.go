package playwright

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Client is a JSON-RPC 2.0 client over stdio.
type Client struct {
	stdin  io.Writer
	stdout io.Reader
	enc   *json.Encoder
	scan  *bufio.Scanner
	mu    sync.Mutex
	id    atomic.Int64
}

// NewClient creates a new JSON-RPC client.
func NewClient(stdin io.Writer, stdout io.Reader) *Client {
	return &Client{
		stdin:  stdin,
		stdout: stdout,
		enc:    json.NewEncoder(stdin),
		scan:   bufio.NewScanner(stdout),
	}
}

// jsonrpcRequest is the outgoing request format.
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcResponse is the incoming response format.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Call invokes a JSON-RPC method and decodes the result into v.
func (c *Client) Call(ctx context.Context, method string, params interface{}, v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.id.Add(1)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if params == nil {
		req.Params = map[string]interface{}{}
	}

	if err := c.enc.Encode(req); err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	// Read response with context
	done := make(chan struct{})
	var resp jsonrpcResponse
	var scanErr error
	go func() {
		defer close(done)
		if c.scan.Scan() {
			scanErr = json.Unmarshal(c.scan.Bytes(), &resp)
		} else {
			if c.scan.Err() != nil {
				scanErr = c.scan.Err()
			} else {
				scanErr = io.EOF
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		if scanErr != nil {
			return fmt.Errorf("read response: %w", scanErr)
		}
	}

	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	if v != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, v); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}
