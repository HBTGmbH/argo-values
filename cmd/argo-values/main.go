package main

import (
	"argo-values/internal/commands"
	"argo-values/internal/config"
	"argo-values/internal/logger"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/utils/env"
)

var RootCmd = &cobra.Command{
	Use:   "argo-values",
	Short: "A CLI tool for managing Argo application with values from additional resources like ConfigMaps and Secrets",
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior when no subcommand is provided
		_ = cmd.Help()
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize logger with the configured settings
		logger.Init(config.LogLevel, config.LogFormat, config.LogOutput)
	},
}

func init() {
	// Add global flags
	RootCmd.PersistentFlags().StringVarP(&config.LogLevel, "log-level", "l",
		env.GetString("LOG_LEVEL", "debug"), "Log level (debug, info, warn, error, fatal, panic)")
	RootCmd.PersistentFlags().StringVar(&config.LogFormat, "log-format",
		env.GetString("LOG_FORMAT", "json"), "Log format (text, json)")
	RootCmd.PersistentFlags().StringVar(&config.LogOutput, "log-output",
		env.GetString("LOG_OUTPUT", "stdout"), "Log output (stderr, stdout)")

	RootCmd.PersistentFlags().StringVarP(&config.KubeConfigPath, "kubeconfig", "k",
		"", "Path to kubeconfig file")

	RootCmd.PersistentFlags().StringVarP(&config.ValuesFileName, "file-values", "v",
		"app-values.yaml", "Path to values file")
	RootCmd.PersistentFlags().StringVarP(&config.TargetDirectory, "file-target", "t",
		".", "Path to application files")

	RootCmd.PersistentFlags().BoolVarP(&config.DryRun, "dry-run", "d",
		false, "dry-run enabled (true, false)")

	// Add subcommands
	RootCmd.AddCommand(commands.DiscoverCommand)
	RootCmd.AddCommand(commands.InitCmd)
	RootCmd.AddCommand(commands.GenerateCmd)
	RootCmd.AddCommand(commands.WatchCmd)
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
