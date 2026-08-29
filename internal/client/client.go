// Package client executes spec-driven requests against an API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/reisenderlabs/api-browser/internal/auth"
	"github.com/reisenderlabs/api-browser/internal/spec"
)

// Request is a single API call to make.
type Request struct {
	// Path is a template relative to the spec base path, e.g. /classes/{sourcedId}.
	Path string
	// PathVars fills the template placeholders.
	PathVars map[string]string
	// Query holds query parameters. Empty values are omitted.
	Query map[string]string
	// ListKey, if set, names the top-level key holding a list of items.
	ListKey string
	// ItemKey, if set, names the top-level key holding a single item.
	ItemKey string
}

// Response is the result of a request.
type Response struct {
	URL      string
	Status   int
	Duration time.Duration
	Headers  http.Header
	Raw      []byte
	Body     any              // parsed JSON, or nil if not JSON
	Items    []map[string]any // when ListKey resolved to a list
	Item     map[string]any   // when ItemKey resolved to an object
	Error    error            // non-2xx or transport error
}

// Pretty returns the body as indented JSON, or the raw text.
func (r *Response) Pretty() string {
	if r.Body == nil {
		return string(r.Raw)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, r.Raw, "", "  "); err != nil {
		return string(r.Raw)
	}
	return buf.String()
}

// Client makes authenticated requests described by a Spec.
type Client struct {
	BaseURL string
	Spec    *spec.Spec
	Auth    auth.Authenticator
	Headers map[string]string
	HTTP    *http.Client
}

// New constructs a Client.
func New(baseURL string, s *spec.Spec, a auth.Authenticator) *Client {
	if a == nil {
		a = auth.None{}
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Spec:    s,
		Auth:    a,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// BuildURL computes the absolute URL for a request without sending it.
func (c *Client) BuildURL(req Request) (string, error) {
	p, err := spec.Expand(req.Path, req.PathVars)
	if err != nil {
		return "", err
	}
	u := c.BaseURL + c.Spec.FullPath(p)
	keys := make([]string, 0, len(req.Query))
	for k, v := range req.Query {
		if strings.TrimSpace(v) != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return u, nil
	}
	sort.Strings(keys)
	q := url.Values{}
	for _, k := range keys {
		q.Set(k, req.Query[k])
	}
	return u + "?" + q.Encode(), nil
}

// Do executes the request.
func (c *Client) Do(ctx context.Context, req Request) *Response {
	resp := &Response{}
	u, err := c.BuildURL(req)
	if err != nil {
		resp.Error = err
		return resp
	}
	resp.URL = u

	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		resp.Error = err
		return resp
	}
	hreq.Header.Set("Accept", "application/json")
	for k, v := range c.Headers {
		hreq.Header.Set(k, v)
	}
	if err := c.Auth.Apply(ctx, hreq); err != nil {
		resp.Error = fmt.Errorf("auth: %w", err)
		return resp
	}

	start := time.Now()
	hresp, err := c.HTTP.Do(hreq)
	resp.Duration = time.Since(start)
	if err != nil {
		resp.Error = err
		return resp
	}
	defer hresp.Body.Close()
	resp.Status = hresp.StatusCode
	resp.Headers = hresp.Header
	resp.Raw, err = io.ReadAll(io.LimitReader(hresp.Body, 64<<20))
	if err != nil {
		resp.Error = err
		return resp
	}
	if len(bytes.TrimSpace(resp.Raw)) > 0 {
		var body any
		dec := json.NewDecoder(bytes.NewReader(resp.Raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err == nil {
			resp.Body = body
		}
	}
	if resp.Status < 200 || resp.Status >= 300 {
		resp.Error = fmt.Errorf("HTTP %d %s", resp.Status, http.StatusText(resp.Status))
		return resp
	}
	c.extract(req, resp)
	return resp
}

// extract populates Items / Item from the parsed body using the request keys,
// with sensible fallbacks for APIs that return bare arrays/objects.
func (c *Client) extract(req Request, resp *Response) {
	obj, isObj := resp.Body.(map[string]any)
	if arr, ok := resp.Body.([]any); ok {
		resp.Items = toItems(arr)
		return
	}
	if !isObj {
		return
	}
	if req.ListKey != "" {
		if arr, ok := obj[req.ListKey].([]any); ok {
			resp.Items = toItems(arr)
			return
		}
	}
	if req.ItemKey != "" {
		if m, ok := obj[req.ItemKey].(map[string]any); ok {
			resp.Item = m
			return
		}
	}
	// Fallback: single top-level key wrapping the payload.
	if len(obj) == 1 {
		for _, v := range obj {
			switch t := v.(type) {
			case []any:
				resp.Items = toItems(t)
				return
			case map[string]any:
				resp.Item = t
				return
			}
		}
	}
	resp.Item = obj
}

func toItems(arr []any) []map[string]any {
	items := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			items = append(items, m)
		} else {
			items = append(items, map[string]any{"value": v})
		}
	}
	return items
}

// Ref is a reference to another resource found in a response object.
type Ref struct {
	Type     string // raw type value, e.g. "org"
	ID       string
	Href     string
	Resource *spec.Resource // resolved resource, nil if unknown
}

// AsRef reports whether v looks like a reference object (has an id and a
// type, per the spec's IDField) and resolves it.
func (c *Client) AsRef(v any) (Ref, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return Ref{}, false
	}
	id, _ := m[c.Spec.IDField].(string)
	typ, _ := m["type"].(string)
	if id == "" || typ == "" {
		return Ref{}, false
	}
	// A reference should be small: id, type, optional href.
	if len(m) > 4 {
		return Ref{}, false
	}
	href, _ := m["href"].(string)
	r := Ref{Type: typ, ID: id, Href: href}
	r.Resource, _ = c.Spec.ResourceForRefType(typ)
	return r, true
}

// ItemRequest builds a request for a single item of a resource.
func ItemRequest(s *spec.Spec, r *spec.Resource, id string) Request {
	path := r.ItemPath
	if path == "" {
		path = strings.TrimRight(r.ListPath, "/") + "/{" + s.IDField + "}"
	}
	vars := map[string]string{}
	for _, ph := range spec.Placeholders(path) {
		vars[ph] = id
	}
	return Request{Path: path, PathVars: vars, ItemKey: r.ItemKey}
}

// ListRequest builds a request for a resource collection with default paging.
func ListRequest(s *spec.Spec, r *spec.Resource) Request {
	q := map[string]string{}
	for _, p := range s.QueryParams {
		if p.Default != "" {
			q[p.Name] = p.Default
		}
	}
	return Request{Path: r.ListPath, PathVars: map[string]string{}, Query: q, ListKey: r.ListKey}
}

// RelatedRequest builds a request for a related sub-collection of an item.
func RelatedRequest(s *spec.Spec, rel spec.Related, id string) Request {
	vars := map[string]string{}
	for _, ph := range spec.Placeholders(rel.Path) {
		vars[ph] = id
	}
	q := map[string]string{}
	for _, p := range s.QueryParams {
		if p.Default != "" {
			q[p.Name] = p.Default
		}
	}
	return Request{Path: rel.Path, PathVars: vars, Query: q, ListKey: rel.ListKey}
}

// ItemID extracts the identifier from an item using the spec's IDField.
func ItemID(s *spec.Spec, item map[string]any) string {
	if v, ok := item[s.IDField]; ok {
		return fmt.Sprint(v)
	}
	if v, ok := item["id"]; ok {
		return fmt.Sprint(v)
	}
	return ""
}
