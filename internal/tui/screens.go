package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Reisender/api-browser/internal/client"
	"github.com/Reisender/api-browser/internal/jsontree"
	"github.com/Reisender/api-browser/internal/spec"
)

// screen is one layer of the navigation stack.
type screen interface {
	title() string
	update(a *App, msg tea.Msg) tea.Cmd
	view(a *App, w, h int) string
	help() []helpEntry
}

type helpEntry struct{ key, desc string }

// ---------------------------------------------------------------- resources

type resourceItem struct{ r *spec.Resource }

func (i resourceItem) Title() string       { return i.r.Name }
func (i resourceItem) Description() string { return i.r.Description }
func (i resourceItem) FilterValue() string { return i.r.Name + " " + i.r.Description }

type resourcesScreen struct {
	list list.Model
}

func newResourcesScreen(s *spec.Spec) *resourcesScreen {
	items := make([]list.Item, 0, len(s.Resources))
	for i := range s.Resources {
		items = append(items, resourceItem{&s.Resources[i]})
	}
	d := list.NewDefaultDelegate()
	l := list.New(items, d, 0, 0)
	l.Title = s.Name
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.Styles.Title = styleTitle
	return &resourcesScreen{list: l}
}

func (s *resourcesScreen) title() string { return "resources" }

func (s *resourcesScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && s.list.FilterState() != list.Filtering {
		switch k.String() {
		case "enter":
			if it, ok := s.list.SelectedItem().(resourceItem); ok {
				return a.openList(it.r, client.ListRequest(a.spec, it.r), it.r.Name)
			}
			return nil
		case "e":
			if it, ok := s.list.SelectedItem().(resourceItem); ok {
				a.push(newRequestScreen(a, it.r, client.ListRequest(a.spec, it.r), it.r.Name))
			}
			return nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

func (s *resourcesScreen) view(a *App, w, h int) string {
	s.list.SetSize(w, h)
	return s.list.View()
}

func (s *resourcesScreen) help() []helpEntry {
	return []helpEntry{{"enter", "list resource"}, {"e", "edit request first"}, {"/", "filter resources"}, {"a", "connection settings"}}
}

// --------------------------------------------------------------- collection

type collectionScreen struct {
	name     string
	resource *spec.Resource
	req      client.Request
	resp     *client.Response
	table    table.Model
	cols     []string

	// Local search: filtered is the index into resp.Items of each visible row.
	search    textinput.Model
	searching bool
	filtered  []int

	// allPages is set when resp.Items is the concatenation of every page.
	allPages bool
	pages    int
}

func newCollectionScreen(a *App, name string, res *spec.Resource, req client.Request, resp *client.Response) *collectionScreen {
	s := &collectionScreen{name: name, resource: res, req: req, resp: resp}
	s.cols = pickColumns(res, resp.Items)
	s.table = table.New(table.WithFocused(true))
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(colKey).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(colDim)
	st.Selected = st.Selected.Foreground(lipgloss.Color("230")).Background(colAccent).Bold(false)
	s.table.SetStyles(st)
	s.search = textinput.New()
	s.search.Prompt = "/"
	s.search.Placeholder = "search rows"
	s.search.Cursor.SetMode(cursorMode)
	s.applySearch()
	s.rebuild(80)
	return s
}

// applySearch recomputes the visible row set from the search text. Rows match
// when any cell (or the raw JSON of the record) contains the query,
// case-insensitively; multiple space-separated words must all match.
func (s *collectionScreen) applySearch() {
	words := strings.Fields(strings.ToLower(s.search.Value()))
	s.filtered = s.filtered[:0]
	for i, it := range s.resp.Items {
		if len(words) == 0 || rowMatches(it, words) {
			s.filtered = append(s.filtered, i)
		}
	}
}

func rowMatches(it map[string]any, words []string) bool {
	hay := strings.ToLower(jsontree.Summary(it, 1<<20))
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// pickColumns chooses table columns: spec columns present in the data, then
// any other scalar keys, capped for readability.
func pickColumns(res *spec.Resource, items []map[string]any) []string {
	present := map[string]bool{}
	for _, it := range items {
		for k, v := range it {
			switch v.(type) {
			case map[string]any, []any:
			default:
				present[k] = true
			}
		}
	}
	var cols []string
	seen := map[string]bool{}
	if res != nil {
		for _, c := range res.Columns {
			if present[c] {
				cols = append(cols, c)
				seen[c] = true
			}
		}
	}
	var rest []string
	for k := range present {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	cols = append(cols, rest...)
	if len(cols) > 8 {
		cols = cols[:8]
	}
	if len(cols) == 0 && len(items) > 0 {
		cols = []string{"value"}
	}
	return cols
}

func (s *collectionScreen) rebuild(width int) {
	if len(s.cols) == 0 {
		return
	}
	widths := make([]int, len(s.cols))
	for i, c := range s.cols {
		widths[i] = len(c)
	}
	rows := make([]table.Row, 0, len(s.filtered))
	for _, idx := range s.filtered {
		it := s.resp.Items[idx]
		row := make(table.Row, len(s.cols))
		for i, c := range s.cols {
			v := cell(it[c])
			row[i] = v
			if l := lipgloss.Width(v); l > widths[i] {
				widths[i] = l
			}
		}
		rows = append(rows, row)
	}
	avail := width - 2*len(s.cols) - 2
	total := 0
	for _, w := range widths {
		total += w
	}
	cols := make([]table.Column, len(s.cols))
	for i, c := range s.cols {
		w := widths[i]
		if total > avail && avail > 0 {
			w = max(6, widths[i]*avail/total)
		}
		w = min(w, 48)
		cols[i] = table.Column{Title: c, Width: w}
	}
	cur := max(0, s.table.Cursor())
	s.table.SetColumns(cols)
	s.table.SetRows(rows)
	s.table.SetCursor(min(cur, max(0, len(rows)-1)))
}

func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.ReplaceAll(x, "\n", " ")
	case map[string]any:
		return jsontree.Summary(x, 40)
	case []any:
		return fmt.Sprintf("[%d]", len(x))
	}
	return fmt.Sprint(v)
}

func (s *collectionScreen) title() string { return s.name }

func (s *collectionScreen) selected() (map[string]any, bool) {
	i := max(0, s.table.Cursor())
	if i >= len(s.filtered) {
		return nil, false
	}
	return s.resp.Items[s.filtered[i]], true
}

func (s *collectionScreen) page(a *App, delta int) tea.Cmd {
	pg := a.spec.Paging
	if pg == nil {
		return setStatus("this API spec has no paging configured", true)
	}
	if s.allPages {
		return setStatus(fmt.Sprintf("all %d pages are already loaded (R reloads page 1)", s.pages), false)
	}
	limit, _ := strconv.Atoi(s.req.Query[pg.LimitParam])
	if limit <= 0 {
		limit = pg.DefaultLimit
		if limit <= 0 {
			limit = 100
		}
	}
	offset, _ := strconv.Atoi(s.req.Query[pg.OffsetParam])
	if delta < 0 && offset == 0 {
		return setStatus("already at first page", false)
	}
	if delta > 0 && len(s.resp.Items) < limit {
		return setStatus("no more pages", false)
	}
	offset = max(0, offset+delta*limit)
	req := cloneRequest(s.req)
	req.Query[pg.LimitParam] = strconv.Itoa(limit)
	req.Query[pg.OffsetParam] = strconv.Itoa(offset)
	return a.replaceList(s.resource, req, s.name)
}

func (s *collectionScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if s.searching {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "enter":
				s.searching = false
				s.search.Blur()
				return nil
			case "ctrl+a":
				s.searching = false
				s.search.Blur()
				return a.fetchAllPages(s)
			case "esc":
				s.searching = false
				s.search.Blur()
				s.search.SetValue("")
				s.applySearch()
				s.rebuild(s.table.Width())
				return nil
			}
		}
		var cmd tea.Cmd
		s.search, cmd = s.search.Update(msg)
		s.applySearch()
		s.rebuild(s.table.Width())
		return cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "/":
			s.searching = true
			return s.search.Focus()
		case "esc":
			if s.search.Value() != "" {
				s.search.SetValue("")
				s.applySearch()
				s.rebuild(s.table.Width())
			}
			return nil
		case "enter":
			it, ok := s.selected()
			if !ok {
				return nil
			}
			id := client.ItemID(a.spec, it)
			if s.resource != nil && id != "" && (s.resource.ItemPath != "" || s.resource.ListPath != "") {
				return a.openItem(s.resource, client.ItemRequest(a.spec, s.resource, id), s.resource.Name+"/"+id)
			}
			a.push(newItemScreen(a, "item", s.resource, s.req, &client.Response{Item: it, Body: it, Status: s.resp.Status, Raw: s.resp.Raw}))
			return nil
		case "r":
			a.push(newRawScreen(s.name+" (raw)", s.resp))
			return nil
		case "e":
			a.push(newRequestScreen(a, s.resource, s.req, s.name))
			return nil
		case "f":
			a.push(newQuickParamScreen(a, s, "filter", "Filter expression, e.g. status='active' AND role='student'"))
			return nil
		case "s":
			a.push(newQuickParamScreen(a, s, "sort", "Field to sort by (orderBy=asc|desc is set separately with e)"))
			return nil
		case "A":
			return a.fetchAllPages(s)
		case "L":
			if pg := a.spec.Paging; pg != nil {
				a.push(newQuickParamScreen(a, s, pg.LimitParam, "Records per page (e.g. 1000). Offset resets to 0."))
				return nil
			}
			return setStatus("this API spec has no paging configured", true)
		case "n", "]":
			return s.page(a, +1)
		case "p", "[":
			return s.page(a, -1)
		case "R":
			return a.replaceList(s.resource, s.req, s.name)
		case "u":
			return setStatus(s.resp.URL, false)
		case "y":
			if it, ok := s.selected(); ok {
				return a.copyText(client.ItemID(a.spec, it))
			}
			return nil
		case "g", "home":
			s.table.GotoTop()
			return nil
		case "G", "end":
			s.table.GotoBottom()
			return nil
		}
	}
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return cmd
}

