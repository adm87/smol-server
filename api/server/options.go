package server

import (
	"log/slog"
	"net/http"
	"time"
)

// Options defines the configuration for the Server.
type Options struct {
	Address           string
	ShutdownTimeout   time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration

	Logger  *slog.Logger
	Handler http.Handler
}

// Option is a functional option for configuring the Server.
type Option func(*Options)

func WithAddress(addr string) Option {
	return func(opts *Options) {
		opts.Address = addr
	}
}

func WithShutdownTimeout(d time.Duration) Option {
	return func(opts *Options) {
		opts.ShutdownTimeout = d
	}
}

func WithReadTimeout(d time.Duration) Option {
	return func(opts *Options) {
		opts.ReadTimeout = d
	}
}

func WithWriteTimeout(d time.Duration) Option {
	return func(opts *Options) {
		opts.WriteTimeout = d
	}
}

func WithIdleTimeout(d time.Duration) Option {
	return func(opts *Options) {
		opts.IdleTimeout = d
	}
}

func WithReadHeaderTimeout(d time.Duration) Option {
	return func(opts *Options) {
		opts.ReadHeaderTimeout = d
	}
}

func WithLogger(logger *slog.Logger) Option {
	if logger == nil {
		logger = slog.Default()
	}
	return func(opts *Options) {
		opts.Logger = logger
	}
}

func WithHandler(handler http.Handler) Option {
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	return func(opts *Options) {
		opts.Handler = handler
	}
}

func defaultOptions() *Options {
	return &Options{
		Address:           ":0",
		ShutdownTimeout:   5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		Logger:            slog.Default(),
		Handler:           http.NotFoundHandler(),
	}
}

func applyOptions(opts *Options, options ...Option) *Options {
	for _, opt := range options {
		opt(opts)
	}
	return opts
}
