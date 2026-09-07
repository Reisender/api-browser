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

func TestLoadBuiltinOneRosterV1p2(t *testing.T) {
	s, err := LoadBuiltin("oneroster-v1p2")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Name != "OneRoster v1p2" {
		t.Errorf("name = %q", s.Name)
	}
	if s.IDField != "sourcedId" {
		t.Errorf("idField = %q", s.IDField)
	}
	// Resources new in v1.2, spread across the gradebook and resources services.
	for _, want := range []string{"scoreScales", "assessmentLineItems", "assessmentResults", "resources"} {
		if _, ok := s.Resource(want); !ok {
			t.Errorf("missing v1.2 resource %q", want)
		}
	}
	// v1.2 splits the API into three per-service base paths.
	for res, want := range map[string]string{
		"users":       "/ims/oneroster/rostering/v1p2/users",
		"lineItems":   "/ims/oneroster/gradebook/v1p2/lineItems",
		"resources":   "/ims/oneroster/resources/v1p2/resources",
		"scoreScales": "/ims/oneroster/gradebook/v1p2/scoreScales",
	} {
		r, ok := s.Resource(res)
		if !ok {
			t.Fatalf("missing resource %q", res)
		}
		if got := s.FullPath(r.ListPath); got != want {
			t.Errorf("%s listPath = %q, want %q", res, got, want)
		}
	}
	if r, ok := s.ResourceForRefType("scoreScale"); !ok || r.Name != "scoreScales" {
		t.Errorf("refType scoreScale -> %v", r)
	}
	// Collections still use the v1.1 payload keys.
	if r, _ := s.Resource("schools"); r.ListKey != "orgs" {
		t.Errorf("schools listKey = %q, want orgs", r.ListKey)
	}
	if r, _ := s.Resource("students"); r.ListKey != "users" {
		t.Errorf("students listKey = %q, want users", r.ListKey)
	}
}

func TestBuiltins(t *testing.T) {
	infos := Builtins()
	if len(infos) != len(BuiltinNames()) {
		t.Fatalf("Builtins() = %d entries, BuiltinNames() = %d", len(infos), len(BuiltinNames()))
	}
	want := map[string]string{"oneroster-v1p1": "OneRoster v1p1", "oneroster-v1p2": "OneRoster v1p2"}
	for _, i := range infos {
		if n, ok := want[i.ID]; ok {
			if i.Name != n {
				t.Errorf("%s name = %q, want %q", i.ID, i.Name, n)
			}
			if i.Description == "" {
				t.Errorf("%s has no description", i.ID)
			}
			delete(want, i.ID)
		}
	}
	for id := range want {
		t.Errorf("Builtins() missing %q", id)
	}
	if _, err := LoadBuiltin(DefaultBuiltin); err != nil {
		t.Errorf("DefaultBuiltin %q does not load: %v", DefaultBuiltin, err)
	}
}