func (s *collectionScreen) view(a *App, w, h int) string {
	s.rebuild(w)
	s.table.SetWidth(w)
	s.table.SetHeight(max(3, h-2))
	pg := ""
	if a.spec.Paging != nil {
		off := s.req.Query[a.spec.Paging.OffsetParam]
		lim := s.req.Query[a.spec.Paging.LimitParam]
		if off == "" {
			off = "0"
		}
		if lim == "" {
			lim = "(server default)"
		}
		pg = fmt.Sprintf("  offset %s  limit %s %s", off, lim, styleDim.Render("[L to change]"))
		if s.allPages {
			pg = fmt.Sprintf("  all pages (%d × %s)", s.pages, lim)
		}
	}
	q := ""
	if f := s.req.Query["filter"]; f != "" {
		q += "  filter=" + f
	}
	if so := s.req.Query["sort"]; so != "" {
		q += "  sort=" + so
	}
	count := fmt.Sprintf("%d items", len(s.resp.Items))
	if s.search.Value() != "" {
		count = fmt.Sprintf("%d of %d items", len(s.filtered), len(s.resp.Items))
	}
	hdr := styleDim.Render(count + pg + q)
	if s.searching || s.search.Value() != "" {
		hint := ""
		if s.searching && !s.allPages {
			hint = styleDim.Render("  ctrl+a: search all pages")
		}
		s.search.Width = max(10, w-lipgloss.Width(hdr)-lipgloss.Width(hint)-6)
		hdr += "   " + s.search.View() + hint
	}
	if len(s.resp.Items) == 0 {
		return hdr + "\n\n" + styleDim.Render("  (empty result)")
	}
	if len(s.filtered) == 0 {
		return hdr + "\n\n" + styleDim.Render("  (no rows match)")
	}
	return hdr + "\n" + s.table.View()
}

