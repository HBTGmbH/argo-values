# Examples

All examples use dummy values. Do not use secrets in `plugin.env` or `helmValues`
in real applications.

## ConfigMap Values and Env Substitution

Two ConfigMaps: one with Helm values and one with env variables.

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
        memory: "100Mi"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-variables
  namespace: default
data:
  cpu: "123m"
```

The Application references both:

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
          value: default/application-values
        - name: env-configs
          value: default/application-variables
  destination:
    server: https://kubernetes.default.svc
    namespace: default
```

Result:

- `init` merges the mapping from `values.yaml` into the values file.
- The env ConfigMap supplies `CPU=123m`.
- `${CPU}` is replaced with `123m` in `values.yaml` and in all other non-ignored
  chart files.
- `generate` renders the chart with the generated `app-values.yaml`.

## Secret Values

Secrets work the same way. For readability, `stringData` is used here; Kubernetes
converts it to `data`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: application-values-secret
  namespace: default
type: Opaque
stringData:
  values.yaml: |
    image:
      tag: "1.2.3"
---
apiVersion: v1
kind: Secret
metadata:
  name: application-env-secret
  namespace: default
type: Opaque
stringData:
  API_TOKEN: "dummy-token"
```

```yaml
spec:
  source:
    plugin:
      env:
        - name: value-secrets
          value: default/application-values-secret
        - name: env-secrets
          value: default/application-env-secret
```

> **Security note:** Secrets are supported, but Argo CD caches rendered manifests
> in plaintext and the generated `app-values.yaml` is stored on disk. For real
> secrets, a destination-cluster approach (External Secrets Operator, Vault Secrets
> Operator, Sealed Secrets) is better. See
> [Operations – Security](operations.md#security).

## Multiple Sources and Precedence

### Order Within a Source List

`valueConfigs` are processed in list order, followed by `valueSecrets`. Later
sources override earlier ones.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: base-values
  namespace: default
data:
  values.yaml: |
    replicaCount: 1
    resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
    tags:
      - base
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: override-values
  namespace: default
data:
  values.yaml: |
    replicaCount: 2
    resources:
      requests:
        cpu: "250m"
    tags:
      - override
```

```yaml
spec:
  source:
    plugin:
      env:
        - name: value-configs
          value: default/base-values,default/override-values
```

Result:

- `replicaCount` is `2`.
- `resources.requests.cpu` is `250m`.
- `resources.requests.memory` remains `64Mi` (deep merge for maps).
- `tags` is `[override]` (lists are replaced, not merged).

### Precedence Between Mechanisms

For the same source list, the order is: **CLI flags > `plugin.parameters` >
`plugin.env`**.

```yaml
spec:
  source:
    plugin:
      env:
        - name: value-configs
          value: default/base-values
      parameters:
        - name: valueConfigs
          array:
            - default/override-values
```

Here `plugin.parameters` wins: only `default/override-values` is used, and
`default/base-values` is not read. This also applies when the parameter array is
empty.

During a manual `init` run, non-empty CLI flags would additionally override both
mechanisms:

```bash
argo-values init \
  --value-maps default/cli-values \
  --file-target ./chart
```

### helmValues Overrides ConfigMap/Secret Values

```yaml
spec:
  source:
    plugin:
      parameters:
        - name: helmValues
          map:
            replicaCount: "3"
            image.tag: "1.2.3"
            env[0].name: "LOG_LEVEL"
            env[0].value: "info"
            feature.enabled: "true"
```

- Dotted paths create nested maps.
- `env[0].name` creates a list with one map entry.
- Scalars are coerced: `"3"` becomes `int`, `"true"` becomes `bool`.
- `helmValues` is merged after ConfigMaps/Secrets and overrides the same paths.

> `helmValues` requires `spec.parameters.static` in `plugin.yaml`. See
> [Reference – plugin.yaml](reference.md#plugin-yaml-reference).

## Value Key Is Arbitrary

For value sources, the data key of the ConfigMap/Secret is irrelevant as long as
the value is a valid YAML mapping:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-values
  namespace: default
data:
  anything.yaml: |
    replicaCount: 2
```

The mapping is merged directly. If the value is not a mapping (for example, a
scalar or a top-level list), the raw text is stored under the key. Avoid
multi-document YAML; only the first document is evaluated.

## ApplicationSet and the examples/helm/app-sets Example

The `examples/helm/app-sets` directory contains a Helm chart that creates Argo CD
Applications (or optionally an ApplicationSet). It shows how to combine
`helmValues` as a map with the source lists as comma-separated strings.

Structure:

```text
examples/helm/app-sets/
├── Chart.yaml
├── root.yaml
├── values.yaml
├── values/
│   ├── values.yaml
│   ├── set1/
│   │   ├── values.yaml
│   │   ├── app1/values.yaml
│   │   └── app2/values.yaml
│   └── set2/
│       ├── values.yaml
│       ├── app3/values.yaml
│       └── app4/values.yaml
└── templates/
    ├── additional-config.yaml
    ├── application-list.yaml
    ├── application-set.yaml
    └── _helpers.tpl
```

- `root.yaml` is the bootstrap Application that renders the chart.
- `values.yaml` controls `withApplicationSet` and the app definitions, among other
  things.
- `templates/application-list.yaml` creates individual Applications when
  `withApplicationSet: false` (the default).
- `templates/application-set.yaml` creates an ApplicationSet when
  `withApplicationSet: true`.
- `templates/additional-config.yaml` creates a ConfigMap named `test`; `app1`
  references it via `valueConfigs: [argocd/test]`.
- The values files are combined from three levels: `values/`, `values/<set>/`, and
  `values/<set>/<app>/`. The exact override order follows from the `merge` calls in
  the templates; do not rely on a direction without checking it in the respective
  template.

A simplified example of the generated Application configuration:

```yaml
spec:
  source:
    plugin:
      parameters:
        - name: helmValues
          map:
            valueFromBase: "base"
            valueFromSet: "set1"
            valueFromApp: "app1"
      env:
        - name: value-configs
          value: "argocd/test"
        # Empty strings are allowed: they enable discovery but provide no sources.
        - name: env-configs
          value: ""
        - name: value-secrets
          value: ""
        - name: env-secrets
          value: ""
```

Important for ApplicationSets:

- `plugin.env` expects comma-separated strings. The templates create them, for
  example, with `join ","`.
- `plugin.parameters` expects arrays/maps. The example uses a map for `helmValues`
  and a `templatePatch` in the ApplicationSet to set it.
- Empty strings in `plugin.env` are allowed; they enable discovery but provide no
  sources.
