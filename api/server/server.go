package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	defaultAddress           = ":8080"
	defaultShutdownTimeout   = 5 * time.Second
	defaultReadTimeout       = 5 * time.Second
	defaultWriteTimeout      = 5 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultReadHeaderTimeout = 2 * time.Second
)

// Server is a small wrapper around net/http with context-driven lifecycle management.
// Configure it before Start(), then call Start() once to serve until context cancellation
// or Stop(). The default handler returns 404 for all routes.
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc

	server *http.Server
	logger *slog.Logger

	mu      sync.Mutex
	started bool
}

// New creates a new server with a background context.
func New() *Server {
	return WithContext(context.Background())
}

// WithContext creates a new server with the provided context. The server will be canceled when the context is canceled.
func WithContext(ctx context.Context) *Server {
	if ctx == nil {
		ctx = context.Background()
	}

	c, cancel := context.WithCancel(ctx)

	return &Server{
		ctx:    c,
		cancel: cancel,
		server: &http.Server{
			Addr:              defaultAddress,
			Handler:           http.NotFoundHandler(),
			ReadTimeout:       defaultReadTimeout,
			WriteTimeout:      defaultWriteTimeout,
			IdleTimeout:       defaultIdleTimeout,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
		},
		logger: slog.Default(),
	}
}

// Context returns the server's context.
func (s *Server) Context() context.Context {
	return s.ctx
}

// SetHandler sets the HTTP handler for the server. If not set, a default handler that returns 404 for all requests will be used.
func (s *Server) SetHandler(handler http.Handler) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	if handler == nil {
		handler = http.NotFoundHandler()
	}

	s.server.Handler = handler
	return s
}

// SetAddress sets the address for the server to listen on.
func (s *Server) SetAddress(addr string) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	s.server.Addr = addr
	return s
}

// SetLogger sets the logger for the server. If not set, a default JSON logger that writes to stdout will be used.
func (s *Server) SetLogger(logger *slog.Logger) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	if logger == nil {
		logger = slog.Default()
	}

	s.logger = logger
	return s
}

// SetIdleTimeout sets the maximum amount of time to wait for the next request when keep-alives are enabled.
func (s *Server) SetIdleTimeout(timeout time.Duration) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	s.server.IdleTimeout = timeout
	return s
}

// SetReadTimeout sets the maximum duration for reading the entire request.
func (s *Server) SetReadTimeout(timeout time.Duration) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	s.server.ReadTimeout = timeout
	return s
}

// SetReadHeaderTimeout sets the amount of time allowed to read request headers.
func (s *Server) SetReadHeaderTimeout(timeout time.Duration) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	s.server.ReadHeaderTimeout = timeout
	return s
}

// SetWriteTimeout sets the amount of time allowed to write a response.
func (s *Server) SetWriteTimeout(timeout time.Duration) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started || s.ctx.Err() != nil {
		return s
	}

	s.server.WriteTimeout = timeout
	return s
}

// ------------------------------------------------------------------------------------------
// Lifecycle
// ------------------------------------------------------------------------------------------

// Start starts the server and blocks until it is stopped.
// It returns any error that occurs during startup or shutdown.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("server already started")
	}

	if err := s.ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}

	s.started = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
	}()

	var task sync.WaitGroup
	errch := make(chan error, 2)

	task.Go(func() {
		errch <- s.startServer()
	})

	task.Go(func() {
		<-s.ctx.Done()
		errch <- s.shutdownServer()
	})

	task.Wait()

	startErr := <-errch
	shutdownErr := <-errch

	return errors.Join(startErr, shutdownErr)
}

// Stop signals the server to stop accepting new requests and shut down gracefully.
func (s *Server) Stop() {
	s.cancel()
}

func (s *Server) startServer() error {
	s.logger.Info("Starting server", "address", s.server.Addr)

	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("Error running server", "error", err)
		s.cancel()
		return err
	}

	return nil
}

func (s *Server) shutdownServer() error {
	s.logger.Info("Shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()

	errs := make([]error, 0, 2)

	if shutdownErr := s.server.Shutdown(ctx); shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
		s.logger.Error("Failed to shutdown server gracefully", "error", shutdownErr)
		errs = append(errs, shutdownErr)

		if closeErr := s.server.Close(); closeErr != nil {
			s.logger.Error("Failed to forcefully close server", "error", closeErr)
			errs = append(errs, closeErr)
		}
	}

	return errors.Join(errs...)
}
