package jsontree

import (
	"encoding/json"
	"strings"
	"testing"
)

func parse(t *testing.T, s string) any {
	t.Helper()
	var v any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func paths(rows []Node) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Path)
	}
	return out
}

func TestRowsAndToggle(t *testing.T) {
	v := parse(t, `{"b":1,"a":{"x":true,"y":[1,2]},"big":[1,2,3,4,5,6,7,8,9,10]}`)
	tr := New(v)
	rows := tr.Rows()
	// "a" is small so expanded by default; "big" is > 8 so collapsed; keys sorted.
	got := strings.Join(paths(rows), ",")
	want := "a,a.x,a.y,b,big"
	if got != want {
		t.Errorf("rows = %s\nwant %s", got, want)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 || rows[0].Count != 2 || !rows[0].IsBranch {
		t.Errorf("node metadata = %+v", rows[0])
	}
	tr.Toggle("big")
	if n := len(tr.Rows()); n != 15 {
		t.Errorf("after expanding big, rows = %d", n)
	}
	tr.ExpandAll(false)
	if got := strings.Join(paths(tr.Rows()), ","); got != "a,b,big" {
		t.Errorf("collapsed = %s", got)
	}
	tr.ExpandAll(true)
	if n := len(tr.Rows()); n != 17 {
		t.Errorf("expandAll rows = %d", n)
	}
}

func TestFormatSummaryLookup(t *testing.T) {
	v := parse(t, `{"s":"hi","n":1.5,"b":false,"z":null,"o":{"k":1},"l":[1]}`)
	m := v.(map[string]any)
	cases := map[string]string{"s": `"hi"`, "n": "1.5", "b": "false", "z": "null", "o": "{1}", "l": "[1]"}
	for k, want := range cases {
		if got := Format(m[k]); got != want {
			t.Errorf("Format(%s) = %s want %s", k, got, want)
		}
	}
	if s := Summary(m["o"], 100); s != `{"k":1}` {
		t.Errorf("Summary = %s", s)
	}
	if s := Summary(m["o"], 4); s != "{\"k…" {
		t.Errorf("truncated Summary = %q", s)
	}
	if x, ok := Lookup(v, "o.k"); !ok || Format(x) != "1" {
		t.Errorf("Lookup o.k = %v %v", x, ok)
	}
	if x, ok := Lookup(v, "l.0"); !ok || Format(x) != "1" {
		t.Errorf("Lookup l.0 = %v %v", x, ok)
	}
	for _, bad := range []string{"l.5", "l.x", "o.nope", "s.x"} {
		if _, ok := Lookup(v, bad); ok {
			t.Errorf("Lookup %s should fail", bad)
		}
	}
	if r, ok := Lookup(v, ""); !ok || r == nil {
		t.Error("Lookup root")
	}
}

func TestArrayRoot(t *testing.T) {
	tr := New(parse(t, `[{"a":1},{"a":2}]`))
	rows := tr.Rows()
	if got := strings.Join(paths(rows), ","); got != "0,1" {
		t.Errorf("rows = %s", got)
	}
	tr.Toggle("1")
	if got := strings.Join(paths(tr.Rows()), ","); got != "0,1,1.a" {
		t.Errorf("rows = %s", got)
	}
}
