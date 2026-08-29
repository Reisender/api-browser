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
| Collection | `enter` open item · `/` live search rows · `n`/`p` next/prev page · `f` server filter · `s` sort · `e` edit all params · `r` raw JSON · `u` show URL · `y` copy id · `R` reload |
| Item | `enter` follow reference / toggle node · `l` related sub-collections · `←`/`→` collapse/expand · `+`/`-` expand/collapse all · `r` raw · `y` copy value |
| Raw | scroll · `y` copy JSON |
| Everywhere | `esc`/`backspace` back · `H` home · `a` connection · `?` help · `q` back/quit · `ctrl+c` quit |

`/` on a collection opens a live search box: rows narrow as you type (any
field, case-insensitive, space-separated words must all match). `enter` keeps
the filter and returns to the table, `esc` clears it. The search survives
paging and reloads. This filters the page you already fetched; `f` sends a
server-side `filter=` expression instead.

References are detected generically: any object carrying the spec's id field
plus a `type` (e.g. `{"sourcedId": "o1", "type": "org", "href": …}`) is
rendered as a link and `enter` fetches it. Related collections come from the
spec, e.g. a class offers `students`, `teachers`, `lineItems`, `results`.

Error responses (401, 404, …) are surfaced in the status line and their body
opened in the raw viewer so you can read what the server said.

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
- `internal/auth` – bearer / OAuth2 client-credentials (cached, auto-refresh) / arbitrary header
- `internal/client` – spec-driven HTTP client, list/item extraction, reference detection
- `internal/jsontree` – collapsible JSON tree rows
- `internal/config` – saved profiles
- `internal/tui` – Bubble Tea screens and stack navigation
