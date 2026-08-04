# Plan: Deprecate Additive Step `deployOutputs`

## Purpose

Warn users before changing `deploy.inputs` so a replacement list overrides
explicit step `deployOutputs`.

This document covers only the compatibility notice period. The eventual
behavior change is planned separately in
[`deploy_inputs_config_change.md`](deploy_inputs_config_change.md).

## Behavior Being Deprecated

Today, an explicit `deploy.inputs` list replaces provider-generated and
implicit step inputs, but non-empty explicit `deployOutputs` can still be
appended afterward.

For example, this currently deploys both `other` and `dist`:

```json
{
  "steps": {
    "build": {
      "deployOutputs": [{ "include": ["dist"] }]
    }
  },
  "deploy": {
    "inputs": [{ "step": "build", "include": ["other"] }]
  }
}
```

After the compatibility period, the same configuration will deploy only
`other`. Users must add `...` if they want to retain generated inputs,
including step `deployOutputs`.

## Warning Trigger

Evaluate the final merged configuration and emit a warning only when all of
these conditions are true:

1. `deploy.inputs` is non-nil.
2. `deploy.inputs` contains no spread layer.
3. At least one configured step has a non-empty explicit `deployOutputs` list.

Do not warn when:

- `deploy.inputs` is omitted or `null`.
- `deploy.inputs` contains `...`.
- Every `deployOutputs` field is omitted, `null`, or empty.

Emit at most one warning and one suggestion per generated plan, regardless of
how many steps are affected.

## User-facing Messages

Deprecation warning:

> `deploy.inputs` without a `...` entry currently retains step
> `deployOutputs`; in a future release it will replace all of them

Suggestion:

> Add `...` to `deploy.inputs` to retain generated inputs and step
> `deployOutputs`

Link the suggestion to `/config/file`.

## Implementation

1. Add a helper that identifies a replacement `deploy.inputs` list:
   - The slice is non-nil.
   - No layer in the slice has `Spread` set.
2. Add a helper that detects non-empty explicit `deployOutputs` across all
   configured steps.
3. Run both checks against the final merged config in
   `GenerateContext.applyConfig`.
4. Log the deprecation and suggestion once when both checks return true.
5. Preserve the current additive behavior throughout the notice period.

Do not inspect individual config sources. A value overridden by a later config
must not produce a stale warning.

## Tests

Add focused tests proving:

1. Replacement inputs plus one non-empty `deployOutputs` logs one warning.
2. Multiple affected steps still log only one warning and one suggestion.
3. Warning text, suggestion text, levels, and docs path match exactly.
4. Omitted or `null` `deploy.inputs` does not warn.
5. A list containing `...` does not warn.
6. Empty, omitted, or `null` `deployOutputs` does not warn.
7. Config merging is evaluated before warning detection.
8. Current additive plan output remains unchanged during the notice period.

Run:

1. `mise run check`
2. `mise run test`
3. Relevant config-file integration cases if fixtures are changed.

## Documentation and Communication

Update `/config/file` during the notice period with:

- The current additive behavior.
- The future replacement behavior.
- The target release or date for the change.
- A `...` migration example.
- An explicit-input migration example.

Include the warning and migration instructions in release notes. Keep the
notice active for at least one announced release cycle.

## Migration Guidance

Users who want to preserve generated inputs should add `...`:

```json
"deploy": {
  "inputs": ["...", { "local": true, "include": ["config.json"] }]
}
```

Users who want complete control should move every required step output into
`deploy.inputs`:

```json
"deploy": {
  "inputs": [
    { "step": "build", "include": ["dist"] },
    { "local": true, "include": ["config.json"] }
  ]
}
```

After moving a filter, remove its corresponding `deployOutputs` entry to avoid
ambiguity and the warning.

## Repository Fixture Migration

Update Railpack-owned examples before enabling the warning so normal test runs
do not emit deprecations.

For `examples/config-file/railpack.other.json`:

- Move the `/hello` filter from `custom.deployOutputs` into `deploy.inputs`.
- Keep `/boop` unselected so the runtime assertion continues to detect leaks.
- Keep `/usr/games` in `deploy.paths` so runtime `cowsay` remains explicit.

## Completion Criteria

- Affected builds receive one actionable warning.
- Unaffected builds receive no warning.
- Documentation and release notes identify the behavior-change release.
- Railpack-owned fixtures no longer rely on the deprecated behavior.
- The notice has shipped for the announced compatibility period.
- The functionality change can proceed using
  [`deploy_inputs_config_change.md`](deploy_inputs_config_change.md).

## Out of Scope

- Changing deploy composition behavior.
- Reordering generated and configured inputs.
- Removing the warning after the behavior change.
- Adding `...` support inside `deployOutputs`.
