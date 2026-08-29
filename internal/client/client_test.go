package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reisenderlabs/api-browser/internal/auth"
	"github.com/reisenderlabs/api-browser/internal/spec"
)

func loadSpec(t *testing.T) *spec.Spec {
	t.Helper()
	s, err := spec.LoadBuiltin("oneroster-v1p1")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ims/oneroster/v1p1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		if r.Header.Get("X-Extra") != "1" {
			http.Error(w, "missing extra header", 400)
			return
		}
		offset := r.URL.Query().Get("offset")
		users := []map[string]any{
			{"sourcedId": "u1", "givenName": "Ada", "role": "student", "orgs": []any{map[string]any{"sourcedId": "o1", "type": "org", "href": "http://x/orgs/o1"}}},
			{"sourcedId": "u2", "givenName": "Bob", "role": "teacher"},
		}
		if offset == "2" {
			users = users[:0]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/users/u1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"sourcedId": "u1", "givenName": "Ada"}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/users/u1/classes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"classes": []any{map[string]any{"sourcedId": "c1", "title": "Math"}}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/orgs/o1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	mux.HandleFunc("/ims/oneroster/v1p1/bare", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"a": 1}, "str"})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/wrapped", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"things": []any{map[string]any{"a": 1}}})
	})
	return httptest.NewServer(mux)
}

func TestBuildURL(t *testing.T) {
	s := loadSpec(t)
	c := New("https://api.example.com/", s, nil)
	u, err := c.BuildURL(Request{Path: "/classes/{sourcedId}/students", PathVars: map[string]string{"sourcedId": "c 1"}, Query: map[string]string{"limit": "10", "filter": "status='active'", "sort": ""}})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://api.example.com/ims/oneroster/v1p1/classes/c 1/students?filter=status%3D%27active%27&limit=10"
	if u != want {
		t.Errorf("url = %q\nwant %q", u, want)
	}
	if _, err := c.BuildURL(Request{Path: "/x/{id}"}); err == nil {
		t.Error("expected missing path var error")
	}
}

func TestDoListItemRelated(t *testing.T) {
	s := loadSpec(t)
	srv := fakeServer(t)
	defer srv.Close()
	c := New(srv.URL, s, auth.Bearer{Token: "tok"})
	c.Headers = map[string]string{"X-Extra": "1"}
	users, _ := s.Resource("users")

	resp := c.Do(context.Background(), ListRequest(s, users))
	if resp.Error != nil {
		t.Fatalf("list: %v", resp.Error)
	}
	if resp.Status != 200 || len(resp.Items) != 2 || resp.Items[0]["givenName"] != "Ada" {
		t.Errorf("list resp = %+v", resp)
	}
	if !strings.Contains(resp.URL, "limit=100") || !strings.Contains(resp.URL, "offset=0") {
		t.Errorf("default paging params missing from %s", resp.URL)
	}
	if !strings.HasPrefix(resp.Pretty(), "{\n  \"users\"") {
		t.Errorf("Pretty = %q", resp.Pretty())
	}

	// Reference detection.
	orgs := resp.Items[0]["orgs"].([]any)
	ref, ok := c.AsRef(orgs[0])
	if !ok || ref.ID != "o1" || ref.Type != "org" || ref.Resource == nil || ref.Resource.Name != "orgs" {
		t.Errorf("AsRef = %+v, %v", ref, ok)
	}
	if _, ok := c.AsRef(resp.Items[0]); ok {
		t.Error("full item should not be a ref")
	}
	if _, ok := c.AsRef("str"); ok {
		t.Error("string should not be a ref")
	}

	// Single item.
	ir := c.Do(context.Background(), ItemRequest(s, users, "u1"))
	if ir.Error != nil || ir.Item == nil || ir.Item["givenName"] != "Ada" {
		t.Errorf("item resp = %+v err=%v", ir.Item, ir.Error)
	}
	if ItemID(s, ir.Item) != "u1" {
		t.Errorf("ItemID = %q", ItemID(s, ir.Item))
	}

	// Related.
	var rel spec.Related
	for _, r := range users.Related {
		if r.Name == "classes" {
			rel = r
		}
	}
	rr := c.Do(context.Background(), RelatedRequest(s, rel, "u1"))
	if rr.Error != nil || len(rr.Items) != 1 || rr.Items[0]["title"] != "Math" {
		t.Errorf("related resp = %+v err=%v", rr.Items, rr.Error)
	}

	// Paging past the end.
	lr := ListRequest(s, users)
	lr.Query["offset"] = "2"
	pr := c.Do(context.Background(), lr)
	if pr.Error != nil || len(pr.Items) != 0 {
		t.Errorf("paged resp = %+v err=%v", pr.Items, pr.Error)
	}
}

func TestDoErrorsAndFallbacks(t *testing.T) {
	s := loadSpec(t)
	srv := fakeServer(t)
	defer srv.Close()

	// Wrong auth -> 401 surfaced with parsed body.
	c := New(srv.URL, s, auth.Bearer{Token: "wrong"})
	c.Headers = map[string]string{"X-Extra": "1"}
	users, _ := s.Resource("users")
	resp := c.Do(context.Background(), ListRequest(s, users))
	if resp.Error == nil || resp.Status != 401 || !strings.Contains(resp.Error.Error(), "401") {
		t.Errorf("expected 401, got status=%d err=%v", resp.Status, resp.Error)
	}
	if resp.Body == nil {
		t.Error("expected error body to be parsed")
	}

	ok := New(srv.URL, s, auth.Bearer{Token: "tok"})
	// Non-JSON body.
	nj := ok.Do(context.Background(), Request{Path: "/orgs/{sourcedId}", PathVars: map[string]string{"sourcedId": "o1"}, ItemKey: "org"})
	if nj.Error != nil || nj.Body != nil || nj.Pretty() != "not json" {
		t.Errorf("non-json = %+v", nj)
	}
	// Bare array.
	ba := ok.Do(context.Background(), Request{Path: "/bare"})
	if len(ba.Items) != 2 || ba.Items[1]["value"] != "str" {
		t.Errorf("bare array items = %+v", ba.Items)
	}
	// Single-key wrapper fallback with no ListKey.
	wr := ok.Do(context.Background(), Request{Path: "/wrapped"})
	if len(wr.Items) != 1 {
		t.Errorf("wrapped items = %+v", wr.Items)
	}
	// Transport error.
	dead := New("http://127.0.0.1:1", s, nil)
	de := dead.Do(context.Background(), Request{Path: "/x"})
	if de.Error == nil {
		t.Error("expected transport error")
	}
	// Auth failure short-circuits.
	af := New(srv.URL, s, auth.NewClientCredentials("a", "b", "http://127.0.0.1:1/token", nil))
	ar := af.Do(context.Background(), Request{Path: "/users"})
	if ar.Error == nil || !strings.Contains(ar.Error.Error(), "auth:") {
		t.Errorf("expected auth error, got %v", ar.Error)
	}
}

func TestItemRequestFallbackPath(t *testing.T) {
	s := loadSpec(t)
	r := &spec.Resource{Name: "x", ListPath: "/x/"}
	req := ItemRequest(s, r, "42")
	if req.Path != "/x/{sourcedId}" || req.PathVars["sourcedId"] != "42" {
		t.Errorf("req = %+v", req)
	}
}
