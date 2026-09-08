package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/auth"
	"github.com/Reisender/api-browser/internal/config"
	"github.com/Reisender/api-browser/internal/spec"
)

// profileApp writes a config file holding profiles and starts the app on the
// first one, exactly as main does for -profile.
func profileApp(t *testing.T, def string, profiles ...config.Profile) (*App, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	f := &config.File{Default: def, Profiles: profiles}
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	active := config.Profile{}
	if len(profiles) > 0 {
		active = profiles[0]
	}
	specName := active.Spec
	if specName == "" {
		specName = spec.DefaultBuiltin
	}
	s, err := spec.LoadBuiltin(specName)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(s, active, path)
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return a, path
}

func bearer(name, url, specName string) config.Profile {
	return config.Profile{
		Name: name, BaseURL: url, Spec: specName,
		Auth: auth.Config{Method: auth.MethodBearer, Token: "tok"},
	}
}

// TestProfilePicker switches connection, auth and spec in one keystroke, then
// browses the new host to prove the client really was repointed.
func TestProfilePicker(t *testing.T) {
	srv1, srv2 := dualServer(t), dualServer(t)
	defer srv1.Close()
	defer srv2.Close()
	a, _ := profileApp(t, "alpha",
		bearer("alpha", srv1.URL, "oneroster-v1p1"),
		bearer("beta", srv2.URL, "oneroster-v1p2"))

	// Browse a couple of levels deep first: switching must not strand us on a
	// screen built from the old connection.
	selectResource(t, a, "classes")
	press(t, a, "enter", "enter")
	if _, ok := a.top().(*itemScreen); !ok {
		t.Fatalf("setup: top=%T status=%q", a.top(), a.status)
	}

	press(t, a, "P")
	ps, ok := a.top().(*profileScreen)
	if !ok {
		t.Fatalf("P: top is %T, want *profileScreen", a.top())
	}
	// A second P does not stack another picker.
	depth := a.Depth()
	press(t, a, "P")
	if a.Depth() != depth {
		t.Errorf("depth after a second P = %d, want %d", a.Depth(), depth)
	}
	// It opens on the profile in effect, marked as current and default.
	it, ok := ps.list.SelectedItem().(profileItem)
	if !ok || it.p.Name != "alpha" {
		t.Fatalf("selected = %v, want alpha", ps.list.SelectedItem())
	}
	if !it.current || !it.isDefault {
		t.Errorf("alpha: current=%v isDefault=%v, want both true", it.current, it.isDefault)
	}
	view := ps.view(a, 120, 30)
	for _, want := range []string{"alpha", "beta", "(current, default)", "oneroster-v1p2"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker view is missing %q", want)
		}
	}

	press(t, a, "down", "enter")
	if a.Depth() != 1 {
		t.Fatalf("depth after choosing = %d, want 1 (stack reset)", a.Depth())
	}
	if a.profile.Name != "beta" {
		t.Errorf("profile = %q, want beta", a.profile.Name)
	}
	if a.client.BaseURL != srv2.URL {
		t.Errorf("client.BaseURL = %q, want %q", a.client.BaseURL, srv2.URL)
	}
	if a.spec.Name != "OneRoster v1p2" || a.profile.Spec != "oneroster-v1p2" {
		t.Errorf("spec = %q / %q, want OneRoster v1p2", a.spec.Name, a.profile.Spec)
	}
	if a.client.Spec != a.spec {
		t.Error("client still points at the old spec")
	}
	rs, ok := a.top().(*resourcesScreen)
	if !ok {
		t.Fatalf("top = %T, want *resourcesScreen", a.top())
	}
	var names []string
	for _, item := range rs.list.Items() {
		names = append(names, item.(resourceItem).r.Name)
	}
	if !slices.Contains(names, "scoreScales") {
		t.Errorf("resource list %v lacks the v1.2 scoreScales resource", names)
	}

	// The new client works: v1.2 paths against the second server.
	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs, ok := a.top().(*collectionScreen)
	if !ok {
		t.Fatalf("after enter: top=%T status=%q", a.top(), a.status)
	}
	if len(cs.resp.Items) != 1 || cs.resp.Items[0]["sourcedId"] != "c9" {
		t.Errorf("records = %v, want the v1.2 record c9 from the second server", cs.resp.Items)
	}
}

