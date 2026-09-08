package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Reisender/api-browser/internal/client"
)

// TestSpecPicker walks the startup picker: it lists every builtin, and
// choosing one swaps the spec, the client base path and the resource list.
func TestSpecPicker(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)

	a.PromptForSpec()
	if a.Depth() != 2 {
		t.Fatalf("depth after PromptForSpec = %d, want 2", a.Depth())
	}
	sc, ok := a.top().(*specScreen)
	if !ok {
		t.Fatalf("top screen = %T, want *specScreen", a.top())
	}
	// The picker starts on the spec already in effect.
	if it, ok := sc.list.SelectedItem().(specItem); !ok || it.info.ID != "oneroster-v1p1" {
		t.Fatalf("selected item = %v, want oneroster-v1p1", sc.list.SelectedItem())
	}
	view := sc.view(a, 120, 30)
	for _, want := range []string{"oneroster-v1p1", "oneroster-v1p2"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker view is missing %q:\n%s", want, view)
		}
	}

	press(t, a, "down", "enter")
	if a.Depth() != 1 {
		t.Fatalf("depth after choosing = %d, want 1", a.Depth())
	}
	if a.spec.Name != "OneRoster v1p2" {
		t.Fatalf("spec = %q, want OneRoster v1p2", a.spec.Name)
	}
	if a.profile.Spec != "oneroster-v1p2" {
		t.Errorf("profile.Spec = %q, want oneroster-v1p2", a.profile.Spec)
	}
	if a.client.Spec != a.spec {
		t.Error("client still points at the old spec")
	}
	// The resource list at the bottom of the stack was rebuilt, so v1.2-only
	// resources are reachable and requests use the v1.2 base path.
	rs, ok := a.top().(*resourcesScreen)
	if !ok {
		t.Fatalf("bottom screen = %T, want *resourcesScreen", a.top())
	}
	var names []string
	for _, it := range rs.list.Items() {
		names = append(names, it.(resourceItem).r.Name)
	}
	if !slices.Contains(names, "scoreScales") {
		t.Errorf("resource list %v does not include the v1.2 scoreScales resource", names)
	}
	r, ok := a.spec.Resource("users")
	if !ok {
		t.Fatal("no users resource")
	}
	u, err := a.client.BuildURL(client.ItemRequest(a.spec, r, "u1"))
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/ims/oneroster/rostering/v1p2/users/u1"; u != want {
		t.Errorf("BuildURL = %q, want %q", u, want)
	}
}

// TestSpecPickerEscKeeps leaves the picker without choosing.
func TestSpecPickerEscKeeps(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)

	a.PromptForSpec()
	press(t, a, "down", "esc")
	if a.Depth() != 1 {
		t.Fatalf("depth after esc = %d, want 1", a.Depth())
	}
	if a.spec.Name != "OneRoster v1p1" {
		t.Errorf("spec = %q, want the original OneRoster v1p1", a.spec.Name)
	}
}

// dualServer answers both the v1.1 and the v1.2 class endpoints, so a test can
// browse across a mid-session spec switch.
func dualServer(t *testing.T) *httptest.Server {
	t.Helper()
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ims/oneroster/v1p1/classes", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"classes": []any{map[string]any{"sourcedId": "c1", "title": "Math"}}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/classes/c1", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"class": map[string]any{"sourcedId": "c1", "title": "Math"}})
	})
	mux.HandleFunc("/ims/oneroster/rostering/v1p2/classes", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"classes": []any{map[string]any{"sourcedId": "c9", "title": "Math 1.2"}}})
	})
	return httptest.NewServer(mux)
}

