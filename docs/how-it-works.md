# How It Works

This page describes the architecture and flows of `argo-values`. Two paths are
important: the **render path** in the repo-server and the optional **refresh path**
in the application-controller.

## Components and Paths

```mermaid
flowchart LR
    subgraph UIAPI["Argo CD UI and API"]
        UI["UI / API<br/>Application resource"]
    end

    subgraph CONTROLLER["application-controller Pod"]
        AC["application-controller"]
        WATCH["watch sidecar<br/>argo-values watch"]
    end

    subgraph REPOSERVER["repo-server Pod"]
        RS["repo-server"]
        CMP["CMP sidecar<br/>argo-values discover / init / generate"]
        HELM["Helm CLI"]
    end

    subgraph CLUSTER["Kubernetes API"]
        CM["ConfigMaps"]
        SEC["Secrets"]
        APP["Applications"]
    end

    GIT["Git repo<br/>Helm chart"]
    OUT["Rendered manifests"]
    REDIS["Redis and repo-server API<br/>Security note: plaintext cache"]

    UI -->|create or sync Application| AC
    AC -->|render request| RS
    RS --> CMP
    GIT -->|chart directory| CMP
    CMP -->|init: get| CM
    CMP -->|init: get| SEC
    CMP --> HELM
    HELM -->|helm template| OUT
    OUT --> AC
    AC -->|apply| CLUSTER

    WATCH -->|list and watch| CM
    WATCH -->|list and watch| SEC
    WATCH -->|list and watch| APP
    WATCH -->|refresh patch| APP

    OUT -.->|cache| REDIS
    AC -.->|cache| REDIS
```

**Reading:** The render path runs exclusively in the repo-server. The `watch`
sidecar observes the Kubernetes API in the application-controller and patches
Applications so that Argo CD starts a new render operation. The two paths share no
process state.

## Roles and Lifecycle

| Phase | Runs in | Purpose |
| --- | --- | --- |
| `discover` | repo-server (CMP sidecar) | decides from the configuration whether `argo-values` is responsible for the Application |
| `init` | repo-server (CMP sidecar) | reads ConfigMaps/Secrets, merges values, writes the values file, and replaces `${VAR}` in chart files |
| `generate` | repo-server (CMP sidecar) | runs `helm template` and emits the manifests on stdout |
| `watch` | application-controller (separate sidecar) | watches resources and triggers refresh patches |

`discover` decides **only from the configuration**, not from the existence of
ConfigMaps or Secrets. `init` is the first step that actually reads resources and
fails on missing or invalid references.

## Sequence: discover → init → generate

```mermaid
sequenceDiagram
    autonumber
    participant Argo as Argo CD repo-server
    participant CMP as CMP sidecar
    participant K8s as Kubernetes API
    participant Helm as Helm CLI

    Argo->>CMP: discover
    alt Configuration present
        CMP-->>Argo: stdout argo-values, exit 0
    else Not configured
        CMP-->>Argo: no stdout, exit 2
    end

    Argo->>CMP: init
    CMP->>K8s: get ConfigMaps and Secrets
    alt Resource missing or reference invalid
        K8s-->>CMP: error
        CMP-->>Argo: exit 1
    else Success
        CMP->>CMP: merge values, write app-values.yaml
        CMP->>CMP: replace placeholders in chart files
        CMP-->>Argo: exit 0
    end

    Argo->>CMP: generate
    CMP->>Helm: helm template ... -f app-values.yaml
    alt Helm succeeded
        Helm-->>Argo: manifests on stdout, exit 0
    else Helm error
        Helm-->>CMP: error
        CMP-->>Argo: exit 1
    end
```

