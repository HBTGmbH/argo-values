# Configuration

This page describes how to configure an Application, how `argo-values` resolves
values, and what limitations exist.

## Application Configuration

`argo-values` is enabled through `spec.source.plugin`. There are two configuration
mechanisms:

### plugin.env

`plugin.env` is a list of name/value pairs. The values are **comma-separated
strings**:

```yaml
spec:
  source:
    plugin:
      env:
        - name: value-configs
          value: default/application-values,other-ns/extra-values
        - name: env-configs
          value: default/application-variables
```

Argo CD exposes these entries to the plugin as `ARGOCD_ENV_*` variables, for
example `value-configs` as `ARGOCD_ENV_VALUE_CONFIGS`.

### plugin.parameters

`plugin.parameters` is a list of parameter objects. The values are **arrays or
maps**:

```yaml
spec:
  source:
    plugin:
      parameters:
        - name: valueConfigs
          array:
            - default/application-values
            - other-ns/extra-values
        - name: helmValues
          map:
            replicaCount: "3"
            resources.requests.cpu: "250m"
```

> **Requirement:** `plugin.parameters` only works when the parameters are declared
> under `spec.parameters.static` in `plugin.yaml`. The full `plugin.yaml` in
> Getting Started contains this declaration; a minimal `plugin.yaml` snippet
> without `spec.parameters.static` does not. The full reference is in
> [reference.md](reference.md#plugin-yaml-reference).

Both mechanisms may be used in parallel, for example `plugin.env` for the source
lists and `plugin.parameters` for `helmValues`. For the same source list, however,
the precedence described below applies.

### plugin.name

If multiple CMPs are installed, you can select the plugin explicitly. The provided
`plugin.yaml` sets `metadata.name: argo-values` and `spec.version: v1.0`, so its
effective name is `argo-values-v1.0`:

```yaml
spec:
  source:
    plugin:
      name: argo-values-v1.0
      env:
        - name: value-configs
          value: default/application-values
```

With `spec.version` set, the effective plugin name is
`<metadata.name>-<spec.version>` (here `argo-values-v1.0`). Without
`spec.version`, it is only `<metadata.name>` (`argo-values`).

Without `plugin.name`, Argo CD decides which plugin is responsible based on the
discovery results.

## Value Sources

| Source | Purpose | Value format |
| --- | --- | --- |
| `value-configs` / `valueConfigs` | additional Helm values | YAML mapping |
| `value-secrets` / `valueSecrets` | additional Helm values from Secrets | YAML mapping |
| `env-configs` / `envConfigs` | values for `${VAR}` substitution | literal, no YAML |
| `env-secrets` / `envSecrets` | values for `${VAR}` substitution from Secrets | literal, no YAML |
| `helmValues` | direct Helm values from the Application | map of strings |

For ConfigMaps, only `data` is evaluated; `binaryData` is ignored. For Secrets,
`data` is read after the Kubernetes client decodes it from base64.

## Reference Formats

A reference is either `name` or `namespace/name`:

- Whitespace around the parts is trimmed.
- Empty parts are invalid: `/name`, `namespace/`, `""`.
- More than one `/` is invalid: `a/b/c`.
- Unqualified names are resolved against the destination namespace:
  - `init`: `ARGOCD_APP_NAMESPACE`, fallback `default`.
  - `watch`: `spec.destination.namespace`, fallback `default`.
- `watch` warns about invalid references and skips them. `init` fails hard on
  invalid references (exit 1).

Recommendation: Always qualify references as `namespace/name` so that the render
path and the refresh path refer to the same resource.

## Precedence

Precedence applies **per source list** (separately for value ConfigMaps, value
Secrets, env ConfigMaps, and env Secrets):

1. **CLI flags** (`--value-maps`, `--env-maps`, `--value-secrets`,
   `--env-secrets`) – relevant only for a manual `init` run.
2. **`plugin.parameters`** (`ARGOCD_APP_PARAMETERS`).
3. **`plugin.env`** (`ARGOCD_ENV_*`).

Details:

- CLI flags override only when they are not empty.
- `plugin.parameters` overrides `plugin.env` as soon as the name is present in the
  parameter object – even with an empty array. An empty parameter therefore
  intentionally disables the sources.
- `helmValues` is available only through `plugin.parameters`.

## Merge Semantics for Values

The order is fixed:

1. `valueConfigs` in list order,
2. then `valueSecrets` in list order.

Later sources override earlier ones. Within a ConfigMap or Secret, data keys are
processed in **lexicographic order**; when maps overlap, the key processed later
wins.

The **key name** of a value source is irrelevant as long as the value is a valid
YAML mapping. The mapping is merged directly. If the value is not a mapping (a
scalar or a top-level list), the raw text is stored under the key; if the value
looks structured, a warning is also logged.

- **Deep merge** applies only to maps.
- **Lists are replaced, not merged.**
- Multi-document YAML is not a supported format: only the first document is
  evaluated. Use exactly one YAML document per data key.

### helmValues

After the ConfigMaps and Secrets, `helmValues` is merged and overrides previous
values. The values are strings; the keys are interpreted as dotted paths including
array syntax:

```yaml
plugin:
  parameters:
    - name: helmValues
      map:
        image.tag: "1.2.3"
        env[0].name: "LOG_LEVEL"
        env[0].value: "info"
        resources.requests.cpu: "250m"
```

- `env[0].name` creates a list with one map entry.
- Sparse arrays are padded with `null`.
- Indices above `10000` and invalid array syntax remain literal keys.
- Scalars are coerced: `"3"` becomes `int`, `"true"` becomes `bool`, and
  `"null"`/`"~"` become `null`. `"0123"` remains a string, as does the empty
  string.
- `helmValues` and `plugin.env` are part of the Application and are **not a
  suitable place for secrets**.

## Env Values and Substitution

Env sources are taken **literally** and are never parsed as YAML. During flattening,
keys are sanitized:

- uppercase,
- `.` and `-` become `_`.

Example: The key `db.host` becomes `DB_HOST`; `${db.host}` in the chart is also
sanitized to `DB_HOST`. Collisions are possible (for example `db.host` and
`db-host`); avoid them, because the key processed last wins.

Substitution:

- supports only `${VAR}` with braces,
- has no shell semantics, no escaping, and no defaults such as `${VAR:-fallback}`,
- does not replace unknown keys; it leaves them in place and logs a warning,
- does **not** expand values recursively.

All regular files in the target directory are processed unless `.helmignore`
excludes them. The `.helmignore` is read only from the chart directory root; the
Helm default `templates/.?*` is added. Invalid patterns cause exit 1; the values
file may already have been written at that point.

Additional details:

- Symlinks are skipped.
- "Binary files" are skipped using a NUL-byte heuristic. This is not real binary
  detection.
- A file is written only when its content changes.
- `--dry-run` writes neither the values file nor substituted files.
- `init` writes the values file **before** substitution. As a result, `${VAR}`
  placeholders in values from ConfigMaps/`helmValues` are replaced as well.
- If `.helmignore` excludes `app-values.yaml`, it is still used by
  `helm template -f`, but it is not substituted.
- `.helmignore` and the generated values file are processed as well. Changed files
  are written with `0644`; the values file is initially created with `0600`.
- If the values file is changed during substitution, it is also written with
  `0644`. This is an operational/security caveat.

## Files and Paths

| Setting | Default | Meaning |
| --- | --- | --- |
| `--file-target` | `.` | Chart code directory that `init` reads from and writes to |
| `--file-values` | `app-values.yaml` | Name of the generated values file, relative to `--file-target` |
| `--kubeconfig` | – | Used only by `init` and `watch`; otherwise in-cluster config |
| `--dry-run` | `false` | Used only by `init` and `watch`; `generate` ignores it |

`generate` invokes exactly the following command:

```text
helm template <app-name> <target-dir> \
  --namespace <app-namespace> \
  --include-crds \
  --dependency-update \
  [--kube-version <normalized-version>] \
  [--api-versions <api-version> ...] \
  -f <target-dir>/<values-file>
```

The `-f` option is always passed; `init` creates the values file beforehand. A
manual `generate` run without a preceding `init` therefore fails if the file does
not exist.

- `<app-namespace>` is `ARGOCD_APP_NAMESPACE`; the flag default `argocd` applies
  only when the variable is missing.
- `KUBE_VERSION` is normalized to the leading numeric dotted core (for example,
  `v1.29.0+` → `1.29.0`).
- `KUBE_API_VERSIONS` is comma-separated.
- Missing chart dependencies are fetched via `--dependency-update`; this may
  require network and Helm repository access.

## Limitations

- No Kustomize support.
- No `valueFiles`/`valuesObject` support in the plugin; values come from
  ConfigMaps/Secrets and `helmValues`.
- No write-back to Git.
- No health, readiness, or metrics endpoints; observability is provided through
  logs.
- No real binary handling; only the NUL-byte heuristic.
- No encryption of Secrets; see [Operations – Security](operations.md#security).
- No list merges; lists are always replaced.
- No state transfer between CMP processes through environment variables; only
  files.
