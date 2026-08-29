# api-browser

A terminal UI for exploring REST APIs, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Load an API definition, list collections, page through results, open items,
follow references between resources and jump to related sub-collections —
without writing a single curl command.

Ships with the **IMS OneRoster v1.1** API spec built in (rostering + gradebook).

```
┌ api-browser  resources › classes › classes/c1 › courses/k1      https://sis.example.com  oauth2 client abc @ …
│ k1  6 fields  1 related (l)
│ ▾ metadata {2}
│     ▸ periods [3] ["1","2","3"]
│   courseCode: "MATH101"
│   org: org/o1 → orgs
│   schoolYear: academicSession/sy2026 → academicSessions
│   sourcedId: "k1"
│   title: "Mathematics"
└ enter follow reference / toggle  l related collections  ←/→ collapse / expand  r raw JSON  y copy value  ? help
```

## Install / run

With Go 1.26+ installed:

```sh
go install github.com/Reisender/api-browser/cmd/apibrowser@latest
```

This puts `apibrowser` in `$(go env GOPATH)/bin` (usually `~/go/bin`); make
sure that is on your `PATH`. To pin a specific release, replace `@latest`
with a tag such as `@v0.1.1`; `apibrowser --version` reports what you have. Or build from a checkout:

```sh
go build -o bin/apibrowser ./cmd/apibrowser

# bearer token
bin/apibrowser -url https://sis.example.com -auth bearer -token "$TOKEN"

# OAuth2 client credentials
bin/apibrowser -url https://sis.example.com -auth oauth2 \
    -client-id "$ID" -client-secret "$SECRET" -token-url https://sis.example.com/oauth/token

# arbitrary header
bin/apibrowser -url https://sis.example.com -auth header -header 'X-Api-Key: abc123'

# no flags: opens the connection screen where you can type everything in
bin/apibrowser
```

Credentials can also come from `APIBROWSER_TOKEN`, `APIBROWSER_CLIENT_ID`,
`APIBROWSER_CLIENT_SECRET` and `APIBROWSER_TOKEN_URL`.

### Profiles

Press `a` (or `ctrl+s` on the connection screen) to save the current
connection as a named profile in `~/.config/api-browser/config.yaml`
(override with `-config` or `APIBROWSER_CONFIG`). Then:

```sh
bin/apibrowser -profile district
bin/apibrowser -list-profiles
```

Flags override profile values, so `-profile district -token X` swaps the token.

## Navigating

| Screen | Keys |
|---|---|
| Resources | `enter` list · `e` edit params before running · `/` filter list |
| Collection | `enter` open item · `/` live search rows · `A` fetch all pages · `n`/`p` next/prev page · `f` server filter · `s` sort · `L` page size · `e` edit all params · `r` raw JSON · `u` show URL · `y` copy id · `w` save records to file · `R` reload |
| Item | `enter` follow reference / toggle node · `t` toggle tree / pretty JSON · `w` save record to file · `l` related sub-collections · `←`/`→` collapse/expand · `+`/`-` expand/collapse all · `r` raw · `y` copy value |
| Raw | scroll · `y` copy JSON · `w` save response to file |
| Everywhere | `esc`/`backspace` back · `H` home · `a` connection · `?` help · `q` back/quit · `ctrl+c` quit |

Every request parameter is editable. On a collection, `L` changes the page
size (e.g. 100 → 1000), `f` and `s` set the server-side filter and sort, and
`e` opens the full editor with every path and query parameter — including
ones the spec doesn't list, via the `extra` field (`k=v&k2=v2`). The editor
shows the exact URL that will be requested as you type. Press `e` on the
resource list to set parameters *before* the first request.

`/` on a collection opens a live search box: rows narrow as you type (any
field, case-insensitive, space-separated words must all match). `enter` keeps
the filter and returns to the table, `esc` clears it. The search survives
paging and reloads. This filters the page you already fetched; `f` sends a
server-side `filter=` expression instead.

