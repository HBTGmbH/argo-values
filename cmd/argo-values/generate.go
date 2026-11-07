package main

import (
	"argo-values/internal/config"
	"argo-values/internal/logger"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GenerateCmd struct {
	AppName      string `name:"app-name" env:"ARGOCD_APP_NAME" help:"Name of the Argo CD application"`
	AppNamespace string `name:"app-namespace" env:"ARGOCD_APP_NAMESPACE" default:"argocd" help:"Namespace of the Argo CD application"`
}

func (c *GenerateCmd) Help() string {
	return `Generates Kubernetes manifests for an Argo CD application using Helm templates.
This command runs 'helm template' with the specified application name, namespace,
and values files to produce the final manifests that will be deployed.

The command uses the Helm chart located in the target directory and includes
any additional values from ConfigMaps and Secrets that were processed by the init command.

Environment variables ARGOCD_APP_NAME and ARGOCD_APP_NAMESPACE can be used to
set the application name and namespace instead of command line flags.`
}

func (c *GenerateCmd) Run(g *Global) error {
	// TODO other parameters, Kustomize
	kubeVersion := normalizeKubeVersion(config.GetStringFromEnv("KUBE_VERSION", ""))
	apiVersions := config.GetArrayFromEnv("KUBE_API_VERSIONS", nil)
	helmArgs := helmTemplateArgs(c.AppName, c.AppNamespace, g.TargetDirectory, g.ValuesFileName, kubeVersion, apiVersions)
	//nolint:gosec // helm is resolved via PATH by design; args come from trusted config
	helmCmd := exec.CommandContext(context.Background(), "helm", helmArgs...)
	helmCmd.Stdout = os.Stdout
	helmCmd.Stderr = os.Stderr

	if err := helmCmd.Run(); err != nil {
		logger.Fatal(err)
	}

	return nil
}

// helmTemplateArgs builds the arguments for "helm template". The values file path
// must follow the same rule as init: filepath.Join(TargetDirectory, ValuesFileName).
func helmTemplateArgs(appName, appNamespace, targetDirectory, valuesFile, kubeVersion string, apiVersions []string) []string {
	args := []string{
		"template", appName, targetDirectory,
		"--namespace", appNamespace,
		"--include-crds",
		"--dependency-update",
	}
	if kubeVersion != "" {
		args = append(args, "--kube-version", kubeVersion)
	}
	for _, apiVersion := range apiVersions {
		if apiVersion != "" {
			args = append(args, "--api-versions", apiVersion)
		}
	}
	args = append(args, "-f", filepath.Join(targetDirectory, valuesFile))
	return args
}

// normalizeKubeVersion converts a Kubernetes version as reported by Argo CD into
// the format Helm expects. Argo CD may supply values such as "v1.29.0" or
// "1.29.0+"/"1.29.0-eks-1234"; only the leading numeric dotted core is kept
// (e.g. "1.29.0"), because Helm rejects suffixes and a leading "v".
func normalizeKubeVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(version), "v"), "V")
	if index := strings.IndexFunc(version, func(r rune) bool {
		return (r < '0' || r > '9') && r != '.'
	}); index >= 0 {
		version = version[:index]
	}
	return version
}
