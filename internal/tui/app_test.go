package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/auth"
	"github.com/Reisender/api-browser/internal/config"
	"github.com/Reisender/api-browser/internal/spec"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/ims/oneroster/v1p1/classes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			j(w, map[string]any{"error": "nope"})
			return
		}
		if r.URL.Query().Get("offset") == "2" {
			j(w, map[string]any{"classes": []any{map[string]any{"sourcedId": "c3", "title": "Page2"}}})
			return
		}
		if f := r.URL.Query().Get("filter"); f != "" {
			j(w, map[string]any{"classes": []any{map[string]any{"sourcedId": "cf", "title": "Filtered " + f}}})
			return
		}
		j(w, map[string]any{"classes": []any{
			map[string]any{"sourcedId": "c1", "title": "Math", "status": "active"},
			map[string]any{"sourcedId": "c2", "title": "Art", "status": "active"},
		}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/classes/c1", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"class": map[string]any{
			"sourcedId": "c1", "title": "Math",
			"course": map[string]any{"sourcedId": "k1", "type": "course", "href": "x"},
			"terms":  []any{map[string]any{"sourcedId": "t1", "type": "term"}},
		}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/courses/k1", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"course": map[string]any{"sourcedId": "k1", "title": "Mathematics"}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/classes/c1/students", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"users": []any{map[string]any{"sourcedId": "s1", "givenName": "Ada"}}})
	})
	mux.HandleFunc("/ims/oneroster/v1p1/academicSessions", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"academicSessions": []any{}})
	})
	return httptest.NewServer(mux)
}

func init() { cursorMode = cursor.CursorStatic }

