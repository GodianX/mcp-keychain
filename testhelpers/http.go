package testhelpers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

const testPort = 8080

type (
	memoryListener struct {
		connections chan net.Conn
		closed      chan struct{}
		once        sync.Once
	}
)

func NewHTTPListener(t *testing.T) (net.Listener, *http.Client) {
	t.Helper()

	listener := &memoryListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	transport := &http.Transport{DialContext: listener.dialContext}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	t.Cleanup(func() {
		transport.CloseIdleConnections()

		err := listener.Close()
		if err != nil {
			t.Error(err)
		}
	})

	return listener, client
}

func (l *memoryListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *memoryListener) Close() error {
	l.once.Do(func() { close(l.closed) })

	return nil
}

func (l *memoryListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: testPort}
}

func (l *memoryListener) dialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()

	select {
	case l.connections <- server:
		return client, nil
	case <-l.closed:
		return nil, errors.Join(net.ErrClosed, client.Close(), server.Close())
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), client.Close(), server.Close())
	}
}
