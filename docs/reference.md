# Reference

This page is the technical reference for commands, flags, environment variables,
exit codes, file conventions, and `plugin.yaml`.

## CLI Commands

| Command | Purpose |
| --- | --- |
| `discover` | checks whether the Application is supported by `argo-values` |
| `init` | reads ConfigMaps/Secrets, merges values, writes the values file, and substitutes `${VAR}` |
| `generate` | runs `helm template` and emits the manifests on stdout |
| `watch` | watches resources and patches affected Applications for a refresh |

`argo-values --help` shows the global help, and `argo-values <command> --help`
shows the help for a command. Without a subcommand, help is displayed (exit 0).

## Global Flags

Global flags can be placed before or after the subcommand.

| Flag | Short | Env | Default | Meaning |
| --- | --- | --- | --- | --- |
| `--log-level` | `-l` | `LOG_LEVEL` | `debug` | `debug`, `info`, `warn`, `error`, `fatal`, `panic` |
| `--log-format` | – | `LOG_FORMAT` | `json` | `json`, `text` |
| `--log-output` | – | `LOG_OUTPUT` | `stderr` | `stderr`, `stdout` |
| `--kubeconfig` | `-k` | – | – | Path to the kubeconfig; used only by `init`/`watch` |
| `--file-values` | `-v` | – | `app-values.yaml` | Name of the values file |
| `--file-target` | `-t` | – | `.` | Directory containing the chart files |
| `--dry-run` | `-d` | – | `false` | no writes/patches for `init`/`watch`; `generate` ignores it |
| `--version` | – | – | `dev` | Print version and exit |

## Command Flags

### discover

No command-specific flags.

Output:

- supported: token `argo-values` on stdout, exit `0`.
- not supported: nothing on stdout, exit `2`.
- parse error from `ARGOCD_APP_PARAMETERS`: error on stderr, exit `1`.

### init

| Flag | Repeatable | Splits commas? | Meaning |
| --- | --- | --- | --- |
| `--value-maps` | yes | no | value ConfigMaps |
| `--env-maps` | yes | no | env ConfigMaps |
| `--value-secrets` | yes | no | value Secrets |
| `--env-secrets` | yes | no | env Secrets |

Each occurrence is taken as-is. `--value-maps a,b` literally looks for the
ConfigMap `a,b`, not for `a` and `b`.

CLI flags override `plugin.parameters` and `plugin.env` only when they are not
empty.

### generate

| Flag | Env | Default | Meaning |
| --- | --- | --- | --- |
| `--app-name` | `ARGOCD_APP_NAME` | – | Application name for `helm template` |
| `--app-namespace` | `ARGOCD_APP_NAMESPACE` | `argocd` (only when env is missing) | Helm `--namespace` |

`generate` ignores `--dry-run` and `--kubeconfig`.

### watch

| Flag | Short | Env | Default | Meaning |
| --- | --- | --- | --- | --- |
| `--refresh-interval` | `-i` | `ARGOCD_VALUES_REFRESH_SECONDS` | `2` | batch/flush interval in seconds; must be `> 0` |
| `--namespaces` | – | `ARGOCD_VALUES_NAMESPACES` | `argocd,default` | namespaces to watch |

`--namespaces` is repeatable and additionally accepts comma-separated values, for
example `--namespaces argocd --namespaces default,workloads`.

## Environment Variables

### Plugin Inputs

| Variable | Meaning |
| --- | --- |
| `ARGOCD_ENV_VALUE_CONFIGS` | comma-separated value ConfigMaps |
| `ARGOCD_ENV_ENV_CONFIGS` | comma-separated env ConfigMaps |
| `ARGOCD_ENV_VALUE_SECRETS` | comma-separated value Secrets |
| `ARGOCD_ENV_ENV_SECRETS` | comma-separated env Secrets |
| `ARGOCD_APP_PARAMETERS` | JSON array containing `plugin.parameters` |
| `ARGOCD_APP_NAME` | Application name for `generate` |
| `ARGOCD_APP_NAMESPACE` | Application destination namespace |
| `KUBE_VERSION` | Kubernetes version for Helm; normalized |
| `KUBE_API_VERSIONS` | comma-separated API versions for Helm |
| `ARGOCD_VALUES_NAMESPACES` | namespaces for `watch` |
| `ARGOCD_VALUES_REFRESH_SECONDS` | flush interval for `watch` in seconds |

For `ARGOCD_ENV_*`, the mere **existence** of one of the four variables is enough
for discovery, even with an empty value. When evaluating the four `ARGOCD_ENV_*`
variables, the plugin treats upper/lower case and `-`/`_` as equivalent.

`ARGOCD_APP_PARAMETERS` has this format:

```json
[
  { "name": "valueConfigs", "array": ["default/application-values"] },
  { "name": "envSecrets", "array": ["default/application-env"] },
  { "name": "helmValues", "map": { "replicaCount": "3" } }
]
```

Unknown parameters and empty arrays do not count as support during discovery.
`helmValues` counts when the map is not empty.

### Logging