**Error paths:** `discover` exits with code `2` when the Application is not
configured. A broken `ARGOCD_APP_PARAMETERS` already causes exit `1` in `discover`.
`init` and `generate` report runtime and configuration errors with exit `1`. See
the [Reference](reference.md#exit-codes) for an overview.

## Value Resolution: Values

```mermaid
flowchart TD
    A["Source lists<br/>valueConfigs and valueSecrets"] --> B{"CLI flags set?"}
    B -->|yes, not empty| C["Use CLI flags"]
    B -->|no| D{"plugin.parameters<br/>name present?"}
    D -->|yes| E["Use parameter array<br/>even if empty"]
    D -->|no| F["Use ARGOCD_ENV_*"]
    C --> G["Resolve references<br/>name or namespace/name"]
    E --> G
    F --> G
    G --> H["ConfigMaps in list order<br/>then Secrets in list order"]
    H --> I["Data keys per source<br/>sorted lexicographically"]
    I --> J{"Value is a YAML mapping?"}
    J -->|yes| K["Deep-merge mapping<br/>lists are replaced"]
    J -->|no| L["Store raw value under key<br/>warn for structured YAML"]
    K --> M["Merge helmValues<br/>overrides ConfigMap and Secret values"]
    L --> M
    M --> N["Unflatten dotted paths and array syntax<br/>coerce scalars"]
    N --> O["Write app-values.yaml<br/>then replace placeholders"]
```

**Reading:** Precedence applies per source list: CLI flags override
`plugin.parameters`, and `plugin.parameters` overrides `plugin.env`. `helmValues`
is available only through `plugin.parameters` and is merged last.

## Value Resolution: Env

```mermaid
flowchart TD
    A["Source lists<br/>envConfigs and envSecrets"] --> B["Same precedence as values"]
    B --> C["Resolve references<br/>name or namespace/name"]
    C --> D["Data keys per source<br/>sorted lexicographically"]
    D --> E["Use value literally<br/>no YAML parsing"]
    E --> F["Merge into a map"]
    F --> G["Flatten: uppercase keys<br/>dot and hyphen to underscore"]
    G --> H["Collisions possible<br/>avoid duplicate sanitized keys"]
    H --> I["Replace placeholders in all non-ignored files"]
    I --> J["Unknown key remains<br/>and emits a warning"]
```

**Reading:** Env values are never interpreted as YAML. The key `db.host` becomes
the environment variable `DB_HOST`; the chart references it as `${DB_HOST}`.
`${db.host}` is also sanitized to `DB_HOST`.

## Sequence: watch and Refresh

```mermaid
sequenceDiagram
    autonumber
    participant API as Kubernetes API
    participant W as watch sidecar
    participant H as event handler
    participant A as Application

    Note over W: Start: Applications are listed<br/>and read into the mapping.<br/>ConfigMaps and Secrets are<br/>only cached initially.
    W->>API: list and watch Applications, ConfigMaps, Secrets
    API-->>W: events
    W->>W: collect events in a batch<br/>flush every ARGOCD_VALUES_REFRESH_SECONDS
    W->>H: pass batch
    H->>H: update Application-to-resource mapping
    alt Changed ConfigMap or Secret is referenced
        H->>A: merge patch with refresh annotation
        A-->>H: success or error
        H->>H: up to 3 attempts<br/>only retryable errors
    else New Application in the same batch
        H->>H: no refresh due to initial seeding
    else No affected Application
        H->>H: do nothing
    end
```

**Reading:** Initial seeding only builds the mapping and does not trigger any
refreshes. ConfigMaps and Secrets are not reported as events on startup. A new
Application that appears in the same batch as a resource change is not refreshed
yet. The patch sets the annotation `argocd.argoproj.io/refresh: hard`.

## Decision Tree: discover

```mermaid
flowchart TD
    A["discover starts"] --> B{"One of the variables<br/>ARGOCD_ENV_VALUE_CONFIGS,<br/>ARGOCD_ENV_ENV_CONFIGS,<br/>ARGOCD_ENV_VALUE_SECRETS,<br/>ARGOCD_ENV_ENV_SECRETS<br/>exists, even if empty?"}
    B -->|yes| C["supported<br/>token argo-values on stdout<br/>exit 0"]
    B -->|no| D["parse ARGOCD_APP_PARAMETERS"]
    D -->|parse error| E["log error<br/>exit 1"]
    D -->|ok| F{"Recognized parameter<br/>with a non-empty value?<br/>valueConfigs, envConfigs,<br/>valueSecrets, envSecrets,<br/>helmValues"}
    F -->|yes| C
    F -->|no| G["no stdout<br/>exit 2"]
```

**Reading:** The mere existence of an `ARGOCD_ENV_*` variable is sufficient, even
with an empty value. If such a variable is present, `ARGOCD_APP_PARAMETERS` is not
parsed during discovery; broken JSON is only detected in `init`. Unknown
parameters and empty arrays do not count as support.

## State and Namespace Resolution

Between `discover`, `init`, and `generate`, state is passed **only through files**,
not through process environment. `init` writes the values file
`<file-target>/<file-values>` (default `./app-values.yaml`), and the subsequent
substitution reads and writes files in the chart directory. `generate` reads the
values file via `-f`.

Namespace resolution is a common pitfall:

- `ARGOCD_APP_NAMESPACE` is the Application's **destination namespace**, not the
  namespace of the Application resource.
- `init` resolves unqualified references against `ARGOCD_APP_NAMESPACE` (fallback
  `default`).
- `watch` resolves unqualified references from Application specs against
  `spec.destination.namespace` (fallback `default`).
- `generate` uses `ARGOCD_APP_NAMESPACE` as the Helm `--namespace`. The flag
  default `argocd` applies only when the environment variable is missing (for
  example, in a manual run).

Therefore, qualify references as `namespace/name` whenever in doubt.
