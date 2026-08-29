// Package auth provides request authenticators: OAuth2 client credentials,
// static bearer tokens, and arbitrary headers.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/clientcredentials"
)

// Method identifies an authentication strategy.
type Method string

const (
	MethodNone   Method = "none"
	MethodBearer Method = "bearer"
	MethodOAuth2 Method = "oauth2"
	MethodHeader Method = "header"
)

// Methods lists all supported methods in display order.
var Methods = []Method{MethodNone, MethodBearer, MethodOAuth2, MethodHeader}

// Config is a serialisable description of how to authenticate.
type Config struct {
	Method Method `yaml:"method" json:"method"`

	// Bearer
	Token string `yaml:"token,omitempty" json:"token,omitempty"`

	// OAuth2 client credentials
	ClientID     string   `yaml:"clientId,omitempty" json:"clientId,omitempty"`
	ClientSecret string   `yaml:"clientSecret,omitempty" json:"clientSecret,omitempty"`
	TokenURL     string   `yaml:"tokenUrl,omitempty" json:"tokenUrl,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`

	// Arbitrary header
	HeaderName  string `yaml:"headerName,omitempty" json:"headerName,omitempty"`
	HeaderValue string `yaml:"headerValue,omitempty" json:"headerValue,omitempty"`
}

// Authenticator decorates outgoing requests with credentials.
type Authenticator interface {
	// Apply adds authentication to req. It may perform network calls (e.g.
	// to fetch an OAuth2 token) using ctx.
	Apply(ctx context.Context, req *http.Request) error
	// Describe returns a short human-readable summary without secrets.
	Describe() string
}

// Validate reports configuration problems for the selected method.
func (c Config) Validate() error {
	switch c.Method {
	case MethodNone, "":
		return nil
	case MethodBearer:
		if strings.TrimSpace(c.Token) == "" {
			return fmt.Errorf("bearer: token is required")
		}
	case MethodOAuth2:
		var missing []string
		if c.ClientID == "" {
			missing = append(missing, "clientId")
		}
		if c.ClientSecret == "" {
			missing = append(missing, "clientSecret")
		}
		if c.TokenURL == "" {
			missing = append(missing, "tokenUrl")
		}
		if len(missing) > 0 {
			return fmt.Errorf("oauth2: missing %s", strings.Join(missing, ", "))
		}
	case MethodHeader:
		if strings.TrimSpace(c.HeaderName) == "" {
			return fmt.Errorf("header: header name is required")
		}
	default:
		return fmt.Errorf("unknown auth method %q", c.Method)
	}
	return nil
}

// New builds an Authenticator from a Config.
func New(c Config) (Authenticator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	switch c.Method {
	case MethodNone, "":
		return None{}, nil
	case MethodBearer:
		return Bearer{Token: strings.TrimSpace(c.Token)}, nil
	case MethodOAuth2:
		return NewClientCredentials(c.ClientID, c.ClientSecret, c.TokenURL, c.Scopes), nil
	case MethodHeader:
		return Header{Name: strings.TrimSpace(c.HeaderName), Value: c.HeaderValue}, nil
	}
	return nil, fmt.Errorf("unknown auth method %q", c.Method)
}

// None performs no authentication.
type None struct{}

func (None) Apply(context.Context, *http.Request) error { return nil }
func (None) Describe() string                           { return "no auth" }

// Bearer sends a static bearer token.
type Bearer struct{ Token string }

func (b Bearer) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+b.Token)
	return nil
}

func (b Bearer) Describe() string { return "bearer " + mask(b.Token) }

// Header sends an arbitrary header.
type Header struct{ Name, Value string }

func (h Header) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set(h.Name, h.Value)
	return nil
}

func (h Header) Describe() string { return h.Name + ": " + mask(h.Value) }

// ClientCredentials performs the OAuth2 client-credentials flow, caching the
// access token and refreshing it when it expires.
type ClientCredentials struct {
	cfg clientcredentials.Config

	mu      sync.Mutex
	token   string
	expires time.Time
	now     func() time.Time
}

// NewClientCredentials creates an OAuth2 client-credentials authenticator.
func NewClientCredentials(clientID, secret, tokenURL string, scopes []string) *ClientCredentials {
	return &ClientCredentials{
		cfg: clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: secret,
			TokenURL:     tokenURL,
			Scopes:       scopes,
		},
		now: time.Now,
	}
}

// Token returns a valid access token, fetching a new one if needed.
func (c *ClientCredentials) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && (c.expires.IsZero() || c.now().Add(30*time.Second).Before(c.expires)) {
		return c.token, nil
	}
	tok, err := c.cfg.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("oauth2 token: %w", err)
	}
	c.token = tok.AccessToken
	c.expires = tok.Expiry
	return c.token, nil
}

// Invalidate discards the cached token so the next request fetches a new one.
func (c *ClientCredentials) Invalidate() {
	c.mu.Lock()
	c.token = ""
	c.expires = time.Time{}
	c.mu.Unlock()
}

func (c *ClientCredentials) Apply(ctx context.Context, req *http.Request) error {
	tok, err := c.Token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

func (c *ClientCredentials) Describe() string {
	return "oauth2 client " + c.cfg.ClientID + " @ " + c.cfg.TokenURL
}

func mask(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", 4) + s[len(s)-4:]
}
