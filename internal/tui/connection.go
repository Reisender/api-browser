package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Reisender/api-browser/internal/auth"
	"github.com/Reisender/api-browser/internal/client"
	"github.com/Reisender/api-browser/internal/config"
)

// connectionScreen edits the base URL and auth settings.
type connectionScreen struct {
	form *form
}

func newConnectionScreen(a *App) *connectionScreen {
	p := a.profile
	methods := make([]string, len(auth.Methods))
	for i, m := range auth.Methods {
		methods[i] = string(m)
	}
	m := string(p.Auth.Method)
	if m == "" {
		m = string(auth.MethodNone)
	}
	base := newField("baseUrl", "Base URL", p.BaseURL, "e.g. https://example.com (spec base path is appended)")
	base.Section = "Connection"
	name := newField("name", "Profile", p.Name, "Name used when saving this profile (ctrl+s)")
	method := newChoiceField("method", "Auth method", methods, m, "")
	method.Section = "Authentication"
	fields := []*field{
		base, name, method,
		newSecretField("token", "Bearer token", p.Auth.Token, "Sent as Authorization: Bearer <token>"),
		newField("clientId", "Client ID", p.Auth.ClientID, "OAuth2 client credentials"),
		newSecretField("clientSecret", "Client secret", p.Auth.ClientSecret, ""),
		newField("tokenUrl", "Token URL", p.Auth.TokenURL, "OAuth2 token endpoint"),
		newField("scopes", "Scopes", strings.Join(p.Auth.Scopes, " "), "Space-separated OAuth2 scopes"),
		newField("headerName", "Header name", p.Auth.HeaderName, "Arbitrary header, e.g. X-Api-Key"),
		newSecretField("headerValue", "Header value", p.Auth.HeaderValue, ""),
	}
	extra := newField("headers", "Extra headers", joinHeaders(p.Headers), "Static headers: Name: value; Other: value")
	extra.Section = "Extra"
	fields = append(fields, extra)
	return &connectionScreen{form: newForm(fields...)}
}

func joinHeaders(h map[string]string) string {
	var parts []string
	for k, v := range h {
		parts = append(parts, k+": "+v)
	}
	return strings.Join(parts, "; ")
}

func parseHeaders(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, ":")
		k = strings.TrimSpace(k)
		if ok && k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

func (s *connectionScreen) title() string { return "connection" }

func (s *connectionScreen) profile() config.Profile {
	v := s.form.values()
	var scopes []string
	for _, sc := range strings.Fields(v["scopes"]) {
		scopes = append(scopes, sc)
	}
	return config.Profile{
		Name:    strings.TrimSpace(v["name"]),
		BaseURL: strings.TrimSpace(v["baseUrl"]),
		Headers: parseHeaders(v["headers"]),
		Auth: auth.Config{
			Method:       auth.Method(v["method"]),
			Token:        v["token"],
			ClientID:     v["clientId"],
			ClientSecret: v["clientSecret"],
			TokenURL:     v["tokenUrl"],
			Scopes:       scopes,
			HeaderName:   v["headerName"],
			HeaderValue:  v["headerValue"],
		},
	}
}

func (s *connectionScreen) apply(a *App) error {
	p := s.profile()
	p.Spec = a.profile.Spec
	if p.BaseURL == "" {
		return errBaseURL
	}
	authn, err := auth.New(p.Auth)
	if err != nil {
		return err
	}
	a.profile = p
	a.client = client.New(p.BaseURL, a.spec, authn)
	a.client.Headers = p.Headers
	return nil
}

type strErr string

func (e strErr) Error() string { return string(e) }

const errBaseURL = strErr("base URL is required")

func (s *connectionScreen) update(a *App, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			if len(s.form.current().Choices) > 0 {
				break // let the form cycle the choice
			}
			if err := s.apply(a); err != nil {
				return setStatus(err.Error(), true)
			}
			a.pop()
			return setStatus("connected: "+a.client.BaseURL+"  ("+a.client.Auth.Describe()+")", false)
		case "ctrl+s":
			if err := s.apply(a); err != nil {
				return setStatus(err.Error(), true)
			}
			if a.profile.Name == "" {
				return setStatus("give the profile a name before saving", true)
			}
			if err := a.saveProfile(); err != nil {
				return setStatus("save failed: "+err.Error(), true)
			}
			return setStatus("saved profile "+a.profile.Name+" to "+a.configPath, false)
		case "ctrl+t":
			if err := s.apply(a); err != nil {
				return setStatus(err.Error(), true)
			}
			if len(a.spec.Resources) == 0 {
				return nil
			}
			r := &a.spec.Resources[0]
			req := client.ListRequest(a.spec, r)
			if pg := a.spec.Paging; pg != nil {
				req.Query[pg.LimitParam] = "1"
			}
			a.loading = "testing connection…"
			return a.fetchCmd(fetchList, "__test__", r, req)
		}
	}
	cmd, _ := s.form.update(msg)
	return cmd
}

func (s *connectionScreen) view(a *App, w, h int) string {
	return s.form.view(w) + "\n" + styleDim.Render("enter: apply   ctrl+t: test   ctrl+s: apply & save profile   esc: cancel")
}

func (s *connectionScreen) help() []helpEntry {
	return []helpEntry{{"enter", "apply settings"}, {"ctrl+t", "test connection"}, {"ctrl+s", "save profile"}, {"←/→", "change auth method"}}
}