func (s *collectionScreen) help() []helpEntry {
	return []helpEntry{{"enter", "open item"}, {"/", "search rows"}, {"A", "fetch all pages"}, {"n/p", "next / prev page"}, {"f", "server filter"}, {"s", "set sort"}, {"L", "page size"}, {"e", "edit all params"}, {"r", "raw JSON"}, {"u", "show URL"}, {"y", "copy id"}, {"R", "reload"}, {"g/G", "top / bottom"}}
}

// --------------------------------------------------------------------- item

type itemScreen struct {
	name     string
	resource *spec.Resource
	req      client.Request
	resp     *client.Response
	tree     *jsontree.Tree
	rows     []jsontree.Node
	cursor   int
	offset   int
}

func newItemScreen(a *App, name string, res *spec.Resource, req client.Request, resp *client.Response) *itemScreen {
	var root any = resp.Item
	if resp.Item == nil {
		root = resp.Body
	}
	s := &itemScreen{name: name, resource: res, req: req, resp: resp, tree: jsontree.New(root)}
	s.rows = s.tree.Rows()
	return s
}

func (s *itemScreen) title() string { return s.name }

func (s *itemScreen) current() (jsontree.Node, bool) {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return jsontree.Node{}, false
	}
	return s.rows[s.cursor], true
}

