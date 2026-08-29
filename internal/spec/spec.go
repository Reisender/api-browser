// Package spec defines the API description model the browser navigates.
//
// A spec is a small, hand-authored description of a REST API: which
// resources exist, how to list them, how to fetch one by id, which related
// sub-collections hang off an item, and how reference objects in responses
// map back to resources. It is deliberately simpler than OpenAPI so that
// navigation semantics (list keys, reference following) are explicit.
package spec

import (
	"embed"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed specs/*.yaml
var builtin embed.FS

// Param describes a query parameter accepted by an endpoint.
type Param struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Default     string `yaml:"default,omitempty"`
}

// Related describes a sub-collection reachable from a single item, e.g.
// /classes/{sourcedId}/students.
type Related struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path"`
	ListKey  string `yaml:"listKey,omitempty"`
	Resource string `yaml:"resource,omitempty"` // resource the returned items belong to
}

// Resource is a top-level collection in the API.
type Resource struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	ListPath    string    `yaml:"listPath"`
	ItemPath    string    `yaml:"itemPath,omitempty"`
	ListKey     string    `yaml:"listKey,omitempty"`
	ItemKey     string    `yaml:"itemKey,omitempty"`
	Columns     []string  `yaml:"columns,omitempty,flow"`
	Related     []Related `yaml:"related,omitempty"`
}

// Spec is a full API description.
type Spec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	BasePath    string `yaml:"basePath,omitempty"`
	IDField     string `yaml:"idField,omitempty"`
	// Paging describes how offset/limit paging is expressed, if at all.
	Paging      *Paging           `yaml:"paging,omitempty"`
	QueryParams []Param           `yaml:"queryParams,omitempty"`
	RefTypes    map[string]string `yaml:"refTypes,omitempty"` // ref "type" value -> resource name
	Resources   []Resource        `yaml:"resources"`
}

// Paging describes offset-based pagination parameters.
type Paging struct {
	LimitParam   string `yaml:"limitParam"`
	OffsetParam  string `yaml:"offsetParam"`
	DefaultLimit int    `yaml:"defaultLimit"`
}

// BuiltinNames returns the names of embedded specs (without extension).
func BuiltinNames() []string {
	entries, _ := builtin.ReadDir("specs")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// LoadBuiltin loads an embedded spec by name (e.g. "oneroster-v1p1").
func LoadBuiltin(name string) (*Spec, error) {
	data, err := builtin.ReadFile("specs/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("unknown builtin spec %q (available: %s)", name, strings.Join(BuiltinNames(), ", "))
	}
	return Parse(data)
}

// LoadFile loads a spec from a YAML file on disk.
func LoadFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Load resolves name as either a builtin spec name or a file path.
func Load(nameOrPath string) (*Spec, error) {
	if _, err := os.Stat(nameOrPath); err == nil {
		return LoadFile(nameOrPath)
	}
	return LoadBuiltin(nameOrPath)
}

// Parse parses and validates YAML spec data.
func Parse(data []byte) (*Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

var placeholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// Validate checks the spec for internal consistency.
func (s *Spec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("spec: name is required")
	}
	if len(s.Resources) == 0 {
		return fmt.Errorf("spec: at least one resource is required")
	}
	if s.IDField == "" {
		s.IDField = "id"
	}
	seen := map[string]bool{}
	for i := range s.Resources {
		r := &s.Resources[i]
		if r.Name == "" {
			return fmt.Errorf("spec: resource %d has no name", i)
		}
		if seen[r.Name] {
			return fmt.Errorf("spec: duplicate resource %q", r.Name)
		}
		seen[r.Name] = true
		if r.ListPath == "" {
			return fmt.Errorf("spec: resource %q has no listPath", r.Name)
		}
		for _, rel := range r.Related {
			if rel.Name == "" || rel.Path == "" {
				return fmt.Errorf("spec: resource %q has an incomplete related entry", r.Name)
			}
		}
	}
	for typ, res := range s.RefTypes {
		if !seen[res] {
			return fmt.Errorf("spec: refTypes[%q] points at unknown resource %q", typ, res)
		}
	}
	for i := range s.Resources {
		for _, rel := range s.Resources[i].Related {
			if rel.Resource != "" && !seen[rel.Resource] {
				return fmt.Errorf("spec: resource %q related %q points at unknown resource %q", s.Resources[i].Name, rel.Name, rel.Resource)
			}
		}
	}
	return nil
}

// Marshal renders the spec as native YAML (e.g. for -dump-spec).
func (s *Spec) Marshal() ([]byte, error) {
	return yaml.Marshal(s)
}

// Resource looks up a resource by name.
func (s *Spec) Resource(name string) (*Resource, bool) {
	for i := range s.Resources {
		if s.Resources[i].Name == name {
			return &s.Resources[i], true
		}
	}
	return nil, false
}

// ResourceForRefType maps a reference "type" value (e.g. "user") to a resource.
func (s *Spec) ResourceForRefType(typ string) (*Resource, bool) {
	if name, ok := s.RefTypes[typ]; ok {
		return s.Resource(name)
	}
	// Fall back to a naive plural match.
	if r, ok := s.Resource(typ + "s"); ok {
		return r, true
	}
	if r, ok := s.Resource(typ); ok {
		return r, true
	}
	return nil, false
}

// Placeholders returns the {name} placeholders in a path template, in order.
func Placeholders(path string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

// Expand substitutes placeholders in a path template. It returns an error if
// any placeholder is missing from vars.
func Expand(path string, vars map[string]string) (string, error) {
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(path, func(m string) string {
		key := m[1 : len(m)-1]
		v, ok := vars[key]
		if !ok || v == "" {
			missing = append(missing, key)
			return m
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("missing path parameters: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// FullPath joins the base path with a resource path.
func (s *Spec) FullPath(p string) string {
	return strings.TrimRight(s.BasePath, "/") + "/" + strings.TrimLeft(p, "/")
}
