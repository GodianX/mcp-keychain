package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"mcp-keychain/utils/logging"

	"mcp-keychain/internal/config"
	"mcp-keychain/internal/server"
	"mcp-keychain/testhelpers"
)

func TestServe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		closedListener bool
	}{
		{name: "context shutdown"},
		{name: "listener failure", closedListener: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			listener, _ := testhelpers.NewHTTPListener(t)

			if tt.closedListener {
				err := listener.Close()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}

			app := server.New(config.Server{ShutdownTimeout: time.Second}, http.NotFoundHandler(), logging.New(io.Discard))

			err := app.Serve(ctx, listener)
			if (err != nil) != tt.closedListener {
				t.Fatalf("Serve() error = %v", err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	app := server.New(
		config.Server{Address: "invalid-address", ShutdownTimeout: time.Second},
		http.NotFoundHandler(),
		logging.New(io.Discard),
	)

	err := app.Run(t.Context())
	if err == nil {
		t.Fatal("Run() accepted invalid address")
	}
}

func TestShutdownActiveRequest(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	listener, client := testhelpers.NewHTTPListener(t)

	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		w.WriteHeader(http.StatusNoContent)
	})
	app := server.New(config.Server{ShutdownTimeout: time.Second}, handler, logging.New(io.Discard))

	finished := make(chan error, 1)
	go func() { finished <- app.Serve(ctx, listener) }()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/mcp", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	requestDone := make(chan error, 1)

	go func() {
		response, requestErr := client.Do(request)
		if requestErr == nil {
			requestErr = response.Body.Close()
		}

		requestDone <- requestErr
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}

	cancel()

	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}

	err = <-requestDone
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
