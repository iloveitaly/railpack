package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/railwayapp/railpack/core"
	a "github.com/railwayapp/railpack/core/app"
	"github.com/railwayapp/railpack/core/config"
	"github.com/railwayapp/railpack/core/logger"
	"github.com/railwayapp/railpack/core/mise"
	"github.com/railwayapp/railpack/core/plan"
	"github.com/railwayapp/railpack/internal/utils"
	"github.com/urfave/cli/v3"
)

var Version string // This will be set by main

const (
	// the command failed for a reason that will not change on a retry
	ExitCodeFailure = 1

	// EX_TEMPFAIL: the command failed for a transient reason (e.g. a network error
	// downloading mise) and is worth retrying. Platforms that run Railpack in a
	// build pipeline key their retry logic on this.
	ExitCodeTransient = 75
)

func exitCodeForError(err error) int {
	if mise.IsTemporary(err) {
		return ExitCodeTransient
	}

	return ExitCodeFailure
}

func commonPlanFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Usage:   "build secret by default; build and runtime variable with --secret or --future-env",
		},
		&cli.StringSliceFlag{
			Name:  "secret",
			Usage: "sensitive environment variable available at build time",
		},
		// TODO(stable): remove this flag when --env always sets environment variables in the 1.0 release
		&cli.BoolFlag{
			Name:  "future-env",
			Usage: "enable the future --env behavior: persist values during build and runtime (temporary compatibility flag, removed in 1.0)",
		},
		&cli.StringSliceFlag{
			Name:  "previous",
			Usage: "versions of packages used for previous builds (e.g. 'package@version')",
		},
		&cli.StringFlag{
			Name:  "build-cmd",
			Usage: "build command to use",
		},
		&cli.StringFlag{
			Name:  "start-cmd",
			Usage: "start command to use",
		},
		&cli.StringFlag{
			Name:  "config-file",
			Usage: "relative path to railpack config file (default: railpack.json)",
		},
		&cli.BoolFlag{
			Name:  "error-missing-start",
			Usage: "error if no start command is found",
		},
	}
}

// extract deduplicated variable names declared through --env or --secret
//
// - flag values use NAME or NAME=VALUE form
// - process lookups are left to the caller, so a missing --secret lookup still suppresses a duplicate --env value
//
// ["TOKEN=abc=123", "API_KEY", "TOKEN=other"] -> {"TOKEN": {}, "API_KEY": {}}
func extractVariableNamesFromFlagArgs(variableFlagValues []string) map[string]struct{} {
	variableNames := make(map[string]struct{}, len(variableFlagValues))
	for _, variableFlagValue := range variableFlagValues {
		variableName, _, _ := strings.Cut(variableFlagValue, "=")
		variableNames[variableName] = struct{}{}
	}
	return variableNames
}

// select persistent --env behavior and record a migration notice unless explicitly opted in
//
// TODO(stable): remove mode selection and migration notices when environment-variable behavior becomes the 1.0 default
func shouldPersistEnvironmentVariables(command *cli.Command, planLogger *logger.Logger) bool {
	if command.Bool("future-env") {
		return true
	}

	// flag presence determines the mode even when a name-only process lookup is missing
	hasEnvironmentAndSecretFlags := command.IsSet("env") && command.IsSet("secret")
	migrationNotice := "In a later release, --env/-e will always set build and runtime environment variables; use --secret for build secrets. Opt in now with --future-env."
	if hasEnvironmentAndSecretFlags {
		migrationNotice = "Using --env with --secret makes --env values persist in the build plan and runtime image, except names also supplied through --secret. " + migrationNotice
	}
	planLogger.LogDeprecation(migrationNotice, "/architecture/secrets/#migration-to-10")

	return hasEnvironmentAndSecretFlags
}

// separate persistent variable values from build-secret values according to the compatibility mode
//
// - --env stays a build secret until --future-env or mixed --env/--secret flags enable persistence
// - explicit --secret declarations win duplicate names, including missing process lookups
//
// --env MODE=prod --secret TOKEN=abc -> variables={"MODE": "prod"}, secrets={"TOKEN": "abc"}
// --env TOKEN=abc -> variables=nil, secrets={"TOKEN": "abc"}
//
// TODO(stable): remove legacy routing when environment-variable behavior becomes the 1.0 default
func parseEnvironmentFlags(command *cli.Command, planLogger *logger.Logger) (environmentVariables map[string]string, buildSecrets map[string]string, err error) {
	environmentFlagValues := command.StringSlice("env")
	environmentVariables, err = a.ParseVariableFlagValues(environmentFlagValues)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing --env values: %w", err)
	}

	secretFlagValues := command.StringSlice("secret")
	buildSecrets, err = a.ParseVariableFlagValues(secretFlagValues)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing --secret values: %w", err)
	}

	if !shouldPersistEnvironmentVariables(command, planLogger) {
		maps.Copy(buildSecrets, environmentVariables)
		return nil, buildSecrets, nil
	}

	environmentVariableNames := extractVariableNamesFromFlagArgs(environmentFlagValues)
	declaredSecretNames := extractVariableNamesFromFlagArgs(secretFlagValues)
	for variableName := range environmentVariableNames {
		if _, declaredAsSecret := declaredSecretNames[variableName]; !declaredAsSecret {
			continue
		}
		planLogger.LogWarn("Variable %q was supplied through both --env and --secret; ignoring the --env value and honoring --secret", variableName)
		delete(environmentVariables, variableName)
	}

	return environmentVariables, buildSecrets, nil
}

func GenerateBuildResultForCommand(cmd *cli.Command) (*core.BuildResult, *a.App, *a.Environment, error) {
	directory := cmd.Args().First()

	if directory == "" {
		return nil, nil, nil, cli.Exit("directory argument is required", ExitCodeFailure)
	}

	app, err := a.NewApp(directory)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating app: %w", err)
	}

	log.Debugf("Building %s", app.Source)

	// share the plan logger so input notices and provider messages retain their order and log types
	planLogger := logger.NewLogger()
	environmentVariables, buildSecrets, err := parseEnvironmentFlags(cmd, planLogger)
	if err != nil {
		return nil, nil, nil, err
	}
	env := a.NewEnvironment(&buildSecrets)

	// if --verbose is passed as a CLI global argument, enable verbose mise logging so the user don't have to understand
	// the railpack build system deeply to get this debugging information.
	if cmd.Bool("verbose") && env.GetVariable("MISE_VERBOSE") == "" {
		env.SetVariable("MISE_VERBOSE", "1")
	}

	previousVersions := utils.ParsePackageWithVersion(cmd.StringSlice("previous"))

	generateOptions := &core.GenerateBuildPlanOptions{
		RailpackVersion:          Version,
		BuildCommand:             cmd.String("build-cmd"),
		StartCommand:             cmd.String("start-cmd"),
		PreviousVersions:         previousVersions,
		ConfigFilePath:           cmd.String("config-file"),
		ErrorMissingStartCommand: cmd.Bool("error-missing-start"),
		Variables:                environmentVariables,
		Logger:                   planLogger,
	}

	buildResult, err := core.GenerateBuildPlan(app, env, generateOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	return buildResult, app, env, nil
}

// add $schema link to resulting map JSON for improved IDE experience when manually editing
func addSchemaToPlanMap(p *plan.BuildPlan) (map[string]any, error) {
	if p == nil {
		return map[string]any{"$schema": config.SchemaUrl}, nil
	}
	planBytes, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var planMap map[string]any
	if err := json.Unmarshal(planBytes, &planMap); err != nil {
		return nil, err
	}
	planMap["$schema"] = config.SchemaUrl
	return planMap, nil
}
