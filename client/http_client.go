package client

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func defaultHTTPClient(proxyURL, sourceAddress string, insecureSkipVerify bool) *http.Client {
	proxyURL = strings.TrimSpace(proxyURL)
	sourceAddress = strings.TrimSpace(sourceAddress)
	if proxyURL == "" && sourceAddress == "" && !insecureSkipVerify {
		return http.DefaultClient
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultClient
	}
	transport := baseTransport.Clone()
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return http.DefaultClient
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	if ip := net.ParseIP(sourceAddress); ip != nil {
		dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: ip}}
		transport.DialContext = dialer.DialContext
	}
	if insecureSkipVerify {
		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		tlsConfig.InsecureSkipVerify = true
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Transport: transport}
}