func newTestApp(t *testing.T, srv *httptest.Server) *App {
	t.Helper()
	s, err := spec.LoadBuiltin("oneroster-v1p1")
	if err != nil {
		t.Fatal(err)
	}
	p := config.Profile{Name: "test", BaseURL: srv.URL, Spec: "oneroster-v1p1", Auth: auth.Config{Method: auth.MethodBearer, Token: "tok"}}
	a, err := New(s, p, filepath.Join(t.TempDir(), "cfg.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return a
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+t":
		return tea.KeyMsg{Type: tea.KeyCtrlT}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press sends a key and runs any resulting command synchronously, feeding
// its message back into the model (handles fetches and status updates).
func press(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		_, cmd := a.Update(key(k))
		drain(t, a, cmd)
	}
}

func drain(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		switch m := msg.(type) {
		case tea.BatchMsg:
			for _, c := range m {
				drain(t, a, c)
			}
			return
		case tea.QuitMsg:
			return
		}
		// tea.Tick messages are functions that block; skip clear-status ticks.
		if _, ok := msg.(clearStatusMsg); ok {
			return
		}
		_, cmd = a.Update(msg)
		if _, ok := msg.(statusMsg); ok {
			return // don't wait on the 8s clear tick
		}
	}
}

func typeText(t *testing.T, a *App, s string) {
	t.Helper()
	for _, r := range s {
		press(t, a, string(r))
	}
}

func selectResource(t *testing.T, a *App, name string) {
	t.Helper()
	rs, ok := a.top().(*resourcesScreen)
	if !ok {
		t.Fatalf("top is %T, want resources", a.top())
	}
	for i, it := range rs.list.Items() {
		if it.(resourceItem).r.Name == name {
			rs.list.Select(i)
			return
		}
	}
	t.Fatalf("resource %s not found", name)
}

func TestNavigateListItemRefRelated(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	if a.Depth() != 1 {
		t.Fatalf("depth = %d", a.Depth())
	}

	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs, ok := a.top().(*collectionScreen)
	if !ok {
		t.Fatalf("top is %T, want collection; status=%q", a.top(), a.status)
	}
	if len(cs.resp.Items) != 2 || cs.cols[0] != "sourcedId" || cs.cols[1] != "title" {
		t.Errorf("items=%d cols=%v", len(cs.resp.Items), cs.cols)
	}
	if !strings.Contains(a.status, "HTTP 200") {
		t.Errorf("status = %q", a.status)
	}
	view := a.View()
	for _, want := range []string{"Math", "Art", "classes", "2 items"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// Open item c1.
	press(t, a, "enter")
	is, ok := a.top().(*itemScreen)
	if !ok {
		t.Fatalf("top is %T, want item; status=%q", a.top(), a.status)
	}
	if is.resp.Item["title"] != "Math" {
		t.Errorf("item = %+v", is.resp.Item)
	}
	view = a.View()
	if !strings.Contains(view, "course/k1") || !strings.Contains(view, "→ courses") {
		t.Errorf("item view should render reference:\n%s", view)
	}

	// Move cursor to the "course" ref (rows are sorted: course, sourcedId, terms, title).
	var idx int
	for i, r := range is.rows {
		if r.Path == "course" {
			idx = i
		}
	}
	for i := 0; i < idx; i++ {
		press(t, a, "down")
	}
	press(t, a, "enter")
	ref, ok := a.top().(*itemScreen)
	if !ok || ref.resp.Item["title"] != "Mathematics" {
		t.Fatalf("expected followed ref course k1, top=%T status=%q", a.top(), a.status)
	}
	if a.Depth() != 4 {
		t.Errorf("depth = %d", a.Depth())
	}
	if !strings.Contains(a.View(), "courses/k1") {
		t.Errorf("breadcrumb missing courses/k1")
	}

	// Back to class, then related -> students.
	press(t, a, "esc")
	if _, ok := a.top().(*itemScreen); !ok {
		t.Fatalf("expected class item after back")
	}
	press(t, a, "l")
	ms, ok := a.top().(*menuScreen)
	if !ok {
		t.Fatalf("top is %T, want menu", a.top())
	}
	if ms.opts[0].label != "students" {
		t.Errorf("first related = %s", ms.opts[0].label)
	}
	press(t, a, "enter")
	rel, ok := a.top().(*collectionScreen)
	if !ok || len(rel.resp.Items) != 1 || rel.resp.Items[0]["givenName"] != "Ada" {
		t.Fatalf("expected students collection, top=%T status=%q", a.top(), a.status)
	}
	if rel.title() != "classes/c1/students" {
		t.Errorf("title = %s", rel.title())
	}

	// Raw view and back.
	press(t, a, "r")
	if _, ok := a.top().(*rawScreen); !ok {
		t.Fatalf("expected raw screen")
	}
	if !strings.Contains(a.View(), `"givenName": "Ada"`) {
		t.Errorf("raw view missing pretty JSON")
	}
	press(t, a, "backspace")
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Fatalf("expected collection after backspace")
	}

	// Home.
	press(t, a, "H")
	if a.Depth() != 1 {
		t.Errorf("H should reset stack, depth = %d", a.Depth())
	}
}

func TestPagingFilterAndEdit(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")
	press(t, a, "enter")

	// Next page: limit is 100 by default and we got 2 -> "no more pages".
	press(t, a, "n")
	if !strings.Contains(a.status, "no more pages") {
		t.Errorf("status = %q", a.status)
	}
	// Shrink the limit via the request editor so paging kicks in.
	press(t, a, "e")
	rq, ok := a.top().(*requestScreen)
	if !ok {
		t.Fatalf("top is %T", a.top())
	}
	rq.form.set("q:limit", "2")
	press(t, a, "enter")
	cs := a.top().(*collectionScreen)
	if cs.req.Query["limit"] != "2" || a.Depth() != 2 {
		t.Errorf("limit=%q depth=%d", cs.req.Query["limit"], a.Depth())
	}
	press(t, a, "n")
	cs = a.top().(*collectionScreen)
	if cs.req.Query["offset"] != "2" || len(cs.resp.Items) != 1 || cs.resp.Items[0]["title"] != "Page2" {
		t.Errorf("after next: offset=%q items=%v", cs.req.Query["offset"], cs.resp.Items)
	}
	if a.Depth() != 2 {
		t.Errorf("paging should replace in place, depth = %d", a.Depth())
	}
	press(t, a, "p")
	cs = a.top().(*collectionScreen)
	if cs.req.Query["offset"] != "0" || len(cs.resp.Items) != 2 {
		t.Errorf("after prev: offset=%q items=%d", cs.req.Query["offset"], len(cs.resp.Items))
	}
	press(t, a, "p")
	if !strings.Contains(a.status, "first page") {
		t.Errorf("status = %q", a.status)
	}

	// Quick filter.
	press(t, a, "f")
	if _, ok := a.top().(*quickParamScreen); !ok {
		t.Fatalf("top is %T", a.top())
	}
	typeText(t, a, "status='active'")
	press(t, a, "enter")
	cs = a.top().(*collectionScreen)
	if cs.req.Query["filter"] != "status='active'" || len(cs.resp.Items) != 1 || !strings.HasPrefix(cs.resp.Items[0]["title"].(string), "Filtered") {
		t.Errorf("filter req=%v items=%v", cs.req.Query, cs.resp.Items)
	}
	if !strings.Contains(a.View(), "filter=status='active'") {
		t.Error("view should show active filter")
	}

	// URL status.
	press(t, a, "u")
	if !strings.Contains(a.status, "/ims/oneroster/v1p1/classes?") {
		t.Errorf("status = %q", a.status)
	}
	// Copy id with a stubbed clipboard.
	var copied string
	copyToClipboard = func(s string) error { copied = s; return nil }
	press(t, a, "y")
	if copied != "cf" {
		t.Errorf("copied = %q", copied)
	}
}

func TestEmptyCollectionAndErrors(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "academicSessions")
	press(t, a, "enter")
	if !strings.Contains(a.View(), "empty result") {
		t.Errorf("expected empty message")
	}
	press(t, a, "enter") // nothing selected; must not panic
	press(t, a, "esc")

	// Bad auth -> error status + raw error body pushed.
	a.client.Auth = auth.Bearer{Token: "bad"}
	selectResource(t, a, "classes")
	press(t, a, "enter")
	if !a.statusErr || !strings.Contains(a.status, "401") {
		t.Errorf("status = %q err=%v", a.status, a.statusErr)
	}
	if rs, ok := a.top().(*rawScreen); !ok || !strings.Contains(rs.text, "nope") {
		t.Errorf("expected raw error screen, top=%T", a.top())
	}
}

