package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProtocolAndTools(t *testing.T) {
	secret := "test-only-not-a-runtime-credential"
	cases := []struct {
		body, auth          string
		httpCode, errorCode int
		contains            string
	}{
		{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "", 401, -32001, ""},
		{`bad`, secret, 200, -32700, ""},
		{`{"jsonrpc":"1.0","id":1,"method":"tools/list"}`, secret, 200, -32600, ""},
		{`{"jsonrpc":"2.0","id":1,"method":"unknown"}`, secret, 200, -32601, ""},
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`, secret, 200, 0, "2024-11-05"},
		{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, secret, 200, 0, "formula_hint"},
		{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unit_convert","arguments":{"value":2,"from":"m","to":"cm"}}}`, secret, 200, 0, "200 cm"},
		{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unit_convert","arguments":{"value":"2","from":"m","to":"cm"}}}`, secret, 200, -32602, ""},
		{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unit_convert","arguments":{"value":2,"from":"kg","to":"m"}}}`, secret, 200, 0, `"isError":true`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(tc.body))
		if tc.auth != "" {
			req.Header.Set("Authorization", "Bearer "+tc.auth)
		}
		handler(secret).ServeHTTP(rec, req)
		if rec.Code != tc.httpCode {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var body struct {
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &body) != nil {
			t.Fatal("non-JSON response")
		}
		if tc.errorCode != 0 && (body.Error == nil || body.Error.Code != tc.errorCode) {
			t.Fatal("wrong RPC error", rec.Body.String())
		}
		if tc.errorCode == 0 && (body.Error != nil || !strings.Contains(rec.Body.String(), tc.contains)) {
			t.Fatal("invalid result", rec.Body.String())
		}
	}
}
