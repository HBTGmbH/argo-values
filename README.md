# Argo Values - Argo CD CMP Plugin

[![License](https://img.shields.io/github/license/HBTGmbH/argo-values)](https://github.com/HBTGmbH/argo-values/blob/main/LICENSE)

Argo Values is a Config Management Plugin (CMP) for Argo CD that enhances Helm-based deployments by integrating values from Kubernetes ConfigMaps and Secrets.

## Features

- **Dynamic Value Injection**: Merge values from ConfigMaps and Secrets into Helm template application files
- **Environment Variable Substitution**: Replace variable placeholders in application files with actual values
- **Automatic Refresh**: Watch for ConfigMap/Secret changes and trigger application refreshes
- **Helm Integration**: Full compatibility with Helm charts and templating
- **Selective Processing**: Respect `.helmignore` patterns

## Installation with ArgoCD Helm chart

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

### Add argo-values to ArgoCD application controller (optional)

To trigger application refresh automatically every time a ConfigMap or Secret changed, add the following snippet to your `values.yaml`:
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
```

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

Every ConfigMap from `value-configs` or Secret from `value-secrets` will be added directly to the Helm templates of the application:

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

All values from `env-configs` or `value-secrets` are available as environment variables and substituted in the Helm templates of the application:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: application-variables
  namespace: default
data:  
  cpu: "123m"
```

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open issues and pull requests on GitHub.