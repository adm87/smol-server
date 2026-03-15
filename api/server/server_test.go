package server_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/adm87/smol-server/api/server"
)

func TestWithContextNil(t *testing.T) {
	s := server.WithContext(nil)
	if s == nil {
		t.Fatal("expected server instance")
	}
}

func TestStartPreCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := server.WithContext(ctx)
	err := s.Start()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestStartReturnsErrorWhenAlreadyStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := reserveLocalAddress(t)
	s := server.WithContext(ctx).SetAddress(addr)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForHTTPReady(t, addr)

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
	addr := reserveLocalAddress(t)
	s := server.WithContext(context.Background()).SetAddress(addr)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForHTTPReady(t, addr)

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
	addr := reserveLocalAddress(t)
	s := server.WithContext(context.Background()).SetAddress(addr).SetHandler(nil)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()
	t.Cleanup(func() {
		s.Stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	waitForHTTPReady(t, addr)

	resp, err := http.Get("http://" + addr + "/missing")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got: %d", resp.StatusCode)
	}
}

func TestSetAddressIgnoredAfterStart(t *testing.T) {
	addrA := reserveLocalAddress(t)
	addrB := reserveLocalAddress(t)
	s := server.WithContext(context.Background()).SetAddress(addrA)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForHTTPReady(t, addrA)

	s.SetAddress(addrB)

	resp, err := http.Get("http://" + addrA + "/after-start")
	if err != nil {
		t.Fatalf("expected server to remain on original address: %v", err)
	}
	resp.Body.Close()

	client := &http.Client{Timeout: 150 * time.Millisecond}
	_, err = client.Get("http://" + addrB + "/after-start")
	if err == nil {
		t.Fatal("expected no server on new address after SetAddress post-start")
	}

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

func TestSetLoggerNilFallsBackToDefault(t *testing.T) {
	addr := reserveLocalAddress(t)
	s := server.WithContext(context.Background()).SetAddress(addr).SetLogger(nil)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	waitForHTTPReady(t, addr)

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

func TestStartReturnsErrorOnBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local port: %v", err)
	}
	defer listener.Close()

	s := server.WithContext(context.Background()).SetAddress(listener.Addr().String())

	err = s.Start()
	if err == nil {
		t.Fatal("expected startup error when binding to occupied port")
	}
}

func reserveLocalAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to release reserved address: %v", err)
	}

	return addr
}

func waitForHTTPReady(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	url := "http://" + addr + "/healthz"
	client := &http.Client{Timeout: 100 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for server to become reachable")
}
