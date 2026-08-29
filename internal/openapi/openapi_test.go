package openapi

import (
	"os"
	"strings"
	"testing"

	"github.com/Reisender/api-browser/internal/spec"
)

func load(t *testing.T, name string) *spec.Spec {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if !IsOpenAPI(data) {
		t.Fatalf("%s not detected as openapi", name)
	}
	s, err := Convert(data)
	if err != nil {
		t.Fatalf("convert %s: %v", name, err)
	}
	return s
}

func names(rs []spec.Resource) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}

func TestPetstore(t *testing.T) {
	s := load(t, "petstore.yaml")
	if s.Name != "Pet Store 1.0.0" || s.Description != "A store for pets." || s.BasePath != "/api/v3" {
		t.Errorf("header = %q %q %q", s.Name, s.Description, s.BasePath)
	}
	// /health has no item endpoint but is still a collection; nested visits is related, not a resource.
	if got := names(s.Resources); got != "health,owners,pets" {
		t.Errorf("resources = %s", got)
	}
	pets, _ := s.Resource("pets")
	if pets.ListPath != "/pets" || pets.ItemPath != "/pets/{petId}" || pets.ListKey != "" || pets.ItemKey != "" {
		t.Errorf("pets = %+v", pets)
	}
	// allOf merged; arrays/objects excluded; id first, name next, status last.
	if got := strings.Join(pets.Columns, ","); got != "id,name,createdAt,status" {
		t.Errorf("pets columns = %s", got)
	}
	if len(pets.Related) != 1 || pets.Related[0].Path != "/pets/{petId}/visits" || pets.Related[0].Resource != "" {
		t.Errorf("pets related = %+v", pets.Related)
	}
	owners, _ := s.Resource("owners")
	if owners.ListKey != "owners" || owners.ItemKey != "" || strings.Join(owners.Columns, ",") != "id,fullName,email" {
		t.Errorf("owners = %+v", owners)
	}
	if pets.Description != "List all pets" {
		t.Errorf("desc = %q", pets.Description)
	}
	// petId placeholder is not a column; falls back to "id".
	if s.IDField != "id" {
		t.Errorf("idField = %q", s.IDField)
	}
	if s.Paging == nil || s.Paging.LimitParam != "limit" || s.Paging.OffsetParam != "offset" || s.Paging.DefaultLimit != 20 {
		t.Errorf("paging = %+v", s.Paging)
	}
	var qn []string
	for _, q := range s.QueryParams {
		qn = append(qn, q.Name+"="+q.Default)
	}
	if got := strings.Join(qn, ","); got != "limit=20,offset=0,status=" {
		t.Errorf("query params = %s", got)
	}
	if s.QueryParams[0].Description != "How many items to return" {
		t.Errorf("param desc = %q", s.QueryParams[0].Description)
	}
	if s.RefTypes["pet"] != "pets" || s.RefTypes["owner"] != "owners" {
		t.Errorf("refTypes = %v", s.RefTypes)
	}
	if _, err := spec.Expand(pets.ItemPath, map[string]string{"petId": "1"}); err != nil {
		t.Error(err)
	}
}

func TestOneRosterShaped(t *testing.T) {
	s := load(t, "oneroster.yaml")
	if s.BasePath != "/ims/oneroster/v1p1" {
		t.Errorf("basePath = %q (server variables should be substituted)", s.BasePath)
	}
	if got := names(s.Resources); got != "classes,users" {
		t.Errorf("resources = %s", got)
	}
	users, _ := s.Resource("users")
	if users.ListKey != "users" || users.ItemKey != "user" || users.ItemPath != "/users/{sourcedId}" {
		t.Errorf("users = %+v", users)
	}
	if got := strings.Join(users.Columns, ","); got != "sourcedId,familyName,givenName,status" {
		t.Errorf("users columns = %s", got)
	}
	if s.IDField != "sourcedId" {
		t.Errorf("idField = %q", s.IDField)
	}
	classes, _ := s.Resource("classes")
	// /classes/{id}/students returns "users" -> resolved to the users resource by list key.
	if len(classes.Related) != 1 || classes.Related[0].Name != "students" || classes.Related[0].ListKey != "users" || classes.Related[0].Resource != "users" {
		t.Errorf("classes related = %+v", classes.Related)
	}
	// /schools/{id}/classes has no /schools resource, so it is dropped rather than failing.
	if _, ok := s.Resource("schools"); ok {
		t.Error("schools should not exist")
	}
	if s.Paging == nil || s.Paging.DefaultLimit != 100 {
		t.Errorf("paging = %+v", s.Paging)
	}
}

func TestSwagger2(t *testing.T) {
	s := load(t, "swagger2.json")
	if s.BasePath != "/v2" || names(s.Resources) != "widgets" {
		t.Errorf("spec = %+v", s)
	}
	w, _ := s.Resource("widgets")
	if w.ItemPath != "/widgets/{id}" || strings.Join(w.Columns, ",") != "id,label" {
		t.Errorf("widgets = %+v", w)
	}
	if s.Paging == nil || s.Paging.LimitParam != "per_page" || s.Paging.DefaultLimit != 50 {
		t.Errorf("paging = %+v", s.Paging)
	}
}

func TestErrors(t *testing.T) {
	if IsOpenAPI([]byte("name: x\nresources: []")) {
		t.Error("native spec detected as openapi")
	}
	if IsOpenAPI([]byte("{{not yaml")) {
		t.Error("garbage detected as openapi")
	}
	if _, err := Convert([]byte("openapi: 3.0.0\npaths: {}")); err == nil || !strings.Contains(err.Error(), "no GET") {
		t.Errorf("expected no-GET error, got %v", err)
	}
	if _, err := Convert([]byte("openapi: 3.0.0\npaths:\n  /a/{id}:\n    get: {responses: {}}")); err == nil {
		t.Error("expected error when only item paths exist")
	}
}

func TestSingular(t *testing.T) {
	cases := map[string]string{"users": "user", "classes": "class", "categories": "category", "status": "status", "boxes": "box", "demographics": "demographic"}
	for in, want := range cases {
		if got := singular(in); got != want {
			t.Errorf("singular(%s) = %s want %s", in, got, want)
		}
	}
}

func TestLoadAnyAndRoundTrip(t *testing.T) {
	// Builtin name.
	if s, err := LoadAny("oneroster-v1p1"); err != nil || s.Name != "OneRoster v1p1" {
		t.Errorf("builtin: %v %v", s, err)
	}
	// OpenAPI file.
	s, err := LoadAny("testdata/petstore.yaml")
	if err != nil || s.Name != "Pet Store 1.0.0" {
		t.Fatalf("openapi: %v %v", s, err)
	}
	// Dump and reload as native.
	out, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "itemKey: \"\"") || !strings.Contains(string(out), "columns: [id, name, createdAt, status]") {
		t.Errorf("dump not clean:\n%s", out)
	}
	path := t.TempDir() + "/native.yaml"
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := LoadAny(path)
	if err != nil {
		t.Fatalf("reload native: %v", err)
	}
	if names(back.Resources) != names(s.Resources) || back.IDField != s.IDField || back.Paging.DefaultLimit != 20 {
		t.Errorf("round trip mismatch: %+v", back)
	}
	if _, err := LoadAny("nope-not-a-spec"); err == nil {
		t.Error("expected error")
	}
}