func (s *itemScreen) refresh() {
	s.rows = s.tree.Rows()
	if s.cursor >= len(s.rows) {
		s.cursor = max(0, len(s.rows)-1)
	}
}

func (s *itemScreen) related(a *App) tea.Cmd {
	if s.resource == nil || len(s.resource.Related) == 0 {
		return setStatus("no related collections for this resource", false)
	}
	id := client.ItemID(a.spec, s.resp.Item)
	if id == "" {
		return setStatus("item has no id to follow", true)
	}
	var opts []menuOption
	for _, rel := range s.resource.Related {
		rel := rel
		target, _ := a.spec.Resource(rel.Resource)
		opts = append(opts, menuOption{
			label: rel.Name,
			desc:  a.spec.FullPath(rel.Path),
			run: func(a *App) tea.Cmd {
				a.pop()
				return a.openList(target, client.RelatedRequest(a.spec, rel, id), s.name+"/"+rel.Name)
			},
		})
	}
	a.push(newMenuScreen("related: "+s.name, opts))
	return nil
}

func (s *itemScreen) update(a *App, msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	n, has := s.current()
	switch k.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.rows)-1 {
			s.cursor++
		}
	case "pgup", "ctrl+u":
		s.cursor = max(0, s.cursor-10)
	case "pgdown", "ctrl+d":
		s.cursor = min(len(s.rows)-1, s.cursor+10)
	case "g", "home":
		s.cursor = 0
	case "G", "end":
		s.cursor = max(0, len(s.rows)-1)
	case "enter", "right", " ":
		if !has {
			return nil
		}
		if ref, ok := a.client.AsRef(n.Value); ok && k.String() != " " {
			return a.followRef(ref)
		}
		if n.IsBranch {
			s.tree.Toggle(n.Path)
			s.refresh()
		}
	case "left", "h":
		if has && n.IsBranch && n.Expanded {
			s.tree.Expand(n.Path, false)
			s.refresh()
		} else if has && n.Depth > 0 {
			parent := n.Path[:strings.LastIndex(n.Path, ".")]
			for i, r := range s.rows {
				if r.Path == parent {
					s.cursor = i
					break
				}
			}
		}
	case "+", "=":
		s.tree.ExpandAll(true)
		s.refresh()
	case "-", "_":
		s.tree.ExpandAll(false)
		s.refresh()
	case "l":
		return s.related(a)
	case "r":
		a.push(newRawScreen(s.name+" (raw)", s.resp))
	case "u":
		return setStatus(s.resp.URL, false)
	case "R":
		if s.resource != nil && s.req.Path != "" {
			return a.replaceItem(s.resource, s.req, s.name)
		}
	case "y":
		if has {
			if n.IsBranch {
				return a.copyText(jsontree.Summary(n.Value, 1<<20))
			}
			if str, ok := n.Value.(string); ok {
				return a.copyText(str)
			}
			return a.copyText(jsontree.Format(n.Value))
		}
	}
	return nil
}

