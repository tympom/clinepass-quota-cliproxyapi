# clinepass-quota-cliproxyapi

A native [CLIProxyAPI](https://help.router-for.me/plugin/development) plugin that adds a
**ClinePass Quota** page to CLIProxyAPI's Management Center, showing the 5-hour rolling,
weekly, and monthly usage windows for each configured [ClinePass](https://docs.cline.bot/getting-started/clinepass)
API key.

This plugin does **not** proxy chat completions — ClinePass models already work through a
plain OpenAI-compatible provider block pointed at `https://api.cline.bot/api/v1`. This
plugin's only job is quota visibility: it declares the `management_api` capability and
polls ClinePass's usage-limits endpoint on demand.

## How it works

- Registers one Management API route (`POST /plugins/clinepass-quota-cliproxyapi/quota-usage`)
  and one resource page (`GET /quota`, menu entry **ClinePass Quota**).
- The quota page shows one card per configured key with a **Refresh** button, plus a
  **Refresh All** button that refreshes every card in turn. On every page
  load the plugin calls `GET https://api.cline.bot/api/v1/users/me` (same bearer key) and
  titles the card `ClinePass · <displayName>` — e.g. `ClinePass · Przemek` — falling back to
  a plain `ClinePass` title if that lookup fails (an unreachable profile endpoint never blanks
  the card list). The plugin's JSON API separately identifies each key by a masked suffix,
  e.g. `ClinePass ••••1234`, via `api-keys[].label` or a default derived from the key itself,
  never the full key.
- Refreshing a card's quota windows calls, through the host's own `host.http.do` callback (so
  it reuses CLIProxyAPI's proxy settings, logging, and HTTP client — the key never leaves that
  path):

  ```
  GET https://api.cline.bot/api/v1/users/me/plan/usage-limits
  Authorization: Bearer <key>
  ```

  and maps the response's `five_hour` / `weekly` / `monthly` limit windows to the page.

## Requirements

- CLIProxyAPI built with plugin support (`X-CPA-SUPPORT-PLUGIN: 1` on its Management API
  responses) and `plugins.enabled: true` in its config.
- A ClinePass subscription and an API key from **app.cline.bot → Settings → API Keys**.

## Build

```bash
docker build -t clinepass-quota-cliproxyapi:build .
CID=$(docker create --entrypoint="" clinepass-quota-cliproxyapi:build /clinepass-quota-cliproxyapi.so)
docker cp "$CID":/clinepass-quota-cliproxyapi.so ./clinepass-quota-cliproxyapi.so
docker rm "$CID"
```

This produces a `linux/amd64` shared object (`CGO_ENABLED=1`, `-buildmode=c-shared`). The
build is deterministic: rebuilding with `--no-cache` reproduces a byte-identical artifact.

To target a different platform/arch, change `GOOS`/`GOARCH` in the Dockerfile's `RUN go
build` step (and its file extension: `.dylib` on macOS, `.dll` on Windows) — this plugin has
no platform-specific code, so cross-compilation is otherwise unconstrained.

## Release (CI)

`.github/workflows/build.yml` runs no tests (run `go test`/`go vet` locally before tagging). On
every push and PR it builds, and on any `v*` tag it cross-compiles `linux/amd64` and `linux/arm64` (the only deploy targets in actual use —
`conductor`/`uc1`/`ivy217` are all Linux; add darwin/windows matrix entries back if you ever need
them), then publishes a GitHub release with one
`clinepass-quota-cliproxyapi_<version>_<goos>_<goarch>.zip` per platform plus a combined
`checksums.txt` — the exact format CLIProxyAPI's Plugin Store installer expects (see
[Install → Option B](#option-b--cliproxyapi-plugin-store)).

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Install

### Option A — manual copy

Copy the built library into CLIProxyAPI's plugin directory for the current platform:

```
plugins/linux/amd64/clinepass-quota-cliproxyapi.so
```

### Option B — CLIProxyAPI Plugin Store

This repo ships a [Plugin Store registry manifest](./registry.json) (`schema_version: 1`,
per CLIProxyAPI's [Plugin Store Publishing Format](https://help.router-for.me/plugin/development#plugin-store-publishing-format))
and a [release workflow](#release-ci) that publishes the required
`<pluginID>_<version>_<goos>_<goarch>.zip` + `checksums.txt` assets on every `v*` tag.

1. Add this repo as a third-party store source in CLIProxyAPI's `config.yaml`:
   ```yaml
   plugins:
     store-sources:
       - "https://raw.githubusercontent.com/tympom/clinepass-quota-cliproxyapi/master/registry.json"
   ```
2. In the Management Center, open the Plugin Store page (or `GET /v0/management/plugin-store`)
   — `clinepass-quota-cliproxyapi` should now be listed.
3. Install it (`POST /v0/management/plugin-store/clinepass-quota-cliproxyapi/install`, or the
   equivalent Management Center button). CLIProxyAPI downloads the release asset for the
   current platform, verifies it against `checksums.txt`, and writes it into `plugins/`.

**Public repository, and requires a tagged release.** GitHub release-asset downloads need an
anonymous, unauthenticated request, which only works against a public repo (this one is
public). The store also resolves the *actual* installed version from the latest GitHub
release tag, not `registry.json`'s `version` field — push a `v*` tag (see [Release
(CI)](#release-ci)) before Option B has anything to install.

## Configuration

Under `plugins.configs.clinepass-quota-cliproxyapi` in CLIProxyAPI's `config.yaml`:

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    clinepass-quota-cliproxyapi:
      enabled: true
      # ClinePass API keys to poll for usage-limit windows. Supports ${ENV_VAR} expansion.
      api-keys:
        - value: "${CLINE_API_KEY}"
          # optional; JSON API only — the quota page itself always titles a
          # card "ClinePass · <account displayName>" (fetched live from
          # /api/v1/users/me), never this value. Omit unless you consume
          # the plugin's own quota-usage JSON directly.
          # label: "ClinePass"
      # Upstream base URL (default: "https://api.cline.bot")
      # base-url: "https://api.cline.bot"
      # Upstream request timeout (default: "15s")
      # request-timeout: "15s"
      # Allow http:// scheme for local mock/testing (default: false)
      # allow-http: false
```

| Option            | Type     | Default                  | Description                                                        |
| ------------------ | -------- | ------------------------- | -------------------------------------------------------------------- |
| `api-keys`         | `[]object` | *(required)* | List of ClinePass API keys (`- value: "...", label: "..."`). `${ENV_VAR}` expansion supported. Duplicates and empty values are rejected. |
| `base-url`         | `string` | `https://api.cline.bot`  | ClinePass API base URL. Must be valid HTTPS (or HTTP if `allow-http: true`), no query/fragment/userinfo. |
| `request-timeout`  | `duration` | `15s`                   | Upstream request timeout for the quota fetch. Must be positive.     |
| `allow-http`       | `bool`   | `false`                   | Permits `http://` scheme in `base-url` for local testing.           |

After editing `config.yaml`, restart CLIProxyAPI (or trigger `plugins.configs` reconfigure
if the host supports hot reload) and open **Management Center → ClinePass Quota**.

Note: the quota page reads the Management Center's stored management key from browser
storage to authenticate its requests — enable **Remember password** in Management Center
first, or the page reports "Enable Remember password... to use this page."

## Verification

```bash
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 go test ./...
```

`internal/plugin/quota_test.go` covers: parsing a real ClinePass usage-limits response
(including tolerating unknown future limit `type` values), surfacing upstream HTTP failures
as errors, the default masked-key-suffix label (never leaking the unmasked key), listing
cards with a best-effort account-profile fetch (a failed/unreachable profile lookup degrades
to no name rather than an empty card list), and the account name actually reaching the JSON
response. `internal/plugin/host_contract_test.go` decodes this plugin's
`management.register`/`management.handle` RPC responses using CLIProxyAPI's own
`sdk/pluginapi` types (`ManagementRoute`, `ResourceRoute`, `ManagementResponse`) — the same
types the real host uses to parse them — catching any wire contract mismatch here instead of
in production.

This plugin was also verified end-to-end against a real CLIProxyAPI instance and a real
ClinePass account: plugin registration, the Management API listing/refresh round trip, and
the bundled quota page's actual browser JavaScript (driven via jsdom against the live
instance) all confirmed working before each release in this history.

## Scope / non-goals

- No chat-completions routing, no model catalog, no multi-protocol translation — use a plain
  OpenAI-compatible provider block for that.
- No OAuth (`cline auth` CLI login) support — static API keys only, which map directly onto
  CLIProxyAPI's own key-rotation model and don't require an interactive login flow inside a
  headless proxy plugin.

## License

Not yet published; no license file included.
