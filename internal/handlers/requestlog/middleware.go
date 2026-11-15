package requestlog

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"mcp-keychain/utils/logging"
)

var tracePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

const traceHeader = "X-Trace-Id"

type (
	responseWriter struct {
		http.ResponseWriter

		status int
	}
)

func New(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(traceHeader)
		if len(r.Header.Values(traceHeader)) != 1 || !tracePattern.MatchString(id) {
			id = rand.Text()
		}

		ctx := logging.WithTrace(r.Context(), id)
		requestLogger := logging.FromContext(ctx, logger)

		w.Header().Set(traceHeader, id)
		writer := &responseWriter{ResponseWriter: w}
		started := time.Now()

		requestLogger.InfoContext(ctx, "HTTP request started", "method", r.Method, "path", r.URL.Path)

		next.ServeHTTP(writer, r.WithContext(ctx))

		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}

		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		requestLogger.Log(ctx, level, "HTTP request completed",
			"method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}

	if status >= http.StatusOK || status == http.StatusSwitchingProtocols {
		w.status = status
	}

	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
