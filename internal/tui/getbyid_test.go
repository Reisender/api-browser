package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/config"
	"github.com/Reisender/api-browser/internal/openapi"
)

// TestGetByIDFromResources fetches a single record straight from the resource
// list, without listing the collection first.
func TestGetByIDFromResources(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")

	press(t, a, "i")
	rq, ok := a.top().(*requestScreen)
	if !ok {
		t.Fatalf("top is %T, want *requestScreen", a.top())
	}
	if rq.kind != fetchItem {
		t.Errorf("kind = %v, want fetchItem", rq.kind)
	}
	if rq.title() != "get: classes" {
		t.Errorf("title = %q", rq.title())
	}
	// The id field is empty and focused, and paging params are left out
	// because they mean nothing for a single record.
	if got := rq.form.get("path:sourcedId"); got != "" {
		t.Errorf("id prefilled with %q, want empty", got)
	}
	for _, k := range []string{"q:limit", "q:offset"} {
		for _, f := range rq.form.fields {
			if f.Key == k {
				t.Errorf("item request should not offer %q", k)
			}
		}
	}
	if !strings.Contains(rq.view(a, 120, 30), "/ims/oneroster/v1p1/classes/{sourcedId}") {
		t.Errorf("view should preview the item path:\n%s", rq.view(a, 120, 30))
	}

	typeText(t, a, "c1")
	press(t, a, "enter")
	is, ok := a.top().(*itemScreen)
	if !ok {
		t.Fatalf("top is %T (status %q), want *itemScreen", a.top(), a.status)
	}
	if is.resp.Item["title"] != "Math" {
		t.Errorf("item = %v", is.resp.Item)
	}
	if is.title() != "classes/c1" {
		t.Errorf("item title = %q, want classes/c1", is.title())
	}
	if !strings.HasSuffix(is.resp.URL, "/ims/oneroster/v1p1/classes/c1") {
		t.Errorf("url = %s", is.resp.URL)
	}
	// The fetched record behaves like any other item: references resolve.
	press(t, a, "l")
	if _, ok := a.top().(*menuScreen); !ok {
		t.Errorf("related menu: top is %T", a.top())
	}
}

// TestGetByIDBlankAndExtraParams covers the empty-id error and a query
// parameter riding along on the single GET.
func TestGetByIDBlankAndExtraParams(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")

	press(t, a, "i", "enter")
	if _, ok := a.top().(*requestScreen); !ok {
		t.Fatalf("a blank id should keep the editor open, got %T", a.top())
	}
	if !strings.Contains(a.status, "sourcedId") {
		t.Errorf("status = %q, want a missing-parameter error", a.status)
	}

	rq := a.top().(*requestScreen)
	rq.form.set("path:sourcedId", "c1")
	rq.form.set("q:fields", "sourcedId,title")
	press(t, a, "enter")
	is, ok := a.top().(*itemScreen)
	if !ok {
		t.Fatalf("top is %T (status %q)", a.top(), a.status)
	}
	if !strings.HasSuffix(is.resp.URL, "/classes/c1?fields=sourcedId%2Ctitle") {
		t.Errorf("url = %s, want the fields param appended", is.resp.URL)
	}
}

// TestGetByIDFromCollection reaches a record that is not on the current page.
func TestGetByIDFromCollection(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs, ok := a.top().(*collectionScreen)
	if !ok {
		t.Fatalf("top is %T", a.top())
	}
	if len(cs.resp.Items) != 2 {
		t.Fatalf("items = %d", len(cs.resp.Items))
	}

	press(t, a, "i")
	if _, ok := a.top().(*requestScreen); !ok {
		t.Fatalf("top is %T, want *requestScreen", a.top())
	}
	typeText(t, a, "c1")
	press(t, a, "enter")
	is, ok := a.top().(*itemScreen)
	if !ok {
		t.Fatalf("top is %T (status %q)", a.top(), a.status)
	}
	if is.resp.Item["sourcedId"] != "c1" {
		t.Errorf("item = %v", is.resp.Item)
	}
	// The collection is still underneath, untouched.
	press(t, a, "esc")
	if back, ok := a.top().(*collectionScreen); !ok || len(back.resp.Items) != 2 {
		t.Errorf("back to collection: top=%T", a.top())
	}
}

// TestGetByIDNoWrapperKeys exercises a spec inferred from OpenAPI, where the
// resource has neither a listKey nor an itemKey and itemPath comes from the
// document. The request kind, not the keys, decides how the response opens.
func TestGetByIDNoWrapperKeys(t *testing.T) {
	srv := petServer(t)
	defer srv.Close()

	s, err := openapi.LoadAny("../openapi/testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(s, config.Profile{BaseURL: srv.URL, Spec: "petstore"}, filepath.Join(t.TempDir(), "c.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	pets, ok := s.Resource("pets")
	if !ok {
		t.Fatal("no pets resource")
	}
	if pets.ItemKey != "" {
		t.Fatalf("expected no itemKey on the inferred spec, got %q", pets.ItemKey)
	}

	selectResource(t, a, "pets")
	press(t, a, "i")
	typeText(t, a, "p1")
	press(t, a, "enter")
	is, ok := a.top().(*itemScreen)
	if !ok {
		t.Fatalf("top is %T (status %q), want *itemScreen", a.top(), a.status)
	}
	if is.resp.Item["name"] != "Rex" {
		t.Errorf("item = %v", is.resp.Item)
	}
	// And listing the same resource still opens a collection, not an item.
	press(t, a, "esc", "enter")
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Errorf("list: top is %T, want *collectionScreen", a.top())
	}
}
