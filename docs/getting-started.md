# Getting Started

This page walks you from installation to your first Application rendered with
`argo-values`.

## Prerequisites

- A running Kubernetes cluster with Argo CD (or the willingness to install Argo CD
  through the chart).
- Helm 3 to install the chart.
- Access to the container registry that hosts the `argo-values` image and the
  Argo CD image.
- A Git repository containing a Helm chart that Argo CD should render.
- Helm must be available in the `argo-values` image. The official image includes
  Helm. `generate` uses `--dependency-update`; missing chart dependencies are
  fetched from the network. Make sure the repo-server pod can reach the required
  Helm repositories.

> **Versions:** `copy-cmp-server` must match the Argo CD / repo-server version in
> use. Use the same image tag for this init container as for the repo-server. The
> chart bundles an Argo CD dependency; make sure `Chart.yaml`, `Chart.lock`, and
> the bundled dependency are consistent.

## Installation Options

### Option A: Chart with Argo CD as a Subchart

The chart installs Argo CD as a subchart when `withArgoCD=true` is set (the
default). RBAC resources are also created when `rbac.create=true` (the default)
applies.

```bash
helm upgrade --install argo-values ./charts/argo-values \
  --namespace argocd --create-namespace \
  --set withArgoCD=true
```

You can enable optional demo resources (ConfigMaps and an example Application)
with:

```bash
helm upgrade --install argo-values ./charts/argo-values \
  --namespace argocd --create-namespace \
  --set withArgoCD=true \
  --set withHelmExample=true
```

`withHelmExample=true` creates the ConfigMaps `test-env` and `test-values` in the
`default` namespace and the Application `nginx-example` in the `argocd` namespace.

### Option B: Existing Argo CD Installation

For an existing installation, set `withArgoCD=false`:

```bash
helm upgrade --install argo-values ./charts/argo-values \
  --namespace argocd \
  --set withArgoCD=false
```

This creates only the plugin-specific resources (the ConfigMap; RBAC is created
only together with `withArgoCD=true`), but not Argo CD itself. Install the chart
in the namespace where the repo-server runs so that the ConfigMap mount works.

Alternatively, you can add the following snippets directly to the `values.yaml`
of your Argo CD Helm installation. The plugin ConfigMap must exist in the Argo CD
namespace for this (for example via `extraObjects` or by applying it separately).

## Full plugin.yaml

The chart creates a ConfigMap named `argocd-cmp-plugin-config` with the following
content. The declaration under `spec.parameters.static` is required so that Argo CD
passes the parameters from `plugin.parameters` to the plugin:

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

`metadata.name` is the base plugin name (`argo-values`). Because this manifest sets
`spec.version: v1.0`, the effective plugin name is **`argo-values-v1.0`**. If you
omit `spec.version`, the effective name is just `argo-values`. If you run multiple
CMPs, keep the names unique and, if necessary, set `spec.source.plugin.name` on the
Application to the effective name so that Argo CD selects the correct plugin.

## repo-server Sidecar

The CMP part runs as a sidecar in the repo-server. The init container copies
`argocd-cmp-server` from the Argo CD image into a shared volume; the
`argo-values` container starts this CMP server, which in turn invokes
`argo-values`.

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

Notes:

- The Argo CD chart provides the `var-files` and `plugins` volumes for the
  repo-server. If you use a custom installation without these volumes, you must
  create them yourself.
- The container must run as UID `999` so that the CMP server can use the expected
  directories.
- `cmp-tmp` provides a writable `/tmp`.
- The ConfigMap mount uses `subPath: plugin.yaml` so that the file appears directly
  as `/home/argocd/cmp-server/config/plugin.yaml`.

## Optional watch Sidecar in the application-controller

`watch` is **not a CMP component** and does not run in the repo-server. For
automatic refresh, add an additional sidecar to the application-controller:

