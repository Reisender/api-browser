package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		cfg  Config
		want string
	}{
		{Config{}, ""},
		{Config{Method: MethodNone}, ""},
		{Config{Method: MethodBearer}, "token is required"},
		{Config{Method: MethodBearer, Token: "abc"}, ""},
		{Config{Method: MethodOAuth2, ClientID: "id"}, "clientSecret, tokenUrl"},
		{Config{Method: MethodOAuth2, ClientID: "id", ClientSecret: "s", TokenURL: "http://x"}, ""},
		{Config{Method: MethodHeader}, "header name is required"},
		{Config{Method: MethodHeader, HeaderName: "X-Api-Key", HeaderValue: "v"}, ""},
		{Config{Method: "magic"}, "unknown auth method"},
	}
	for _, c := range cases {
		err := c.cfg.Validate()
		if c.want == "" && err != nil {
			t.Errorf("%+v: unexpected error %v", c.cfg, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%+v: err = %v, want %q", c.cfg, err, c.want)
		}
	}
}

func TestBearerAndHeader(t *testing.T) {
	a, err := New(Config{Method: MethodBearer, Token: "  tok123  "})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://x/", nil)
	if err := a.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization = %q", got)
	}
	if strings.Contains(a.Describe(), "tok123") {
		t.Errorf("Describe leaks token: %q", a.Describe())
	}

	h, err := New(Config{Method: MethodHeader, HeaderName: "X-Api-Key", HeaderValue: "secret-value-here"})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "http://x/", nil)
	_ = h.Apply(context.Background(), req)
	if got := req.Header.Get("X-Api-Key"); got != "secret-value-here" {
		t.Errorf("X-Api-Key = %q", got)
	}
	if strings.Contains(h.Describe(), "secret-value-here") {
		t.Errorf("Describe leaks value: %q", h.Describe())
	}

	n, _ := New(Config{})
	if _, ok := n.(None); !ok {
		t.Errorf("empty config should be None, got %T", n)
	}
}

func TestClientCredentials(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		_ = r.ParseForm()
		user, pass, ok := r.BasicAuth()
		grant := r.Form.Get("grant_type")
		if !ok || user != "cid" || pass != "csec" {
			// x/oauth2 may fall back to body params; accept either.
			if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csec" {
				http.Error(w, "bad creds", 401)
				return
			}
		}
		if grant != "client_credentials" {
			http.Error(w, "bad grant", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-" + string(rune('0'+calls)),
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	cc := NewClientCredentials("cid", "csec", srv.URL+"/token", []string{"roster"})
	now := time.Now()
	cc.now = func() time.Time { return now }

	req := httptest.NewRequest("GET", "http://api/", nil)
	if err := cc.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer at-1" {
		t.Errorf("Authorization = %q", got)
	}
	// Second call is served from cache.
	req2 := httptest.NewRequest("GET", "http://api/", nil)
	_ = cc.Apply(context.Background(), req2)
	if calls != 1 {
		t.Errorf("expected cached token, token endpoint called %d times", calls)
	}
	// Advance the clock past expiry -> refetch.
	now = now.Add(2 * time.Hour)
	req3 := httptest.NewRequest("GET", "http://api/", nil)
	_ = cc.Apply(context.Background(), req3)
	if calls != 2 || req3.Header.Get("Authorization") != "Bearer at-2" {
		t.Errorf("expected refresh; calls=%d auth=%q", calls, req3.Header.Get("Authorization"))
	}
	// Invalidate forces refetch.
	cc.Invalidate()
	_ = cc.Apply(context.Background(), httptest.NewRequest("GET", "http://api/", nil))
	if calls != 3 {
		t.Errorf("expected refetch after Invalidate, calls=%d", calls)
	}
	if strings.Contains(cc.Describe(), "csec") {
		t.Errorf("Describe leaks secret")
	}
}

func TestClientCredentialsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, 401)
	}))
	defer srv.Close()
	cc := NewClientCredentials("cid", "bad", srv.URL, nil)
	err := cc.Apply(context.Background(), httptest.NewRequest("GET", "http://api/", nil))
	if err == nil || !strings.Contains(err.Error(), "oauth2 token") {
		t.Errorf("expected token error, got %v", err)
	}
}
