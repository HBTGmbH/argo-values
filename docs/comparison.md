# Comparison

**Last verified: November 2026**

## Scope

`argo-values` is an Argo CD Config Management Plugin (CMP) for Helm values from
ConfigMaps and Secrets, `${VAR}` substitution, and optional automatic refresh.

This page places `argo-values` in context relative to Flux, native Argo CD options,
and related plugins. It focuses on fit and scope rather than ranking one approach
above another. The community landscape changes; the links and statuses below were
last checked in November 2026. This page is not a statement about the official
Argo CD roadmap.

## Native Argo CD Options

### Multi-source Applications (Argo CD 2.6+)

Applications can use multiple Git sources, for example a chart from one repository
and values from another. Helm sources can use `valueFiles` or `valuesObject`. This
solves "values in Git", but it does **not** make ConfigMaps or Secrets a values
source.

### `valuesObject`

`valuesObject` embeds inline Helm values in the Application. It is useful for small
overrides that belong with the Application, but it is not a mechanism for external
ConfigMaps or Secrets.

### ApplicationSet plugin generator (Argo CD 2.8+)

The ApplicationSet plugin generator is an extension point for dynamic generators.
Several small projects listed below use it for ConfigMap- or Secret-driven
generation.

### Helm `lookup`

Helm's `lookup` function can read cluster resources at render time, but it is not a
clean ConfigMap values mechanism. It requires cluster access during rendering and
has security and operability trade-offs; see
[#5202](https://github.com/argoproj/argo-cd/issues/5202) and
[#21745](https://github.com/argoproj/argo-cd/issues/21745).

## Upstream Argo CD Discussions

Statuses reflect the linked issue or discussion pages as of November 2026. This is
not an inference about whether or when a feature will be implemented.

| Item | Status | Summary |
| --- | --- | --- |
| [#5664 – Support helm values also via Secret and ConfigMap](https://github.com/argoproj/argo-cd/issues/5664) | Open issue (since 2021) | The closest match: support Helm values from Secrets and ConfigMaps, inspired by Flux `valuesFrom`. |
| [#1786 – Add support for secrets in Application parameters](https://github.com/argoproj/argo-cd/issues/1786) | Open issue (since 2019) | Proposes `valueFrom`/`secretKeyRef` for Application parameters, including secret rotation and parameter handling. |
| [#14850 – Refer to values in ConfigMap and Secret resources from Application](https://github.com/argoproj/argo-cd/discussions/14850) | Discussion / Q&A | Asks for a Flux `valuesFrom` equivalent in Argo CD. |
| [#5826 – independent version controlled helm values file](https://github.com/argoproj/argo-cd/issues/5826) | Closed as duplicate of #2789 | An independent version-controlled Helm values file; partly addressed by multi-source Applications. |
| [#1705 – ENV variables from "params"](https://github.com/argoproj/argo-cd/issues/1705) | Open issue (since 2019) | Env/post-build substitution for parameters; mostly a CMP workaround. |
| [#14882 – envFrom generator for ApplicationSet](https://github.com/argoproj/argo-cd/issues/14882) | Open issue (since 2023) | ApplicationSet generator approach for ConfigMap/Secret values. |
| [#3698 – Support helm post renderers](https://github.com/argoproj/argo-cd/issues/3698) | Open issue (since 2020) | Post-renderer support for Helm output. |
| [#5202](https://github.com/argoproj/argo-cd/issues/5202) / [#21745](https://github.com/argoproj/argo-cd/issues/21745) | Open issues | Helm `lookup` support, including access-model and security concerns. |

Maintainer concerns raised in
[#5664](https://github.com/argoproj/argo-cd/issues/5664), summarized neutrally:

- Secrets rendered into manifests can end up on repo-server disk and in the Redis
  cache.
- The repo-server would need Kubernetes API access, which increases the impact of a
  repo-server compromise (RCE risk).
- Cache invalidation on ConfigMap/Secret changes is non-trivial.
- There is interest in an opt-in, ConfigMap-only approach.

These are concerns, not a statement that such a feature will or will not be
implemented.

## Flux

### `HelmRelease.spec.valuesFrom`

- ConfigMaps and Secrets must be in the same namespace as the `HelmRelease`.
- `valuesKey` defaults to `values.yaml`; `targetPath`, `optional`, and `literal` are
  supported.
- Merge order: `valuesFrom` entries in list order, then inline `spec.values`;
  `targetPath` overrides everything before it.
- Changes to referenced ConfigMaps or Secrets can trigger reconciliation
  immediately via the label `reconcile.fluxcd.io/watch: Enabled` or a controller
  flag.
- Documentation:
  [HelmRelease values references](https://fluxcd.io/flux/components/helm/helmreleases/#values-references)

### `Kustomization.spec.postBuild.substituteFrom`

- ConfigMaps and Secrets can be used; their data keys become variables.
- `${VAR}` substitution happens on the Kustomize output. Bash-like replacements and
  defaults are possible; undefined variables default to an empty string.
- Inline `substitute` takes precedence; later `substituteFrom` entries override
  earlier ones.
- This is Kustomize-centric. For Helm values, `valuesFrom` is the direct path.
- Changes to referenced ConfigMaps or Secrets trigger reconciliation via the label
  or controller flag.
- Documentation:
  [Kustomization post-build variable substitution](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution)

### Fit Summary

Flux covers a large part of the problem natively. Differences from `argo-values`:
`valuesFrom` is same-namespace, there is no chart-file mutation, substitution is
Kustomize-centric, and there is no CMP/repo-server architecture. Use `valuesFrom`
for Helm values and `postBuild` for substitution in Kustomize output.

## Related Plugins and Projects

Community projects, listed for orientation only:

- [argoproj-labs/argocd-vault-plugin](https://github.com/argoproj-labs/argocd-vault-plugin) –
  a CMP that injects secrets from Vault, AWS, IBM, and other secret managers into
  Kubernetes resources; secret-focused.
- [crumbhole/argocd-lovely-plugin](https://github.com/crumbhole/argocd-lovely-plugin) –
  a CMP runner/composer for Helm, Kustomize, and plain YAML with plugin chaining;
  it does not provide native ConfigMap values.
- [crumbhole/argocd-vault-replacer](https://github.com/crumbhole/argocd-vault-replacer) –
  replaces placeholders in manifests with Vault secrets.
- [mmalyska/argocd-secret-replacer](https://github.com/mmalyska/argocd-secret-replacer) –
  a CMP that replaces `<secret:key>` placeholders from SOPS or mounted Secrets.
- [vadimzharov/argocd-secrets-plugin](https://github.com/vadimzharov/argocd-secrets-plugin) –
  an ApplicationSet plugin generator that reads Secrets as `{{ key }}` variables;
  ApplicationSets only.
- [johan-vandeweerd-hs/cluster-config-generator](https://github.com/johan-vandeweerd-hs/cluster-config-generator) –
  a prototype ApplicationSet plugin generator for ConfigMap values; not
  production-ready.
- [HBTGmbH/argo-values](https://github.com/HBTGmbH/argo-values) – this project.

## Comparison Table

| Approach | Type | Values from ConfigMap/Secret | `${VAR}` substitution | Auto-refresh on ConfigMap/Secret change | Cross-namespace | Best fit |
| --- | --- | --- | --- | --- | --- | --- |
| `argo-values` | Argo CD CMP | Yes | Yes | Yes (optional `watch` sidecar) | Yes (`namespace/name`) | Argo CD with Helm charts that consume values from ConfigMaps/Secrets and optionally auto-refresh |
| Flux `HelmRelease.valuesFrom` | Flux Helm controller | Yes | No | Yes (label or controller flag) | No (same namespace as `HelmRelease`) | Flux-managed Helm releases with values from ConfigMaps/Secrets |
| Flux `Kustomization.postBuild` | Flux Kustomize controller | Yes (as variables) | Yes (Bash-like) | Yes (label or controller flag) | Typically same namespace | Kustomize post-build variable substitution |
| Argo CD multi-source / `valuesObject` | Native Argo CD | No | No | n/a | Yes for Git sources; not applicable to ConfigMap/Secret values | Values in Git and small inline overrides |
| `argocd-vault-plugin` | Argo CD CMP | No (secret managers) | No (secret placeholders) | Depends on setup | Depends | Injecting secrets from external secret managers |
| `argocd-lovely-plugin` | Argo CD CMP runner/composer | No native ConfigMap values (can chain plugins) | Depends on chained plugins | Depends on chained plugins | Depends | Composing Helm, Kustomize, and plain YAML |
| Secret replacers (`argocd-secret-replacer` / `argocd-vault-replacer`) | Argo CD CMP / placeholder replacement | Secrets/placeholders | Placeholder replacement (not generally `${VAR}`) | Depends | Depends | Replacing placeholders with secret values |
| ApplicationSet plugin generators | Argo CD ApplicationSet extension | Depends on generator | Depends on generator | Depends on generator | Depends on generator | Dynamic Application generation, including ConfigMap/Secret-driven prototypes |

For `argo-values`, cross-namespace references work through `namespace/name`. For automatic refresh, the referenced namespaces must also be listed in `ARGOCD_VALUES_NAMESPACES`.