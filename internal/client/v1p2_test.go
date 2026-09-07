package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Reisender/api-browser/internal/spec"
)

// TestV1p2ServicePaths fetches through the v1.2 spec against a server that
// mounts the three services at their own base paths, including a related
// sub-collection that crosses from rostering into gradebook.
func TestV1p2ServicePaths(t *testing.T) {
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ims/oneroster/rostering/v1p2/schools", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"orgs": []any{map[string]any{"sourcedId": "s1", "name": "Central High", "type": "school"}}})
	})
	mux.HandleFunc("/ims/oneroster/gradebook/v1p2/classes/c1/lineItems", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"lineItems": []any{map[string]any{"sourcedId": "li1", "title": "Quiz 1"}}})
	})
	mux.HandleFunc("/ims/oneroster/resources/v1p2/resources", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"resources": []any{map[string]any{"sourcedId": "r1", "title": "Textbook"}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sp, err := spec.LoadBuiltin("oneroster-v1p2")
	if err != nil {
		t.Fatal(err)
	}
	c := New(srv.URL, sp, nil)

	for _, tc := range []struct{ resource, want string }{
		{"schools", "Central High"},
		{"resources", "Textbook"},
	} {
		r, ok := sp.Resource(tc.resource)
		if !ok {
			t.Fatalf("no %s resource", tc.resource)
		}
		resp := c.Do(context.Background(), ListRequest(sp, r))
		if resp.Error != nil {
			t.Fatalf("%s: %v (%s)", tc.resource, resp.Error, resp.URL)
		}
		if len(resp.Items) != 1 || !strings.Contains(jsonStr(t, resp.Items[0]), tc.want) {
			t.Errorf("%s items = %v, want one containing %q", tc.resource, resp.Items, tc.want)
		}
	}

	classes, _ := sp.Resource("classes")
	var lineItems *spec.Related
	for i := range classes.Related {
		if classes.Related[i].Name == "lineItems" {
			lineItems = &classes.Related[i]
		}
	}
	if lineItems == nil {
		t.Fatal("classes has no lineItems related entry")
	}
	resp := c.Do(context.Background(), RelatedRequest(sp, *lineItems, "c1"))
	if resp.Error != nil {
		t.Fatalf("class lineItems: %v (%s)", resp.Error, resp.URL)
	}
	if len(resp.Items) != 1 || resp.Items[0]["title"] != "Quiz 1" {
		t.Errorf("class lineItems = %v", resp.Items)
	}
}

func jsonStr(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
