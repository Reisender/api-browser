// Package tui implements the BubbleTea user interface.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/reisenderlabs/api-browser/internal/auth"
	"github.com/reisenderlabs/api-browser/internal/client"
	"github.com/reisenderlabs/api-browser/internal/config"
	"github.com/reisenderlabs/api-browser/internal/spec"
)

// App is the root BubbleTea model.
type App struct {
	spec       *spec.Spec
	client     *client.Client
	profile    config.Profile
	configPath string

	stack     []screen
	width     int
	height    int
	loading   string
	spin      spinner.Model
	status    string
	statusErr bool
	statusSeq int
	showHelp  bool
	seq       int
	quitting  bool
}

// New constructs the application.
func New(s *spec.Spec, p config.Profile, configPath string) (*App, error) {
	authn, err := auth.New(p.Auth)
	if err != nil {
		return nil, err
	}
	c := client.New(p.BaseURL, s, authn)
	c.Headers = p.Headers
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	a := &App{spec: s, client: c, profile: p, configPath: configPath, spin: sp, width: 80, height: 24}
	a.stack = []screen{newResourcesScreen(s)}
	if p.BaseURL == "" {
		a.push(newConnectionScreen(a))
	}
	return a, nil
}

func (a *App) Init() tea.Cmd { return a.spin.Tick }

// --- navigation --------------------------------------------------------

func (a *App) push(s screen) { a.stack = append(a.stack, s) }

func (a *App) pop() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) top() screen { return a.stack[len(a.stack)-1] }

// Depth returns the navigation stack depth (for tests).
func (a *App) Depth() int { return len(a.stack) }

func (a *App) openList(res *spec.Resource, req client.Request, title string) tea.Cmd {
	a.loading = "GET " + title
	return a.fetchCmd(fetchList, title, res, req)
}

func (a *App) openItem(res *spec.Resource, req client.Request, title string) tea.Cmd {
	a.loading = "GET " + title
	return a.fetchCmd(fetchItem, title, res, req)
}

// replaceList re-runs a request and swaps the current collection in place.
func (a *App) replaceList(res *spec.Resource, req client.Request, title string) tea.Cmd {
	a.loading = "GET " + title
	return a.fetchCmd(fetchList, "\x00replace:"+title, res, req)
}

func (a *App) replaceItem(res *spec.Resource, req client.Request, title string) tea.Cmd {
	a.loading = "GET " + title
	return a.fetchCmd(fetchItem, "\x00replace:"+title, res, req)
}

func (a *App) followRef(ref client.Ref) tea.Cmd {
	if ref.Resource == nil {
		return setStatus(fmt.Sprintf("no resource known for reference type %q", ref.Type), true)
	}
	req := client.ItemRequest(a.spec, ref.Resource, ref.ID)
	return a.openItem(ref.Resource, req, ref.Resource.Name+"/"+ref.ID)
}

func (a *App) saveProfile() error {
	f, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	f.Put(a.profile)
	if f.Default == "" {
		f.Default = a.profile.Name
	}
	return f.Save(a.configPath)
}

func (a *App) copyText(s string) tea.Cmd {
	return func() tea.Msg {
		if err := copyToClipboard(s); err != nil {
			return statusMsg{text: "copy failed: " + err.Error(), err: true}
		}
		return statusMsg{text: fmt.Sprintf("copied %d bytes", len(s))}
	}
}

// copyToClipboard writes to the system clipboard; overridable in tests.
var copyToClipboard = clipboard.WriteAll

// --- update ------------------------------------------------------------

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(m)
		return a, cmd

	case statusMsg:
		a.status, a.statusErr = m.text, m.err
		a.statusSeq++
		seq := a.statusSeq
		return a, tea.Tick(8*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{seq: seq} })

	case clearStatusMsg:
		if m.seq == a.statusSeq {
			a.status = ""
		}
		return a, nil

	case fetchMsg:
		return a, a.handleFetch(m)

	case tea.KeyMsg:
		return a.handleKey(m)
	}
	return a, a.top().update(a, msg)
}

func (a *App) handleFetch(m fetchMsg) tea.Cmd {
	if m.seq != a.seq {
		return nil // stale
	}
	a.loading = ""
	resp := m.resp
	if m.title == "__test__" {
		if resp.Error != nil {
			return setStatus("test failed: "+resp.Error.Error(), true)
		}
		return setStatus(fmt.Sprintf("test OK: HTTP %d in %s (%s)", resp.Status, resp.Duration.Round(time.Millisecond), a.client.Auth.Describe()), false)
	}
	if resp.Error != nil {
		// Still allow inspecting the error body.
		if resp.Body != nil {
			a.push(newRawScreen("error "+m.title, resp))
		}
		return setStatus(resp.Error.Error(), true)
	}
	replace := strings.HasPrefix(m.title, "\x00replace:")
	title := strings.TrimPrefix(m.title, "\x00replace:")
	var s screen
	switch m.kind {
	case fetchList:
		s = newCollectionScreen(a, title, m.resource, m.req, resp)
		if replace {
			if old, ok := a.top().(*collectionScreen); ok {
				ns := s.(*collectionScreen)
				ns.search.SetValue(old.search.Value())
				ns.applySearch()
				ns.table.SetCursor(old.table.Cursor())
				a.pop()
			}
		}
	case fetchItem:
		if resp.Item == nil && resp.Body == nil {
			return setStatus("empty response", true)
		}
		s = newItemScreen(a, title, m.resource, m.req, resp)
		if replace {
			if _, ok := a.top().(*itemScreen); ok {
				a.pop()
			}
		}
	}
	a.push(s)
	n := len(resp.Items)
	if m.kind == fetchItem {
		n = 1
	}
	return setStatus(fmt.Sprintf("HTTP %d  %d bytes  %s  %d record(s)", resp.Status, len(resp.Raw), resp.Duration.Round(time.Millisecond), n), false)
}

