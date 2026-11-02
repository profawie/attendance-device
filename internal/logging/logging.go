// Package logging records every request and response (stdout + file).
package logging

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// New creates a JSON logger that writes to stdout and to a file.
func New(path string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	h := slog.NewJSONHandler(io.MultiWriter(os.Stdout, f), &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), func() { _ = f.Close() }, nil
}

const maxLogBody = 4096 // truncate long bodies so the log stays readable

type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.body.Len() < maxLogBody {
		r.body.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

// Middleware logs the request (method, path, query, body) and the
// response (status, body, duration) for EVERY call.
func Middleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var reqBody []byte
		if r.Body != nil {
			reqBody, _ = io.ReadAll(io.LimitReader(r.Body, 10<<20))
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(reqBody)) // restore for the handler
		}

		logger.Info("REQUEST",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"body", cut(reqBody),
		)

		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		logger.Info("RESPONSE",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"body", cut(rec.body.Bytes()),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func cut(b []byte) string {
	if len(b) > maxLogBody {
		return string(b[:maxLogBody]) + "...(truncated)"
	}
	return string(b)
}
