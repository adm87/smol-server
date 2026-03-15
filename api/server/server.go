package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
)

var ErrServerAlreadyRunning = errors.New("server already running")

// Server represents an HTTP server with configurable options and graceful shutdown capabilities.
type Server struct {
	cfg *Options
	svr *http.Server

	mu      sync.Mutex
	running bool
}

// New creates a new Server instance with the provided options.
// It applies default values for any options that are not specified.
func New(opts ...Option) *Server {
	cfg := applyOptions(defaultOptions(), opts...)
	return &Server{
		cfg: cfg,
		svr: &http.Server{
			Addr:              cfg.Address,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			Handler:           cfg.Handler,
		},
	}
}

// Address returns the configured listen address.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.cfg.Address
}

// Run starts the HTTP server and blocks until the context is canceled or an error occurs.
func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrServerAlreadyRunning
	}

	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}

	// Notes: Using net.Listen directly to support dynamic port assignment.
	// This will likely be unnecessary for simple services. However, if used in a more dynamic environment,
	// the flexibility is there if needed.

	listener, err := net.Listen("tcp", s.svr.Addr)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	s.cfg.Address = listener.Addr().String()
	s.svr.Addr = s.cfg.Address
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	errch := make(chan error, 1)

	go func() {
		s.cfg.Logger.Info("starting server", "address", s.svr.Addr)

		if err := s.svr.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errch <- err
			return
		}

		errch <- nil
	}()

	select {
	case err := <-errch:
		return err
	case <-ctx.Done():
		s.cfg.Logger.Info("shutting down server", "reason", ctx.Err())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()

		// Notes: Ensure the server fully shuts down, and capture any errors from Shutdown or Serve.
		// Using errors.Join to make sure the full context of any errors are preserved.

		shutdownErr := s.svr.Shutdown(shutdownCtx)
		if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			closeErr := s.svr.Close()
			serveErr := <-errch
			return errors.Join(serveErr, shutdownErr, closeErr)
		}

		serveErr := <-errch
		return errors.Join(serveErr, shutdownErr)
	}
}
