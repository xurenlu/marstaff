package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNodeRegistry_ListEmpty(t *testing.T) {
	r := NewNodeRegistry()
	if n := len(r.List("u1")); n != 0 {
		t.Fatalf("expected 0 nodes, got %d", n)
	}
}

func TestNodeRegistry_InvokeNotConnected(t *testing.T) {
	r := NewNodeRegistry()
	_, err := r.Invoke(context.Background(), "u1", "missing", "m", nil, time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNodeRegistry_InvokeRoundTrip(t *testing.T) {
	r := NewNodeRegistry()
	c := &Client{Send: make(chan []byte, 2), UserID: "u1"}
	r.Register("u1", "n1", "d", nil, c)
	defer r.Remove("u1", "n1")

	done := make(chan struct{})
	go func() {
		var raw []byte
		select {
		case raw = <-c.Send:
		case <-time.After(time.Second):
			t.Error("no invoke sent")
			close(done)
			return
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			close(done)
			return
		}
		if msg.InvokeID == "" {
			t.Error("empty invoke_id")
		}
		r.CompleteInvoke(msg.InvokeID, true, json.RawMessage(`{"ok":true}`), "")
		close(done)
	}()

	raw, err := r.Invoke(context.Background(), "u1", "n1", "m", nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("unexpected %s", raw)
	}
	<-done
}
