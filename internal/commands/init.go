package commands

import (
	"argo-values/internal/config"
	"argo-values/internal/helm"
	"argo-values/internal/kubernetes"
	"argo-values/internal/logger"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
)

var (
	valueConfigs []string
	envConfigs   []string
	valueSecrets []string
	envSecrets   []string
	envPattern   = regexp.MustCompile(`\$\{([^}]+)\}`)
)

func init() {
	InitCmd.Flags().StringArrayVar(&valueConfigs, "value-maps",
		[]string{}, "ConfigMaps containing values to merge into the application files")
	InitCmd.Flags().StringArrayVar(&envConfigs, "env-maps",
		[]string{}, "ConfigMaps containing environment variables to inject into application files")
	InitCmd.Flags().StringArrayVar(&valueSecrets, "value-secrets",
		[]string{}, "Secrets containing values to merge into the application files")
	InitCmd.Flags().StringArrayVar(&envSecrets, "env-secrets",
		[]string{}, "Secrets containing environment variables to inject into application files")
}

var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Argo application values from ConfigMaps and Secrets",
	Long: `Initializes and prepares Argo application manifests by extracting values from Kubernetes
ConfigMaps and Secrets and merging them into the other application resources.

This command performs two main functions:
1. Merges values from specified ConfigMaps and Secrets into the application resources provided by Argo Repo-Server
2. Injects environment variables from ConfigMaps and Secrets into application files

The command respects .helmignore patterns and processes all files in the application
directory, replacing environment variable placeholders (${VAR_NAME}) with actual values.

Values are merged in the order: ConfigMaps first, then Secrets, with later sources
overwriting earlier ones for the same keys.`,

	Run: func(cmd *cobra.Command, args []string) {
		appParameters, err := config.ParseAppParameters()
		if err != nil {
			logger.Fatalf("Failed to parse parameters: %v", err)
		}

		if len(valueConfigs) != 0 {
			appParameters.Values.ConfigMaps = valueConfigs
		}
		if len(valueSecrets) != 0 {
			appParameters.Values.Secrets = valueSecrets
		}
		if len(envConfigs) != 0 {
			appParameters.Env.ConfigMaps = envConfigs
		}
		if len(envSecrets) != 0 {
			appParameters.Env.Secrets = envSecrets
		}

		logger.Debug("Connecting to Kubernetes cluster")
		kubeClient, err := kubernetes.NewClient(config.KubeConfigPath)
		if err != nil {
			logger.Fatalf("Failed to create Kubernetes client: %v", err)
		}

		appValues, appEnv, err := generateValues(kubeClient, &appParameters)
		if err != nil {
			logger.Fatalf("Init failed with: %v", err)
		}

		if !config.DryRun {
			err = config.SaveYAML(appValues, filepath.Join(config.TargetDirectory, config.ValuesFileName))
			if err != nil {
				logger.Fatalf("Failed to write values YAML file %s: %v", config.ValuesFileName, err)
			}
		}

		helmIgnore, err := helm.ReadHelmIgnore(config.TargetDirectory)
		if err != nil {
			logger.Fatalf("Failed to read .helmignore: %v", err)
		}
		err = prepareApplicationFiles(config.TargetDirectory, helmIgnore, appEnv, config.DryRun)
		if err != nil {
			logger.Fatalf("Failed to inject ENV values: %v", err)
		}
	},
}

