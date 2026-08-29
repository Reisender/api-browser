package config

import (
	"path/filepath"
	"testing"

	"github.com/reisenderlabs/api-browser/internal/auth"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(f.Profiles) != 0 {
		t.Fatal("expected empty config")
	}
	f.Put(Profile{Name: "zeta", BaseURL: "https://z", Spec: "oneroster-v1p1", Auth: auth.Config{Method: auth.MethodBearer, Token: "t"}})
	f.Put(Profile{Name: "alpha", BaseURL: "https://a", Spec: "oneroster-v1p1", Auth: auth.Config{Method: auth.MethodOAuth2, ClientID: "c", ClientSecret: "s", TokenURL: "https://a/token"}})
	f.Put(Profile{Name: "alpha", BaseURL: "https://a2"}) // replace
	f.Default = "alpha"
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Profiles) != 2 || g.Profiles[0].Name != "alpha" || g.Profiles[0].BaseURL != "https://a2" {
		t.Errorf("profiles = %+v", g.Profiles)
	}
	p, ok := g.Get("")
	if !ok || p.Name != "alpha" {
		t.Errorf("default profile = %+v", p)
	}
	z, ok := g.Get("zeta")
	if !ok || z.Auth.Token != "t" {
		t.Errorf("zeta = %+v", z)
	}
	if _, ok := g.Get("nope"); ok {
		t.Error("expected miss")
	}
}

func TestDefaultPathEnv(t *testing.T) {
	t.Setenv("APIBROWSER_CONFIG", "/tmp/x.yaml")
	if DefaultPath() != "/tmp/x.yaml" {
		t.Errorf("DefaultPath = %q", DefaultPath())
	}
}
