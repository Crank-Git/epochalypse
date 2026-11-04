package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWindowCreationRequiresOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan Addresses, 1), make(chan error, 1)
	go func() {
		done <- Serve(ctx, Config{Mode: "server", HTTPAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true}, func(a Addresses) { ready <- a })
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case address := <-ready:
		client := &http.Client{Timeout: time.Second}
		response, err := client.Post("http://"+address.HTTP+"/api/v1/windows", "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 401 {
			t.Fatalf("window creation status = %d, want 401", response.StatusCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server start timeout")
	}
}
