// Package openapi infers a navigation spec from an OpenAPI 3.x or Swagger 2
// document. The mapping is heuristic: every GET on a collection path becomes
// a resource, GET on the same path plus a trailing {id} becomes its item
// endpoint, nested collections become related links, and response schemas
// are inspected to find the wrapper keys and display columns.
package openapi

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Reisender/api-browser/internal/spec"
)

// IsOpenAPI reports whether data looks like an OpenAPI/Swagger document.
func IsOpenAPI(data []byte) bool {
	var probe struct {
		OpenAPI string `yaml:"openapi" json:"openapi"`
		Swagger string `yaml:"swagger" json:"swagger"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.OpenAPI != "" || probe.Swagger != ""
}

// Convert parses an OpenAPI/Swagger document and infers a Spec.
func Convert(data []byte) (*spec.Spec, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}
	c := &converter{doc: doc, v2: str(doc["swagger"]) != ""}
	return c.convert()
}

type converter struct {
	doc map[string]any
	v2  bool
}

// op is a GET operation we care about.
type op struct {
	path     string   // as written, e.g. /classes/{sourcedId}/students
	segments []string // path split on /
	params   []param
	schema   map[string]any // resolved 200 response schema, may be nil
}

type param struct {
	name, in, desc, def string
}

func (c *converter) convert() (*spec.Spec, error) {
	s := &spec.Spec{RefTypes: map[string]string{}}
	info, _ := c.doc["info"].(map[string]any)
	s.Name = str(info["title"])
	if s.Name == "" {
		s.Name = "OpenAPI"
	}
	if v := str(info["version"]); v != "" {
		s.Name += " " + v
	}
	s.Description = firstLine(str(info["description"]))
	s.BasePath = c.basePath()

	ops := c.gets()
	if len(ops) == 0 {
		return nil, fmt.Errorf("openapi: no GET operations found")
	}

	// Index operations by path.
	byPath := map[string]*op{}
	for i := range ops {
		byPath[ops[i].path] = &ops[i]
	}

	// Collections are GET paths not ending in a placeholder.
	type coll struct {
		list *op
		item *op
		id   string // placeholder name of the item path
	}
	colls := map[string]*coll{}
	var order []string
	for i := range ops {
		o := &ops[i]
		if isPlaceholder(last(o.segments)) {
			continue
		}
		colls[o.path] = &coll{list: o}
		order = append(order, o.path)
	}
	for i := range ops {
		o := &ops[i]
		if len(o.segments) < 2 || !isPlaceholder(last(o.segments)) {
			continue
		}
		parent := "/" + strings.Join(o.segments[:len(o.segments)-1], "/")
		if cl, ok := colls[parent]; ok {
			cl.item = o
			cl.id = trimPlaceholder(last(o.segments))
		}
	}
	sort.Strings(order)

	// Top-level resources: collection paths whose only placeholders are none.
	// Nested collections (/a/{id}/b) become related links on resource a.
	type pending struct {
		parent string
		rel    spec.Related
	}
	var rels []pending
	idVotes := map[string]int{}
	queryParams := map[string]spec.Param{}
	var queryOrder []string
	names := map[string]bool{}

	for _, p := range order {
		cl := colls[p]
		segs := cl.list.segments
		hasPH := false
		for _, sg := range segs {
			if isPlaceholder(sg) {
				hasPH = true
			}
		}
		listKey, itemSchema := listShape(cl.list.schema)
		for _, q := range cl.list.params {
			if q.in == "query" {
				if _, seen := queryParams[q.name]; !seen {
					queryParams[q.name] = spec.Param{Name: q.name, Description: q.desc, Default: q.def}
					queryOrder = append(queryOrder, q.name)
				}
			}
		}
		if hasPH {
			// Nested: parent is the longest prefix ending before the placeholder.
			for i := len(segs) - 1; i >= 0; i-- {
				if isPlaceholder(segs[i]) {
					parentPath := "/" + strings.Join(segs[:i], "/")
					rels = append(rels, pending{parent: parentPath, rel: spec.Related{
						Name: last(segs), Path: p, ListKey: listKey, Resource: last(segs),
					}})
					break
				}
			}
			continue
		}
		name := uniqueName(last(segs), names)
		r := spec.Resource{
			Name:        name,
			Description: firstLine(opDesc(c, cl.list.path)),
			ListPath:    p,
			ListKey:     listKey,
		}
		if cl.item != nil {
			r.ItemPath = cl.item.path
			var one map[string]any
			r.ItemKey, one = itemShape(cl.item.schema)
			if itemSchema == nil {
				itemSchema = one
			}
			if cl.id != "" {
				idVotes[cl.id]++
			}
		}
		r.Columns = columns(c, itemSchema)
		s.Resources = append(s.Resources, r)
		if sing := singular(name); sing != name {
			s.RefTypes[sing] = name
		}
	}

	// Attach related links whose parent resolved to a resource and whose
	// target resource exists (otherwise leave Resource empty).
	resByPath := map[string]*spec.Resource{}
	for i := range s.Resources {
		resByPath[s.Resources[i].ListPath] = &s.Resources[i]
	}
	for _, pr := range rels {
		parent, ok := resByPath[pr.parent]
		if !ok {
			continue
		}
		rel := pr.rel
		if _, ok := s.Resource(rel.Resource); !ok {
			rel.Resource = guessResource(s, rel)
		}
		parent.Related = append(parent.Related, rel)
	}

	// Id field: the most common item placeholder that is also a column, else "id".
	s.IDField = "id"
	best := 0
	for name, n := range idVotes {
		if n > best || (n == best && name < s.IDField) {
			s.IDField, best = name, n
		}
	}
	// Placeholder names like petId often map to a plain "id" property.
	if !anyColumn(s, s.IDField) && anyColumn(s, "id") {
		s.IDField = "id"
	}

	for _, q := range queryOrder {
		s.QueryParams = append(s.QueryParams, queryParams[q])
	}
	s.Paging = detectPaging(queryParams)
	if s.Paging != nil {
		for i := range s.QueryParams {
			if s.QueryParams[i].Name == s.Paging.LimitParam && s.QueryParams[i].Default == "" {
				s.QueryParams[i].Default = fmt.Sprint(s.Paging.DefaultLimit)
			}
			if s.QueryParams[i].Name == s.Paging.OffsetParam && s.QueryParams[i].Default == "" {
				s.QueryParams[i].Default = "0"
			}
		}
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("openapi: inferred spec invalid: %w", err)
	}
	return s, nil
}

// --- document access -------------------------------------------------------

func (c *converter) basePath() string {
	if c.v2 {
		return str(c.doc["basePath"])
	}
	servers, _ := c.doc["servers"].([]any)
	if len(servers) == 0 {
		return ""
	}
	srv, _ := servers[0].(map[string]any)
	raw := str(srv["url"])
	// Substitute server variables with their defaults.
	if vars, ok := srv["variables"].(map[string]any); ok {
		for k, v := range vars {
			vm, _ := v.(map[string]any)
			raw = strings.ReplaceAll(raw, "{"+k+"}", str(vm["default"]))
		}
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimRight(u.Path, "/")
	}
	return strings.TrimRight(raw, "/")
}

func (c *converter) gets() []op {
	paths, _ := c.doc["paths"].(map[string]any)
	var out []op
	for p, v := range paths {
		pi, _ := v.(map[string]any)
		g, ok := pi["get"].(map[string]any)
		if !ok {
			continue
		}
		o := op{path: p, segments: strings.Split(strings.Trim(p, "/"), "/")}
		// Path-level params apply to all operations.
		o.params = append(o.params, c.params(pi["parameters"])...)
		o.params = append(o.params, c.params(g["parameters"])...)
		o.schema = c.responseSchema(g)
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func (c *converter) params(v any) []param {
	arr, _ := v.([]any)
	var out []param
	for _, it := range arr {
		m := c.resolve(it)
		if m == nil {
			continue
		}
		p := param{name: str(m["name"]), in: str(m["in"]), desc: firstLine(str(m["description"]))}
		if sch := c.resolve(m["schema"]); sch != nil {
			if d, ok := sch["default"]; ok {
				p.def = fmt.Sprint(d)
			}
		} else if d, ok := m["default"]; ok { // swagger 2
			p.def = fmt.Sprint(d)
		}
		if p.name != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *converter) responseSchema(g map[string]any) map[string]any {
	resps, _ := g["responses"].(map[string]any)
	var r map[string]any
	for _, code := range []string{"200", "2XX", "default"} {
		if v, ok := resps[code]; ok {
			r = c.resolve(v)
			break
		}
	}
	if r == nil {
		return nil
	}
	if c.v2 {
		return c.deref(c.resolve(r["schema"]))
	}
	content, _ := r["content"].(map[string]any)
	for _, mt := range []string{"application/json", "application/*+json", "*/*"} {
		if m, ok := content[mt].(map[string]any); ok {
			return c.deref(c.resolve(m["schema"]))
		}
	}
	for _, v := range content { // any json-ish media type
		if m, ok := v.(map[string]any); ok {
			return c.deref(c.resolve(m["schema"]))
		}
	}
	return nil
}

// resolve follows a local $ref (repeatedly) and returns the target map.
func (c *converter) resolve(v any) map[string]any {
	for i := 0; i < 16; i++ {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		ref := str(m["$ref"])
		if ref == "" {
			return m
		}
		if !strings.HasPrefix(ref, "#/") {
			return nil
		}
		var cur any = c.doc
		for _, seg := range strings.Split(ref[2:], "/") {
			seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
			cm, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = cm[seg]
		}
		v = cur
	}
	return nil
}

// deref resolves $ref inside a schema's composition (allOf) shallowly so that
// properties from referenced parts are visible.
func (c *converter) deref(sch map[string]any) map[string]any {
	if sch == nil {
		return nil
	}
	all, ok := sch["allOf"].([]any)
	if !ok {
		return sch
	}
	merged := map[string]any{}
	props := map[string]any{}
	for _, part := range all {
		pm := c.deref(c.resolve(part))
		for k, v := range pm {
			if k != "properties" {
				merged[k] = v
			}
		}
		if pp, ok := pm["properties"].(map[string]any); ok {
			for k, v := range pp {
				props[k] = v
			}
		}
	}
	for k, v := range sch {
		if k != "allOf" && k != "properties" {
			merged[k] = v
		}
	}
	if pp, ok := sch["properties"].(map[string]any); ok {
		for k, v := range pp {
			props[k] = v
		}
	}
	if len(props) > 0 {
		merged["properties"] = props
	}
	if merged["type"] == nil {
		merged["type"] = "object"
	}
	return merged
}

func (c *converter) props(sch map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	pp, _ := sch["properties"].(map[string]any)
	for k, v := range pp {
		if m := c.deref(c.resolve(v)); m != nil {
			out[k] = m
		} else {
			out[k] = map[string]any{}
		}
	}
	return out
}

// --- shape inference -------------------------------------------------------

// listShape returns the wrapper key holding the array (empty for a bare
// array) and the item schema, for a list response.
func listShape(sch map[string]any) (string, map[string]any) {
	if sch == nil {
		return "", nil
	}
	if str(sch["type"]) == "array" {
		items, _ := sch["items"].(map[string]any)
		return "", items
	}
	props, _ := sch["properties"].(map[string]any)
	var arrays []string
	for k, v := range props {
		if m, ok := v.(map[string]any); ok && str(m["type"]) == "array" {
			arrays = append(arrays, k)
		}
	}
	if len(arrays) == 0 {
		return "", nil
	}
	sort.Strings(arrays)
	// Prefer an array of objects; with several, prefer the one not named like paging metadata.
	best := arrays[0]
	for _, k := range arrays {
		m := props[k].(map[string]any)
		if items, ok := m["items"].(map[string]any); ok && (str(items["type"]) == "object" || items["properties"] != nil || items["$ref"] != nil) {
			best = k
			break
		}
	}
	items, _ := props[best].(map[string]any)["items"].(map[string]any)
	return best, items
}

// itemShape returns the wrapper key holding a single object (empty for a
// bare object) and the object schema, for an item response.
func itemShape(sch map[string]any) (string, map[string]any) {
	if sch == nil {
		return "", nil
	}
	props, _ := sch["properties"].(map[string]any)
	if len(props) == 1 {
		for k, v := range props {
			if m, ok := v.(map[string]any); ok && (str(m["type"]) == "object" || m["properties"] != nil || m["$ref"] != nil) {
				return k, m
			}
		}
	}
	return "", sch
}

func columns(c *converter, sch map[string]any) []string {
	sch = c.deref(c.resolve(sch))
	if sch == nil {
		return nil
	}
	props := c.props(sch)
	var scalars []string
	for k, p := range props {
		switch str(p["type"]) {
		case "object", "array":
			continue
		}
		if p["properties"] != nil || p["items"] != nil {
			continue
		}
		scalars = append(scalars, k)
	}
	sort.Slice(scalars, func(i, j int) bool {
		return colRank(scalars[i]) < colRank(scalars[j]) || (colRank(scalars[i]) == colRank(scalars[j]) && scalars[i] < scalars[j])
	})
	if len(scalars) > 8 {
		scalars = scalars[:8]
	}
	return scalars
}

// colRank puts identifiers and names first, status-like fields last.
func colRank(k string) int {
	lk := strings.ToLower(k)
	switch {
	case lk == "id" || strings.HasSuffix(lk, "id"):
		return 0
	case strings.Contains(lk, "name") || strings.Contains(lk, "title"):
		return 1
	case lk == "status" || lk == "state":
		return 3
	}
	return 2
}

var pagingPairs = [][2]string{
	{"limit", "offset"}, {"pageSize", "offset"}, {"page_size", "offset"}, {"count", "offset"}, {"per_page", "offset"}, {"perPage", "offset"}, {"limit", "skip"}, {"top", "skip"}, {"$top", "$skip"},
}

func detectPaging(qp map[string]spec.Param) *spec.Paging {
	for _, pair := range pagingPairs {
		l, okL := qp[pair[0]]
		_, okO := qp[pair[1]]
		if okL && okO {
			pg := &spec.Paging{LimitParam: pair[0], OffsetParam: pair[1], DefaultLimit: 100}
			if l.Default != "" {
				var n int
				if _, err := fmt.Sscanf(l.Default, "%d", &n); err == nil && n > 0 {
					pg.DefaultLimit = n
				}
			}
			return pg
		}
	}
	return nil
}

func guessResource(s *spec.Spec, rel spec.Related) string {
	// Match by list key (e.g. /schools/{id}/students returns "users").
	if rel.ListKey != "" {
		for _, r := range s.Resources {
			if r.ListKey == rel.ListKey && r.Name == rel.ListKey {
				return r.Name
			}
		}
		for _, r := range s.Resources {
			if r.ListKey == rel.ListKey {
				return r.Name
			}
		}
	}
	return ""
}

// --- helpers -----------------------------------------------------------------

func opDesc(c *converter, path string) string {
	paths, _ := c.doc["paths"].(map[string]any)
	pi, _ := paths[path].(map[string]any)
	g, _ := pi["get"].(map[string]any)
	if s := str(g["summary"]); s != "" {
		return s
	}
	return str(g["description"])
}

func anyColumn(s *spec.Spec, col string) bool {
	for _, r := range s.Resources {
		for _, c := range r.Columns {
			if c == col {
				return true
			}
		}
	}
	return false
}

var phRe = regexp.MustCompile(`^\{[^}]+\}$`)

func isPlaceholder(seg string) bool     { return phRe.MatchString(seg) }
func trimPlaceholder(seg string) string { return strings.Trim(seg, "{}") }

func last(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:119] + "…"
	}
	return s
}

func uniqueName(base string, seen map[string]bool) string {
	name := base
	for i := 2; seen[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	seen[name] = true
	return name
}

// singular derives a reference type name from a plural resource name.
func singular(n string) string {
	switch {
	case strings.HasSuffix(n, "ies"):
		return n[:len(n)-3] + "y"
	case strings.HasSuffix(n, "ses") || strings.HasSuffix(n, "xes") || strings.HasSuffix(n, "ches") || strings.HasSuffix(n, "shes"):
		return n[:len(n)-2]
	case strings.HasSuffix(n, "s") && !strings.HasSuffix(n, "ss") && !strings.HasSuffix(n, "us"):
		return n[:len(n)-1]
	}
	return n
}

// LoadAny loads a spec from a builtin name or a file path, accepting either
// the native format or an OpenAPI/Swagger document.
func LoadAny(nameOrPath string) (*spec.Spec, error) {
	data, err := os.ReadFile(nameOrPath)
	if err != nil {
		if os.IsNotExist(err) {
			return spec.LoadBuiltin(nameOrPath)
		}
		return nil, err
	}
	if IsOpenAPI(data) {
		return Convert(data)
	}
	return spec.Parse(data)
}
