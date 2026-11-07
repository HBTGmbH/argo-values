# Argo Values - Argo CD CMP Plugin

[![License](https://img.shields.io/github/license/HBTGmbH/argo-values)](https://github.com/HBTGmbH/argo-values/blob/main/LICENSE)

Argo Values is a Config Management Plugin (CMP) for Argo CD that enhances Helm-based deployments by integrating values 
from Kubernetes ConfigMaps and Secrets.

## Features

- **ConfigMap-driven Values**: Merge values from ConfigMaps (and optionally Secrets) into Helm template application files
- **Environment Variable Substitution**: Replace variable placeholders in application files with actual values
- **Automatic Refresh**: Watch for ConfigMap/Secret changes and trigger application refreshes
- **Helm Integration**: Full compatibility with Helm charts and templating
- **Selective Processing**: Respect `.helmignore` patterns

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

To run argo-values as sidecar with the repo server, add the following snippet to your `values.yaml`:
```yaml
repoServer:
  initContainers:
    - name: copy-cmp-server
      image: quay.io/argoproj/argocd:v3.3.2
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

All values from `env-configs` or `value-secrets` are available as environment variables and substituted in the Helm 
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

> **Note:** Use ConfigMaps for non-sensitive configuration. For secrets, see [Scope and Positioning](#scope-and-positioning) 
> above and prefer a destination-cluster solution such as External Secrets Operator.

## License

MIT License, Copyright © 2026 HBT GmbH. See [LICENSE](LICENSE).

## Contributing

Contributions are welcome! Please open issues and pull requests on GitHub.