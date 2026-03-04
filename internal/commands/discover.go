package commands

import (
	"argo-values/internal/config"
	"argo-values/internal/logger"
	"os"

	"github.com/spf13/cobra"
)

var DiscoverCommand = &cobra.Command{
	Use:   "discover",
	Short: "Validates Argo values CMP plugin usage for an application",
	Long: `Validates whether the Argo CD Config Management Plugin (CMP) is properly configured
for an application. This command checks if the application parameters include the
required configuration for the argo-values plugin to function correctly.

This is typically used by Argo CD to determine if the argo-values plugin should be
used for a particular application during the discovery phase.`,

	Run: func(cmd *cobra.Command, args []string) {
		appParameters, err := config.ParseAppParameters()
		if err != nil {
			logger.Fatalf("Failed to parse parameters: %v", err)
		}

		if !appParameters.IsEmpty() {
			logger.Debugf("Supported application")
			os.Exit(0)
		}

		logger.Debugf("Unsupported application")
		os.Exit(1)
	},
}
