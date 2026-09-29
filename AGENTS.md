# AGENTS.md — plugin-service

Standalone plugin repo for the `service` typed-step verb (`verb:service`). The
plugin is a Go module at `candy/plugin-service/` (module path
`github.com/opencharly/plugin-service/candy/plugin-service`); the root
`charly.yml` only declares `discover: candy` so the repo is a project and its
candy is scanned.

Canonical files:

- `candy/plugin-service/charly.yml` — the `plugin-service:` candy entity
  (`plugin:` block, `plan:` check).
- `candy/plugin-service/plugin.go` — the verb implementation
  (`CheckVerbProvider` + `ProvisionActor` + `StepProvider`) + `NewMeta()`.
- `candy/plugin-service/failed_units.go` — the failed-unit reporting helper.
- `candy/plugin-service/schema/service.cue` — the self-contained input schema.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, the typed-step contract, the per-plugin
  CUE-schema contract, placement. Load before touching the provider or schema.
- `/charly-core:service` — the service lifecycle surface this verb provisions.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-service/` — compile the plugin module.
- `go test ./...` in `candy/plugin-service/` — the plugin's Go tests
  (`plugin_test.go`, `failed_units_test.go`, `step_provider_coverage_test.go`).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- There is no dedicated live bed: the verb's evidence is its `plan:` `check:`
  step plus the Go tests.

## Modify this repo

- Edit the `plugin-service:` candy entity, the Go source, and
  `schema/service.cue` **together** — the schema is the single source for the
  `params/` struct, so a field change not mirrored in the schema desyncs the
  generated types.
- This plugin is **compiled-in only** (a host-coupled kit verb); do not describe
  it as out-of-process.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
