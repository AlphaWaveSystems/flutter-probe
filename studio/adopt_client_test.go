package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// closeFake is a frame source that counts Close.
type closeFake struct {
	frameFake
	closed atomic.Int32
}

func (c *closeFake) Close() error    { c.closed.Add(1); return nil }
func (c *closeFake) Connected() bool { return c.closed.Load() == 0 }
func (c *closeFake) HTTPLog(context.Context, probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	return probelink.HTTPLogResult{}, nil
}

func newAdoptConn(old *closeFake) (*App, *connection) {
	a := &App{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	conn := &connection{client: old, streamCancel: cancel, streamDone: done}
	a.conn = conn
	go a.streamLoop(ctx, done, old)
	return a, conn
}

// The client a run reconnects with (after a restart step) becomes the connection's client: the old one is closed, its stream stops, and a
// stream starts on the new one. Without it the live view reads a closed connection for good.
func TestAdoptClientMovesTheConnectionAndItsStreamToTheNewClient(t *testing.T) {
	old := &closeFake{frameFake: frameFake{shot: []byte("old")}}
	nc := &closeFake{frameFake: frameFake{shot: []byte("new")}}
	a, conn := newAdoptConn(old)
	a.adoptClient(conn, nc)
	a.mu.Lock()
	got := conn.client
	a.mu.Unlock()
	if got != nc {
		t.Fatal("the connection still holds the old client")
	}
	if old.closed.Load() != 1 {
		t.Errorf("the old client was closed %d times, want 1", old.closed.Load())
	}
	deadline := time.Now().Add(2 * time.Second)
	for nc.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if nc.callCount() == 0 {
		t.Error("no stream started on the new client")
	}
	before := old.callCount()
	time.Sleep(100 * time.Millisecond)
	if old.callCount() != before {
		t.Error("the stream on the old client is still running")
	}
	conn.streamCancel()
	<-conn.streamDone
}

func TestAdoptClientLeavesAReplacedConnectionAlone(t *testing.T) {
	old := &closeFake{frameFake: frameFake{shot: []byte("old")}}
	nc := &closeFake{frameFake: frameFake{shot: []byte("new")}}
	a, conn := newAdoptConn(old)
	a.mu.Lock()
	a.conn = &connection{} // the person connected to another device meanwhile
	a.mu.Unlock()
	a.adoptClient(conn, nc)
	if conn.client != old || old.closed.Load() != 0 || nc.callCount() != 0 {
		t.Error("a connection that is no longer the current one was changed")
	}
	conn.streamCancel()
	<-conn.streamDone
}

func TestAdoptClientIgnoresTheSameClient(t *testing.T) {
	old := &closeFake{frameFake: frameFake{shot: []byte("old")}}
	a, conn := newAdoptConn(old)
	a.adoptClient(conn, old)
	if old.closed.Load() != 0 {
		t.Error("the client in use was closed")
	}
	conn.streamCancel()
	<-conn.streamDone
}
