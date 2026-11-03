package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/railwayapp/railpack/core"
	"github.com/railwayapp/railpack/core/config"
	"github.com/railwayapp/railpack/core/logger"
	"github.com/railwayapp/railpack/core/mise"
	"github.com/railwayapp/railpack/core/plan"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestGenerateBuildResultForCommand_EnvOverridesStartCommandWithoutBecomingSecret(t *testing.T) {
	const startCommand = "echo custom start"

	// inspect the planner result through production flag parsing without invoking BuildKit
	cmd := &cli.Command{
		Name:  "test",
		Flags: commonPlanFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			result, _, _, err := GenerateBuildResultForCommand(cmd)
			require.NoError(t, err)
			require.True(t, result.Success, "%v", result.Logs)
			require.NotNil(t, result.Plan)

			require.ElementsMatch(t, []string{"TOKEN"}, result.Plan.Secrets)
			require.Equal(t, startCommand, result.Plan.Deploy.Variables["RAILPACK_START_CMD"])
			require.Equal(t, startCommand, result.Plan.Deploy.StartCmd)
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--env", "RAILPACK_START_CMD=" + startCommand,
		"--secret", "TOKEN=test-token",
		"../examples/shell-script",
	})
	require.NoError(t, err)
}

func TestGenerateBuildResultForCommand_EnvOverridesGlobalVariables(t *testing.T) {
	cmd := &cli.Command{
		Name:  "test",
		Flags: commonPlanFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			result, _, _, err := GenerateBuildResultForCommand(cmd)
			require.NoError(t, err)
			require.True(t, result.Success, "%v", result.Logs)
			require.Empty(t, result.Plan.Secrets)
			for _, step := range result.Plan.Steps {
				require.Equal(t, "cli", step.Variables["GLOBAL_VALUE"], step.Name)
				if step.Name == "build" {
					require.Equal(t, "step", step.Variables["MODE"])
					continue
				}
				require.Equal(t, "cli", step.Variables["MODE"], step.Name)
			}
			require.Equal(t, "cli", result.Plan.Deploy.Variables["GLOBAL_VALUE"])
			require.Equal(t, "deploy", result.Plan.Deploy.Variables["MODE"])
			return nil
		},
	}
	require.NoError(t, cmd.Run(context.Background(), []string{
		"test", "--future-env", "--env", "MODE=cli", "--env", "GLOBAL_VALUE=cli", "../examples/variables",
	}))
}

