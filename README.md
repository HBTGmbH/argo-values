# Argo Values - Argo CD CMP Plugin

[![License](https://img.shields.io/github/license/HBTGmbH/argo-values)](https://github.com/HBTGmbH/argo-values/blob/main/LICENSE)

Argo Values is a Config Management Plugin (CMP) for Argo CD that enhances Helm-based deployments by integrating values from Kubernetes ConfigMaps and Secrets.

## Features

- **Dynamic Value Injection**: Merge values from ConfigMaps and Secrets into Helm template application files
- **Environment Variable Substitution**: Replace `${VAR_NAME}` placeholders in application files with actual values
- **Automatic Refresh**: Watch for ConfigMap/Secret changes and trigger application refreshes
- **Helm Integration**: Full compatibility with Helm charts and templating
- **Selective Processing**: Respect `.helmignore` patterns

## Example

Install ArgoCD with CMP and an example application:
```bash
helm upgrade --install argocd argo/argo-cd -n argocd -f values.yaml
```

values.yaml:
```yaml

# add argo-values as sidecar to Argo controller for automatic application refresh
controller:
  extraContainers:
    - name: argo-values
      image: argo-values:latest
      command: [argo-values]
      args: [watch]

# add argo-values as CMP to Argo's repo-server
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
      image: argo-values:latest
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

# the CMP configuration
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
        
# an example application with two additional ConfigMaps
  - apiVersion: argoproj.io/v1alpha1
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
            - name: env-configs
              value: test-env
            - name: value-configs
              value: test-values
      destination:
        server: https://kubernetes.default.svc  # Targets the same cluster as Argo CD
        namespace: default
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: test-env
      namespace: default
    data:
      values.yaml: |
        cpu: "100m"
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: test-values
      namespace: default
    data:
      values.yaml: |
        resources:
          requests:
            cpu: "${CPU}"
```

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open issues and pull requests on GitHub.