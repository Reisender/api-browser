# api-browser

A terminal UI for exploring REST APIs, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Load an API definition, list collections, page through results, open items,
follow references between resources and jump to related sub-collections —
without writing a single curl command.

Ships with the **OneRoster v1.1** and **v1.2** API specs built in.

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

# pick a builtin spec explicitly
bin/apibrowser -spec oneroster-v1p2 -url https://sis.example.com -auth bearer -token "$TOKEN"

# no flags: pick a spec, then type the connection details in
bin/apibrowser
```

Credentials can also come from `APIBROWSER_TOKEN`, `APIBROWSER_CLIENT_ID`,
`APIBROWSER_CLIENT_SECRET` and `APIBROWSER_TOKEN_URL`.

### Choosing a spec

`apibrowser -list-specs` prints the built-in specs:

| Spec | Base paths |
| --- | --- |
| `oneroster-v1p1` | `/ims/oneroster/v1p1` (rostering + gradebook) |
| `oneroster-v1p2` | `/ims/oneroster/{rostering,gradebook,resources}/v1p2` |

OneRoster v1.2 splits the API into three services with their own base paths,
adds `scoreScales`, `assessmentLineItems`, `assessmentResults` and a standalone
`resources` service, and hangs the gradebook and resource collections off
classes, courses, schools and users as related sub-collections.

Name one with `-spec`. If neither `-spec` nor the profile names a spec,
apibrowser opens a picker on start; `esc` there keeps the default
(`oneroster-v1p1`).

`S` reopens that picker at any time, so you can switch between v1.1 and v1.2
against the same host without restarting. Switching resets the navigation
back to the resource list — the collections and records you were looking at
belong to the old spec's endpoints — and keeps the connection and auth as they
are. Picking the spec you are already on just returns you to what you were
doing. `ctrl+s` on the connection screen saves the current spec with the
profile.

### Profiles

Press `a` (or `ctrl+s` on the connection screen) to save the current
connection as a named profile in `~/.config/api-browser/config.yaml`
(override with `-config` or `APIBROWSER_CONFIG`). Then:

```sh
bin/apibrowser -profile district
bin/apibrowser -list-profiles
```

Flags override profile values, so `-profile district -token X` swaps the token.

`P` opens the profile picker at any point in a session: pick one and the base
URL, auth, extra headers and spec all switch at once. As with `S`, the
navigation resets to the resource list — the records on screen came from the
old host — and picking the profile you are already on just returns you to what
you were doing. A profile that names a spec you cannot load, or auth that does
not validate, is reported in the status line and changes nothing. The header
shows the active profile's name.

`d` in the picker makes the highlighted profile the default, or clears the
default when it already is that profile.

The default profile is what a bare `apibrowser` loads. Without one — and
without `-profile` or `-url` — the picker opens on start instead, so long as
there are at least two profiles to choose between.

## Navigating

| Screen | Keys |
|---|---|
| Resources | `enter` list · `i` GET one record by id · `e` edit params before running · `/` filter list |
| Collection | `enter` open item · `i` GET by id · `/` live search rows · `A` fetch all pages · `n`/`p` next/prev page · `f` server filter · `s` sort · `L` page size · `e` edit all params · `r` raw JSON · `u` show URL · `y` copy id · `w` save records to file · `R` reload |
| Item | `enter` follow reference / toggle node · `t` toggle tree / pretty JSON · `w` save record to file · `l` related sub-collections · `←`/`→` collapse/expand · `+`/`-` expand/collapse all · `r` raw · `y` copy value |
| Raw | scroll · `y` copy JSON · `w` save response to file |
| Everywhere | `esc`/`backspace` back · `H` home · `a` connection · `P` switch profile · `S` switch spec · `?` help · `q` back/quit · `ctrl+c` quit |

Every request parameter is editable. On a collection, `L` changes the page
size (e.g. 100 → 1000), `f` and `s` set the server-side filter and sort, and
`e` opens the full editor with every path and query parameter — including
ones the spec doesn't list, via the `extra` field (`k=v&k2=v2`). The editor
shows the exact URL that will be requested as you type. Press `e` on the
resource list to set parameters *before* the first request.

The item endpoints (`GET /users/{sourcedId}`) are reachable directly, not
only by drilling into a listing. Press `i` on the resource list or on a
collection, type the id, and `enter` fetches that one record straight into
the item view — handy when you already know the id, or when the record isn't
on the page you're looking at. Resources with no `itemPath` fall back to
`listPath` + `/{idField}`. The editor is the same one `e` opens, minus the
paging parameters, so you can still send `fields=` or anything in `extra`.

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

`-dump-spec -` writes to stdout. It also works on the builtin specs
(`-spec oneroster-v1p2 -dump-spec -`) as a template.

## Writing your own spec

Specs are small YAML files (see `internal/spec/specs/`).
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

## Development

```sh
make test     # go test ./... -cover
make lint     # go vet + gofmt
```

Layout:

- `internal/spec` – spec model, YAML loading, embedded OneRoster definitions
- `internal/openapi` – infers a spec from OpenAPI 3.x / Swagger 2 documents
- `internal/auth` – bearer / OAuth2 client-credentials (cached, auto-refresh) / arbitrary header
- `internal/client` – spec-driven HTTP client, list/item extraction, reference detection
- `internal/jsontree` – collapsible JSON tree rows
- `internal/config` – saved profiles
- `internal/tui` – Bubble Tea screens and stack navigation
