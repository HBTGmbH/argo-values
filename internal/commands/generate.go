package commands

import (
	"argo-values/internal/config"
	"argo-values/internal/logger"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"k8s.io/utils/env"
)

var (
	appName      string
	appNamespace string
)

func init() {
	GenerateCmd.Flags().StringVar(&appName, "app-name", env.GetString("ARGOCD_APP_NAME", ""), "Name of the Argo CD application")
	GenerateCmd.Flags().StringVar(&appNamespace, "app-namespace", env.GetString("ARGOCD_APP_NAMESPACE", ""), "Namespace of the Argo CD application")
}

var GenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Argo application resources using Helm",
	Long: `Generates Kubernetes manifests for an Argo CD application using Helm templates.
This command runs 'helm template' with the specified application name, namespace,
and values files to produce the final manifests that will be deployed.

The command uses the Helm chart located in the target directory and includes
any additional values from ConfigMaps and Secrets that were processed by the init command.

Environment variables ARGOCD_APP_NAME and ARGOCD_APP_NAMESPACE can be used to
set the application name and namespace instead of command line flags.`,

	Run: func(cmd *cobra.Command, args []string) {
		// TODO other parameters, kustomize
		helmArgs := []string{"template", appName, config.TargetDirectory, "--namespace", appNamespace, "--include-crds", "-f", config.ValuesFileName}
		helmCmd := exec.Command("helm", helmArgs...)
		helmCmd.Stdout = os.Stdout
		helmCmd.Stderr = os.Stderr

		if err := helmCmd.Run(); err != nil {
			logger.Fatal(err)
		}
	},
}
