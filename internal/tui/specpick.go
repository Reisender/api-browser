package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/spec"
)

// specScreen lets the user pick one of the builtin specs. It is pushed at
// startup when neither the -spec flag nor the profile named one.
type specScreen struct {
	list list.Model
}

type specItem struct {
	info    spec.Info
	current bool
}

func (i specItem) Title() string {
	if i.current {
		return i.info.Name + "  (current)"
	}
	return i.info.Name
}

func (i specItem) Description() string {
	if i.info.Description == "" {
		return i.info.ID
	}
	return i.info.ID + " — " + i.info.Description
}

func (i specItem) FilterValue() string {
	return i.info.ID + " " + i.info.Name + " " + i.info.Description
}

func newSpecScreen(a *App) *specScreen {
	infos := spec.Builtins()
	items := make([]list.Item, 0, len(infos))
	sel := 0
	for i, info := range infos {
		if info.ID == a.profile.Spec {
			sel = i
		}
		items = append(items, specItem{info: info, current: info.ID == a.profile.Spec})
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Choose an API spec"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.Styles.Title = styleTitle
	l.Select(sel)
	return &specScreen{list: l}
}

func (s *specScreen) title() string { return "spec" }

func (s *specScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && s.list.FilterState() != list.Filtering {
		if k.String() == "enter" {
			it, ok := s.list.SelectedItem().(specItem)
			if !ok {
				return nil
			}
			loaded, err := spec.LoadBuiltin(it.info.ID)
			if err != nil {
				return setStatus(err.Error(), true)
			}
			a.setSpec(loaded, it.info.ID)
			a.pop()
			return setStatus("using spec "+loaded.Name, false)
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

func (s *specScreen) view(a *App, w, h int) string {
	s.list.SetSize(w, h)
	return s.list.View()
}

func (s *specScreen) help() []helpEntry {
	return []helpEntry{{"enter", "use this spec"}, {"/", "filter specs"}, {"esc", "keep " + s.currentName()}}
}

func (s *specScreen) currentName() string {
	for _, it := range s.list.Items() {
		if si, ok := it.(specItem); ok && si.current {
			return si.info.ID
		}
	}
	return "the current spec"
}