func TestHelpQuitAndStaleFetch(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	press(t, a, "?")
	if !a.showHelp || !strings.Contains(a.View(), "Everywhere") {
		t.Error("help overlay not shown")
	}
	press(t, a, "x")
	if a.showHelp {
		t.Error("help should close on any key")
	}
	// Stale fetch results are ignored.
	a.seq = 5
	_, cmd := a.Update(fetchMsg{seq: 4, kind: fetchList})
	if cmd != nil || a.Depth() != 1 {
		t.Error("stale fetch should be dropped")
	}
	// Cancel in-flight.
	a.loading = "x"
	press(t, a, "esc")
	if a.loading != "" || a.status != "cancelled" {
		t.Errorf("loading=%q status=%q", a.loading, a.status)
	}
	// q at top quits.
	_, cmd = a.Update(key("q"))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q at top level should quit")
	}
}

func TestConnectionScreen(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	s, _ := spec.LoadBuiltin("oneroster-v1p1")
	cfgPath := filepath.Join(t.TempDir(), "c.yaml")
	a, err := New(s, config.Profile{}, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	cs, ok := a.top().(*connectionScreen)
	if !ok {
		t.Fatalf("empty profile should open connection screen, got %T", a.top())
	}
	// Enter without base URL -> error.
	press(t, a, "enter")
	if !a.statusErr || !strings.Contains(a.status, "base URL") {
		t.Errorf("status = %q", a.status)
	}
	typeText(t, a, srv.URL)
	press(t, a, "tab")
	typeText(t, a, "district")
	press(t, a, "tab") // method choice
	// Cycle to "bearer" with right arrow.
	press(t, a, "right")
	if cs.form.get("method") != "bearer" {
		t.Errorf("method = %q", cs.form.get("method"))
	}
	press(t, a, "enter") // enter on a choice cycles it
	if cs.form.get("method") != "oauth2" {
		t.Errorf("method after enter = %q", cs.form.get("method"))
	}
	press(t, a, "left")
	press(t, a, "tab") // token
	typeText(t, a, "tok")
	// Test connection.
	press(t, a, "ctrl+t")
	if a.statusErr || !strings.Contains(a.status, "test OK") {
		t.Errorf("test status = %q", a.status)
	}
	if a.Depth() != 2 {
		t.Errorf("test should not push a screen, depth=%d", a.Depth())
	}
	// Save.
	press(t, a, "ctrl+s")
	if a.statusErr {
		t.Fatalf("save: %s", a.status)
	}
	f, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := f.Get("district")
	if !ok || p.BaseURL != srv.URL || p.Auth.Method != auth.MethodBearer || p.Auth.Token != "tok" || f.Default != "district" {
		t.Errorf("saved profile = %+v default=%q", p, f.Default)
	}
	// Apply and go browse.
	press(t, a, "enter")
	if a.Depth() != 1 {
		t.Errorf("depth after apply = %d", a.Depth())
	}
	if !strings.Contains(a.View(), "bearer") {
		t.Error("header should describe auth")
	}
	selectResource(t, a, "classes")
	press(t, a, "enter")
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Fatalf("browse after connect failed: %q", a.status)
	}
	// 'a' reopens the connection screen; esc cancels.
	press(t, a, "a")
	if _, ok := a.top().(*connectionScreen); !ok {
		t.Fatal("a should open connection")
	}
	press(t, a, "esc")
	if _, ok := a.top().(*collectionScreen); !ok {
		t.Fatal("esc should return")
	}
}

