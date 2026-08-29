// Package jsontree flattens JSON into an expandable list of rows for display.
package jsontree

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Node is one row of the tree.
type Node struct {
	Path     string // dotted path, e.g. orgs.0.sourcedId
	Key      string // display key (last path segment)
	Depth    int
	Value    any  // the JSON value at this node
	IsBranch bool // object or array
	Expanded bool
	Count    int // children count for branches
}

// Tree holds the full node set and expansion state.
type Tree struct {
	Root     any
	expanded map[string]bool
}

// New creates a tree with the top level expanded.
func New(root any) *Tree {
	t := &Tree{Root: root, expanded: map[string]bool{}}
	t.expanded[""] = true
	// Expand first level of children by default for quick scanning.
	switch v := root.(type) {
	case map[string]any:
		for k, c := range v {
			if isBranch(c) && size(c) <= 8 {
				t.expanded[k] = true
			}
		}
	}
	return t
}

// Toggle flips the expansion state of a path.
func (t *Tree) Toggle(path string) {
	t.expanded[path] = !t.expanded[path]
}

// Expand sets the expansion state of a path.
func (t *Tree) Expand(path string, on bool) { t.expanded[path] = on }

// ExpandAll expands (or collapses) every branch.
func (t *Tree) ExpandAll(on bool) {
	t.expanded = map[string]bool{"": true}
	if !on {
		return
	}
	var walk func(p string, v any)
	walk = func(p string, v any) {
		if !isBranch(v) {
			return
		}
		t.expanded[p] = true
		each(v, func(k string, c any) { walk(join(p, k), c) })
	}
	walk("", t.Root)
}

// IsExpanded reports the expansion state of a path.
func (t *Tree) IsExpanded(path string) bool { return t.expanded[path] }

// Rows returns the visible rows given the current expansion state.
func (t *Tree) Rows() []Node {
	var rows []Node
	var walk func(p string, key string, depth int, v any)
	walk = func(p, key string, depth int, v any) {
		n := Node{Path: p, Key: key, Depth: depth, Value: v, IsBranch: isBranch(v)}
		if n.IsBranch {
			n.Count = size(v)
			n.Expanded = t.expanded[p]
		}
		if p != "" {
			rows = append(rows, n)
		}
		if n.IsBranch && (p == "" || n.Expanded) {
			each(v, func(k string, c any) { walk(join(p, k), k, depth+1, c) })
		}
	}
	walk("", "", -1, t.Root)
	return rows
}

// Format renders a scalar value for display.
func Format(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("%q", x)
	case json.Number:
		return x.String()
	case bool:
		return fmt.Sprint(x)
	case map[string]any:
		return fmt.Sprintf("{%d}", len(x))
	case []any:
		return fmt.Sprintf("[%d]", len(x))
	}
	return fmt.Sprint(v)
}

// Summary renders a one-line preview of a branch value.
func Summary(v any, max int) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

func isBranch(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func size(v any) int {
	switch x := v.(type) {
	case map[string]any:
		return len(x)
	case []any:
		return len(x)
	}
	return 0
}

func each(v any, f func(k string, c any)) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			f(k, x[k])
		}
	case []any:
		for i, c := range x {
			f(fmt.Sprint(i), c)
		}
	}
}

func join(p, k string) string {
	if p == "" {
		return k
	}
	return p + "." + k
}

// Lookup returns the value at a dotted path.
func Lookup(root any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch x := cur.(type) {
		case map[string]any:
			v, ok := x[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			var i int
			if _, err := fmt.Sscanf(seg, "%d", &i); err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			cur = x[i]
		default:
			return nil, false
		}
	}
	return cur, true
}
