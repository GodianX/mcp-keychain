package main

import "testing"

func TestRun(t *testing.T) {
	t.Parallel()

	err := run(t.Context())
	if err == nil {
		t.Fatal("run() accepted missing root configuration")
	}
}
