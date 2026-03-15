package server_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adm87/smol-server/api/server"
)

func TestRunReturnsContextCanceledForPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := server.New(server.WithAddress("127.0.0.1:0"))
	err := s.Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	addr := reserveLocalAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := server.New(server.WithAddress(addr))
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	waitForHTTPReady(t, addr)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}

func TestWithHandlerNilFallsBackToNotFound(t *testing.T) {
	addr := reserveLocalAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := server.New(
		server.WithAddress(addr),
		server.WithHandler(nil),
	)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	waitForHTTPReady(t, addr)

	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/missing")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got: %d", resp.StatusCode)
	}
}

func TestRunReturnsErrorOnBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local port: %v", err)
	}
	defer listener.Close()

	s := server.New(server.WithAddress(listener.Addr().String()))
	err = s.Run(context.Background())
	if err == nil {
		t.Fatal("expected startup error when binding to occupied port")
	}
}

func TestAddressUpdatesAfterEphemeralBind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := server.New(server.WithAddress("127.0.0.1:0"))
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	boundAddr := waitForBoundAddress(t, s)
	if strings.HasSuffix(boundAddr, ":0") {
		t.Fatalf("expected effective bound address, got: %q", boundAddr)
	}

	waitForHTTPReady(t, boundAddr)
}

func TestRunWithNilLoggerDoesNotPanic(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local port: %v", err)
	}
	defer listener.Close()

	s := server.New(
		server.WithAddress(listener.Addr().String()),
		server.WithLogger(nil),
	)

	err = s.Run(nil)
	if err == nil {
		t.Fatal("expected startup error on occupied address")
	}
}

func TestWithHandlerUsesCustomHandler(t *testing.T) {
	addr := reserveLocalAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			http.NotFound(w, r)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s := server.New(
		server.WithAddress(addr),
		server.WithHandler(handler),
	)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	waitForHTTPReady(t, addr)

	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", string(body))
	}
}

func TestRunReturnsErrorWhenAlreadyRunning(t *testing.T) {
	addr := reserveLocalAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := server.New(server.WithAddress(addr))

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	waitForHTTPReady(t, addr)

	err := s.Run(context.Background())
	if !errors.Is(err, server.ErrServerAlreadyRunning) {
		t.Fatalf("expected ErrServerAlreadyRunning, got: %v", err)
	}

	cancel()
	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("expected clean shutdown, got: %v", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}

func TestRunReturnsDeadlineExceededWhenShutdownTimesOut(t *testing.T) {
	addr := reserveLocalAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requestStarted := make(chan struct{}, 1)
	releaseRequest := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/healthz") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		select {
		case requestStarted <- struct{}{}:
		default:
		}

		<-releaseRequest
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("released"))
	})

	s := server.New(
		server.WithAddress(addr),
		server.WithHandler(handler),
		server.WithShutdownTimeout(50*time.Millisecond),
	)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	waitForHTTPReady(t, addr)

	client := &http.Client{Timeout: 2 * time.Second}
	requestDone := make(chan struct{}, 1)
	go func() {
		resp, err := client.Get("http://" + addr + "/block")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
		requestDone <- struct{}{}
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocking request to start")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded from shutdown timeout, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server to stop after cancellation")
	}

	close(releaseRequest)
	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked request goroutine to exit")
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

func waitForBoundAddress(t *testing.T, s *server.Server) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		addr := s.Address()
		if addr != "" && !strings.HasSuffix(addr, ":0") {
			return addr
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for effective bound address")
	return ""
}
