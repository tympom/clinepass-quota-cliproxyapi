# clinepass-quota-cliproxyapi

A [CLIProxyAPI](https://help.router-for.me/plugin/development) plugin that adds a
**ClinePass Quota** page to the Management Center: one card per API key showing the
5-hour, weekly and monthly usage windows, titled with the account name, with **Refresh**
and **Refresh All**.

It does not proxy requests — use a plain OpenAI-compatible provider pointed at
`https://api.cline.bot/api/v1` for that. Needs a [ClinePass](https://docs.cline.bot/getting-started/clinepass)
API key (**app.cline.bot → Settings → API Keys**).

## Install

Add this repo as a store source in CLIProxyAPI's `config.yaml`:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/tympom/clinepass-quota-cliproxyapi/master/registry.json"
```

Then install it from the Management Center's Plugin Store page.

## Configuration

After install, add keys in **Management Center → Plugins → Edit config**, e.g. `["key"]` or
`[{"value": "key", "label": "work"}]`. In `config.yaml` the same options live under
`plugins.configs.clinepass-quota-cliproxyapi`.

| Option            | Default                 | Description |
| ----------------- | ----------------------- | ----------- |
| `api-keys`        | *(none)*                | Keys as strings or `{value, label}` objects; `${ENV_VAR}` is expanded; duplicates and empty values are rejected. `label` only appears in the plugin's JSON API. |
| `base-url`        | `https://api.cline.bot` | ClinePass API base URL; HTTPS unless `allow-http` is set. |
| `request-timeout` | `15s`                   | Upstream request timeout. |
| `allow-http`      | `false`                 | Allow `http://` in `base-url` for local testing. |

The quota page authenticates with the Management Center's stored key: enable
**Remember password** in the Management Center first.

## License

MIT — see [LICENSE](LICENSE).
