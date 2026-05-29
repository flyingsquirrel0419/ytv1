package client

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultHTTPClient_WithProxyURL(t *testing.T) {
	httpClient := defaultHTTPClient("http://127.0.0.1:3128", "", false)
	if httpClient == nil {
		t.Fatalf("defaultHTTPClient() returned nil")
	}
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", httpClient.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "https://www.youtube.com/watch?v=jNQXAC9IVRw", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy function error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:3128" {
		t.Fatalf("proxyURL = %v, want http://127.0.0.1:3128", proxyURL)
	}
}

func TestDefaultHTTPClient_InvalidProxyFallsBack(t *testing.T) {
	httpClient := defaultHTTPClient("://bad-url", "", false)
	if httpClient != http.DefaultClient {
		t.Fatalf("expected fallback to http.DefaultClient")
	}
}

func TestDefaultHTTPClient_WithSourceAddress(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("tcp4 loopback unavailable: %v", err)
	}
	defer listener.Close()

	remoteAddrCh := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteAddrCh <- r.RemoteAddr
		fmt.Fprint(w, "ok")
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()

	httpClient := defaultHTTPClient("", "127.0.0.1", false)
	resp, err := httpClient.Get(server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	resp.Body.Close()

	remoteAddr := <-remoteAddrCh
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", remoteAddr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("remote host=%q, want 127.0.0.1", host)
	}
}

func TestDefaultHTTPClient_InvalidSourceAddressDoesNotPanic(t *testing.T) {
	httpClient := defaultHTTPClient("", "not-an-ip", false)
	if httpClient == nil {
		t.Fatalf("defaultHTTPClient() returned nil")
	}
}

func TestDefaultHTTPClient_InsecureSkipVerify(t *testing.T) {
	httpClient := defaultHTTPClient("", "", true)
	if httpClient == nil {
		t.Fatalf("defaultHTTPClient() returned nil")
	}
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", httpClient.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify=false, want true")
	}
}