// TestSwitchSpecMidSession uses the S keybinding to change spec while browsing.
func TestSwitchSpecMidSession(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a := newTestAppOn(t, srv, "oneroster-v1p1")

	// Browse a couple of levels deep on v1.1.
	selectResource(t, a, "classes")
	press(t, a, "enter", "enter")
	if _, ok := a.top().(*itemScreen); !ok || a.Depth() != 3 {
		t.Fatalf("setup: top=%T depth=%d status=%q", a.top(), a.Depth(), a.status)
	}

	press(t, a, "S")
	if _, ok := a.top().(*specScreen); !ok {
		t.Fatalf("S: top is %T, want *specScreen", a.top())
	}
	// A second S does not stack another picker.
	press(t, a, "S")
	if a.Depth() != 4 {
		t.Errorf("depth after a second S = %d, want 4", a.Depth())
	}

	press(t, a, "down", "enter")
	if a.spec.Name != "OneRoster v1p2" || a.profile.Spec != "oneroster-v1p2" {
		t.Fatalf("spec = %q / %q", a.spec.Name, a.profile.Spec)
	}
	// Screens built from the old spec are gone.
	if a.Depth() != 1 {
		t.Errorf("depth after switching = %d, want 1", a.Depth())
	}
	if _, ok := a.top().(*resourcesScreen); !ok {
		t.Fatalf("top is %T, want *resourcesScreen", a.top())
	}
	if !strings.Contains(a.status, "OneRoster v1p2") {
		t.Errorf("status = %q", a.status)
	}

	// Browsing now goes to the v1.2 service path.
	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs, ok := a.top().(*collectionScreen)
	if !ok {
		t.Fatalf("top is %T (status %q)", a.top(), a.status)
	}
	if !strings.Contains(cs.resp.URL, "/ims/oneroster/rostering/v1p2/classes") {
		t.Errorf("url = %s", cs.resp.URL)
	}
	if len(cs.resp.Items) != 1 || cs.resp.Items[0]["title"] != "Math 1.2" {
		t.Errorf("items = %v", cs.resp.Items)
	}
}

// TestSwitchSpecSameChoiceKeepsStack: re-picking the current spec is a no-op.
func TestSwitchSpecSameChoiceKeepsStack(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a := newTestAppOn(t, srv, "oneroster-v1p1")
	selectResource(t, a, "classes")
	press(t, a, "enter")
	if a.Depth() != 2 {
		t.Fatalf("setup depth = %d", a.Depth())
	}

	press(t, a, "S", "enter")
	if a.Depth() != 2 {
		t.Errorf("depth = %d, want the collection kept at 2", a.Depth())
	}
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Errorf("top is %T, want the collection back", a.top())
	}
	if a.spec.Name != "OneRoster v1p1" {
		t.Errorf("spec = %q", a.spec.Name)
	}
	if !strings.Contains(a.status, "already using") {
		t.Errorf("status = %q", a.status)
	}
}

// TestSwitchSpecKeyIgnoredInForms: S is a normal character while typing.
func TestSwitchSpecKeyIgnoredInForms(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a := newTestAppOn(t, srv, "oneroster-v1p1")

	// Connection screen: S goes into the focused field.
	press(t, a, "a")
	cn, ok := a.top().(*connectionScreen)
	if !ok {
		t.Fatalf("top is %T", a.top())
	}
	cn.form.set("baseUrl", "")
	press(t, a, "S")
	if got := cn.form.get("baseUrl"); got != "S" {
		t.Errorf("baseUrl = %q, want the typed S", got)
	}
	press(t, a, "esc")

	// Resource list filter: S narrows the filter instead of switching spec.
	// Driven directly because the list's own filter input schedules a cursor
	// blink that press/drain would block on.
	a.Update(key("/"))
	rs := a.top().(*resourcesScreen)
	if !rs.list.SettingFilter() {
		t.Fatal("/ did not start filtering the resource list")
	}
	a.Update(key("S"))
	if _, ok := a.top().(*specScreen); ok {
		t.Error("S while filtering should not open the spec picker")
	}
	if !strings.Contains(rs.list.FilterValue(), "S") {
		t.Errorf("filter = %q, want the typed S", rs.list.FilterValue())
	}
}
