# smol-server

A small HTTP server wrapper with context-based shutdown.

## Features

- Sensible defaults (`:8080`, 404 default handler)
- Fluent configuration (`SetAddress`, timeout setters, `SetHandler`)
- Graceful shutdown on context cancellation or `Stop()`

## net/http Example

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"smol-server/api/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s := server.WithContext(ctx).
		SetHandler(mux).
		SetAddress(":8080")

	if err := s.Start(); err != nil {
		slog.Error("server stopped", "error", err)
	}
}
```

## chi Example

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"smol-server/api/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s := server.WithContext(ctx).
		SetHandler(r).
		SetAddress(":8080")

	if err := s.Start(); err != nil {
		slog.Error("server stopped", "error", err)
	}
}
```

## Notes

- Configure the server before `Start()`.
- `Start()` is single-use per server instance.
- Cancel the parent context (or call `Stop()`) to trigger shutdown.