// TODO(stable): remove legacy cases when --env always sets environment variables in the 1.0 release
func TestGenerateBuildResultForCommand_EnvironmentCompatibility(t *testing.T) {
	t.Setenv("MODE", "from-process")
	t.Setenv("TOKEN", "secret-from-process")
	t.Setenv("EMPTY", "must-not-be-inherited")
	t.Setenv("WITH_EQUALS", "value=with=equals==")
	// keep the name-only lookup absent while allowing t.Setenv to restore any original process value
	t.Setenv("RAILPACK_TEST_MISSING_SECRET", "")
	require.NoError(t, os.Unsetenv("RAILPACK_TEST_MISSING_SECRET"))

	tests := []struct {
		name               string
		flagArguments      []string
		variables          map[string]string
		secrets            map[string]string
		discardedVariables []string
	}{
		{name: "no flags"},
		{name: "legacy env", flagArguments: []string{"--env", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "legacy short alias", flagArguments: []string{"-e", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "false preserves legacy mode", flagArguments: []string{"--future-env=false", "--env", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "secret only", flagArguments: []string{"--secret", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "mixed flags", flagArguments: []string{"--env", "MODE=production", "--secret", "TOKEN=private-token"}, variables: map[string]string{"MODE": "production"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "mixed flags with alias", flagArguments: []string{"-e", "MODE=production", "--secret", "TOKEN=private-token"}, variables: map[string]string{"MODE": "production"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "mixed flags in reverse order", flagArguments: []string{"--secret", "TOKEN=private-token", "--env", "MODE=production"}, variables: map[string]string{"MODE": "production"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "false does not disable automatic mode", flagArguments: []string{"--future-env=false", "--env", "MODE=production", "--secret", "TOKEN=private-token"}, variables: map[string]string{"MODE": "production"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "explicit opt in", flagArguments: []string{"--future-env", "--env", "MODE=production"}, variables: map[string]string{"MODE": "production"}},
		{name: "explicit opt in with alias", flagArguments: []string{"--future-env", "-e", "MODE=production"}, variables: map[string]string{"MODE": "production"}},
		{name: "explicit opt in without env", flagArguments: []string{"--future-env", "--secret", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "missing lookup still enables automatic mode", flagArguments: []string{"--env", "MODE=production", "--secret", "RAILPACK_TEST_MISSING_SECRET"}, variables: map[string]string{"MODE": "production"}},
		{name: "legacy process lookup", flagArguments: []string{"--env", "TOKEN"}, secrets: map[string]string{"TOKEN": "secret-from-process"}},
		{name: "new process lookup", flagArguments: []string{"--env", "MODE", "--secret", "TOKEN"}, variables: map[string]string{"MODE": "from-process"}, secrets: map[string]string{"TOKEN": "secret-from-process"}},
		{name: "legacy explicit empty", flagArguments: []string{"--env", "EMPTY="}, secrets: map[string]string{"EMPTY": ""}},
		{name: "new explicit empty", flagArguments: []string{"--future-env", "--env", "EMPTY="}, variables: map[string]string{"EMPTY": ""}},
		{name: "last legacy value wins", flagArguments: []string{"--env", "TOKEN=first", "--env", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "last new value wins", flagArguments: []string{"--future-env", "--env", "MODE=first", "--env", "MODE=production"}, variables: map[string]string{"MODE": "production"}},
		{name: "literal commas", flagArguments: []string{"--future-env", "--env", "MODE=one,two"}, variables: map[string]string{"MODE": "one,two"}},
		{name: "legacy process lookup with equals", flagArguments: []string{"--env", "WITH_EQUALS"}, secrets: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "new process lookup with equals", flagArguments: []string{"--future-env", "--env", "WITH_EQUALS"}, variables: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "secret process lookup with equals", flagArguments: []string{"--secret", "WITH_EQUALS"}, secrets: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "legacy literal equals", flagArguments: []string{"--env", "WITH_EQUALS=value=with=equals=="}, secrets: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "new literal equals", flagArguments: []string{"--future-env", "--env", "WITH_EQUALS=value=with=equals=="}, variables: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "secret literal equals", flagArguments: []string{"--secret", "WITH_EQUALS=value=with=equals=="}, secrets: map[string]string{"WITH_EQUALS": "value=with=equals=="}},
		{name: "mixed duplicate uses secret", flagArguments: []string{"--env", "TOKEN=public", "--secret", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "explicit duplicate uses secret", flagArguments: []string{"--future-env", "--env", "TOKEN=public", "--secret", "TOKEN=private-token"}, secrets: map[string]string{"TOKEN": "private-token"}},
		{name: "duplicate with missing secret lookup drops env", flagArguments: []string{"--env", "RAILPACK_TEST_MISSING_SECRET=public", "--secret", "RAILPACK_TEST_MISSING_SECRET"}, discardedVariables: []string{"RAILPACK_TEST_MISSING_SECRET"}},
		{name: "duplicate with missing env lookup uses secret", flagArguments: []string{"--env", "RAILPACK_TEST_MISSING_SECRET", "--secret", "RAILPACK_TEST_MISSING_SECRET=private-token"}, secrets: map[string]string{"RAILPACK_TEST_MISSING_SECRET": "private-token"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// reuse production flags in a fresh command so aliases and explicit flag presence are exercised
			cmd := &cli.Command{
				Name:                      "test",
				Flags:                     commonPlanFlags(),
				DisableSliceFlagSeparator: true,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					result, _, env, err := GenerateBuildResultForCommand(cmd)
					require.NoError(t, err)
					require.True(t, result.Success, "%v", result.Logs)
					require.Equal(t, len(tt.secrets), len(env.Variables))
					require.Len(t, result.Plan.Secrets, len(tt.secrets))

					for _, name := range tt.discardedVariables {
						require.NotContains(t, result.Plan.Deploy.Variables, name)
						for _, step := range result.Plan.Steps {
							require.NotContains(t, step.Variables, name)
						}
					}

					for name, value := range tt.variables {
						require.Contains(t, result.Plan.Deploy.Variables, name)
						require.Equal(t, value, result.Plan.Deploy.Variables[name])
						for _, step := range result.Plan.Steps {
							require.Contains(t, step.Variables, name)
							require.Equal(t, value, step.Variables[name])
						}
						require.NotContains(t, env.Variables, name)
						require.NotContains(t, result.Plan.Secrets, name)
					}
					for name, value := range tt.secrets {
						require.Contains(t, env.Variables, name)
						require.Equal(t, value, env.Variables[name])
						require.Contains(t, result.Plan.Secrets, name)
						require.NotContains(t, result.Plan.Deploy.Variables, name)
						for _, step := range result.Plan.Steps {
							require.NotContains(t, step.Variables, name)
						}
					}
					return nil
				},
			}
			commandArguments := append([]string{"test"}, tt.flagArguments...)
			commandArguments = append(commandArguments, "../examples/shell-script")
			require.NoError(t, cmd.Run(context.Background(), commandArguments))
		})
	}
}

func TestParseEnvironmentFlags_SecretOverridesEnvironmentVariable(t *testing.T) {
	t.Setenv("TOKEN", "process-secret=value")
	t.Setenv("RAILPACK_TEST_MISSING_SECRET", "")
	require.NoError(t, os.Unsetenv("RAILPACK_TEST_MISSING_SECRET"))

	tests := []struct {
		name                  string
		flagArguments         []string
		variables             map[string]string
		secrets               map[string]string
		duplicateVariableName string
	}{
		{name: "explicit values", flagArguments: []string{"--env", "TOKEN=public-value", "--secret", "TOKEN=private-value"}, secrets: map[string]string{"TOKEN": "private-value"}, duplicateVariableName: "TOKEN"},
		{name: "secret process lookup", flagArguments: []string{"--env", "TOKEN=public-value", "--secret", "TOKEN"}, secrets: map[string]string{"TOKEN": "process-secret=value"}, duplicateVariableName: "TOKEN"},
		{name: "missing secret lookup", flagArguments: []string{"--env", "RAILPACK_TEST_MISSING_SECRET=public-value", "--secret", "RAILPACK_TEST_MISSING_SECRET"}, duplicateVariableName: "RAILPACK_TEST_MISSING_SECRET"},
		{name: "both lookups missing", flagArguments: []string{"--env", "RAILPACK_TEST_MISSING_SECRET", "--secret", "RAILPACK_TEST_MISSING_SECRET"}, duplicateVariableName: "RAILPACK_TEST_MISSING_SECRET"},
		{name: "empty secret", flagArguments: []string{"--env", "TOKEN=public-value", "--secret", "TOKEN="}, secrets: map[string]string{"TOKEN": ""}, duplicateVariableName: "TOKEN"},
		{name: "repeated inputs warn once", flagArguments: []string{"--env", "TOKEN=first", "--env", "TOKEN=public-value", "--secret", "TOKEN=first", "--secret", "TOKEN=private-value"}, secrets: map[string]string{"TOKEN": "private-value"}, duplicateVariableName: "TOKEN"},
		{name: "distinct names", flagArguments: []string{"--env", "MODE=production", "--secret", "TOKEN=private-value"}, variables: map[string]string{"MODE": "production"}, secrets: map[string]string{"TOKEN": "private-value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildLogger := logger.NewLogger()
			cmd := &cli.Command{
				Name: "test", Flags: commonPlanFlags(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					variables, secrets, err := parseEnvironmentFlags(cmd, buildLogger)
					require.NoError(t, err)
					require.Len(t, variables, len(tt.variables))
					require.Len(t, secrets, len(tt.secrets))
					for name, value := range tt.variables {
						require.Contains(t, variables, name)
						require.Equal(t, value, variables[name])
					}
					for name, value := range tt.secrets {
						require.Contains(t, secrets, name)
						require.Equal(t, value, secrets[name])
					}
					return nil
				},
			}
			// explicit opt-in suppresses the migration notice so only duplicate warnings are captured
			commandArguments := append([]string{"test", "--future-env"}, tt.flagArguments...)
			require.NoError(t, cmd.Run(context.Background(), commandArguments))
			if tt.duplicateVariableName == "" {
				require.Empty(t, buildLogger.Logs)
				return
			}
			require.Len(t, buildLogger.Logs, 1)
			warning := buildLogger.Logs[0]
			require.Equal(t, logger.Warn, warning.Level)
			require.Contains(t, warning.Msg, tt.duplicateVariableName)
			require.NotContains(t, warning.Msg, "public-value")
			require.NotContains(t, warning.Msg, "private-value")
			require.NotContains(t, warning.Msg, "process-secret=value")
		})
	}
}

// TODO(stable): keep the provider checks but remove the legacy-mode loop and opt-in flag for the 1.0 release
func TestGenerateBuildResultForCommand_EnvAffectsConfigurationAndProvidersInBothModes(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		variable  string
		check     func(*testing.T, *core.BuildResult)
	}{
		{
			name: "start command", directory: "../examples/shell-script", variable: "RAILPACK_START_CMD=echo custom start",
			check: func(t *testing.T, result *core.BuildResult) {
				require.Equal(t, "echo custom start", result.Plan.Deploy.StartCmd)
			},
		},
		{
			name: "Node version", directory: "../examples/node-npm", variable: "RAILPACK_NODE_VERSION=22",
			check: func(t *testing.T, result *core.BuildResult) {
				require.NotNil(t, result.ResolvedPackages["node"])
				require.NotNil(t, result.ResolvedPackages["node"].ResolvedVersion)
				require.Regexp(t, `^22\.`, *result.ResolvedPackages["node"].ResolvedVersion)
			},
		},
		{
			name: "Go CGO", directory: "../examples/go-mod", variable: "CGO_ENABLED=1",
			check: func(t *testing.T, result *core.BuildResult) {
				// with CGO, planning requires a runtime libc package step instead of the bare runtime image
				require.Equal(t, "packages:apt:runtime", result.Plan.Deploy.Base.Step)
			},
		},
	}

	for _, tt := range tests {
		for _, persistVariables := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/new-mode=%t", tt.name, persistVariables), func(t *testing.T) {
				cmd := &cli.Command{
					Name: "test", Flags: commonPlanFlags(),
					Action: func(ctx context.Context, cmd *cli.Command) error {
						result, _, _, err := GenerateBuildResultForCommand(cmd)
						require.NoError(t, err)
						require.True(t, result.Success, "%v", result.Logs)
						tt.check(t, result)
						return nil
					},
				}
				commandArguments := []string{"test", "--env", tt.variable}
				if persistVariables {
					commandArguments = append(commandArguments, "--future-env")
				}
				commandArguments = append(commandArguments, tt.directory)
				require.NoError(t, cmd.Run(context.Background(), commandArguments))
			})
		}
	}
}

// TODO(stable): remove the compatibility notice test when the 1.0 release removes migration notices
func TestShouldPersistEnvironmentVariables_RecordsDeprecationsUnlessExplicitlyEnabled(t *testing.T) {
	tests := []struct {
		name             string
		flagArguments    []string
		optedIn          bool
		persistVariables bool
	}{
		{name: "no flags"},
		{name: "legacy env", flagArguments: []string{"--env", "TOKEN=private-value"}},
		{name: "legacy alias", flagArguments: []string{"-e", "TOKEN=private-value"}},
		{name: "secret only", flagArguments: []string{"--secret", "TOKEN=private-value"}},
		{name: "mixed flags", flagArguments: []string{"--env", "MODE=production", "--secret", "TOKEN=private-value"}, persistVariables: true},
		{name: "explicit false", flagArguments: []string{"--future-env=false", "--env", "TOKEN=private-value"}},
		{name: "mixed flags with explicit false", flagArguments: []string{"--future-env=false", "--env", "MODE=production", "--secret", "TOKEN=private-value"}, persistVariables: true},
		{name: "explicit true without inputs", flagArguments: []string{"--future-env"}, optedIn: true, persistVariables: true},
		{name: "explicit true", flagArguments: []string{"--future-env", "--env", "MODE=production"}, optedIn: true, persistVariables: true},
		{name: "explicit true with mixed flags", flagArguments: []string{"--future-env", "--env", "MODE=production", "--secret", "TOKEN=private-value"}, optedIn: true, persistVariables: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// inspect structured migration notices before providers add their own plan messages
			buildLogger := logger.NewLogger()
			cmd := &cli.Command{
				Name: "test", Flags: commonPlanFlags(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					require.Equal(t, tt.persistVariables, shouldPersistEnvironmentVariables(cmd, buildLogger))
					return nil
				},
			}
			require.NoError(t, cmd.Run(context.Background(), append([]string{"test"}, tt.flagArguments...)))
			if tt.optedIn {
				require.Empty(t, buildLogger.Logs)
				return
			}
			require.Len(t, buildLogger.Logs, 1)
			notice := buildLogger.Logs[0]
			require.Equal(t, logger.Deprecation, notice.Level)
			require.Contains(t, notice.Msg, "In a later release,")
			require.Equal(t, "/architecture/secrets/#migration-to-10", notice.DocsPath)
			require.NotContains(t, notice.Msg, "private-value")
		})
	}
}

func TestGenerateBuildResultForCommand_IncludesEnvironmentLogsInBuildResult(t *testing.T) {
	tests := []struct {
		name          string
		flagArguments []string
		noticeCount   int
		failsToPlan   bool
	}{
		{name: "default migration notice", noticeCount: 1},
		{name: "legacy env notice", flagArguments: []string{"--env", "TOKEN=private-token"}, noticeCount: 1},
		{name: "automatic opt in notice", flagArguments: []string{"--env", "MODE=production", "--secret", "TOKEN=private-token"}, noticeCount: 1},
		{name: "migration and duplicate notices", flagArguments: []string{"--env", "TOKEN=public-token", "--secret", "TOKEN=private-token"}, noticeCount: 2},
		{name: "explicit opt in suppresses migration notice", flagArguments: []string{"--future-env", "--env", "MODE=production"}},
		{name: "explicit opt in retains duplicate warning", flagArguments: []string{"--future-env", "--env", "TOKEN=public-token", "--secret", "TOKEN=private-token"}, noticeCount: 1},
		{name: "notice retained on planning failure", flagArguments: []string{"--config-file", "missing-config.json"}, noticeCount: 1, failsToPlan: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cli.Command{
				Name: "test", Flags: commonPlanFlags(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					result, _, _, err := GenerateBuildResultForCommand(cmd)
					require.NoError(t, err)
					require.Equal(t, !tt.failsToPlan, result.Success)

					var notices []logger.Msg
					for _, message := range result.Logs {
						if message.Level == logger.Warn || message.Level == logger.Deprecation {
							notices = append(notices, message)
						}
					}
					require.Len(t, notices, tt.noticeCount)
					for _, notice := range notices {
						require.Contains(t, core.FormatBuildResult(result), notice.Msg)
						if notice.Level == logger.Deprecation {
							require.Equal(t, "/architecture/secrets/#migration-to-10", notice.DocsPath)
							require.Contains(t, core.FormatBuildResult(result), logger.DocsURL(notice.DocsPath))
						}
						require.NotContains(t, notice.Msg, "public-token")
						require.NotContains(t, notice.Msg, "private-token")
					}
					return nil
				},
			}
			commandArguments := append([]string{"test"}, tt.flagArguments...)
			commandArguments = append(commandArguments, "../examples/shell-script")
			require.NoError(t, cmd.Run(context.Background(), commandArguments))
		})
	}
}

func TestAddSchemaToPlanMap_IncludesSchemaAndPreservesPlan(t *testing.T) {
	// Create a minimal non-empty plan
	p := plan.NewBuildPlan()
	p.Deploy.StartCmd = "run"
	p.Secrets = []string{"FOO"}

	// Marshal the original plan to a map for comparison
	baseBytes, err := json.Marshal(p)
	require.NoError(t, err)
	var baseMap map[string]any
	require.NoError(t, json.Unmarshal(baseBytes, &baseMap))

	// Get the map with schema and validate
	outMap, err := addSchemaToPlanMap(p)
	require.NoError(t, err)
	require.Equal(t, config.SchemaUrl, outMap["$schema"])

	// Remove the added $schema and compare with the original
	delete(outMap, "$schema")
	require.Equal(t, baseMap, outMap)
}

func TestExitCodeForError(t *testing.T) {
	temporary := &mise.TemporaryError{URL: "https://example.com/mise.tar.gz", Err: fmt.Errorf("i/o timeout")}

	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{name: "temporary", err: temporary, expected: ExitCodeTransient},
		{name: "wrapped temporary", err: fmt.Errorf("failed to ensure mise is installed: %w", temporary), expected: ExitCodeTransient},
		{name: "deterministic", err: fmt.Errorf("no start command was found"), expected: ExitCodeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, exitCodeForError(tt.err))
		})
	}
}

func TestAddSchemaToPlanMap_NilPlan(t *testing.T) {
	outMap, err := addSchemaToPlanMap(nil)
	require.NoError(t, err)
	require.Len(t, outMap, 1)
	require.Equal(t, config.SchemaUrl, outMap["$schema"])
}
