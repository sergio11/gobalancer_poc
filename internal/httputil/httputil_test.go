package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:8080"

	ip := GetClientIP(req, false)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", ip)
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")

	ip := GetClientIP(req, true)
	if ip != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:8080"
	req.Header.Set("X-Real-IP", "10.0.0.1")

	ip := GetClientIP(req, true)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

func TestGetClientIP_XForwardedForTakesPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Real-IP", "10.0.0.1")

	ip := GetClientIP(req, true)
	if ip != "203.0.113.50" {
		t.Errorf("expected X-Forwarded-For to take precedence, got %s", ip)
	}
}

func TestGetClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100"

	ip := GetClientIP(req, false)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", ip)
	}
}

func TestGetClientIP_NilRequest(t *testing.T) {
	ip := GetClientIP(nil, false)
	if ip != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1 for nil request, got %s", ip)
	}
}

func TestGetClientIP_UntrustedIgnoresForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Real-IP", "10.0.0.1")

	ip := GetClientIP(req, false)
	if ip != "192.168.1.100" {
		t.Errorf("expected RemoteAddr 192.168.1.100 when untrusted, got %s", ip)
	}
}
