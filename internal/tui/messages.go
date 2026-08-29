package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/reisenderlabs/api-browser/internal/client"
	"github.com/reisenderlabs/api-browser/internal/spec"
)

// fetchKind says how to present a response once it arrives.
type fetchKind int

const (
	fetchList fetchKind = iota
	fetchItem
)

// fetchMsg carries a completed request.
type fetchMsg struct {
	seq      int
	kind     fetchKind
	title    string
	resource *spec.Resource
	req      client.Request
	resp     *client.Response
}

// statusMsg updates the transient status line.
type statusMsg struct {
	text string
	err  bool
}

type clearStatusMsg struct{ seq int }

func (a *App) fetchCmd(kind fetchKind, title string, res *spec.Resource, req client.Request) tea.Cmd {
	a.seq++
	seq := a.seq
	c := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		resp := c.Do(ctx, req)
		return fetchMsg{seq: seq, kind: kind, title: title, resource: res, req: req, resp: resp}
	}
}

func setStatus(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return statusMsg{text: text, err: isErr} }
}
