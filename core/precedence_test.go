package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/railwayapp/railpack/core/app"
	c "github.com/railwayapp/railpack/core/config"
	"github.com/railwayapp/railpack/core/logger"
	"github.com/railwayapp/railpack/core/plan"
	"github.com/stretchr/testify/require"
)

func TestGetConfig_StartCommandPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		env     map[string]string
		options *GenerateBuildPlanOptions
		want    string
	}{
		{
			name:    "cli overrides env and file",
			file:    `{"deploy":{"startCommand":"from-file"}}`,
			env:     map[string]string{"RAILPACK_START_CMD": "from-env"},
			options: &GenerateBuildPlanOptions{StartCommand: "from-cli"},
			want:    "from-cli",
		},
		{
			name: "env overrides file",
			file: `{"deploy":{"startCommand":"from-file"}}`,
			env:  map[string]string{"RAILPACK_START_CMD": "from-env"},
			want: "from-env",
		},
		{
			name: "file used when cli and env are unset",
			file: `{"deploy":{"startCommand":"from-file"}}`,
			want: "from-file",
		},
		{
			name: "empty env does not override file",
			file: `{"deploy":{"startCommand":"from-file"}}`,
			env:  map[string]string{"RAILPACK_START_CMD": ""},
			want: "from-file",
		},
		{
			name: "empty cli does not override env",
			file: `{"deploy":{"startCommand":"from-file"}}`,
			env:  map[string]string{"RAILPACK_START_CMD": "from-env"},
			want: "from-env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadConfigForPrecedenceTest(t, tt.file, tt.env, tt.options)
			require.Equal(t, tt.want, cfg.Deploy.StartCmd)
		})
	}
}

func TestGetConfig_BuildCommandPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		env     map[string]string
		options *GenerateBuildPlanOptions
		want    string
	}{
		{
			name:    "cli overrides env and file",
			file:    `{"steps":{"build":{"commands":["from-file"]}}}`,
			env:     map[string]string{"RAILPACK_BUILD_CMD": "from-env"},
			options: &GenerateBuildPlanOptions{BuildCommand: "from-cli"},
			want:    "from-cli",
		},
		{
			name: "env overrides file",
			file: `{"steps":{"build":{"commands":["from-file"]}}}`,
			env:  map[string]string{"RAILPACK_BUILD_CMD": "from-env"},
			want: "from-env",
		},
		{
			name: "file used when cli and env are unset",
			file: `{"steps":{"build":{"commands":["from-file"]}}}`,
			want: "from-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadConfigForPrecedenceTest(t, tt.file, tt.env, tt.options)
			require.Equal(t, tt.want, lastBuildCommandText(cfg))
		})
	}
}

func TestGetConfig_ConfigFilePathPrecedence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, defaultConfigFileName), []byte(`{"deploy":{"startCommand":"from-default-file"}}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alt.json"), []byte(`{"deploy":{"startCommand":"from-cli-file"}}`), 0644))

	userApp, err := app.NewApp(dir)
	require.NoError(t, err)

	env := app.NewEnvironment(&map[string]string{
		"RAILPACK_CONFIG_FILE": defaultConfigFileName,
	})
	cfg, err := GetConfig(userApp, env, &GenerateBuildPlanOptions{ConfigFilePath: "alt.json"}, logger.NewLogger())
	require.NoError(t, err)
	require.Equal(t, "from-cli-file", cfg.Deploy.StartCmd)
}

func TestGenerateBuildPlan_StartCommandPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		procfile string
		env      map[string]string
		options  *GenerateBuildPlanOptions
		want     string
	}{
		{
			name:     "cli overrides env file and procfile",
			file:     `{"provider":"shell","deploy":{"startCommand":"from-file"}}`,
			procfile: "web: from-procfile\n",
			env:      map[string]string{"RAILPACK_START_CMD": "from-env"},
			options:  &GenerateBuildPlanOptions{StartCommand: "from-cli"},
			want:     "from-cli",
		},
		{
			name:     "env overrides file and procfile",
			file:     `{"provider":"shell","deploy":{"startCommand":"from-file"}}`,
			procfile: "web: from-procfile\n",
			env:      map[string]string{"RAILPACK_START_CMD": "from-env"},
			want:     "from-env",
		},
		{
			name:     "file overrides procfile",
			file:     `{"provider":"shell","deploy":{"startCommand":"from-file"}}`,
			procfile: "web: from-procfile\n",
			want:     "from-file",
		},
		{
			name:     "procfile overrides provider default",
			file:     `{"provider":"shell"}`,
			procfile: "web: from-procfile\n",
			want:     "from-procfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "start.sh"), []byte("#!/bin/sh\necho from-provider\n"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, defaultConfigFileName), []byte(tt.file), 0644))
			if tt.procfile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Procfile"), []byte(tt.procfile), 0644))
			}

			userApp, err := app.NewApp(dir)
			require.NoError(t, err)

			options := tt.options
			if options == nil {
				options = &GenerateBuildPlanOptions{}
			}

			result, err := GenerateBuildPlan(userApp, app.NewEnvironment(&tt.env), options)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, tt.want, result.Plan.Deploy.StartCmd)
		})
	}
}

// exercise production configuration precedence using a temporary config file
//
// configJSON={"deploy":{"startCommand":"file"}}, planningVariables={"RAILPACK_START_CMD":"env"}
// -> Deploy.StartCmd="env" when no CLI command override is supplied
func loadConfigForPrecedenceTest(t *testing.T, configJSON string, planningVariables map[string]string, planOptions *GenerateBuildPlanOptions) *c.Config {
	t.Helper()

	dir := t.TempDir()
	if configJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, defaultConfigFileName), []byte(configJSON), 0644))
	}

	userApp, err := app.NewApp(dir)
	require.NoError(t, err)

	if planOptions == nil {
		planOptions = &GenerateBuildPlanOptions{}
	}

	buildConfig, err := GetConfig(userApp, app.NewEnvironment(&planningVariables), planOptions, logger.NewLogger())
	require.NoError(t, err)
	return buildConfig
}

// use the display name to recover original command text before shell wrapping
func lastBuildCommandText(buildConfig *c.Config) string {
	step, ok := buildConfig.Steps["build"]
	if !ok {
		return ""
	}

	for i := len(step.Commands) - 1; i >= 0; i-- {
		switch cmd := step.Commands[i].(type) {
		case plan.ExecCommand:
			if cmd.CustomName != "" {
				return cmd.CustomName
			}
			return cmd.Cmd
		case *plan.ExecCommand:
			if cmd.CustomName != "" {
				return cmd.CustomName
			}
			return cmd.Cmd
		}
	}

	return ""
}
