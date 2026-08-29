//go:build windows

package ipc

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"
)

func uniqueName(t *testing.T) string {
	t.Helper()
	var b [8]byte
	_, _ = rand.Read(b[:])
	return `\\.\pipe\jxleet-test-` + hex.EncodeToString(b[:])
}

func TestSendNoOwner(t *testing.T) {
	sent, err := SendName(uniqueName(t), Message{Paths: []string{"x"}}, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent {
		t.Error("send should report false when no owner is listening")
	}
}

func TestAcquireBecomesOwner(t *testing.T) {
	name := uniqueName(t)
	srv, handedOver, err := AcquireName(name, Message{}, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if handedOver {
		t.Error("first process should become the owner, not hand over")
	}
	if srv == nil {
		t.Fatal("expected a server")
	}
}

func TestHandoverAndCoalesce(t *testing.T) {
	name := uniqueName(t)
	srv, handedOver, err := AcquireName(name, Message{}, 500*time.Millisecond)
	if err != nil || handedOver {
		t.Fatalf("acquire owner: handedOver=%v err=%v", handedOver, err)
	}
	defer srv.Close()

	var mu sync.Mutex
	var got [][]string
	done := make(chan struct{})
	go func() {
		srv.Serve(func(m Message) {
			mu.Lock()
			got = append(got, m.Paths)
			n := len(got)
			mu.Unlock()
			if n == 3 {
				close(done)
			}
		})
	}()

	// A second invocation must hand over rather than become a second owner.
	_, handedOver2, err := AcquireName(name, Message{Paths: []string{"a", "b"}}, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !handedOver2 {
		t.Error("second invocation should hand over to the owner")
	}

	// Two more handovers to exercise coalescing of several invocations.
	if ok, err := SendName(name, Message{Paths: []string{"c"}}, 500*time.Millisecond); err != nil || !ok {
		t.Fatalf("send 2: ok=%v err=%v", ok, err)
	}
	if ok, err := SendName(name, Message{Paths: []string{"d", "e", "f"}}, 500*time.Millisecond); err != nil || !ok {
		t.Fatalf("send 3: ok=%v err=%v", ok, err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive all handovers in time")
	}

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for _, p := range got {
		total += len(p)
	}
	if total != 6 { // a,b + c + d,e,f
		t.Errorf("received %d paths across %d handovers, want 6", total, len(got))
	}
}

func TestPipeNamePerUser(t *testing.T) {
	name, err := PipeName()
	if err != nil {
		t.Fatal(err)
	}
	if len(name) < len(pipePrefix) || name[:len(pipePrefix)] != pipePrefix {
		t.Errorf("unexpected pipe name %q", name)
	}
}