func generateValues(client *kubernetes.Client, appParameters *config.AppParameters) (map[string]interface{}, map[string]string, error) {
	defaultNamespace := config.GetStringFromEnv("ARGOCD_APP_NAMESPACE", "default")

	var configMaps = map[string]*v1.ConfigMap{}
	for _, name := range append(appParameters.Values.ConfigMaps, appParameters.Env.ConfigMaps...) {
		if _, found := configMaps[name]; !found {
			configMap, err := client.GetConfigmap(name, defaultNamespace)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to get ConfigMap %s: %v", name, err)
			}
			configMaps[name] = configMap
		}
	}

	var secrets = map[string]*v1.Secret{}
	for _, name := range append(appParameters.Values.Secrets, appParameters.Env.Secrets...) {
		if _, found := secrets[name]; !found {
			secret, err := client.GetSecret(name, defaultNamespace)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to get Secret %s: %v", name, err)
			}
			secrets[name] = secret
		}
	}

	// Parse YAML from configmaps and secrets
	mergedValues, err := mergeData(configMaps, appParameters.Values.ConfigMaps, secrets, appParameters.Values.Secrets, config.ParseYAML)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init values: %v", err)
	}

	helmValues := config.UnflattenYAML(appParameters.Helm.Values, false)
	helmValues = config.CoerceScalars(helmValues)
	mergedValues = config.MergeYAML(mergedValues, helmValues)

	mergedEnv, err := mergeData(configMaps, appParameters.Env.ConfigMaps, secrets, appParameters.Env.Secrets, config.ParseEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init env: %v", err)
	}
	flattenEnv := map[string]string{}
	config.FlattenYAML(mergedEnv, "", flattenEnv)

	return mergedValues, flattenEnv, nil
}

// sortedKeys returns the keys of m in ascending lexicographic order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeData(configMaps map[string]*v1.ConfigMap, configMapNames []string, secrets map[string]*v1.Secret, secretNames []string, parse func(key, value string) map[string]interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}

	for _, name := range configMapNames {
		configMap := configMaps[name]
		if configMap == nil {
			return nil, fmt.Errorf("ConfigMap %q was not loaded", name)
		}
		data := configMap.Data
		for _, key := range sortedKeys(data) {
			result = config.MergeYAML(result, parse(key, data[key]))
		}
	}

	for _, name := range secretNames {
		secret := secrets[name]
		if secret == nil {
			return nil, fmt.Errorf("Secret %q was not loaded", name)
		}
		data := secret.Data
		for _, key := range sortedKeys(data) {
			result = config.MergeYAML(result, parse(key, string(data[key])))
		}
	}

	return result, nil
}

func prepareApplicationFiles(directory string, ignorePatterns *helm.IgnoreFile, env map[string]string, dryRun bool) error {
	err := filepath.Walk(directory, func(path string, file os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check whether the file or directory should be ignored. The base
		// directory itself resolves to "." and is never ignored.
		if ignorePatterns.ShouldIgnore(path, directory, file.IsDir()) {
			if file.IsDir() {
				logger.Debugf("Ignore directory %s", path)
				return filepath.SkipDir
			}
			logger.Debugf("Ignore file %s", path)
			return nil
		}

		if file.IsDir() {
			return nil
		}

		if !file.Mode().IsRegular() {
			logger.Debugf("Skip non-regular file %s", path)
			return nil
		}

		err = processFile(directory, path, env, dryRun)
		if err != nil {
			return fmt.Errorf("failed to process file %s: %v", path, err)
		}

		return nil
	})

	return err
}

func processFile(baseDirectory string, filePath string, env map[string]string, dryRun bool) error {
	relPath, _ := filepath.Rel(baseDirectory, filePath)
	logger.Debugf("Processing %s ...", relPath)

	info, err := os.Lstat(filePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to process non-regular file %s", relPath)
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read content of file %s: %v", relPath, err)
	}

	if bytes.IndexByte(content, 0) >= 0 {
		logger.Debugf("Skip binary file %s", relPath)
		return nil
	}

	processedContent := envPattern.ReplaceAllStringFunc(string(content), func(match string) string {
		// Extract the key from ${KEY}
		key := config.SanitizeKey(envPattern.ReplaceAllString(match, "$1"))

		// Look up the key in the environment map
		if value, exists := env[key]; exists {
			logger.Debugf("Replaced key %s in %s", key, relPath)
			return value
		}

		logger.Warnf("Unknown key found in %s: %s", relPath, key)
		return match
	})

	if !dryRun && processedContent != string(content) {
		// Only write if the content actually changed.
		if err := os.WriteFile(filePath, []byte(processedContent), 0644); err != nil {
			return fmt.Errorf("failed to write content of file %s: %v", relPath, err)
		}
	}

	return nil
}
