package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/config"
)

// profileScreen lets the user switch to another saved profile. It is pushed
// by P at any time, and at startup when nothing named a connection.
type profileScreen struct {
	list list.Model
}

type profileItem struct {
	p         config.Profile
	current   bool
	isDefault bool
}

func (i profileItem) Title() string {
	t := i.p.Name
	switch {
	case i.current && i.isDefault:
		t += "  (current, default)"
	case i.current:
		t += "  (current)"
	case i.isDefault:
		t += "  (default)"
	}
	return t
}

func (i profileItem) Description() string {
	parts := []string{i.p.BaseURL}
	if i.p.Spec != "" {
		parts = append(parts, i.p.Spec)
	}
	m := string(i.p.Auth.Method)
	if m == "" {
		m = "none"
	}
	return joinDot(append(parts, m))
}

func (i profileItem) FilterValue() string {
	return i.p.Name + " " + i.p.BaseURL + " " + i.p.Spec
}

func joinDot(parts []string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += p
	}
	return out
}

func newProfileScreen(a *App, f *config.File) *profileScreen {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Choose a profile"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.Styles.Title = styleTitle
	s := &profileScreen{list: l}
	s.reload(a, f)
	return s
}

// reload rebuilds the rows from f, keeping the cursor where it was. The first
// build starts it on the profile in effect.
func (s *profileScreen) reload(a *App, f *config.File) {
	items := make([]list.Item, 0, len(f.Profiles))
	sel := s.list.Index()
	for i, p := range f.Profiles {
		if p.Name != "" && p.Name == a.profile.Name && len(s.list.Items()) == 0 {
			sel = i
		}
		items = append(items, profileItem{
			p:         p,
			current:   p.Name != "" && p.Name == a.profile.Name,
			isDefault: p.Name == f.Default,
		})
	}
	s.list.SetItems(items)
	s.list.Select(sel)
}

func (s *profileScreen) title() string { return "profile" }

func (s *profileScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && s.list.FilterState() != list.Filtering {
		it, hasItem := s.list.SelectedItem().(profileItem)
		switch k.String() {
		case "enter":
			if !hasItem {
				return nil
			}
			// The profile already in effect is a no-op: keep the navigation
			// the user came from rather than resetting it.
			if it.current {
				a.pop()
				return setStatus("already using "+it.p.Name, false)
			}
			if err := a.useProfile(it.p); err != nil {
				return setStatus(err.Error(), true)
			}
			return setStatus("switched to "+it.p.Name+"  ("+a.client.BaseURL+")", false)
		case "d":
			if !hasItem {
				return nil
			}
			f, err := a.toggleDefaultProfile(it.p.Name)
			if err != nil {
				return setStatus("save failed: "+err.Error(), true)
			}
			s.reload(a, f)
			if f.Default == it.p.Name {
				return setStatus(it.p.Name+" is now the default profile", false)
			}
			return setStatus("cleared the default profile", false)
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

func (s *profileScreen) view(a *App, w, h int) string {
	s.list.SetSize(w, h)
	return s.list.View()
}

func (s *profileScreen) help() []helpEntry {
	return []helpEntry{{"enter", "use this profile"}, {"d", "set / clear default"}, {"/", "filter profiles"}, {"esc", "keep " + s.currentName()}}
}

func (s *profileScreen) currentName() string {
	for _, it := range s.list.Items() {
		if pi, ok := it.(profileItem); ok && pi.current {
			return pi.p.Name
		}
	}
	return "the current connection"
}