func (s *itemScreen) view(a *App, w, h int) string {
	if len(s.rows) == 0 {
		return styleDim.Render("(empty)")
	}
	body := h - 1
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+body {
		s.offset = s.cursor - body + 1
	}
	var b strings.Builder
	id := ""
	if s.resp.Item != nil {
		id = client.ItemID(a.spec, s.resp.Item)
	}
	rel := ""
	if s.resource != nil && len(s.resource.Related) > 0 {
		rel = fmt.Sprintf("  %d related (l)", len(s.resource.Related))
	}
	b.WriteString(styleDim.Render(fmt.Sprintf("%s  %d fields%s", id, len(s.rows), rel)) + "\n")
	for i := s.offset; i < len(s.rows) && i < s.offset+body; i++ {
		n := s.rows[i]
		indent := strings.Repeat("  ", n.Depth)
		var line string
		if ref, ok := a.client.AsRef(n.Value); ok {
			target := "?"
			if ref.Resource != nil {
				target = ref.Resource.Name
			}
			line = fmt.Sprintf("%s%s %s %s", indent, styleKey.Render(n.Key+":"), styleRef.Render(ref.Type+"/"+ref.ID), styleDim.Render("→ "+target))
		} else if n.IsBranch {
			marker := "▸"
			if n.Expanded {
				marker = "▾"
			}
			preview := ""
			if !n.Expanded {
				preview = " " + styleDim.Render(jsontree.Summary(n.Value, max(10, w-len(indent)-len(n.Key)-12)))
			}
			line = fmt.Sprintf("%s%s %s %s%s", indent, marker, styleKey.Render(n.Key), styleDim.Render(jsontree.Format(n.Value)), preview)
		} else {
			line = fmt.Sprintf("%s  %s %s", indent, styleKey.Render(n.Key+":"), jsontree.Format(n.Value))
		}
		line = truncate(line, w)
		if i == s.cursor {
			line = styleCursor.Render(padRight(line, w))
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *itemScreen) help() []helpEntry {
	return []helpEntry{{"enter", "follow reference / toggle"}, {"l", "related collections"}, {"←/→", "collapse / expand"}, {"+/-", "expand / collapse all"}, {"r", "raw JSON"}, {"y", "copy value"}, {"u", "show URL"}, {"R", "reload"}}
}

// ---------------------------------------------------------------------- raw

type rawScreen struct {
	name string
	vp   viewport.Model
	text string
	resp *client.Response
	init bool
}

func newRawScreen(name string, resp *client.Response) *rawScreen {
	return &rawScreen{name: name, resp: resp, text: resp.Pretty()}
}

func (s *rawScreen) title() string { return s.name }

func (s *rawScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y":
			return a.copyText(s.text)
		case "u":
			return setStatus(s.resp.URL, false)
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return cmd
}

func (s *rawScreen) view(a *App, w, h int) string {
	if !s.init || s.vp.Width != w || s.vp.Height != h-1 {
		s.vp = viewport.New(w, h-1)
		s.vp.SetContent(s.text)
		s.init = true
	}
	hdr := styleDim.Render(fmt.Sprintf("HTTP %d  %s  %d bytes  %d%%", s.resp.Status, s.resp.Duration.Round(1e6), len(s.resp.Raw), int(s.vp.ScrollPercent()*100)))
	return hdr + "\n" + s.vp.View()
}

func (s *rawScreen) help() []helpEntry {
	return []helpEntry{{"↑/↓ pgup/pgdn", "scroll"}, {"y", "copy JSON"}, {"u", "show URL"}}
}

// ------------------------------------------------------------------ request

type requestScreen struct {
	name     string
	resource *spec.Resource
	req      client.Request
	form     *form
	pathVars []string
}

func newRequestScreen(a *App, res *spec.Resource, req client.Request, name string) *requestScreen {
	req = cloneRequest(req)
	s := &requestScreen{name: name, resource: res, req: req}
	var fields []*field
	s.pathVars = spec.Placeholders(req.Path)
	for i, ph := range s.pathVars {
		f := newField("path:"+ph, "{"+ph+"}", req.PathVars[ph], "path parameter")
		if i == 0 {
			f.Section = "Path  " + a.spec.FullPath(req.Path)
		}
		fields = append(fields, f)
	}
	first := true
	seen := map[string]bool{}
	for _, p := range a.spec.QueryParams {
		f := newField("q:"+p.Name, p.Name, req.Query[p.Name], p.Description)
		if first {
			f.Section = "Query"
			first = false
		}
		seen[p.Name] = true
		fields = append(fields, f)
	}
	// Any extra params already on the request that the spec doesn't list.
	var extra []string
	for k := range req.Query {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		fields = append(fields, newField("q:"+k, k, req.Query[k], "custom parameter"))
	}
	ex := newField("extra", "extra", "", "Additional params as k=v&k2=v2")
	ex.Section = "Custom"
	fields = append(fields, ex)
	s.form = newForm(fields...)
	return s
}

func (s *requestScreen) title() string { return "request: " + s.name }

func (s *requestScreen) build() (client.Request, error) {
	req := cloneRequest(s.req)
	for _, ph := range s.pathVars {
		req.PathVars[ph] = strings.TrimSpace(s.form.get("path:" + ph))
	}
	for _, f := range s.form.fields {
		if strings.HasPrefix(f.Key, "q:") {
			k := strings.TrimPrefix(f.Key, "q:")
			v := strings.TrimSpace(f.value())
			if v == "" {
				delete(req.Query, k)
			} else {
				req.Query[k] = v
			}
		}
	}
	for _, kv := range strings.Split(s.form.get("extra"), "&") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return req, fmt.Errorf("bad extra param %q (want k=v)", kv)
		}
		req.Query[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if _, err := spec.Expand(req.Path, req.PathVars); err != nil {
		return req, err
	}
	return req, nil
}

func (s *requestScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
		req, err := s.build()
		if err != nil {
			return setStatus(err.Error(), true)
		}
		a.pop()
		// If the previous screen is a collection of the same request, replace it.
		if cs, ok := a.top().(*collectionScreen); ok && cs.req.Path == req.Path {
			return a.replaceList(s.resource, req, s.name)
		}
		if req.ListKey != "" || s.resource == nil || req.ItemKey == "" {
			return a.openList(s.resource, req, s.name)
		}
		return a.openItem(s.resource, req, s.name)
	}
	cmd, _ := s.form.update(msg)
	return cmd
}

func (s *requestScreen) view(a *App, w, h int) string {
	url, err := a.client.BuildURL(func() client.Request { r, _ := s.build(); return r }())
	preview := styleDim.Render(url)
	if err != nil {
		preview = styleWarn.Render(err.Error())
	}
	return s.form.view(w) + "\n" + styleBold.Render("GET ") + preview + "\n\n" + styleDim.Render("enter: run   tab/↑↓: move   ctrl+u: clear field   esc: cancel")
}

func (s *requestScreen) help() []helpEntry {
	return []helpEntry{{"enter", "run request"}, {"tab / shift+tab", "next / previous field"}, {"ctrl+u", "clear field"}}
}

// --------------------------------------------------------------- quick param

// quickParamScreen edits a single query parameter of a collection in place.
type quickParamScreen struct {
	coll  *collectionScreen
	param string
	form  *form
}

func newQuickParamScreen(a *App, coll *collectionScreen, param, help string) *quickParamScreen {
	return &quickParamScreen{coll: coll, param: param, form: newForm(newField(param, param, coll.req.Query[param], help))}
}

func (s *quickParamScreen) title() string { return s.param }

func (s *quickParamScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
		req := cloneRequest(s.coll.req)
		v := strings.TrimSpace(s.form.get(s.param))
		if pg := a.spec.Paging; pg != nil && s.param == pg.LimitParam && v != "" {
			if n, err := strconv.Atoi(v); err != nil || n <= 0 {
				return setStatus("page size must be a positive integer", true)
			}
		}
		if v == "" {
			delete(req.Query, s.param)
		} else {
			req.Query[s.param] = v
		}
		if pg := a.spec.Paging; pg != nil {
			req.Query[pg.OffsetParam] = "0"
		}
		a.pop()
		return a.replaceList(s.coll.resource, req, s.coll.name)
	}
	cmd, _ := s.form.update(msg)
	return cmd
}

