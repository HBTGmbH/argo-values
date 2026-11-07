# Local Testing with MicroK8s

This page is a hands-on walkthrough for trying `argo-values` on a Linux
workstation. You will create a small local Kubernetes cluster with
[MicroK8s](https://microk8s.io/), install Argo CD from its official Helm chart,
integrate the `argo-values` CMP, and sync an example Application that uses
values from a ConfigMap.

The whole walkthrough takes a few minutes and does not touch any remote cluster.
All commands assume that you are in the root of the `argo-values` repository.

## Prerequisites

- Linux with [snapd](https://snapcraft.io/docs/installing-snapd) and a working
  Docker daemon (Docker is only used to build the plugin image).
- `kubectl`, `helm`, and `git` in your `PATH`.
- Network access to GitHub (example application and Helm repository),
  `quay.io` (Argo CD image), and the Helm chart sources of the example
  application.

## 1. Install MicroK8s

Install MicroK8s and give your user access to the cluster:

```bash
sudo snap install microk8s --classic
sudo usermod -aG microk8s "$USER"
newgrp microk8s

microk8s status --wait-ready
microk8s enable dns
```

`newgrp` starts a shell with the new group membership. Keep this shell open for
the rest of the walkthrough.

## 2. Point kubectl and Helm at the cluster

Export a kubeconfig for the current shell so that the standard `kubectl` and
`helm` commands work:

```bash
microk8s config > /tmp/argo-values-kubeconfig
export KUBECONFIG=/tmp/argo-values-kubeconfig

kubectl get nodes
```

## 3. Build and import the plugin image

`argo-values` runs as a sidecar in the repo-server, so the image has to be
available in the MicroK8s containerd. Build it locally, export it, and import it
into MicroK8s:

```bash
docker build -t argo-values:local .
docker save argo-values:local -o /tmp/argo-values-local.tar
microk8s ctr image import /tmp/argo-values-local.tar
microk8s ctr images ls | grep argo-values
```

## 4. Install Argo CD with the CMP

Install the official Argo CD Helm chart. The CMP is added through a plugin
ConfigMap and additional sidecars in a local values file.

```bash
ARGO_CD_VERSION=10.14.0

helm repo add argo https://argoproj.github.io/argo-helm
helm repo update
ARGOCD_TAG=$(helm show chart argo/argo-cd --version "${ARGO_CD_VERSION}" \
  | awk '/^appVersion:/ {print $2}')
```

Create the plugin ConfigMap. This minimal `plugin.yaml` is sufficient for the
`plugin.env` sources used below; for `plugin.parameters`/`helmValues`, use the
full manifest from [Getting Started](getting-started.md#full-pluginyaml).

```bash
cat > /tmp/argocd-cmp-plugin-config.yaml <<'EOF'
apiVersion: v1
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
EOF
```

Create the Argo CD values file. The `copy-cmp-server` init container copies the
CMP server binary from the Argo CD image; the `argo-values` sidecar starts it.
The second `argo-values` container in the controller provides automatic refresh.

```bash
cat > /tmp/argo-cd-values.yaml <<EOF
repoServer:
  initContainers:
    - name: copy-cmp-server
      image: quay.io/argoproj/argocd:${ARGOCD_TAG}
      command: [cp, /usr/local/bin/argocd-cmp-server, /custom-tools/]
      volumeMounts:
        - name: custom-tools
          mountPath: /custom-tools

  extraContainers:
    - name: argo-values
      image: argo-values:local
      imagePullPolicy: Never
      command: [/custom-tools/argocd-cmp-server]
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

controller:
  extraContainers:
    - name: argo-values
      image: argo-values:local
      imagePullPolicy: Never
      command: [argo-values]
      args: [watch]
EOF
```

Create the RBAC resources (see [Getting Started – RBAC](getting-started.md#rbac)
for details):

```bash
cat > /tmp/argo-values-rbac.yaml <<'EOF'
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
    name: argocd-repo-server
    namespace: argocd
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
    name: argocd-application-controller
    namespace: argocd
EOF
```

Create the namespace, apply the plugin ConfigMap and RBAC, and install Argo CD:

```bash
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f /tmp/argocd-cmp-plugin-config.yaml
kubectl apply -f /tmp/argo-values-rbac.yaml

helm install argocd argo/argo-cd \
  --namespace argocd \
  --version "${ARGO_CD_VERSION}" \
  -f /tmp/argo-cd-values.yaml
```

Notes:

- The `copy-cmp-server` init container must use the same image version as the
  repo-server. `ARGOCD_TAG` is taken from the selected Argo CD chart so that
  both match.
- `imagePullPolicy: Never` makes the node use the imported image and prevents
  MicroK8s from trying to pull `argo-values:local` from a registry.
- The plugin ConfigMap must exist before the repo-server pod starts, because it
  is mounted as a volume.

## 5. Wait for the rollout

```bash
kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=5m
kubectl -n argocd rollout status deploy/argocd-server --timeout=5m
kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=5m

kubectl -n argocd get pods
```

Check that the CMP sidecar is running:

```bash
kubectl -n argocd logs deploy/argocd-repo-server -c argo-values --tail=20
kubectl -n argocd logs statefulset/argocd-application-controller -c argo-values --tail=20
```

## 6. Open the Argo CD UI

Forward the Argo CD server port:

```bash
kubectl -n argocd port-forward svc/argocd-server 8080:443
```

Open <https://localhost:8080> and log in as `admin`. Print the initial password
with:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath='{.data.password}' | base64 -d; echo
```

Accept the self-signed certificate warning of your browser.

## 7. Create and sync the example Application

Create two ConfigMaps and an Application with automated sync:

```bash
cat > /tmp/argo-values-example.yaml <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-values
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
  name: test-env
  namespace: default
data:
  cpu: "123m"
---
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
          value: test-values
        - name: env-configs
          value: test-env
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
EOF

kubectl apply -f /tmp/argo-values-example.yaml
```

Wait for the first sync and verify the result:

```bash
kubectl -n argocd wait --for=jsonpath='{.status.sync.status}'=Synced \
  application/nginx-example --timeout=5m
kubectl -n argocd get application nginx-example

kubectl -n default get deploy nginx-example-helm-guestbook
kubectl -n default get deploy nginx-example-helm-guestbook \
  -o jsonpath='replicas={.spec.replicas} cpu={.spec.template.spec.containers[0].resources.requests.cpu}{"\n"}'
```

Expected output: `replicas=2 cpu=123m`.

This confirms the full flow:

- `value-configs` supplied `replicaCount: 2` and
  `resources.requests.cpu: "${CPU}"` from the `test-values` ConfigMap.
- `env-configs` supplied `CPU=123m` from the `test-env` ConfigMap.
- `init` substituted `${CPU}` and wrote the merged values file.
- `generate` rendered the chart with those values.

## 8. Try the refresh path

The `watch` sidecar triggers a hard refresh when a referenced ConfigMap changes.
Change `replicaCount` in the values ConfigMap:

```bash
kubectl -n default patch configmap test-values --type merge -p \
  '{"data":{"values.yaml":"replicaCount: 3\nresources:\n  requests:\n    cpu: \"${CPU}\"\n"}}'
```

Within a few seconds Argo CD should refresh and roll out the new replica count:

```bash
kubectl -n default rollout status deploy/nginx-example-helm-guestbook --timeout=2m
kubectl -n default get deploy nginx-example-helm-guestbook \
  -o jsonpath='{.spec.replicas}{"\n"}'
```

Expected output: `3`, without a manual sync. If nothing happens, see
[Troubleshooting](#troubleshooting).

## 9. Iterate on the plugin code (optional)

After changing the Go code, build a new image, import it into MicroK8s, and
update the release. Use a unique tag; with the same tag the node may keep using
its cached image.

```bash
TAG="dev-$(date +%s)"

make test
docker build -t "argo-values:${TAG}" .
docker save "argo-values:${TAG}" -o "/tmp/argo-values-${TAG}.tar"
microk8s ctr image import "/tmp/argo-values-${TAG}.tar"

helm upgrade argocd argo/argo-cd \
  --namespace argocd \
  --version "${ARGO_CD_VERSION}" \
  -f /tmp/argo-cd-values.yaml \
  --set "repoServer.extraContainers[0].image=argo-values:${TAG}" \
  --set "controller.extraContainers[0].image=argo-values:${TAG}"
```

Then trigger a new render by syncing or refreshing `nginx-example` in the UI.

## 10. Clean up

Stop the local cluster:

```bash
microk8s stop
```

If you installed MicroK8s only for this test, you can remove it completely:

```bash
sudo snap remove microk8s
```

> **Caution:** `snap remove microk8s` deletes the local cluster and all
> workloads running in it. Do not run it if MicroK8s is also used for other
> purposes.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| `microk8s` commands fail with permission errors | Your user is not in the `microk8s` group yet. Run `sudo usermod -aG microk8s "$USER"`, then `newgrp microk8s` (or log in again). |
| Pods stay in `ErrImageNeverPull` / `ImagePullBackOff` | The image was not imported, or `imagePullPolicy` is not `Never`. Re-run `microk8s ctr image import ...` and check the values file. |
| MicroK8s is not ready | Check `microk8s status --wait-ready` and `microk8s inspect`. Make sure the machine has enough memory and disk space for a local Kubernetes cluster. |
| Pods cannot resolve external hostnames | Enable DNS: `microk8s enable dns`. The repo-server needs it to reach GitHub and Helm repositories. |
| Repo-server pod stays in `ContainerCreating` | The `argocd-cmp-plugin-config` ConfigMap is missing. Apply it before installing Argo CD. |
| Repo-server sidecar crashes or `argocd-cmp-server` is incompatible | `copy-cmp-server` and the repo-server must use the same image version. Check `ARGOCD_TAG` and the image in `/tmp/argo-cd-values.yaml`. |
| Application is `Unknown` or shows a render error | Inspect the repo-server CMP sidecar: `kubectl -n argocd logs deploy/argocd-repo-server -c argo-values`. Check that the repo-server can reach the Git repository and Helm repositories. |
| `watch` does not refresh the Application | Check the sidecar logs (`-c argo-values` on the application-controller). Make sure the namespaces of the Application and of the referenced ConfigMaps are watched (`ARGOCD_VALUES_NAMESPACES`, default `argocd,default`) and that the RBAC resources were created. |
| Example Application cannot be fetched | The example uses `https://github.com/argoproj/argocd-example-apps.git` with `targetRevision: HEAD`. A fork or an internal mirror requires adjusting the Application. |

For further details, see [Operations – Troubleshooting](operations.md#troubleshooting).
