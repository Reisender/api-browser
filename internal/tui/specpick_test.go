package tui

import (
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
