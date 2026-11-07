# Operations

This page describes how to operate the optional `watch` sidecar, logging, security,
RBAC, compatibility, and troubleshooting.

## watch and Refresh

`watch` runs as an additional sidecar in the **application-controller**. It is not a
CMP component and is not invoked by the repo-server.

### Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| `ARGOCD_VALUES_NAMESPACES` / `--namespaces` | `argocd,default` | Namespaces to watch |
| `ARGOCD_VALUES_REFRESH_SECONDS` / `--refresh-interval` | `2` | Batch/flush interval in seconds; must be `> 0` |

`ARGOCD_VALUES_NAMESPACES` must contain the namespaces of the Applications **and**
of all referenced ConfigMaps/Secrets. If a namespace is missing, changes there are
not observed and the affected Application is not refreshed.

The interval is a **batch/flush interval**, not a reset-based debounce: events are
collected and processed as a batch on the next tick.

### Flow

1. The watcher lists Applications, ConfigMaps, and Secrets in the configured
   namespaces and keeps watches open.
2. On startup, Applications are read as `ADDED` to build the mapping from
   Application to referenced resources. ConfigMaps and Secrets are initially only
   cached, not reported as events. As a result, initial seeding does not trigger
   any refreshes.
3. Changes are collected until the next flush.
4. The event handler updates the mapping and determines affected Applications.
5. For each affected Application, a merge patch sets the annotation
   `argocd.argoproj.io/refresh: hard`.

An Application that appears for the first time in the same batch is not refreshed
yet (initial seeding). Already known Applications are refreshed even if they did
not receive their own event in the batch, as long as a referenced
ConfigMap/Secret changed.

### Mapping

The mapping reads `spec.source.plugin.env`, `spec.source.plugin.parameters`, and
`spec.sources`. For `plugin.env`, comma-separated values are split and trimmed.
For `plugin.parameters`, only the `array` field is read. Names are recognized
heuristically: prefix `env` or `value`, suffix `configs` or `secrets`.

> **Important:** If you accidentally use `value: "a,b"` instead of
> `array: ["a", "b"]` in `plugin.parameters`, `watch` does not map the resources
> and does not refresh the Application. `init` may also see no sources if the
> parameter arrives with an empty array; a present parameter name overrides the
> corresponding `ARGOCD_ENV_*` variable.

`watch` only warns about invalid references and skips them. `init`, by contrast,
fails hard.

### Refresh Patch

- Merge patch on `applications.argoproj.io`.
- Annotation `argocd.argoproj.io/refresh: hard`.
- Up to **3 attempts**, **10 s timeout**, **concurrency 5**.
- Timeout, throttling (429), 5xx, network, and conflict errors are retryable.
- **Not** retryable: 404, 403, 401, 400, and 422.

### Robustness

- Every 5 minutes, a resync is performed by listing and diffing again.
- Reconnect backoff ranges from 1 s to 30 s.
- Watch/RBAC errors are logged and retried; the watcher does not exit because of
  them.
- `--dry-run` disables refresh patches. The watcher still observes and logs, but
  does not patch.

## Logging

| Variable | Default | Values |
| --- | --- | --- |
| `LOG_LEVEL` | `debug` | `debug`, `info`, `warn`, `error`, `fatal`, `panic` |
| `LOG_FORMAT` | `json` | `json`, `text` |
| `LOG_OUTPUT` | `stderr` | `stderr`, `stdout` |

- `stdout` is reserved for command output: the discovery token (`argo-values`) and
  the rendered Helm manifests.
- **Never set `LOG_OUTPUT=stdout` in the CMP.** It corrupts the discovery token and
  the Helm manifests.
- Debug logs do not contain secret values, but they do contain resource names and
  keys. `LOG_LEVEL=info` is recommended for production.
- There are no health, readiness, or metrics endpoints. Observability is provided
  through logs.

## Security

- **ConfigMaps are the primary use case.** They are intended for non-sensitive
  values.
- **Secrets** are supported, but Argo CD caches rendered manifests in plaintext in
  Redis and exposes them through the repo-server API. In addition, the generated
  `app-values.yaml` is stored on the repo-server pod's disk.
- For sensitive values, External Secrets Operator, Vault Secrets Operator, or
  Sealed Secrets are better choices. Reference the resulting Kubernetes Secret from
  the chart instead of rendering its value.
- `helmValues` and `plugin.env` are part of the Application resource and are not a
  suitable place for secrets.
- There is no write-back to Git. Rendered manifests cannot be reproduced from Git
  alone; this is incompatible with the "Rendered Manifests" pattern.
- Changed files are written with `0644`; the values file is initially created with
  `0600`. Check whether this meets your file permission requirements.

## RBAC

| Service account | Resources | Verbs |
| --- | --- | --- |
| repo-server | `configmaps`, `secrets` | `get` |
| application-controller | `configmaps`, `secrets` | `get`, `list`, `watch` |
| application-controller | `applications.argoproj.io` | `get`, `list`, `watch`, `patch` |

- `rbac.create` only takes effect together with `withArgoCD=true` and an existing
  Argo CD subchart.
- With `withArgoCD=false`, you must create the RBAC resources manually.
- The repo-server ClusterRole grants cluster-wide `get` on ConfigMaps and Secrets.
  This is necessary for references in arbitrary namespaces, but it violates the
  principle of least privilege. Restrict the permissions if your setup allows it.

