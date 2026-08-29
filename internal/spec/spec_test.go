package spec

import (
	"strings"
	"testing"
)

func TestLoadBuiltinOneRoster(t *testing.T) {
	s, err := LoadBuiltin("oneroster-v1p1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Name != "OneRoster v1p1" {
		t.Errorf("name = %q", s.Name)
	}
	if s.IDField != "sourcedId" {
		t.Errorf("idField = %q", s.IDField)
	}
	for _, want := range []string{"users", "classes", "orgs", "enrollments", "lineItems", "results"} {
		if _, ok := s.Resource(want); !ok {
			t.Errorf("missing resource %q", want)
		}
	}
	if r, ok := s.ResourceForRefType("org"); !ok || r.Name != "orgs" {
		t.Errorf("refType org -> %v", r)
	}
	// Naive plural fallback when refTypes does not have an entry.
	if r, ok := s.ResourceForRefType("bogus"); ok {
		t.Errorf("expected no resource for bogus, got %v", r.Name)
	}
	if s.FullPath("/users") != "/ims/oneroster/v1p1/users" {
		t.Errorf("FullPath = %q", s.FullPath("/users"))
	}
}

func TestBuiltinNames(t *testing.T) {
	names := BuiltinNames()
	found := false
	for _, n := range names {
		if n == "oneroster-v1p1" {
			found = true
		}
	}
	if !found {
		t.Errorf("builtin names %v missing oneroster-v1p1", names)
	}
	if _, err := LoadBuiltin("does-not-exist"); err == nil {
		t.Error("expected error for unknown builtin")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"no name", "resources: [{name: a, listPath: /a}]", "name is required"},
		{"no resources", "name: x", "at least one resource"},
		{"dup resource", "name: x\nresources: [{name: a, listPath: /a}, {name: a, listPath: /a}]", "duplicate resource"},
		{"no listPath", "name: x\nresources: [{name: a}]", "no listPath"},
		{"bad refType", "name: x\nrefTypes: {foo: nope}\nresources: [{name: a, listPath: /a}]", "unknown resource"},
		{"bad related", "name: x\nresources: [{name: a, listPath: /a, related: [{name: b, path: \"/a/{id}/b\", resource: zzz}]}]", "unknown resource"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want containing %q", err, c.want)
			}
		})
	}
	s, err := Parse([]byte("name: x\nresources: [{name: a, listPath: /a}]"))
	if err != nil {
		t.Fatal(err)
	}
	if s.IDField != "id" {
		t.Errorf("default idField = %q", s.IDField)
	}
}

func TestPlaceholdersAndExpand(t *testing.T) {
	p := "/classes/{sourcedId}/students/{other}"
	got := Placeholders(p)
	if len(got) != 2 || got[0] != "sourcedId" || got[1] != "other" {
		t.Errorf("Placeholders = %v", got)
	}
	out, err := Expand(p, map[string]string{"sourcedId": "c1", "other": "x"})
	if err != nil || out != "/classes/c1/students/x" {
		t.Errorf("Expand = %q, %v", out, err)
	}
	if _, err := Expand(p, map[string]string{"sourcedId": "c1"}); err == nil || !strings.Contains(err.Error(), "other") {
		t.Errorf("expected missing error, got %v", err)
	}
}