func TestHeadersParse(t *testing.T) {
	h := parseHeaders("X-A: 1; X-B: two words ;bad; : nokey")
	if len(h) != 2 || h["X-A"] != "1" || h["X-B"] != "two words" {
		t.Errorf("parseHeaders = %v", h)
	}
	if j := joinHeaders(map[string]string{"K": "v"}); j != "K: v" {
		t.Errorf("joinHeaders = %q", j)
	}
}

func TestPickColumns(t *testing.T) {
	res := &spec.Resource{Columns: []string{"sourcedId", "title", "missing"}}
	items := []map[string]any{{"sourcedId": "1", "zeta": "z", "title": "t", "nested": map[string]any{}}, {"alpha": 1}}
	cols := pickColumns(res, items)
	want := "sourcedId,title,alpha,zeta"
	if got := strings.Join(cols, ","); got != want {
		t.Errorf("cols = %s want %s", got, want)
	}
	if got := pickColumns(nil, nil); got != nil {
		t.Errorf("no items -> nil cols, got %v", got)
	}
}

func TestCollectionSearch(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs := a.top().(*collectionScreen)
	if len(cs.filtered) != 2 {
		t.Fatalf("filtered = %v", cs.filtered)
	}

	// "/" opens live search; typing narrows immediately.
	press(t, a, "/")
	if !cs.searching {
		t.Fatal("expected searching")
	}
	typeText(t, a, "ar")
	if len(cs.filtered) != 1 || cs.resp.Items[cs.filtered[0]]["title"] != "Art" {
		t.Errorf("filtered = %v", cs.filtered)
	}
	view := a.View()
	if !strings.Contains(view, "1 of 2 items") || !strings.Contains(view, "/ar") || strings.Contains(view, "Math") {
		t.Errorf("view:\n%s", view)
	}
	// Case-insensitive, multi-word AND, matches nested/any field.
	cs.search.SetValue("ACTIVE math")
	cs.applySearch()
	if len(cs.filtered) != 1 || cs.resp.Items[cs.filtered[0]]["title"] != "Math" {
		t.Errorf("multi-word filtered = %v", cs.filtered)
	}
	cs.search.SetValue("zzz")
	cs.applySearch()
	if len(cs.filtered) != 0 || !strings.Contains(a.View(), "no rows match") {
		t.Errorf("expected no match, filtered=%v", cs.filtered)
	}
	cs.search.SetValue("ar")
	cs.applySearch()

	// Enter keeps the filter and returns focus to the table; keys work again.
	press(t, a, "enter")
	if cs.searching || a.top() != cs {
		t.Fatalf("enter should close search box; searching=%v top=%T", cs.searching, a.top())
	}
	// Opening the selected row resolves to the *filtered* record (Art = c2 -> 404 on server,
	// so check the request rather than the response).
	if it, ok := cs.selected(); !ok || it["sourcedId"] != "c2" {
		t.Errorf("selected = %v", it)
	}
	// Paging/reload preserves the search text.
	press(t, a, "R")
	cs = a.top().(*collectionScreen)
	if cs.search.Value() != "ar" || len(cs.filtered) != 1 {
		t.Errorf("reload lost search: %q %v", cs.search.Value(), cs.filtered)
	}
	// First esc clears the search, second esc goes back.
	press(t, a, "esc")
	if cs.search.Value() != "" || len(cs.filtered) != 2 || a.Depth() != 2 {
		t.Errorf("esc should clear search: %q depth=%d", cs.search.Value(), a.Depth())
	}
	press(t, a, "esc")
	if a.Depth() != 1 {
		t.Errorf("second esc should pop, depth=%d", a.Depth())
	}

	// Esc while typing cancels and clears.
	selectResource(t, a, "classes")
	press(t, a, "enter")
	cs = a.top().(*collectionScreen)
	press(t, a, "/")
	typeText(t, a, "q") // 'q' must be typed, not treated as quit/back
	if a.Depth() != 2 || cs.search.Value() != "q" {
		t.Fatalf("q inside search box should type; depth=%d value=%q", a.Depth(), cs.search.Value())
	}
	press(t, a, "esc")
	if cs.searching || cs.search.Value() != "" || len(cs.filtered) != 2 || a.Depth() != 2 {
		t.Errorf("esc in search: searching=%v value=%q filtered=%v depth=%d", cs.searching, cs.search.Value(), cs.filtered, a.Depth())
	}
}

