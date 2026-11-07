package main

import (
	"fmt"
	"io"
	"os"

	"argo-values/internal/config"
	"argo-values/internal/logger"
)

// discoveryToken is written to stdout when the application is supported.
// Argo CD requires both exit code 0 and non-empty stdout during discovery.
const discoveryToken = "argo-values"

type DiscoverCmd struct{}

func (DiscoverCmd) Help() string {
	return `Validates whether the Argo CD Config Management Plugin (CMP) is properly configured
for an application. This command checks if the application parameters include the
required configuration for the argo-values plugin to function correctly.

This is typically used by Argo CD to determine if the argo-values plugin should be
used for a particular application during the discovery phase.

If the application is supported, a discovery token is written to stdout and the
command exits with code 0. Otherwise nothing is written to stdout and the command
exits with code 2.`
}

func (c *DiscoverCmd) Run(g *Global) error {
	supported, err := discoverApplication()
	if err != nil {
		logger.Fatalf("Failed to parse parameters: %v", err)
	}

	writeDiscoveryResult(os.Stdout, supported)

	if !supported {
		logger.Debugf("Unsupported application")
		os.Exit(2)
	}

	logger.Debugf("Supported application")
	os.Exit(0)
	return nil
}

// discoverApplication checks whether the current application is supported by
// the argo-values plugin.
func discoverApplication() (bool, error) {
	if config.HasEnvParameters() {
		return true, nil
	}

	appParameters, err := config.ParseAppParameters()
	if err != nil {
		return false, err
	}

	return !appParameters.IsEmpty(), nil
}

// writeDiscoveryResult writes the discovery token to w if the application is
// supported, and writes nothing otherwise.
func writeDiscoveryResult(w io.Writer, supported bool) {
	if !supported {
		return
	}
	_, _ = fmt.Fprintln(w, discoveryToken)
}