See [Operations – Logging](operations.md#logging).

## Exit Codes

| Code | Meaning |
| --- | --- |
| `0` | success (also: `discover` supports the Application, help displayed) |
| `1` | runtime/configuration error after parsing |
| `2` | exclusively: `discover` does not support the Application |
| `80` | invalid CLI usage (unknown flags/commands, missing flag values, invalid CLI values) |

Examples of exit `1`:

- missing ConfigMap/Secret,
- invalid reference in `init`,
- broken `ARGOCD_APP_PARAMETERS`,
- Helm error in `generate`,
- `--refresh-interval <= 0`,
- invalid environment values such as `ARGOCD_VALUES_REFRESH_SECONDS=abc`,
- invalid `.helmignore` patterns.

> **Important:** Invalid **environment values** are exit `1`, not exit `80`. Exit
> `80` is reserved exclusively for invalid CLI usage.

## File Conventions

| Path/file | Meaning |
| --- | --- |
| `<file-target>` | chart directory, default `.` |
| `<file-target>/<file-values>` | generated values file, default `./app-values.yaml` |
| `<file-target>/.helmignore` | ignore rules; read only from the root |
| `templates/.?*` | additional Helm default; always added |

- `init` writes the values file before substitution.
- The values file is initially created with `0600`; changed files (including the
  values file if it is changed during substitution) are written with `0644`.
- A file is written only when its content changes.
- `--dry-run` writes neither the values file nor substituted files.
- Symlinks are skipped.
- Files containing a NUL byte are skipped as "binary".
- If `.helmignore` excludes the values file, it is still used by
  `helm template -f`, but it is not substituted.

## Plugin YAML Reference

Full example:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ConfigManagementPlugin
metadata:
  name: argo-values
spec:
  version: v1.0
  parameters:
    static:
      - name: helmValues
        title: Helm Values
        collectionType: map
      - name: valueConfigs
        title: Value ConfigMap Names
        tooltip: List of ConfigMap names with additional values.yaml files for the Helm chart of the application
        required: false
        itemType: "string"
        collectionType: "array"
      - name: valueSecrets
        title: Value Secret Names
        tooltip: List of Secret names with additional values.yaml files for the Helm chart of the application
        required: false
        itemType: "string"
        collectionType: "array"
      - name: envConfigs
        title: Environment ConfigMap Names
        tooltip: List of ConfigMap names with values which can be used as ENV variables in the Helm chart before rendering the application
        required: false
        itemType: "string"
        collectionType: "array"
      - name: envSecrets
        title: Environment Secret Names
        tooltip: List of Secret names with values which can be used as ENV variables in the Helm chart before rendering the application
        required: false
        itemType: "string"
        collectionType: "array"
  init:
    command: [argo-values]
    args: [init]
  generate:
    command: [argo-values]
    args: [generate]
  discover:
    find:
      command: [argo-values]
      args: [discover]
```

| Field | Meaning |
| --- | --- |
| `metadata.name` | base plugin name, e.g. `argo-values`; keep it unique when running multiple CMPs |
| `spec.version` | version of the plugin contract; when set, the effective plugin name is `<metadata.name>-<spec.version>` |
| `spec.parameters.static` | declares the parameters that Argo CD passes in `ARGOCD_APP_PARAMETERS` |
| `collectionType: array` | parameter is an array, e.g. `valueConfigs` |
| `collectionType: map` | parameter is a map, e.g. `helmValues` |
| `itemType` | element type of an array (here `string`) |
| `required` | whether the parameter is required in the UI |
| `init.command`/`args` | invocation for the init phase |
| `generate.command`/`args` | invocation for the render phase |
| `discover.find.command`/`args` | invocation for discovery |

With `spec.version` set, the effective plugin name is
`<metadata.name>-<spec.version>`; without `spec.version`, it is only
`<metadata.name>`. The bundled manifest sets `metadata.name: argo-values` and
`spec.version: v1.0`, so the effective name is **`argo-values-v1.0`**. Use that
value for `spec.source.plugin.name`.

Without `spec.parameters.static`, `plugin.parameters` and `helmValues` do not work
reliably. The chart ConfigMap contains the declaration; a minimal snippet with only
`init`/`generate`/`discover` does not.

## Naming Mapping

| Purpose | `plugin.env` (kebab-case, comma-separated string) | `plugin.parameters` (camelCase, array/map) | CLI flag |
| --- | --- | --- | --- |
| Value ConfigMaps | `value-configs` | `valueConfigs` (array) | `--value-maps` |
| Env ConfigMaps | `env-configs` | `envConfigs` (array) | `--env-maps` |
| Value Secrets | `value-secrets` | `valueSecrets` (array) | `--value-secrets` |
| Env Secrets | `env-secrets` | `envSecrets` (array) | `--env-secrets` |
| Helm Values | – | `helmValues` (map) | – |

- `plugin.env` uses comma-separated strings.
- `plugin.parameters` uses arrays or maps.
- CLI flags are repeatable but do not split commas.
- `helmValues` is available only through `plugin.parameters`.

## Reference Format

`name` or `namespace/name`; whitespace is trimmed. Empty parts and more than one
`/` are invalid. `watch` warns about and skips invalid references; `init` fails.
Details: [Configuration – Reference Formats](configuration.md#reference-formats).
