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
- The quota page lists every configured key (identified by a SHA-256 hash of the key value,
  never the key itself) with a **Refresh** button per card.
- Refreshing a card calls, through the host's own `host.http.do` callback (so it reuses
  CLIProxyAPI's proxy settings, logging, and HTTP client — the key never leaves that path):

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

## Install

Copy the built library into CLIProxyAPI's plugin directory for the current platform:

```
plugins/linux/amd64/clinepass-quota-cliproxyapi.so
```

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
          label: "ClinePass" # optional; defaults to a hash-derived label
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
go vet ./...
go test ./...
```

`internal/plugin/quota_test.go` covers: parsing a real ClinePass usage-limits response
(including tolerating unknown future limit `type` values), surfacing upstream HTTP failures
as errors, and listing configured keys without contacting the upstream API.

## Scope / non-goals

- No chat-completions routing, no model catalog, no multi-protocol translation — use a plain
  OpenAI-compatible provider block for that.
- No OAuth (`cline auth` CLI login) support — static API keys only, which map directly onto
  CLIProxyAPI's own key-rotation model and don't require an interactive login flow inside a
  headless proxy plugin.

## License

Not yet published; no license file included.