func TestQuickPageSize(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "classes")
	press(t, a, "enter")
	if !strings.Contains(a.View(), "limit 100") || !strings.Contains(a.View(), "L to change") {
		t.Errorf("header should show limit and hint:\n%s", a.View())
	}
	press(t, a, "L")
	qp, ok := a.top().(*quickParamScreen)
	if !ok || qp.param != "limit" || qp.form.get("limit") != "100" {
		t.Fatalf("top=%T param=%v", a.top(), qp)
	}
	// Invalid value is rejected and the editor stays open.
	press(t, a, "ctrl+u")
	typeText(t, a, "lots")
	press(t, a, "enter")
	if !a.statusErr || a.top() != qp {
		t.Errorf("expected validation error, status=%q top=%T", a.status, a.top())
	}
	press(t, a, "ctrl+u")
	typeText(t, a, "1000")
	press(t, a, "enter")
	cs, ok := a.top().(*collectionScreen)
	if !ok || cs.req.Query["limit"] != "1000" || cs.req.Query["offset"] != "0" {
		t.Fatalf("top=%T query=%v", a.top(), cs.req.Query)
	}
	if !strings.Contains(cs.resp.URL, "limit=1000") || !strings.Contains(a.View(), "limit 1000") {
		t.Errorf("url=%s", cs.resp.URL)
	}
	// Paging math uses the new size.
	press(t, a, "n")
	if !strings.Contains(a.status, "no more pages") {
		t.Errorf("status = %q", a.status)
	}
	// The full editor also exposes limit, pre-filled with the current value.
	press(t, a, "e")
	rq := a.top().(*requestScreen)
	if rq.form.get("q:limit") != "1000" {
		t.Errorf("editor limit = %q", rq.form.get("q:limit"))
	}
}

