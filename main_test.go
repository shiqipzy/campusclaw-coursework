package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnauthenticatedProtectedRoutes(t *testing.T) {
	a := &app{}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/me"},
		{"GET", "/api/materials"},
		{"GET", "/api/search?q=示例"},
		{"GET", "/api/materials/1"},
		{"GET", "/api/materials/1/file"},
		{"POST", "/api/materials"},
	} {
		rec := httptest.NewRecorder()
		a.routes().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "login required") {
			t.Errorf("%s %s: got %d %q", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestHealthAndToken(t *testing.T) {
	a := &app{}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 || rec.Body.String() != "ok\n" {
		t.Fatalf("health: %d %q", rec.Code, rec.Body.String())
	}
	a.jwtSecret = []byte("12345678901234567890123456789012")
	a.tokenTTL = 8 * time.Hour
	jwt, err := a.issueToken(42)
	if err != nil {
		t.Fatal(err)
	}
	id, _, _, ok := a.verifyToken("Bearer " + jwt)
	if !ok || id != 42 {
		t.Fatal("valid bearer token rejected")
	}
	if _, _, _, ok := a.verifyToken("Bearer " + jwt + "x"); ok {
		t.Fatal("tampered token accepted")
	}
	if _, _, _, ok := a.verifyToken("session=" + jwt); ok {
		t.Fatal("non-bearer token accepted")
	}
	a.tokenTTL = -time.Hour
	expired, _ := a.issueToken(42)
	if _, _, _, ok := a.verifyToken("Bearer " + expired); ok {
		t.Fatal("expired token accepted")
	}
}

func TestRandomTokenAndFilename(t *testing.T) {
	a, err := token()
	if err != nil {
		t.Fatal(err)
	}
	b, err := token()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || a == b || tokenHash(a) == a {
		t.Fatal("token generation or hashing failed")
	}
	if got := cleanFilename(`C:\fake\path\name.txt`); got != "name.txt" {
		t.Fatalf("filename: %q", got)
	}
	if got := cleanFilename("bad\r\nname.md"); got != "badname.md" {
		t.Fatalf("header unsafe filename: %q", got)
	}
}