func (a *App) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		a.quitting = true
		return a, tea.Quit
	}
	if a.showHelp {
		a.showHelp = false
		return a, nil
	}
	if a.loading != "" {
		if k.String() == "esc" {
			a.seq++ // invalidate in-flight request
			a.loading = ""
			return a, setStatus("cancelled", false)
		}
		return a, nil
	}
	top := a.top()
	inForm := isForm(top)
	switch k.String() {
	case "?":
		if !inForm {
			a.showHelp = true
			return a, nil
		}
	case "esc":
		if inForm && isSearchingCollection(top) {
			break // let the collection close its search box
		}
		if cs, ok := top.(*collectionScreen); ok && cs.search.Value() != "" {
			break // first esc clears an applied search
		}
		if len(a.stack) == 1 {
			return a, nil
		}
		a.pop()
		return a, nil
	case "backspace":
		if !inForm && len(a.stack) > 1 {
			a.pop()
			return a, nil
		}
	case "q":
		if !inForm && !isFiltering(top) {
			if len(a.stack) == 1 {
				a.quitting = true
				return a, tea.Quit
			}
			a.pop()
			return a, nil
		}
	case "a":
		if !inForm && !isFiltering(top) {
			a.push(newConnectionScreen(a))
			return a, nil
		}
	case "H":
		if !inForm && !isFiltering(top) {
			a.stack = a.stack[:1]
			return a, nil
		}
	}
	return a, top.update(a, k)
}

func isForm(s screen) bool {
	switch x := s.(type) {
	case *requestScreen, *connectionScreen, *quickParamScreen:
		return true
	case *collectionScreen:
		return x.searching
	}
	return false
}

func isSearchingCollection(s screen) bool {
	cs, ok := s.(*collectionScreen)
	return ok && cs.searching
}

func isFiltering(s screen) bool {
	if rs, ok := s.(*resourcesScreen); ok {
		return rs.list.SettingFilter()
	}
	return false
}

// --- view --------------------------------------------------------------

func (a *App) View() string {
	if a.quitting {
		return ""
	}
	w, h := a.width, a.height
	header := a.headerView(w)
	footer := a.footerView(w)
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 3 {
		bodyH = 3
	}
	body := a.top().view(a, w, bodyH)
	if a.showHelp {
		body = a.helpView(w, bodyH)
	}
	body = lipgloss.NewStyle().Width(w).Height(bodyH).MaxHeight(bodyH).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (a *App) headerView(w int) string {
	var crumbs []string
	for i, s := range a.stack {
		t := s.title()
		if i == len(a.stack)-1 {
			crumbs = append(crumbs, styleBold.Render(t))
		} else {
			crumbs = append(crumbs, styleCrumb.Render(t))
		}
	}
	left := styleTitle.Render("api-browser") + " " + strings.Join(crumbs, styleCrumbSep.Render(" › "))
	right := styleDim.Render(a.client.BaseURL + "  " + a.client.Auth.Describe())
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		right = ""
		gap = max(0, w-lipgloss.Width(left))
	}
	return truncate(left+strings.Repeat(" ", gap)+right, w) + "\n"
}

func (a *App) footerView(w int) string {
	var line string
	switch {
	case a.loading != "":
		line = a.spin.View() + " " + a.loading + styleDim.Render("  (esc to cancel)")
	case a.status != "":
		if a.statusErr {
			line = styleErr.Render("✗ ") + styleErr.Render(a.status)
		} else {
			line = styleOK.Render("✓ ") + a.status
		}
	default:
		var parts []string
		for _, h := range a.top().help() {
			parts = append(parts, styleHelpKey.Render(h.key)+" "+styleDim.Render(h.desc))
		}
		parts = append(parts, styleHelpKey.Render("?")+" "+styleDim.Render("help"))
		line = strings.Join(parts, "  ")
	}
	return truncate(line, w)
}

func (a *App) helpView(w, h int) string {
	var b strings.Builder
	b.WriteString(styleBold.Render("This screen") + "\n")
	for _, e := range a.top().help() {
		b.WriteString(fmt.Sprintf("  %-18s %s\n", styleHelpKey.Render(e.key), e.desc))
	}
	b.WriteString("\n" + styleBold.Render("Everywhere") + "\n")
	for _, e := range []helpEntry{
		{"esc / backspace", "go back"},
		{"q", "back (quit at top level)"},
		{"H", "jump to resource list"},
		{"a", "connection & auth settings"},
		{"ctrl+c", "quit"},
	} {
		b.WriteString(fmt.Sprintf("  %-18s %s\n", styleHelpKey.Render(e.key), e.desc))
	}
	b.WriteString("\n" + styleDim.Render("Spec: "+a.spec.Name+"  ·  config: "+a.configPath))
	return styleHelpBox.Width(min(w-2, 70)).Render(b.String())
}

// Run starts the program.
func Run(a *App) error {
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Stderr is a helper for callers that want consistent error printing.
func Stderr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