// pagedServer serves N records in pages according to limit/offset.
func pagedServer(t *testing.T, total int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ims/oneroster/v1p1/users", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		var users []any
		for i := offset; i < total && i < offset+limit; i++ {
			name := "User"
			if i%7 == 0 {
				name = "Zed"
			}
			users = append(users, map[string]any{"sourcedId": fmt.Sprintf("u%03d", i), "givenName": name})
		}
		if users == nil {
			users = []any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
	})
	return httptest.NewServer(mux)
}

func TestFetchAllPagesAndSearch(t *testing.T) {
	srv := pagedServer(t, 23)
	defer srv.Close()
	a := newTestApp(t, srv)
	selectResource(t, a, "users")
	press(t, a, "e")
	a.top().(*requestScreen).form.set("q:limit", "5")
	press(t, a, "enter")
	cs := a.top().(*collectionScreen)
	if len(cs.resp.Items) != 5 {
		t.Fatalf("page 1 items = %d", len(cs.resp.Items))
	}

	// Search on one page, then ctrl+a to search across all pages.
	press(t, a, "/")
	typeText(t, a, "zed")
	if len(cs.filtered) != 1 {
		t.Fatalf("page-1 matches = %d", len(cs.filtered))
	}
	press(t, a, "ctrl+a")
	all, ok := a.top().(*collectionScreen)
	if !ok || !all.allPages {
		t.Fatalf("expected aggregated collection, top=%T status=%q", a.top(), a.status)
	}
	if len(all.resp.Items) != 23 || all.pages != 5 {
		t.Errorf("items=%d pages=%d", len(all.resp.Items), all.pages)
	}
	if all.search.Value() != "zed" || len(all.filtered) != 4 { // 0,7,14,21
		t.Errorf("search=%q matches=%d", all.search.Value(), len(all.filtered))
	}
	if a.Depth() != 2 {
		t.Errorf("should replace in place, depth=%d", a.Depth())
	}
	if !strings.Contains(a.status, "23 records from 5 page(s)") {
		t.Errorf("status = %q", a.status)
	}
	view := a.View()
	if !strings.Contains(view, "4 of 23 items") || !strings.Contains(view, "all pages (5 × 5)") {
		t.Errorf("view:\n%s", view)
	}
	// Paging is a no-op now; raw viewer shows the combined body.
	press(t, a, "n")
	if !strings.Contains(a.status, "already loaded") || a.top() != all {
		t.Errorf("status=%q", a.status)
	}
	press(t, a, "r")
	if rs, ok := a.top().(*rawScreen); !ok || strings.Count(rs.text, `"sourcedId"`) != 23 {
		t.Errorf("raw should contain all records")
	}
	press(t, a, "esc")
	// Selecting a filtered row maps to the right record.
	press(t, a, "esc") // clear search
	press(t, a, "G")
	if it, _ := all.selected(); it["sourcedId"] != "u022" {
		t.Errorf("last record = %v", it)
	}

	// 'A' from the table with a fresh page-1 view; and cancellation mid-walk.
	press(t, a, "R")
	cs = a.top().(*collectionScreen)
	if cs.allPages {
		t.Fatal("reload should return to single page")
	}
	_, cmd := a.Update(key("A"))
	if a.agg == nil || a.loading == "" {
		t.Fatal("expected aggregation in progress")
	}
	// Deliver the first page, then cancel before the second.
	_, cmd = a.Update(cmd())
	if a.agg == nil || a.agg.pages != 1 || !strings.Contains(a.loading, "page 2") {
		t.Fatalf("after page 1: agg=%+v loading=%q", a.agg, a.loading)
	}
	press(t, a, "esc")
	if a.agg != nil || a.loading != "" || !strings.Contains(a.status, "cancelled after 1 page") {
		t.Errorf("cancel: agg=%v loading=%q status=%q", a.agg, a.loading, a.status)
	}
	if a.top() != cs {
		t.Error("cancel should keep the original screen")
	}
	// The stale page-2 result (if it arrived) is dropped.
	_, _ = a.Update(cmd())
	if a.top() != cs {
		t.Error("stale result should be ignored")
	}
	_ = cmd
}
