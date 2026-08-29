package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// saveScreen prompts for a file path and writes content to it.
type saveScreen struct {
	what      string // description shown in the title, e.g. "record classes/c1"
	content   []byte
	form      *form
	confirmed string // path the user has confirmed overwriting
}

func newSaveScreen(what, suggested string, content []byte) *saveScreen {
	f := newField("path", "Save to", suggested, "File path (~ expands to your home). enter: save, esc: cancel")
	return &saveScreen{what: what, content: content, form: newForm(f)}
}

func (s *saveScreen) title() string { return "save " + s.what }

// writeFile is the file writer; overridable in tests.
var writeFile = os.WriteFile

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func (s *saveScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
		path := expandHome(strings.TrimSpace(s.form.get("path")))
		if path == "" {
			return setStatus("enter a file path", true)
		}
		if _, err := os.Stat(path); err == nil && s.confirmed != path {
			s.confirmed = path
			return setStatus(path+" exists — press enter again to overwrite", true)
		}
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return setStatus("save failed: "+err.Error(), true)
			}
		}
		if err := writeFile(path, s.content, 0o644); err != nil {
			return setStatus("save failed: "+err.Error(), true)
		}
		a.pop()
		return setStatus(fmt.Sprintf("saved %d bytes to %s", len(s.content), path), false)
	}
	s.confirmed = ""
	cmd, _ := s.form.update(msg)
	return cmd
}

func (s *saveScreen) view(a *App, w, h int) string {
	return s.form.view(w) + "\n" + styleDim.Render(fmt.Sprintf("%d bytes", len(s.content)))
}

func (s *saveScreen) help() []helpEntry {
	return []helpEntry{{"enter", "save"}, {"ctrl+u", "clear"}}
}

var unsafeRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// suggestFilename builds a safe default file name from parts.
func suggestFilename(parts ...string) string {
	var clean []string
	for _, p := range parts {
		p = unsafeRe.ReplaceAllString(p, "-")
		p = strings.Trim(p, "-")
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return "record.json"
	}
	return strings.Join(clean, "-") + ".json"
}

var errNoContent = errors.New("nothing to save")