To search across *every* page, press `ctrl+a` inside the search box (or `A`
on the table). The browser walks `offset`/`limit` until the server returns a
short page, showing progress in the footer (`esc` cancels and keeps what you
had), then shows the combined result with your search applied. The header
reads `all pages (12 × 100)`; `n`/`p` are no-ops until you `R`eload.

On an item, `t` toggles between the collapsible tree and a syntax-highlighted
pretty-JSON view of the record itself (without the response wrapper). Scroll
with the arrow keys, `y` copies the JSON; `l`, `r`, `u` and `R` work in both
views.

`w` saves what you're looking at as pretty JSON: the record on an item screen
(default name `classes-c1.json`), the visible — i.e. searched — records as an
array on a collection, or the full response on the raw screen. You're
prompted for the path (`~` expands, directories are created, overwriting an
existing file asks for a second `enter`).

References are detected generically: any object carrying the spec's id field
plus a `type` (e.g. `{"sourcedId": "o1", "type": "org", "href": …}`) is
rendered as a link and `enter` fetches it. Related collections come from the
spec, e.g. a class offers `students`, `teachers`, `lineItems`, `results`.

Error responses (401, 404, …) are surfaced in the status line and their body
opened in the raw viewer so you can read what the server said.

## Using an OpenAPI / Swagger document

Point `-spec` at any OpenAPI 3.x or Swagger 2 file (YAML or JSON) and the
browser infers a navigation spec from it:

```sh
apibrowser -spec ./openapi.yaml -url https://api.example.com
```

What gets inferred:

- every `GET /things` becomes a resource; `GET /things/{id}` is its item endpoint
- `GET /things/{id}/others` becomes a *related* link on `things`
- list/item wrapper keys (`{"users": [...]}` vs a bare array) from the 200 response schema
- display columns from the item schema's scalar properties (`allOf` is merged)
- query parameters and their defaults; `limit`/`offset`-style paging is detected
  (`per_page`, `pageSize`, `skip`, `$top`/`$skip` …)
- the id field from the item path placeholder, `servers[0].url` for the base path

The heuristics are decent but not perfect. To tune the result, dump the
inferred spec to native YAML, edit it, and use that instead:

```sh
apibrowser -spec ./openapi.yaml -dump-spec my-api.yaml
$EDITOR my-api.yaml          # fix wrapper keys, add refTypes, reorder columns…
apibrowser -spec my-api.yaml -url https://api.example.com
```

`-dump-spec -` writes to stdout. It also works on the builtin spec
(`-spec oneroster-v1p1 -dump-spec -`) as a template.

## Writing your own spec

Specs are small YAML files (see `internal/spec/specs/oneroster-v1p1.yaml`).
Pass a path with `-spec ./my-api.yaml`.

```yaml
name: Pet Store
basePath: /v2
idField: id
paging: {limitParam: limit, offsetParam: offset, defaultLimit: 50}
queryParams:
  - {name: limit, default: "50"}
  - {name: offset, default: "0"}
  - {name: status, description: "available | pending | sold"}
refTypes:        # value of the "type" field in a reference -> resource
  owner: users
resources:
  - name: pets
    listPath: /pets
    itemPath: "/pets/{id}"
    listKey: pets        # top-level key holding the array (omit for bare arrays)
    itemKey: pet         # top-level key holding the object
    columns: [id, name, status]
    related:
      - {name: visits, path: "/pets/{id}/visits", listKey: visits, resource: visits}
  - name: users
    listPath: /users
  - name: visits
    listPath: /visits
```

`apibrowser -list-specs` prints the built-in specs.

## Development

```sh
make test     # go test ./... -cover
make lint     # go vet + gofmt
```

Layout:

- `internal/spec` – spec model, YAML loading, embedded OneRoster definition
- `internal/openapi` – infers a spec from OpenAPI 3.x / Swagger 2 documents
- `internal/auth` – bearer / OAuth2 client-credentials (cached, auto-refresh) / arbitrary header
- `internal/client` – spec-driven HTTP client, list/item extraction, reference detection
- `internal/jsontree` – collapsible JSON tree rows
- `internal/config` – saved profiles
- `internal/tui` – Bubble Tea screens and stack navigation