```yaml
controller:
  extraContainers:
    - name: argo-values
      image: ghcr.io/hbtgmbh/argo-values:latest
      command: [argo-values]
      args: [watch]
      env:
        - name: ARGOCD_VALUES_REFRESH_SECONDS
          value: "2"
        - name: ARGOCD_VALUES_NAMESPACES
          value: "argocd,default"
        - name: LOG_OUTPUT
          value: "stderr"
        - name: LOG_LEVEL
          value: "info"
```

- `ARGOCD_VALUES_NAMESPACES` must contain the namespaces of the Applications
  **and** of all referenced ConfigMaps/Secrets (default `argocd,default`).
- `ARGOCD_VALUES_REFRESH_SECONDS` is a batch/flush interval in seconds (default
  `2`, must be `> 0`). It is not a reset-based debounce.
- `LOG_OUTPUT=stderr` is mandatory. `stdout` is reserved for command output.
- The sidecar requires additional RBAC permissions (see below).

## RBAC

`init` runs in the repo-server and must be able to read referenced ConfigMaps and
Secrets. `watch` runs in the application-controller and must additionally be able
to watch and patch Applications.

| Service account | Resources | Verbs |
| --- | --- | --- |
| repo-server | `configmaps`, `secrets` | `get` |
| application-controller | `configmaps`, `secrets` | `get`, `list`, `watch` |
| application-controller | `applications.argoproj.io` | `get`, `list`, `watch`, `patch` |

With `withArgoCD=true` and `rbac.create=true`, the chart creates suitable
ClusterRoles and ClusterRoleBindings. `rbac.create=false` disables this. With
`withArgoCD=false`, you must create the permissions yourself.

Example manual RBAC resources (replace the placeholders):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: argo-values-render
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: argo-values-render
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: argo-values-render
subjects:
  - kind: ServiceAccount
    name: <repo-server-service-account>
    namespace: <argocd-namespace>
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: argo-values-watch
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["argoproj.io"]
    resources: ["applications"]
    verbs: ["get", "list", "watch", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: argo-values-watch
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: argo-values-watch
subjects:
  - kind: ServiceAccount
    name: <application-controller-service-account>
    namespace: <argocd-namespace>
```

> **Least privilege:** The repo-server ClusterRole grants cluster-wide `get` on
> ConfigMaps and Secrets. `init` needs this to read referenced objects from
> arbitrary namespaces. If your setup allows it, restrict these permissions to
> specific namespaces.

## Verification

After installation, check the following:

```bash
# Is the plugin ConfigMap present?
kubectl -n argocd get configmap argocd-cmp-plugin-config

# Are the repo-server and controller running with their sidecars?
kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-repo-server
kubectl -n argocd get pods -l app.kubernetes.io/name=argocd-application-controller

# Logs from the CMP sidecar
kubectl -n argocd logs -l app.kubernetes.io/name=argocd-repo-server -c argo-values --tail=50

# Logs from the watch sidecar (if enabled)
kubectl -n argocd logs -l app.kubernetes.io/name=argocd-application-controller -c argo-values --tail=50
```

An Application counts as successfully rendered when it is synced in Argo CD and
shows the expected resources. If problems occur, see
[Troubleshooting](operations.md#troubleshooting).

## First Example Application

Create two ConfigMaps:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-values
  namespace: default
data:
  values.yaml: |
    replicaCount: 2
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

And an Application that references both sources:

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
      name: argo-values-v1.0   # only needed with multiple CMPs; name = <metadata.name>-<spec.version>
      env:
        - name: value-configs
          value: default/application-values
        - name: env-configs
          value: default/application-variables
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

Flow during sync:

1. `discover` recognizes `value-configs`/`env-configs` and emits the token
   `argo-values`.
2. `init` reads the ConfigMaps, merges `values.yaml`, writes `app-values.yaml`,
   and replaces `${CPU}` in the chart files.
3. `generate` runs `helm template` with `-f app-values.yaml`.

More examples: [Examples](examples.md).
