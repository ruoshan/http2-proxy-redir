package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func httpServer(listenPort int, test func(r *http.Request)) *http.Server {
	handle := func(w http.ResponseWriter, r *http.Request) {
		test(r)
		w.WriteHeader(200)
	}
	s := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", listenPort),
		Handler: http.HandlerFunc(handle),
	}
	go s.ListenAndServeTLS("./testdata/root.pem", "./testdata/root.key")
	return s
}

func TestHttpProxy(t *testing.T) {
	hp := NewHttpProxy("127.0.0.1:4433", "user", "passwd")
	s := httpServer(4433, func(r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Fatalf("Expect http2, got %s", r.Proto)
		}
		if r.RequestURI != "google.com:443" {
			t.Fatalf("Expect request line google.com:443, got %s", r.RequestURI)
		}
	})
	time.Sleep(1 * time.Second)
	_, err := hp.DialTunnel("https://google.com:443")
	if err != nil {
		t.Fatalf("Failed to DialTunnel: %s", err)
	}

	s.Shutdown(context.Background())
}
