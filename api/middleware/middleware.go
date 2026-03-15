package middleware

import "net/http"

type MiddlewareFunc func(http.Handler) http.Handler

func (f MiddlewareFunc) Apply(next http.Handler) http.Handler {
	return f(next)
}

type Middleware interface {
	Apply(http.Handler) http.Handler
}

type MiddlewareStack struct {
	middlewares []Middleware
}

func NewStack(middlewares ...Middleware) *MiddlewareStack {
	return &MiddlewareStack{middlewares: middlewares}
}

func (s *MiddlewareStack) Apply(handler http.Handler) http.Handler {
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i].Apply(handler)
	}
	return handler
}
