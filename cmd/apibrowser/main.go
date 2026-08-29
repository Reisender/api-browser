// Command apibrowser is a terminal UI for exploring REST APIs described by a
// small navigation spec. It ships with the OneRoster v1p1 API built in.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Reisender/api-browser/internal/auth"
	"github.com/Reisender/api-browser/internal/config"
	"github.com/Reisender/api-browser/internal/spec"
	"github.com/Reisender/api-browser/internal/tui"
)

func main() {
	var (
		specName   = flag.String("spec", "", "builtin spec name or path to a spec YAML (default: profile's spec, else oneroster-v1p1)")
		baseURL    = flag.String("url", "", "API base URL, e.g. https://example.com")
		profile    = flag.String("profile", "", "saved profile name from the config file")
		configPath = flag.String("config", config.DefaultPath(), "config file path")
		authMethod = flag.String("auth", "", "auth method: none, bearer, oauth2, header")
		token      = flag.String("token", os.Getenv("APIBROWSER_TOKEN"), "bearer token (env APIBROWSER_TOKEN)")
		clientID   = flag.String("client-id", os.Getenv("APIBROWSER_CLIENT_ID"), "OAuth2 client id (env APIBROWSER_CLIENT_ID)")
		secret     = flag.String("client-secret", os.Getenv("APIBROWSER_CLIENT_SECRET"), "OAuth2 client secret (env APIBROWSER_CLIENT_SECRET)")
		tokenURL   = flag.String("token-url", os.Getenv("APIBROWSER_TOKEN_URL"), "OAuth2 token URL (env APIBROWSER_TOKEN_URL)")
		scopes     = flag.String("scopes", "", "space-separated OAuth2 scopes")
		header     = flag.String("header", "", "arbitrary auth header as 'Name: value'")
		listSpecs  = flag.Bool("list-specs", false, "list builtin specs and exit")
		listProfs  = flag.Bool("list-profiles", false, "list saved profiles and exit")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: apibrowser [flags]\n\nExplore a REST API from the terminal.\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n  apibrowser -url https://host -auth bearer -token XYZ\n  apibrowser -url https://host -auth oauth2 -client-id ID -client-secret S -token-url https://host/oauth/token\n  apibrowser -url https://host -auth header -header 'X-Api-Key: abc'\n  apibrowser -profile district\n")
	}
	flag.Parse()

	if *showVer {
		fmt.Println("apibrowser", versionString())
		return
	}
	if *listSpecs {
		for _, n := range spec.BuiltinNames() {
			fmt.Println(n)
		}
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	if *listProfs {
		for _, p := range cfg.Profiles {
			def := ""
			if p.Name == cfg.Default {
				def = " (default)"
			}
			fmt.Printf("%s\t%s\t%s\t%s%s\n", p.Name, p.BaseURL, p.Spec, p.Auth.Method, def)
		}
		return
	}

	p := config.Profile{}
	if saved, ok := cfg.Get(*profile); ok {
		p = *saved
	} else if *profile != "" {
		fail(fmt.Errorf("profile %q not found in %s", *profile, *configPath))
	}

	// Flags override the profile.
	if *baseURL != "" {
		p.BaseURL = *baseURL
	}
	if *specName != "" {
		p.Spec = *specName
	}
	if p.Spec == "" {
		p.Spec = "oneroster-v1p1"
	}
	if *authMethod != "" {
		p.Auth.Method = auth.Method(*authMethod)
	}
	if *token != "" {
		p.Auth.Token = *token
		if p.Auth.Method == "" {
			p.Auth.Method = auth.MethodBearer
		}
	}
	if *clientID != "" || *secret != "" || *tokenURL != "" {
		if *clientID != "" {
			p.Auth.ClientID = *clientID
		}
		if *secret != "" {
			p.Auth.ClientSecret = *secret
		}
		if *tokenURL != "" {
			p.Auth.TokenURL = *tokenURL
		}
		if p.Auth.Method == "" {
			p.Auth.Method = auth.MethodOAuth2
		}
	}
	if *scopes != "" {
		p.Auth.Scopes = strings.Fields(*scopes)
	}
	if *header != "" {
		name, value, ok := strings.Cut(*header, ":")
		if !ok {
			fail(fmt.Errorf("-header must be 'Name: value'"))
		}
		p.Auth.HeaderName = strings.TrimSpace(name)
		p.Auth.HeaderValue = strings.TrimSpace(value)
		if p.Auth.Method == "" {
			p.Auth.Method = auth.MethodHeader
		}
	}
	if err := p.Auth.Validate(); err != nil {
		fail(err)
	}

	s, err := spec.Load(p.Spec)
	if err != nil {
		fail(err)
	}

	app, err := tui.New(s, p, *configPath)
	if err != nil {
		fail(err)
	}
	if err := tui.Run(app); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "apibrowser:", err)
	os.Exit(1)
}