func (s *quickParamScreen) view(a *App, w, h int) string {
	return s.form.view(w) + "\n" + styleDim.Render("enter: apply   esc: cancel")
}

func (s *quickParamScreen) help() []helpEntry { return []helpEntry{{"enter", "apply"}} }

// --------------------------------------------------------------------- menu

type menuOption struct {
	label, desc string
	run         func(a *App) tea.Cmd
}

type menuScreen struct {
	name   string
	opts   []menuOption
	cursor int
}

func newMenuScreen(name string, opts []menuOption) *menuScreen {
	return &menuScreen{name: name, opts: opts}
}

func (s *menuScreen) title() string { return s.name }

func (s *menuScreen) update(a *App, msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.opts)-1 {
			s.cursor++
		}
	case "enter", "right", "l":
		if s.cursor < len(s.opts) {
			return s.opts[s.cursor].run(a)
		}
	default:
		// Number shortcuts.
		if n, err := strconv.Atoi(k.String()); err == nil && n >= 1 && n <= len(s.opts) {
			return s.opts[n-1].run(a)
		}
	}
	return nil
}

func (s *menuScreen) view(a *App, w, h int) string {
	var b strings.Builder
	for i, o := range s.opts {
		line := fmt.Sprintf(" %d  %-20s %s", i+1, o.label, styleDim.Render(o.desc))
		if i == s.cursor {
			line = styleCursor.Render(padRight(line, w))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (s *menuScreen) help() []helpEntry { return []helpEntry{{"enter / 1-9", "choose"}} }

// ------------------------------------------------------------------ helpers

func cloneRequest(r client.Request) client.Request {
	out := r
	out.PathVars = map[string]string{}
	for k, v := range r.PathVars {
		out.PathVars[k] = v
	}
	out.Query = map[string]string{}
	for k, v := range r.Query {
		out.Query[k] = v
	}
	return out
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	// Strip styling-aware: cheap approach, cut runes until width fits.
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
