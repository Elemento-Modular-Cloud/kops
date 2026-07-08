/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gcetokensource

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/util/flowcontrol"
)

func TestAltTokenSource_Token(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"accessToken":"abc123","expireTime":"2030-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	a := &AltTokenSource{
		oauthClient: srv.Client(),
		tokenURL:    srv.URL,
		tokenBody:   "request-body",
		throttle:    flowcontrol.NewTokenBucketRateLimiter(100, 100),
	}
	tok, err := a.Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok.AccessToken != "abc123" {
		t.Errorf("AccessToken = %q, want abc123", tok.AccessToken)
	}
	if tok.Expiry.Year() != 2030 {
		t.Errorf("Expiry = %v, want 2030", tok.Expiry)
	}
	if gotBody != "request-body" {
		t.Errorf("server received body %q, want request-body", gotBody)
	}
}

func TestAltTokenSource_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := &AltTokenSource{
		oauthClient: srv.Client(),
		tokenURL:    srv.URL,
		tokenBody:   "",
		throttle:    flowcontrol.NewTokenBucketRateLimiter(100, 100),
	}
	_, err := a.Token()
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention 500 status", err)
	}
}
