package config

import (
	"argo-values/internal/utils"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/utils/env"
)

// pluginParameter represents a single parameter entry from ARGOCD_APP_PARAMETERS
type pluginParameter struct {
	Name   string            `json:"name"`
	Array  []string          `json:"array,omitempty"`
	String string            `json:"string,omitempty"`
	Map    map[string]string `json:"map,omitempty"`
}

type PluginParameters []pluginParameter

// Parse populates PluginParameters from a JSON string
func Parse(raw string) (PluginParameters, error) {
	var p = PluginParameters{}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("failed to parse plugin parameters: %w", err)
	}
	return p, nil
}

func sanitizeEnvName(name string) string {
	return strings.ReplaceAll(strings.ToUpper(name), "-", "_")
}

func HasEnvParameter(name string) bool {
	name = sanitizeEnvName(name) + "="
	for _, envVariable := range os.Environ() {
		if sanitizeEnvName(envVariable) == name {
			return true
		}
	}
	return false
}

func HasEnvParameters() bool {
	return HasEnvParameter("ARGOCD_ENV_VALUE_CONFIGS") || HasEnvParameter("ARGOCD_ENV_ENV_CONFIGS") || HasEnvParameter("ARGOCD_ENV_VALUE_SECRETS") || HasEnvParameter("ARGOCD_ENV_ENV_SECRETS")
}

func GetStringFromEnv(name string, def string) string {
	name = sanitizeEnvName(name) // Convert input key to uppercase for comparison

	for _, varFromEnv := range os.Environ() {
		pair := strings.SplitN(varFromEnv, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.ReplaceAll(strings.ToUpper(pair[0]), "-", "_")
		if key == name {
			return pair[1]
		}
	}

	return def
}

func GetSecondsFromEnv(name string, def time.Duration) time.Duration {
	secondsFromEnv := GetStringFromEnv(name, "")
	if seconds, err := strconv.Atoi(secondsFromEnv); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return def
}

func GetArrayFromEnv(name string, def []string) []string {
	arrayFromEnv := strings.Split(GetStringFromEnv(name, ""), ",")
	arrayFiltered := utils.NonEmpty(arrayFromEnv)
	if len(arrayFiltered) == 0 {
		return def
	}
	return arrayFiltered
}

// HasArray finds an array parameter by name
func (p PluginParameters) HasArray(name string) bool {
	for _, param := range p {
		if param.Name == name {
			return true
		}
	}
	return false
}

// GetArray finds an array parameter by name
func (p PluginParameters) GetArray(name string, def []string) []string {
	for _, param := range p {
		if param.Name == name {
			return param.Array
		}
	}
	return def
}

// GetMap finds a map parameter by name
func (p PluginParameters) GetMap(name string, def map[string]string) map[string]string {
	for _, param := range p {
		if param.Name == name {
			return param.Map
		}
	}
	return def
}

// GetString finds a string parameter by name
func (p PluginParameters) GetString(name string, def string) string {
	for _, param := range p {
		if param.Name == name {
			return param.String
		}
	}
	return def
}

type AppParameters struct {
	Values struct {
		ConfigMaps []string
		Secrets    []string
	}
	Env struct {
		ConfigMaps []string
		Secrets    []string
	}
	Helm struct {
		Values map[string]string
	}
}

func ParseAppParameters() (AppParameters, error) {
	appParameters := AppParameters{}

	appParametersFromEnv, err := Parse(env.GetString("ARGOCD_APP_PARAMETERS", "[]"))
	if err != nil {
		return appParameters, fmt.Errorf("failed to parse ARGOCD_APP_PARAMETERS: %v", err)
	}

	valueConfigsFromEnv := GetArrayFromEnv("ARGOCD_ENV_VALUE_CONFIGS", []string{})
	appParameters.Values.ConfigMaps = appParametersFromEnv.GetArray("valueConfigs", valueConfigsFromEnv)

	envConfigsFromEnv := GetArrayFromEnv("ARGOCD_ENV_ENV_CONFIGS", []string{})
	appParameters.Env.ConfigMaps = appParametersFromEnv.GetArray("envConfigs", envConfigsFromEnv)

	valueSecretsFromEnv := GetArrayFromEnv("ARGOCD_ENV_VALUE_SECRETS", []string{})
	appParameters.Values.Secrets = appParametersFromEnv.GetArray("valueSecrets", valueSecretsFromEnv)

	envSecretsFromEnv := GetArrayFromEnv("ARGOCD_ENV_ENV_SECRETS", []string{})
	appParameters.Env.Secrets = appParametersFromEnv.GetArray("envSecrets", envSecretsFromEnv)

	appParameters.Helm.Values = appParametersFromEnv.GetMap("helmValues", map[string]string{})

	return appParameters, nil
}

func (p AppParameters) IsEmpty() bool {
	return (len(p.Values.ConfigMaps) + len(p.Values.Secrets) + len(p.Env.ConfigMaps) + len(p.Env.Secrets) + len(p.Helm.Values)) == 0
}
