package tools

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"mcp-keychain/utils/logging"
)

func withAllowedHosts(next http.Handler, logger *slog.Logger, allowedHosts []string) http.Handler {
	if len(allowedHosts) == 0 {
		return next
	}

	hosts := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		hosts[strings.ToLower(host)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[strings.ToLower(r.Host)] {
			logging.FromContext(r.Context(), logger).WarnContext(r.Context(), "HTTP host rejected", "host", r.Host)
			http.Error(w, fmt.Sprintf("Forbidden: invalid Host header %q", r.Host), http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r)
	})
}
