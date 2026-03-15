# smol-server

A small HTTP server wrapper with context-based shutdown.

## Features

- Sensible defaults (`:0` for dynamic port binding, 404 default handler)
- Functional options (`WithAddress`, timeout options, `WithHandler`, `WithLogger`)
- Graceful shutdown on context cancellation

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

	"github.com/adm87/smol-server/api/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s := server.New(
		server.WithAddress(":8080"),
		server.WithHandler(mux),
	)

	if err := s.Run(ctx); err != nil {
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
	"github.com/adm87/smol-server/api/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s := server.New(
		server.WithAddress(":8080"),
		server.WithHandler(r),
	)

	if err := s.Run(ctx); err != nil {
		slog.Error("server stopped", "error", err)
	}
}
```

## Notes

- Configure the server before calling `Run(ctx)`.
- `Run(ctx)` is single-use per server instance.
- Cancel the run context to trigger graceful shutdown.
