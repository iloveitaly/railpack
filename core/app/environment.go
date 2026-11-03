package app

import (
	"fmt"
	"maps"
	"os"
	"strings"
)

type Environment struct {
	// environment variables and secrets used to read configuration and detect providers
	Variables map[string]string
	// values supplied as build secrets, kept separate from environment variables
	Secrets map[string]string
}

// seed planning variables and build secrets from the same supplied map
//
// {"TOKEN": "abc"} -> Variables={"TOKEN": "abc"}, Secrets={"TOKEN": "abc"}
func NewEnvironment(buildSecrets *map[string]string) *Environment {
	if buildSecrets == nil {
		buildSecrets = &map[string]string{}
	}

	return &Environment{Variables: *buildSecrets, Secrets: *buildSecrets}
}

// clone planning variables with overrides while sharing the original build secrets
//
// Variables={"MODE": "dev"}, overrides={"MODE": "prod", "EMPTY": ""}
// -> Variables={"MODE": "prod", "EMPTY": ""}, Secrets unchanged
func (e *Environment) WithVariables(variableOverrides map[string]string) *Environment {
	mergedVariables := maps.Collect(maps.All(e.Variables))
	maps.Copy(mergedVariables, variableOverrides)
	return &Environment{Variables: mergedVariables, Secrets: e.Secrets}
}

// resolve variable flag values, using the process environment for name-only entries
//
// - explicit values are literal, including empty strings
// - missing process lookups are omitted
// - repeated names use the last resolved value
//
// with TOKEN=abc in the process: ["MODE=prod", "TOKEN", "EMPTY="]
// -> {"MODE": "prod", "TOKEN": "abc", "EMPTY": ""}, nil
func ParseVariableFlagValues(variableFlagValues []string) (map[string]string, error) {
	variableValues := make(map[string]string)

	for _, variableFlagValue := range variableFlagValues {
		variableName, variableValue, hasExplicitValue := strings.Cut(variableFlagValue, "=")
		if hasExplicitValue {
			variableValues[variableName] = variableValue
			continue
		}

		if processValue, existsInProcess := os.LookupEnv(variableName); existsInProcess {
			variableValues[variableName] = processValue
		}
	}

	return variableValues, nil
}

// GetVariable returns the value of the given variable name
func (e *Environment) GetVariable(name string) string {
	return e.Variables[name]
}

// SetVariable stores a variable in the Environment
func (e *Environment) SetVariable(name, value string) {
	e.Variables[name] = value
}

// ConfigVariable returns the RAILPACK_ prefixed version of a variable name
func (e *Environment) ConfigVariable(name string) string {
	return fmt.Sprintf("RAILPACK_%s", name)
}

// returns the value of a RAILPACK_ prefixed variable with newlines removed
// Returns both the value and the name of the config variable
func (e *Environment) GetConfigVariable(name string) (string, string) {
	configVar := e.ConfigVariable(name)

	if val, exists := e.Variables[configVar]; exists {
		return strings.TrimSpace(val), configVar
	}
	return "", ""
}

// GetConfigVariableList returns a space-separated config variable as a list
// Returns both the list and the name of the config variable
func (e *Environment) GetConfigVariableList(name string) ([]string, string) {
	val, configVar := e.GetConfigVariable(name)
	if val == "" {
		return nil, ""
	}
	return strings.Split(val, " "), configVar
}

// checks if a RAILPACK_ prefixed variable is set to "1" or "true"
func (e *Environment) IsConfigVariableTruthy(name string) bool {
	if val, _ := e.GetConfigVariable(name); val != "" {
		lowerVal := strings.ToLower(val)
		return lowerVal == "1" || lowerVal == "true"
	}
	return false
}

func (e *Environment) GetSecretsWithPrefix(secretNamePrefix string) []string {
	secretNames := []string{}
	for secretName := range e.Secrets {
		if strings.HasPrefix(secretName, secretNamePrefix) {
			secretNames = append(secretNames, secretName)
		}
	}
	return secretNames
}
