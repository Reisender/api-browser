package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// cursorMode controls input cursor blinking; tests set it to static so no
// blink timers are scheduled.
var cursorMode = cursor.CursorBlink

// field is a labelled input in a form.
type field struct {
	Key     string
	Label   string
	Help    string
	Secret  bool
	Section string // optional section header rendered above this field
	Choices []string
	choice  int
	input   textinput.Model
}

// form is a vertical list of fields with tab navigation.
type form struct {
	fields []*field
	focus  int
}

func newField(key, label, value, help string) *field {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(value)
	ti.CharLimit = 4096
	ti.Width = 60
	ti.Cursor.SetMode(cursorMode)
	return &field{Key: key, Label: label, Help: help, input: ti}
}

func newSecretField(key, label, value, help string) *field {
	f := newField(key, label, value, help)
	f.Secret = true
	f.input.EchoMode = textinput.EchoPassword
	f.input.EchoCharacter = '•'
	return f
}

func newChoiceField(key, label string, choices []string, current string, help string) *field {
	f := newField(key, label, "", help)
	f.Choices = choices
	for i, c := range choices {
		if c == current {
			f.choice = i
		}
	}
	return f
}

func (f *field) value() string {
	if len(f.Choices) > 0 {
		return f.Choices[f.choice]
	}
	return f.input.Value()
}

func (f *field) setValue(v string) {
	if len(f.Choices) > 0 {
		for i, c := range f.Choices {
			if c == v {
				f.choice = i
			}
		}
		return
	}
	f.input.SetValue(v)
}

func newForm(fields ...*field) *form {
	fm := &form{fields: fields}
	fm.setFocus(0)
	return fm
}

func (fm *form) setFocus(i int) {
	if len(fm.fields) == 0 {
		return
	}
	if i < 0 {
		i = len(fm.fields) - 1
	}
	i %= len(fm.fields)
	for j, f := range fm.fields {
		if j == i {
			f.input.Focus()
		} else {
			f.input.Blur()
		}
	}
	fm.focus = i
}

func (fm *form) get(key string) string {
	for _, f := range fm.fields {
		if f.Key == key {
			return f.value()
		}
	}
	return ""
}

func (fm *form) set(key, v string) {
	for _, f := range fm.fields {
		if f.Key == key {
			f.setValue(v)
		}
	}
}

func (fm *form) values() map[string]string {
	out := map[string]string{}
	for _, f := range fm.fields {
		out[f.Key] = f.value()
	}
	return out
}

func (fm *form) current() *field {
	if len(fm.fields) == 0 {
		return nil
	}
	return fm.fields[fm.focus]
}

// update handles navigation keys and forwards the rest to the focused input.
// It reports whether the key was consumed.
func (fm *form) update(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	cur := fm.current()
	if cur == nil {
		return nil, false
	}
	switch key.String() {
	case "tab", "down":
		fm.setFocus(fm.focus + 1)
		return nil, true
	case "shift+tab", "up":
		fm.setFocus(fm.focus - 1)
		return nil, true
	case "ctrl+u":
		if len(cur.Choices) == 0 {
			cur.input.SetValue("")
		}
		return nil, true
	}
	if len(cur.Choices) > 0 {
		switch key.String() {
		case "left", "h":
			cur.choice = (cur.choice - 1 + len(cur.Choices)) % len(cur.Choices)
			return nil, true
		case "right", "l", " ", "enter":
			cur.choice = (cur.choice + 1) % len(cur.Choices)
			return nil, true
		}
		return nil, false
	}
	var cmd tea.Cmd
	cur.input, cmd = cur.input.Update(msg)
	return cmd, true
}

func (fm *form) view(width int) string {
	var b strings.Builder
	for i, f := range fm.fields {
		if f.Section != "" {
			b.WriteString(styleSection.Render(f.Section) + "\n")
		}
		label := styleLabel
		if i == fm.focus {
			label = styleLabelHot
		}
		var val string
		if len(f.Choices) > 0 {
			var parts []string
			for j, c := range f.Choices {
				if j == f.choice {
					parts = append(parts, styleTitle.Render(c))
				} else {
					parts = append(parts, styleDim.Render(c))
				}
			}
			val = strings.Join(parts, " ")
			if i == fm.focus {
				val += styleDim.Render("  ←/→ to change")
			}
		} else {
			f.input.Width = max(20, width-18)
			val = f.input.View()
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, label.Render(f.Label), val) + "\n")
		if f.Help != "" && i == fm.focus {
			b.WriteString(strings.Repeat(" ", 14) + styleDim.Render(f.Help) + "\n")
		}
	}
	return b.String()
}
