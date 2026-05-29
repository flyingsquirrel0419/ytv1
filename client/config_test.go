package client

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/famomatic/ytv1/internal/innertube"
)

func TestToInnerTubeConfig_DisablesFallbackInOverrideModeByDefault(t *testing.T) {
	cfg := Config{
		ClientOverrides: []string{"android_vr", "web", "web_safari"},
	}
	inner := cfg.ToInnerTubeConfig()
	if !inner.DisableFallbackClients {
		t.Fatalf("expected DisableFallbackClients=true when ClientOverrides is set")
	}
}

func TestToInnerTubeConfig_AllowsFallbackInOverrideModeWhenOptedIn(t *testing.T) {
	cfg := Config{
		ClientOverrides:                 []string{"android_vr", "web", "web_safari"},
		AppendFallbackOnClientOverrides: true,
	}
	inner := cfg.ToInnerTubeConfig()
	if inner.DisableFallbackClients {
		t.Fatalf("expected DisableFallbackClients=false when AppendFallbackOnClientOverrides=true")
	}
}

func TestToInnerTubeConfig_ExplicitDisableFallbackStillWins(t *testing.T) {
	cfg := Config{
		ClientOverrides:                 []string{"android_vr", "web", "web_safari"},
		AppendFallbackOnClientOverrides: true,
		DisableFallbackClients:          true,
	}
	inner := cfg.ToInnerTubeConfig()
	if !inner.DisableFallbackClients {
		t.Fatalf("expected DisableFallbackClients=true when explicitly configured")
	}
}

func TestToInnerTubeConfig_MapsExtractionEventHandler(t *testing.T) {
	var called bool
	cfg := Config{
		OnExtractionEvent: func(evt ExtractionEvent) {
			called = evt.Stage == "player_api_json" && evt.Phase == "success" && evt.Client == "WEB"
		},
	}
	inner := cfg.ToInnerTubeConfig()
	if inner.OnExtractionEvent == nil {
		t.Fatalf("expected OnExtractionEvent to be mapped")
	}
	inner.OnExtractionEvent(innertube.ExtractionEvent{
		Stage:  "player_api_json",
		Phase:  "success",
		Client: "WEB",
	})
	if !called {
		t.Fatalf("expected mapped handler to be called")
	}
}

func TestToInnerTubeConfig_MapsRequestHeaders(t *testing.T) {
	cfg := Config{
		RequestHeaders: http.Header{
			"User-Agent": []string{"ytv1-test/1.0"},
			"Referer":    []string{"https://example.com/watch"},
		},
	}
	inner := cfg.ToInnerTubeConfig()
	if got := inner.RequestHeaders.Get("User-Agent"); got != "ytv1-test/1.0" {
		t.Fatalf("User-Agent=%q, want ytv1-test/1.0", got)
	}
	if got := inner.RequestHeaders.Get("Referer"); got != "https://example.com/watch" {
		t.Fatalf("Referer=%q, want https://example.com/watch", got)
	}
}

func TestNewClient_AppliesDefaultNetworkConfig(t *testing.T) {
	c := New(Config{
		ProxyURL:           "http://127.0.0.1:3128",
		SourceAddress:      "127.0.0.1",
		InsecureSkipVerify: true,
	})
	httpClient := c.HTTPClient()
	if httpClient == nil {
		t.Fatalf("HTTPClient() returned nil")
	}
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", httpClient.Transport)
	}
	reqURL, err := url.Parse("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	req := &http.Request{URL: reqURL}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:3128" {
		t.Fatalf("proxyURL=%v, want http://127.0.0.1:3128", proxyURL)
	}
	if transport.DialContext == nil {
		t.Fatalf("DialContext=nil, want source-address-aware dialer")
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify=false, want true")
	}
}
