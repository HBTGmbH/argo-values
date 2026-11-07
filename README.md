# Argo Values - Argo CD CMP Plugin

[![License](https://img.shields.io/github/license/HBTGmbH/argo-values)](https://github.com/HBTGmbH/argo-values/blob/main/LICENSE)

Argo Values is a Config Management Plugin (CMP) for Argo CD that enhances Helm-based deployments by integrating values 
from Kubernetes ConfigMaps and Secrets.

## Features

- **ConfigMap-driven Values**: Merge values from ConfigMaps (and optionally Secrets) into Helm template application files
- **Environment Variable Substitution**: Replace variable placeholders in application files with actual values
- **Automatic Refresh**: Watch for ConfigMap/Secret changes and trigger application refreshes
- **Helm Integration**: Full compatibility with Helm charts and templating
- **Selective Processing**: Respect `.helmignore` patterns using Helm-compatible semantics (including path and directory rules)

## TL;DR – CMP in 5 Minutes

1. **Install:** Install the chart with `withArgoCD=true` (the default) or adopt the sidecar snippets for an existing Argo CD installation from [Getting Started](docs/getting-started.md#installation-options). Make sure the `copy-cmp-server` image matches the Argo CD / repo-server version.
2. **Create value sources:** Create ConfigMap(s) with additional Helm values (`values.yaml`) and/or environment variables; Secrets are an alternative.
3. **Configure the Application:** In `spec.source.plugin.env`, set the sources as comma-separated strings (`value-configs`, `env-configs`, `value-secrets`, `env-secrets`), or use `plugin.parameters` with `array`/`map`. `plugin.parameters`/`helmValues` require `spec.parameters.static` in `plugin.yaml`.
4. **Sync:** Argo CD calls `discover` → `init` → `generate` in the repo-server. `init` writes `app-values.yaml` and replaces `${VAR}` in the chart files; `generate` runs `helm template`.
5. **Optional refresh:** Enable the `watch` sidecar in the application-controller so that changes to referenced ConfigMaps/Secrets trigger a hard refresh.

**Key Pitfalls**

- Unqualified references are resolved against the Application's **destination namespace** (`ARGOCD_APP_NAMESPACE` in `init`, `spec.destination.namespace` in `watch`, fallback `default`) – not against the Application resource namespace. Always use `namespace/name` when in doubt.
- `plugin.env` uses comma-separated strings; `plugin.parameters` uses arrays/maps. Do not mix them for the same source list. CLI flags are repeatable but do not split commas.
- `plugin.parameters` and `helmValues` require `spec.parameters.static` in `plugin.yaml`.
- Never set `LOG_OUTPUT=stdout` in the CMP – it corrupts the discovery token and the Helm manifests.
- `watch` requires the namespaces of the Applications **and** of all referenced ConfigMaps/Secrets in `ARGOCD_VALUES_NAMESPACES`. Without `watch`, there is no automatic refresh.
- Missing ConfigMaps/Secrets make `init` fail (exit 1); `discover` does not check their existence.

More details: [Getting Started](docs/getting-started.md), [Configuration](docs/configuration.md), [Operations](docs/operations.md).

## Documentation

The full user documentation lives in [`docs/`](docs/README.md):

- [Getting Started](docs/getting-started.md) – installation, sidecars, RBAC, first Application
- [How It Works](docs/how-it-works.md) – architecture and lifecycle
- [Configuration](docs/configuration.md) – sources, precedence, merge, substitution
- [Operations](docs/operations.md) – watch/refresh, logging, security, troubleshooting
- [Reference](docs/reference.md) – CLI, env vars, exit codes, `plugin.yaml`
- [Examples](docs/examples.md) – ConfigMaps, Secrets, `helmValues`, ApplicationSet
- [Comparison](docs/comparison.md) – Flux, native Argo CD options, and related plugins

## CLI

`argo-values` provides the subcommands `init`, `generate`, `discover`, and `watch`.
Run `argo-values --help` for an overview and `argo-values <command> --help` for the
flags of a specific command. The command help also shows the environment variable
that can be used instead of each flag. `argo-values --version` prints the build
version (for local builds without ldflags: `dev`).

- The `init` list flags (`--value-maps`, `--env-maps`, `--value-secrets`, `--env-secrets`)
  are repeatable. Each occurrence is added as-is, so comma-separated values are not split.
- `watch --namespaces` is repeatable and also accepts comma-separated values
  (for example `--namespaces argocd,default`).
- `watch --refresh-interval` and `ARGOCD_VALUES_REFRESH_SECONDS` are specified in
  whole seconds (for example `--refresh-interval 3`).
- Invalid CLI usage (unknown flags or commands, missing flag values) exits with code `80`
  and writes the error to stderr.
- Stdout is reserved for command output such as the discovery token (`discover`) and the
  rendered Helm manifests (`generate`); logs go to stderr by default.

## Scope and Positioning

argo-values is built around **non-sensitive configuration** from ConfigMaps. Its primary purpose is to let Helm charts
consume values from ConfigMaps (and to substitute `${VAR}` placeholders) at render time, without duplicating those values into Git.

This fills a gap in Argo CD: Argo CD does not watch external ConfigMaps and has no native way to use their contents as
Helm values. The `watch` command closes the refresh gap by triggering a targeted application refresh only for the applications
that reference a changed ConfigMap.

Using ConfigMaps as a value source at render time is a deliberate trade-off, not an antipattern. Note that the rendered 
manifests then depend on a second source of truth besides Git, which makes them less reproducible from Git alone and is 
incompatible with the "Rendered Manifests" pattern. If you need fully reproducible manifests, keep values in Git
(`valuesObject`, `valueFiles`, multiple sources) instead.

### Secrets

Secrets are supported (`value-secrets` / `env-secrets`), but they are **not the primary use case**. Argo CD explicitly 
cautions against injecting secrets during manifest generation, because generated manifests (including injected secrets)
are cached in plaintext in Redis and exposed via the repo-server API. See the [Argo CD Secret Management](https://argo-cd.readthedocs.io/en/stable/operator-manual/secret-management/) documentation.

For sensitive values, prefer destination-cluster solutions such as External Secrets Operator, Vault Secrets Operator, 
or Sealed Secrets, and reference the resulting Kubernetes Secret from the chart instead of rendering its value.

## Installation with Argo CD Helm chart

### Configure argo-values CMP

Add the following snippet to your `values.yaml` to configure argo-values as CMP:
```yaml
extraObjects:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: argocd-cmp-plugin-config
      namespace: argocd               
    data:
      plugin.yaml: |
        apiVersion: argoproj.io/v1alpha1
        kind: ConfigManagementPlugin
        metadata:
          name: argo-values
        spec:
          version: v1.0
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

The `generate` command passes `KUBE_VERSION` (normalized to a plain version such as
`1.29.0`) and the comma-separated `KUBE_API_VERSIONS` to Helm as `--kube-version` and
`--api-versions`. Missing chart dependencies are fetched automatically via
`--dependency-update`.

### Add argo-values to Argo CD application controller (optional)

To trigger application refresh automatically every time a ConfigMap or Secret changed, add the following snippet to your
`values.yaml`:
```yaml
controller:
  extraContainers:
    - name: argo-values
      image: ghcr.io/hbtgmbh/argo-values:latest
      command: [argo-values]
      args: [watch]
      env:
        - name: ARGOCD_VALUES_REFRESH_SECONDS
          value: "3"
        - name: ARGOCD_VALUES_NAMESPACES
          value: "argocd,default"
        - name: LOG_OUTPUT
          value: "stderr"
```

Logs are written to stderr by default (`LOG_OUTPUT=stderr`). Stdout is reserved for the
actual command output, such as the discovery token from `discover` and the rendered Helm
manifests from `generate`. Set `LOG_OUTPUT=stdout` only if you explicitly want logs on stdout.

### Add argo-values to ArgoCD repo server

To run argo-values as sidecar with the repo server, add the following snippet to your `values.yaml`.
Make sure the `copy-cmp-server` image matches your Argo CD / repo-server version:
```yaml
repoServer:
  initContainers:
    - name: copy-cmp-server
      image: "quay.io/argoproj/argocd:<same-tag-as-your-repo-server>"
      command: [cp, /usr/local/bin/argocd-cmp-server, /custom-tools/]
      volumeMounts:
        - name: custom-tools
          mountPath: /custom-tools
  extraContainers:
    - name: argo-values
      image: ghcr.io/hbtgmbh/argo-values:latest
      command: [/custom-tools/argocd-cmp-server]
      imagePullPolicy: Always
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
      volumeMounts:
        - name: var-files
          mountPath: /var/run/argocd
        - name: plugins
          mountPath: /home/argocd/cmp-server/plugins
        - name: cmp-tmp
          mountPath: /tmp
        - name: cmp-plugin-config
          mountPath: /home/argocd/cmp-server/config/plugin.yaml
          subPath: plugin.yaml
        - name: custom-tools
          mountPath: /custom-tools
  volumes:
    - name: cmp-plugin-config
      configMap:
        name: argocd-cmp-plugin-config
    - name: cmp-tmp
      emptyDir: {}
    - name: custom-tools
      emptyDir: {}
```

## RBAC

`argo-values init` runs in the repo-server pod and must be able to read the referenced ConfigMaps and Secrets.
`argo-values watch` runs in the application-controller pod and additionally needs access to Argo CD Applications.

Required permissions:

| Service account | Resources | Verbs |
| --- | --- | --- |
| repo-server | `configmaps`, `secrets` | `get` |
| application-controller | `configmaps`, `secrets` | `get`, `list`, `watch` |
| application-controller | `applications.argoproj.io` | `get`, `list`, `watch`, `patch` |

With `withArgoCD: true` the chart creates the required ClusterRoles and ClusterRoleBindings automatically.
Set `rbac.create: false` to disable this, for example when you manage RBAC yourself. When you integrate the
plugin manually into an existing Argo CD installation (`withArgoCD: false`), you have to create these permissions yourself.

> **Security note:** The ClusterRole for the repo-server service account grants cluster-wide `get` on ConfigMaps and
> Secrets. `argo-values init` needs this to read referenced objects from arbitrary namespaces. If your setup allows it,
> restrict these permissions (for example to specific namespaces) to follow the principle of least privilege.

## Examples

ConfigMap and Secret references are written either as `name` or as `namespace/name`. Unqualified
names are resolved in the destination namespace of the Application (`ARGOCD_APP_NAMESPACE` during
`init`, fallback `default`; `watch` uses `spec.destination.namespace`). Empty parts (for example
`/name` or `namespace/`) and more than one `/` (for example `a/b/c`) are rejected.

### Add additional values.yaml from ConfigMap to Helm chart

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: nginx-example
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: helm-guestbook
    plugin:
      env:
        - name: value-configs
          value: default/application-values # <- list of ConfigMaps or namespace/ConfigMap separated by ","
        - name: env-configs
          value: default/application-variables # <- list of ConfigMaps or namespace/ConfigMap separated by ","
```

> **Note:** `plugin.parameters` and `helmValues` require the parameters to be declared under
> `spec.parameters.static` in `plugin.yaml`. The chart's ConfigMap includes this declaration; the
> minimal snippet above does not. See [docs/reference.md](docs/reference.md#plugin-yaml-reference) for
> the full reference.

Every ConfigMap from `value-configs` or Secret from `value-secrets` will be added directly to the Helm templates of the 
application:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-values
  namespace: default
data:  
  # additional values.yaml
  values.yaml: |
    resources:
      requests:
        cpu: "${CPU}"
        memory: "100Mi"
```

All values from `env-configs` or `env-secrets` are available as environment variables and substituted in the Helm 
templates of the application:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-variables
  namespace: default
data:  
  cpu: "123m"
```

> **Note:** For `env-configs` and `env-secrets`, every data key is the name of the environment variable and the value is
> taken literally — it is never interpreted as YAML. In contrast, values from `value-configs`/`value-secrets` are parsed
> as YAML.

> **Note:** Use ConfigMaps for non-sensitive configuration. For secrets, see [Scope and Positioning](#scope-and-positioning) 
> above and prefer a destination-cluster solution such as External Secrets Operator.

## License

MIT License, Copyright © 2026 HBT GmbH. See [LICENSE](LICENSE).

## Contributing

Contributions are welcome! Please open issues and pull requests on GitHub.