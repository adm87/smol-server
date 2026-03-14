package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithContextNil(t *testing.T) {
	s := WithContext(nil)
	if s == nil {
		t.Fatal("expected server instance")
	}
}

func TestStartPreCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := WithContext(ctx)
	err := s.Start()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestStartReturnsErrorWhenAlreadyStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := WithContext(ctx).SetAddress(":0")

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForStarted(t, s)

	err := s.Start()
	if err == nil || err.Error() != "server already started" {
		t.Fatalf("expected already started error, got: %v", err)
	}

	cancel()
	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("expected first Start() to exit cleanly, got: %v", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for running server to stop")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	s := WithContext(context.Background()).SetAddress(":0")

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForStarted(t, s)

	s.Stop()
	s.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected Start() to exit cleanly after Stop(), got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server stop")
	}
}

func TestSetHandlerNilUsesNotFoundHandler(t *testing.T) {
	s := WithContext(context.Background()).SetHandler(nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	s.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got: %d", rr.Code)
	}
}

func TestSetAddressIgnoredAfterStart(t *testing.T) {
	s := WithContext(context.Background()).SetAddress(":0")

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForStarted(t, s)

	before := s.server.Addr
	s.SetAddress("127.0.0.1:9999")
	after := s.server.Addr

	if before != after {
		t.Fatalf("expected address unchanged after start, before=%q after=%q", before, after)
	}

	s.Stop()
	<-done
}

func TestSetLoggerNilFallsBackToDefault(t *testing.T) {
	s := WithContext(context.Background()).SetLogger(nil)
	if s.logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestStartReturnsErrorOnBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local port: %v", err)
	}
	defer listener.Close()

	s := WithContext(context.Background()).SetAddress(listener.Addr().String())

	err = s.Start()
	if err == nil {
		t.Fatal("expected startup error when binding to occupied port")
	}
}

func waitForStarted(t *testing.T, s *Server) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		started := s.started
		s.mu.Unlock()
		if started {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for server to enter started state")
}
