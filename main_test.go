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

func TestHealthAndCookie(t *testing.T) {
	a := &app{}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 || rec.Body.String() != "ok\n" {
		t.Fatalf("health: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	setSessionCookie(rec, "opaque", 8*time.Hour)
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != "session" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge != 8*3600 {
		t.Fatalf("unexpected cookie: %+v", cookie)
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
