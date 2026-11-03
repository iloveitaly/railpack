package generate

import (
	"slices"

	"github.com/railwayapp/railpack/core/plan"
	"github.com/railwayapp/railpack/internal/utils"
)

// resolve step selectors and collect all names needed by BuildKit without changing the input steps
//
// wildcards expand only to top-level names, keeping inferred names scoped to their declaring steps
//
// configured=["GLOBAL"], steps=[{Secrets:["*", "LOCAL"]}]
// -> names=["GLOBAL", "LOCAL"], steps=[{Secrets:["GLOBAL", "LOCAL"]}]
func resolvePlanSecrets(configuredSecretDeclarations []string, buildSteps []plan.Step) (planSecretNames []string, resolvedSteps []plan.Step) {
	configuredSecretNames := filterSecretNamesFromDeclarations(configuredSecretDeclarations)
	planSecretNames = collectPlanSecretNames(configuredSecretNames, buildSteps)
	preserveWildcardForCacheInvalidation := len(planSecretNames) == 0
	resolvedSteps = slices.Clone(buildSteps)

	for stepIndex := range resolvedSteps {
		resolvedSteps[stepIndex].Secrets = resolveStepSecretDeclarations(
			resolvedSteps[stepIndex].Secrets,
			configuredSecretNames,
			preserveWildcardForCacheInvalidation,
		)
	}

	return planSecretNames, resolvedSteps
}

func collectPlanSecretNames(configuredSecretNames []string, buildSteps []plan.Step) []string {
	secretNames := slices.Clone(configuredSecretNames)
	for _, step := range buildSteps {
		secretNames = append(secretNames, filterSecretNamesFromDeclarations(step.Secrets)...)
	}

	return utils.RemoveDuplicates(secretNames)
}

// resolve a step's secret declarations for cache invalidation
//
// - "*" expands to root-configured names; plans with no named secrets keep "*" for cache invalidation
// - "..." was handled when merging step configuration and must not reach BuildKit as a secret name
//
// (["*", "LOCAL", "..."], ["GLOBAL"], false) -> ["GLOBAL", "LOCAL"]
// (["*"], [], true) -> ["*"]
func resolveStepSecretDeclarations(stepSecretDeclarations, configuredSecretNames []string, preserveWildcardForCacheInvalidation bool) []string {
	resolvedSecretNames := make([]string, 0, len(stepSecretDeclarations)+len(configuredSecretNames))
	for _, secretDeclaration := range stepSecretDeclarations {
		switch secretDeclaration {
		case "*":
			if preserveWildcardForCacheInvalidation {
				resolvedSecretNames = append(resolvedSecretNames, secretDeclaration)
				continue
			}
			resolvedSecretNames = append(resolvedSecretNames, configuredSecretNames...)
		case "...":
			continue
		default:
			resolvedSecretNames = append(resolvedSecretNames, secretDeclaration)
		}
	}

	return utils.RemoveDuplicates(resolvedSecretNames)
}

func filterSecretNamesFromDeclarations(secretDeclarations []string) []string {
	return slices.DeleteFunc(slices.Clone(secretDeclarations), func(secretDeclaration string) bool {
		return secretDeclaration == "*" || secretDeclaration == "..."
	})
}
