# argo-values – Documentation

`argo-values` is a [Config Management Plugin (CMP)](https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/)
for Argo CD. It renders Helm charts with additional values from Kubernetes
ConfigMaps and Secrets without duplicating those values into Git. It also replaces
`${VAR}` placeholders in chart files and can automatically refresh Argo CD
Applications when a referenced ConfigMap or Secret changes.

This documentation is intended for platform teams and users who

- operate Argo CD and install `argo-values` as a CMP,
- configure Applications with `argo-values`,
- combine values from ConfigMaps/Secrets and `helmValues`,
- use automatic refresh through the optional `watch` sidecar,
- want to understand operations, security, and troubleshooting.

> **In short:** `argo-values` is designed for **non-sensitive configuration**.
> Secrets are technically supported, but they are not the primary use case. See
> [Scope and Security](#scope-and-security) for details.

## Quick Start (TL;DR)

### 1. Integrate into an existing Argo CD installation

Create the plugin ConfigMap and add the repo-server/controller sidecar snippets from
[Getting Started](getting-started.md#installation). Make sure the
`copy-cmp-server` image matches the Argo CD / repo-server version in use.

### 2. Create value sources

Create at least one ConfigMap with additional Helm values (`values.yaml`) and/or
environment variables. Example:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-values
  namespace: default
data:
  values.yaml: |
    resources:
      requests:
        cpu: "${CPU}"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-variables
  namespace: default
data:
  cpu: "123m"
```

Value sources supply YAML mappings; values that are not mappings are stored as raw
text under the respective key. Env sources supply literal values for
`${VAR}` substitution.

### 3. Configure the Application

Enable the plugin through `spec.source.plugin.env`:

```yaml
spec:
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: helm-guestbook
    plugin:
      env:
        - name: value-configs
          value: default/application-values
        - name: env-configs
          value: default/application-variables
```

Alternatively, you can use `plugin.parameters` with `array`/`map`. This requires
the parameters to be declared under `spec.parameters.static` in `plugin.yaml` (the
full `plugin.yaml` in Getting Started contains this declaration; a minimal snippet
does not). See
[Configuration](configuration.md) and [Reference](reference.md#naming-mapping).

### 4. Sync

Argo CD automatically calls the following CMP phases in the **repo-server** during
rendering:

1. `discover` – determines whether `argo-values` is responsible for the Application.
2. `init` – reads ConfigMaps/Secrets, merges values, writes the values file, and
   replaces `${VAR}` placeholders.
3. `generate` – runs `helm template` and emits the manifests.

You do not need to invoke `argo-values` manually; Argo CD runs these phases during
rendering (depending on your sync policy).

### 5. Optional: enable automatic refresh

The `watch` sidecar does **not** run in the repo-server; it runs in the
**application-controller**. It watches ConfigMaps, Secrets, and Applications and
triggers a hard refresh for affected Applications. Details:
[Getting Started – watch sidecar](getting-started.md#optional-watch-sidecar-in-the-application-controller).

## Key Pitfalls

1. **Namespace resolution:** Unqualified references are **not** resolved in the
   `default` namespace or in the Application namespace. `init` uses
   `ARGOCD_APP_NAMESPACE` (the Application's destination namespace, fallback
   `default`), and `watch` uses `spec.destination.namespace` (fallback `default`).
   Qualify references as `namespace/name` whenever in doubt.
2. **Do not mix parameter forms for the same source list:** `plugin.env` expects
   comma-separated strings, while `plugin.parameters` expects arrays or maps. CLI
   flags are repeatable but do not split commas.
3. **`plugin.parameters` requires `spec.parameters.static`:** Without the
   declaration in `plugin.yaml`, the parameters do not appear in
   `ARGOCD_APP_PARAMETERS`. `helmValues` is available only through
   `plugin.parameters`.
4. **Never set `LOG_OUTPUT=stdout` in the CMP:** Logs on stdout corrupt the
   discovery token and the rendered Helm manifests.
5. **`watch` needs the correct namespaces:** `ARGOCD_VALUES_NAMESPACES` must
   contain both the Application namespaces and the namespaces of all referenced
   ConfigMaps/Secrets. Without `watch`, there is no automatic refresh.
6. **Missing resources make `init` fail:** `discover` checks neither the existence
   nor the validity of references. A missing ConfigMap/Secret is only detected in
   `init` (fail-fast, exit 1).

## Documentation Map

| Page | Contents |
| --- | --- |
| [Getting Started](getting-started.md) | Prerequisites, installation, full `plugin.yaml`, sidecars, RBAC, verification, first Application |
| [How It Works](how-it-works.md) | Architecture, `discover`/`init`/`generate` lifecycle, value resolution, `watch`, discovery decision |
| [Configuration](configuration.md) | Application configuration, value sources, reference formats, precedence, merge, env substitution, files/paths, limitations |
| [Operations](operations.md) | `watch`/refresh, logging, security, RBAC, compatibility/upgrade, [Troubleshooting](operations.md#troubleshooting) |
| [Reference](reference.md) | CLI commands/flags, environment variables, exit codes, file conventions, `plugin.yaml`, naming mapping |
| [Examples](examples.md) | ConfigMap values, Secret values, multiple sources, `helmValues`, ApplicationSet |
| [Comparison](comparison.md) | Flux, native Argo CD options, upstream discussions, related plugins |

Back to the [project README](../README.md).

## Render Path vs. Refresh Path

`argo-values` has two separate paths. They run in different pods and are triggered
differently:

| | Render path | Refresh path |
| --- | --- | --- |
| Location | repo-server (CMP sidecar) | application-controller (`watch` sidecar) |
| Trigger | Sync, manual refresh, Argo CD reconciliation | Change to a ConfigMap/Secret/Application |
| Commands | `discover` → `init` → `generate` | `watch` |
| Result | Rendered manifests on stdout | Merge patch with `argocd.argoproj.io/refresh: hard` |
| State | Files in the chart directory (`app-values.yaml`) | In-memory mapping of Application → resources |

The render path is the actual CMP part. The refresh path is optional and ensures
that Argo CD starts a new render operation after a change to an external value
source.

## Scope and Security

- **Primary use case:** non-sensitive values from ConfigMaps.
- **Secrets:** technically supported (`value-secrets`/`env-secrets`), but Argo CD
  caches rendered manifests in plaintext in Redis and exposes them through the
  repo-server API. In addition, the generated `app-values.yaml` is stored on the
  repo-server pod's disk. For sensitive values, External Secrets Operator, Vault
  Secrets Operator, or Sealed Secrets are better choices; reference the resulting
  Kubernetes Secret from the chart instead of rendering its value.
- **No secrets in `plugin.env` or `helmValues`:** These values are part of the
  Application resource and are visible to authorized users there.
- **No write-back to Git:** The rendered manifests cannot be reproduced from Git
  alone. This is intentional and incompatible with the "Rendered Manifests"
  pattern.
- **Logging:** Defaults are `LOG_LEVEL=debug`, `LOG_FORMAT=json`, and
  `LOG_OUTPUT=stderr`. Debug logs do not contain secret values, but they do contain
  resource names and keys. `LOG_LEVEL=info` is recommended for production.
  `LOG_OUTPUT=stdout` is off-limits in the CMP.

For a comparison with Flux, native Argo CD options, and related plugins, see
[Comparison](comparison.md).

For details on security and RBAC, see [Operations](operations.md#security) and
[Getting Started – RBAC](getting-started.md#rbac).