// TestProfilePickerCurrentIsNoOp keeps the navigation when the highlighted
// profile is the one already in effect.
func TestProfilePickerCurrentIsNoOp(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a, _ := profileApp(t, "", bearer("alpha", srv.URL, "oneroster-v1p1"), bearer("beta", srv.URL, "oneroster-v1p2"))

	selectResource(t, a, "classes")
	press(t, a, "enter")
	press(t, a, "P", "enter")
	if a.Depth() != 2 {
		t.Errorf("depth = %d, want 2 (back to the collection)", a.Depth())
	}
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Errorf("top = %T, want the collection we came from", a.top())
	}
	if !strings.Contains(a.status, "already using alpha") {
		t.Errorf("status = %q, want 'already using alpha'", a.status)
	}
}

// TestProfilePickerToggleDefault sets and clears default: in the config file.
func TestProfilePickerToggleDefault(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a, path := profileApp(t, "", bearer("alpha", srv.URL, "oneroster-v1p1"), bearer("beta", srv.URL, "oneroster-v1p1"))

	press(t, a, "P", "down", "d")
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Default != "beta" {
		t.Errorf("default = %q, want beta", f.Default)
	}
	ps := a.top().(*profileScreen)
	if it := ps.list.SelectedItem().(profileItem); !it.isDefault {
		t.Error("row is not marked default after d")
	}
	if !strings.Contains(a.status, "beta is now the default") {
		t.Errorf("status = %q", a.status)
	}

	press(t, a, "d")
	if f, err = config.Load(path); err != nil {
		t.Fatal(err)
	}
	if f.Default != "" {
		t.Errorf("default = %q after a second d, want cleared", f.Default)
	}
	if it := ps.list.SelectedItem().(profileItem); it.isDefault {
		t.Error("row still marked default after clearing")
	}
	// Toggling the default does not switch profiles or leave the picker.
	if a.profile.Name != "alpha" || a.Depth() != 2 {
		t.Errorf("profile=%q depth=%d, want alpha on the picker", a.profile.Name, a.Depth())
	}
}

// TestProfilePickerNoProfiles reports where profiles would live instead of
// opening an empty list.
func TestProfilePickerNoProfiles(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a, path := profileApp(t, "")
	// New() opens on the connection form with no base URL; connect, which also
	// drops us on the resource list where P is live.
	if err := a.useProfile(bearer("scratch", srv.URL, "oneroster-v1p1")); err != nil {
		t.Fatal(err)
	}

	press(t, a, "P")
	if a.Depth() != 1 {
		t.Errorf("depth = %d, want 1 (nothing pushed)", a.Depth())
	}
	if !a.statusErr || !strings.Contains(a.status, "no saved profiles") || !strings.Contains(a.status, path) {
		t.Errorf("status = %q (err=%v), want a 'no saved profiles' error naming %s", a.status, a.statusErr, path)
	}
}

// TestProfilePickerBadSpec leaves the session untouched when a profile names a
// spec that cannot be loaded.
func TestProfilePickerBadSpec(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()
	a, _ := profileApp(t, "",
		bearer("alpha", srv.URL, "oneroster-v1p1"),
		bearer("broken", "https://elsewhere.example", "no-such-spec"))

	press(t, a, "P", "down", "enter")
	if _, ok := a.top().(*profileScreen); !ok {
		t.Fatalf("top = %T, want to stay on the picker", a.top())
	}
	if !a.statusErr {
		t.Errorf("status = %q, want an error", a.status)
	}
	if a.profile.Name != "alpha" || a.client.BaseURL != srv.URL || a.spec.Name != "OneRoster v1p1" {
		t.Errorf("session changed: profile=%q url=%q spec=%q", a.profile.Name, a.client.BaseURL, a.spec.Name)
	}
}

// TestPromptForProfile is the startup path: offer the saved profiles only when
// there is a choice to make.
func TestPromptForProfile(t *testing.T) {
	srv := dualServer(t)
	defer srv.Close()

	a, _ := profileApp(t, "", bearer("alpha", srv.URL, "oneroster-v1p1"), bearer("beta", srv.URL, "oneroster-v1p2"))
	a.PromptForProfile()
	if _, ok := a.top().(*profileScreen); !ok {
		t.Errorf("two profiles: top = %T, want the picker", a.top())
	}

	b, _ := profileApp(t, "", bearer("alpha", srv.URL, "oneroster-v1p1"))
	b.PromptForProfile()
	if _, ok := b.top().(*profileScreen); ok {
		t.Error("one profile: picker opened, want no prompt")
	}

	c, path := profileApp(t, "")
	_ = os.Remove(path)
	c.PromptForProfile()
	if _, ok := c.top().(*profileScreen); ok {
		t.Error("missing config: picker opened, want no prompt")
	}
}
