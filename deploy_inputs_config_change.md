# Plan: Make `deploy.inputs` Authoritative

## Summary

Change deploy composition so an explicit `deploy.inputs` list without a
`...` entry replaces every generated deploy input, including all explicit and
implicit `deployOutputs` from every configured step.

Treat step-level `deployOutputs` as generated deploy inputs. This gives `...`
one consistent meaning: retain all inputs Railpack would have generated if
`deploy.inputs` had been omitted.

This plan covers only the functionality change. The warning period and user
communication are tracked in
[`deploy_inputs_deprecation_notice.md`](deploy_inputs_deprecation_notice.md)
and must be completed before this change ships.

## Target Semantics

Apply these rules to the final merged configuration:

- Omitted or `null` inputs keep provider inputs and all step outputs.
- `deploy.inputs: []` deploys no inputs.
- Explicit inputs without `...` become the complete deploy input list.
- Inputs containing `...` replace each spread entry with every generated
  input, including implicit and explicit step outputs.

Step output generation remains:

- Omitted or `null` `deployOutputs` generates the implicit `.` output.
- `deployOutputs: []` generates no output for that step.
- A non-empty `deployOutputs` list generates its specified filters.

The difference is that all of these step outputs are generated before
`deploy.inputs` is applied. None can bypass replacement afterward.

## Examples

This configuration deploys only `other`:

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

This configuration deploys generated outputs, including `dist`, followed by
`other`:

```json
{
  "steps": {
    "build": {
      "deployOutputs": [{ "include": ["dist"] }]
    }
  },
  "deploy": {
    "inputs": [
      "...",
      { "step": "build", "include": ["other"] }
    ]
  }
}
```

## Implementation Plan

### 1. Defer configured input spreading

In `GenerateContext.applyConfig`, retain the final configured
`c.Config.Deploy.Inputs` value but do not immediately spread it into
`c.Deploy.DeployInputs`.

Continue applying unrelated deploy settings in their current location:

- Base image.
- Start command.
- Apt packages.
- Paths.
- Variables.

At this point, `c.Deploy.DeployInputs` should contain only provider-generated
inputs.

### 2. Generate all step outputs

Process configured steps against the generated input list:

1. Apply commands, inputs, secrets, caches, variables, and assets.
2. For omitted or `null` `deployOutputs`, generate the implicit `.` filter.
3. For `deployOutputs: []`, generate no layer.
4. For non-empty `deployOutputs`, generate each explicit filter.
5. Append each uncovered output to the generated deploy input list.

Do not suppress explicit step output filters merely because another input
references the same step. Two filters from one step can intentionally select
different paths.

### 3. Preserve generated-input deduplication

Keep `HasInputForStep` for implicit defaults:

- If a provider input already references a configured step, do not append a
  second implicit `.` input.
- This prevents duplicate full-step copies and avoids broadening a provider
  filter with an implicit default.

Keep filter-aware deduplication for explicit `deployOutputs`:

- Do not add the same include path twice.
- Do not add a narrower filter when an existing `.` input already covers it.
- Do not discard a distinct explicit filter solely because the step already
  has another filtered input.

If necessary, clarify how an unfiltered layer with an empty `Include` slice is
treated. It should be considered a full-step input for coverage purposes.

### 4. Apply `deploy.inputs` once

After provider and step outputs have been generated, calculate the final list:

```go
c.Deploy.DeployInputs = plan.Spread(
    c.Config.Deploy.Inputs,
    generatedDeployInputs,
)
```

Guard access when `c.Config.Deploy` is nil. A nil configured list must preserve
the generated list through the existing `plan.Spread` behavior.

Applying spread exactly once and at the end guarantees:

- An empty list removes every generated input.
- A replacement list cannot be repopulated by later step processing.
- `...` includes provider, implicit, and explicit step outputs.
- Generated inputs appear at the spread operator's exact position.

### 5. Remove transitional branching

Remove `replacesGeneratedDeployInputs` from the step loop. Replacement should
be expressed by the final `plan.Spread` operation rather than by selectively
skipping different categories of generated input.

Retain `HasInputForStep`, but use it only while building the generated list.

## Unit Test Plan

Update `TestGenerateContextDeployInputs` to cover:

1. Omitted inputs retain provider-generated inputs.
2. Omitted inputs retain implicit step outputs.
3. Omitted inputs retain explicit step `deployOutputs`.
4. Empty inputs suppress provider inputs.
5. Empty inputs suppress implicit step outputs.
6. Empty inputs suppress explicit step `deployOutputs`.
7. Replacement inputs suppress explicit outputs from the same step.
8. Replacement inputs suppress explicit outputs from every other step.
9. Replacement inputs suppress implicit outputs from every configured step.
10. Spread inputs retain provider-generated inputs.
11. Spread inputs retain implicit step outputs.
12. Spread inputs retain explicit step `deployOutputs`.
13. Generated inputs appear at the exact position of `...`.
14. Existing full-step inputs are not duplicated.
15. Distinct explicit filters from one step remain additive before spreading.
16. `deployOutputs: []` remains an explicit step-level opt-out.

Replace the current test named:

```text
explicit deploy outputs remain additive
```

with two tests:

```text
replacement inputs override explicit deploy outputs
spread inputs retain explicit deploy outputs
```

Use exact layer-list assertions where ordering is part of the contract.

## Integration Test Plan

Use `examples/config-file` to verify actual image composition.

For `railpack.other.json`:

- Select `/hello` directly through a filtered `deploy.inputs` step layer.
- Do not select the `playwright` step that creates `/boop`.
- Keep `/usr/games` in `deploy.paths` so runtime `cowsay` is independent of
  build-step layers.

In `custom-start.sh`:

- Assert `/hello` contains `world`.
- Assert `/boop` does not exist.

Retain a separate integration case using `deploy.inputs: ["..."]` to verify
generated inputs remain available when spread is requested.

## Documentation and Schema Updates

Update `/config/file` to describe the active behavior:

- `deploy.inputs` replaces generated inputs by default.
- Replacement includes all implicit and explicit step outputs.
- `...` retains provider and step-generated inputs.
- `deploy.inputs: []` intentionally produces no deploy inputs.
- Required step filters can be expressed directly in `deploy.inputs`.

Update schema descriptions:

- `DeployConfig.Inputs`: document replacement and spread semantics.
- `StepConfig.DeployOutputs`: identify these as generated inputs that can be
  replaced by `deploy.inputs`.

Update release notes to state that the previously announced behavior change is
now active.

## Validation

Run in this order:

1. `mise run check`
2. `mise run test`
3. `mise run test-integration-cwd -- 0` from `examples/config-file`
4. `mise run test-integration-cwd -- 1` from `examples/config-file`
5. Inspect the generated plans to confirm input ordering and the absence of
   unselected step layers.

## Non-goals

- Implementing or changing the deprecation warning.
- Adding `...` support inside `deployOutputs`.
- Changing `include` or `exclude` matching semantics.
- Changing unrelated config-list merge behavior.
- Giving `deploy.inputs` different meanings based on provider or step type.

## Acceptance Criteria

- `deploy.inputs` without `...` is the sole source of final deploy inputs.
- No explicit or implicit step output can bypass replacement.
- `deploy.inputs: []` produces no deploy inputs.
- `...` retains provider inputs and every generated step output.
- Generated inputs are inserted at the spread operator's position.
- Existing full-step inputs are not duplicated or broadened.
- Distinct explicit filters remain available when generated inputs are kept.
- Unit and integration tests verify plan and runtime-image composition.
- Documentation, schema text, examples, and release notes agree.