Manual RBAC examples are in
[Getting Started – RBAC](getting-started.md#rbac).

## Compatibility and Upgrade

- The `copy-cmp-server` image in the repo-server init container must match the
  Argo CD / repo-server version in use. Use the same image tag as the repo-server.
- Make sure `Chart.yaml`, `Chart.lock`, and the bundled chart dependency are
  consistent. The chart includes an Argo CD dependency; the versions must match.
- The `argo-values` image must include Helm. `generate` uses `--dependency-update`;
  missing dependencies are fetched at runtime. Make sure the repo-server can reach
  the required Helm repositories.
- When upgrading Argo CD, also update the `copy-cmp-server` image and verify that
  `plugin.yaml` and the parameter declaration still match.
- With multiple CMPs, use unique `metadata.name` values in `plugin.yaml`. Set
  `spec.source.plugin.name` on the Application to
  `<metadata.name>-<spec.version>` when a version is set (for the bundled manifest,
  `argo-values-v1.0`); without `spec.version`, use `<metadata.name>`
  (`argo-values`).
- There are no CRDs, database migrations, or persistent plugin state to consider
  during upgrades.

## Troubleshooting

### Application is not rendered by the plugin

**Symptom:** Argo CD uses a different plugin or does not render the Application
with `argo-values`.

**Check:**

- Does at least one of the variables `ARGOCD_ENV_VALUE_CONFIGS`,
  `ARGOCD_ENV_ENV_CONFIGS`, `ARGOCD_ENV_VALUE_SECRETS`, or
  `ARGOCD_ENV_ENV_SECRETS` exist? An empty value also counts.
- Or does `ARGOCD_APP_PARAMETERS` contain a recognized, non-empty parameter
  (`valueConfigs`, `envConfigs`, `valueSecrets`, `envSecrets`, `helmValues`)?
- Are the parameters declared under `spec.parameters.static` in `plugin.yaml`?
- Is `plugin.name` set to the effective plugin name when multiple CMPs are present?

`discover` exits with code `2` when the Application is not supported. Broken
`ARGOCD_APP_PARAMETERS` exits with code `1`.

### Discovery token or manifests are corrupted

**Cause:** `LOG_OUTPUT=stdout`.

**Fix:** Set `LOG_OUTPUT=stderr` (the default) and restart the pod.

### init fails

**Symptoms:** The Application remains in an error state; logs contain messages such
as `unable to get ConfigMap` or `invalid resource reference`.

**Causes:**

- The ConfigMap/Secret does not exist in the expected namespace.
- The reference is invalid (`/name`, `namespace/`, `a/b/c`).
- `ARGOCD_APP_PARAMETERS` is not valid JSON.
- `.helmignore` contains an invalid pattern (for example `**` or `[`).
- The Kubernetes client cannot connect or lacks RBAC permissions.

`init` is fail-fast: a missing resource or invalid reference terminates the run
with exit `1`. Check the logs of the `argo-values` container in the repo-server.

### Unqualified reference resolves to the wrong namespace

**Cause:** Unqualified references are resolved against the destination namespace,
not against the namespace of the Application resource.

**Fix:** Write references as `namespace/name`. Note that `init` uses
`ARGOCD_APP_NAMESPACE` and `watch` uses `spec.destination.namespace`.

### watch does not refresh

**Check:**

- Is the `watch` sidecar running in the application-controller?
- Does `ARGOCD_VALUES_NAMESPACES` contain both the Application namespace and the
  namespace of the ConfigMap/Secret?
- Are the references valid? `watch` only skips invalid references with a warning.
- Does `plugin.parameters` use the `array` field (not `value`)?
- Is `--dry-run` active? Then no patches are sent.
- Does the application-controller have `get`, `list`, `watch` on
  ConfigMaps/Secrets and `get`, `list`, `watch`, `patch` on Applications?
- Has the Application just been created? New Applications in the same batch are
  not refreshed yet because of initial seeding.

### Helm error in generate

**Causes:**

- Chart dependencies could not be downloaded (`--dependency-update`).
- The Helm repository is not reachable from the repo-server.
- `--kube-version`/`--api-versions` do not match the chart.
- The `argo-values` image does not include Helm.

Check the `generate` logs and the Helm output. `generate` writes Helm manifests to
stdout; errors appear on stderr and cause exit `1`.

### Invalid environment values

An invalid value such as `ARGOCD_VALUES_REFRESH_SECONDS=abc` causes **exit 1**,
not exit `80`. Exit `80` is reserved exclusively for invalid CLI usage. See
[Exit Codes](reference.md#exit-codes).

### Files are changed unexpectedly

`init` replaces `${VAR}` in all regular, non-ignored files in the chart directory
– including `.helmignore` and the generated values file. Changed files are written
with `0644`. If you do not want this, exclude files via `.helmignore` or avoid
placeholders in them.

### Debugging Tips

```bash
# Check discovery manually in the repo-server container
kubectl -n argocd exec deploy/argocd-repo-server -c argo-values -- \
  sh -c 'ARGOCD_ENV_VALUE_CONFIGS=test argo-values discover'

# Run init with dry-run and debug logs
kubectl -n argocd exec deploy/argocd-repo-server -c argo-values -- \
  argo-values init --dry-run --log-level debug --file-target /tmp
```

These commands must run in the repo-server container context with the correct
mounts and paths. The exact paths and mounts may differ depending on the
installation. Check the pod spec if in doubt.
