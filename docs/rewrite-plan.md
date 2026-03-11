# HTTPDSL Compiler Rewrite Plan

## 1. Purpose

This document defines a complete rewrite strategy for the HTTPDSL compiler backend, with strict behavior parity and controlled rollout.

Primary outcome:

- Replace the current monolithic `compiler/native.go` codegen model with a modular architecture built around:
  - a typed intermediate representation (IR),
  - an explicit backend layer,
  - embedded runtime source/templates,
  - and a parity-first migration path.

## 2. Goals

1. Preserve current language behavior and runtime semantics.
2. Keep a single distributed compiler binary.
3. Make backend logic maintainable, testable, and reviewable.
4. Reduce risk by migrating incrementally behind flags.
5. Enable future backends and/or optimizations without parser rewrites.

## 3. Non-Goals

1. No language syntax changes during rewrite.
2. No end-user feature expansion during rewrite, except fixes needed for parity.
3. No major CLI UX redesign during rewrite.
4. No push to a second output language/runtime in this rewrite.

## 4. Constraints

1. Output remains a native Go binary per app.
2. Compiler itself remains a single binary.
3. Existing DSL projects and tests must continue to build.
4. Existing top-level validation rules remain enforced.

## 5. Design Principles

1. Behavior is a contract: tests define truth.
2. Small, reversible slices over big-bang rewrite.
3. Keep static runtime code as static files, not giant inline strings.
4. Keep dynamic generation focused on DSL-specific wiring.
5. Every migration step has acceptance gates and rollback path.

## 6. Target Architecture

## 6.1 High-level pipeline

1. Lexer/Parser -> AST
2. AST Validation -> validated AST
3. AST Lowering -> IR
4. Backend (IR -> generated Go files)
5. Build stage (`go build`) -> app binary

## 6.2 New packages/directories

- `compiler/frontend/`
  - parser/validation wrappers (can initially adapt existing code)
- `compiler/ir/`
  - IR data model
  - lowering from AST to IR
  - IR validation
- `compiler/backend/goemit/`
  - Go backend emitter from IR
  - expression emitter
  - statement emitter
  - route emitter
- `compiler/runtime/`
  - embedded runtime files/templates
  - render/materialize helpers
- `compiler/pipeline/`
  - orchestration and feature flags

## 6.3 Runtime code strategy

Static runtime source should live as files under `compiler/runtime/templates/` (or `compiler/runtime/src/`), embedded via `go:embed`.

Compiler writes multiple files into temp build dir, for example:

- `main.go`
- `runtime_core.go`
- `runtime_session.go`
- `runtime_sse.go`
- `runtime_db.go`
- `gen_routes.go`
- `gen_functions.go`

This preserves single compiler binary while making runtime code readable and testable.

## 7. IR Plan

## 7.1 IR requirements

IR must be:

1. Explicit about runtime features (session, csrf, sse, db, templates, cron).
2. Decoupled from parser token details.
3. Structured enough for deterministic code emission.
4. Validatable before backend emission.

## 7.2 Initial IR shape

- Program
  - ServerConfig
  - Globals (`init`, `before`, `after`, `shutdown`)
  - Routes (including groups merged/scoped)
  - ErrorHandlers
  - Schedules (`every` interval/cron)
  - Functions
  - FeatureFlags
- Statements and expressions in typed IR nodes mirroring current AST semantics.

## 7.3 IR validation

Checks include:

1. Top-level restrictions.
2. Route rule constraints (`disconnect` only for SSE routes, etc).
3. Scheduler validity (cron already parse-validated, IR keeps normalized shape).
4. Hook ordering and scope flattening.

## 8. Migration Strategy

Use dual backend mode:

- Legacy backend: current `native.go` path.
- New backend: IR + goemit path.
- Selection flag: `HTTPDSL_BACKEND=legacy|ir` (or CLI hidden flag).

Default remains `legacy` until final cutover.

## 9. Milestones

## M0. Baseline and parity harness

Deliverables:

1. Consolidated parity test command (build + run assertions).
2. Golden snapshots for representative generated Go output (selected fixtures).
3. Failure diagnostics that show semantic diffs, not just text diffs.

Acceptance criteria:

1. Existing tests pass on current backend.
2. Golden baseline committed for selected fixtures.

Rollback:

- N/A (baseline phase).

## M1. Runtime asset platform

Deliverables:

1. Embedded runtime renderer in `compiler/runtime`.
2. Extract large static blocks from legacy generator into templates/files.
3. No behavior changes.

Acceptance criteria:

1. Test suite passes unchanged.
2. Generated binaries and key response behaviors match baseline.

Rollback:

- Revert extraction commit(s), keep old inline emission.

## M2. Introduce IR model and lowering

Deliverables:

1. `compiler/ir` package with core structs.
2. AST -> IR lowering for current supported constructs.
3. IR validation layer.

Acceptance criteria:

1. Lowering covers all existing parser constructs used by tests/examples.
2. IR validation catches known invalid states.

Rollback:

- Keep IR package unused by default if gaps found.

## M3. New expression backend

Deliverables:

1. IR expression emitter with parity tests.
2. Side-by-side comparison harness with legacy expression emitter output semantics.

Acceptance criteria:

1. Expression-heavy tests pass with `HTTPDSL_BACKEND=ir` where statement handling is still delegated or adapted.

Rollback:

- Route expression emission routed back to legacy path.

## M4. New statement/control-flow backend

Deliverables:

1. Assignment, return, if/switch/while/each, try/catch/throw emission from IR.
2. Unit tests per statement family.

Acceptance criteria:

1. Control-flow tests pass under IR backend.

Rollback:

- Statement families can be toggled to legacy emitter selectively.

## M5. Route/hook/backend wiring

Deliverables:

1. Full route generation from IR.
2. Group/before/after/init/shutdown/error/every wiring from IR.
3. Response lifecycle parity (including after semantics).

Acceptance criteria:

1. Route and middleware tests pass under IR backend.
2. No regressions in top-level validation behavior.

Rollback:

- Route emission fallback to legacy while keeping IR for other pieces.

## M6. Advanced subsystems

Deliverables:

1. Session/CSRF runtime integration parity.
2. DB runtime parity (sqlite/postgres/mysql/mongo paths).
3. SSE runtime parity.
4. Template runtime parity.

Acceptance criteria:

1. Subsystem tests pass under IR backend.
2. Build output remains stable for representative apps.

Rollback:

- Feature flags to route specific subsystem generation to legacy.

## M7. Cutover and cleanup

Deliverables:

1. IR backend set as default.
2. Legacy backend removed or archived behind build tag.
3. Documentation and contributor guide updated.

Acceptance criteria:

1. Full test suite green with IR default.
2. No open parity blockers.
3. Perf/build metrics within agreed thresholds.

Rollback:

- Toggle default back to legacy until blockers are resolved.

## 10. Test Strategy

## 10.1 Parity checks

1. Existing integration tests (`tests/run.sh`) remain primary gate.
2. Add fixtures covering edge semantics:
   - top-level assignment rejection,
   - `after {}` post-response behavior,
   - cron validation,
   - session cookie lifecycle,
   - SSE route semantics.

## 10.2 Golden tests

1. Golden tests on normalized generated Go fragments for key fixtures.
2. Focus on semantic sections (routes, runtime wiring), not full file exactness where unstable.

## 10.3 Differential execution

1. Compile same fixture with both backends.
2. Run HTTP scenario scripts.
3. Compare status/body/header/cookie outputs.

## 11. Performance and Reliability Gates

Track on each milestone:

1. Compiler build time for standard fixtures.
2. Generated binary size (trend, not strict byte parity).
3. Runtime smoke latency for fixed scenario set.
4. Panic/crash rate under test matrix.

## 12. Branching and Delivery

1. Use branch prefix `codex/` for rewrite slices.
2. Keep milestones as small mergeable PR-sized commits.
3. No commit should break legacy backend default path.
4. Use feature flags for incomplete subsystems.

## 13. Risk Register

1. Hidden semantic regressions in route lifecycle.
   - Mitigation: explicit parity scenarios around response/session/hook ordering.
2. Dual-backend complexity drags timeline.
   - Mitigation: strict milestone boundaries and scheduled removal date.
3. Template sprawl without ownership.
   - Mitigation: file naming conventions and subsystem ownership map.
4. Build instability from multi-file generation.
   - Mitigation: deterministic file emission order + golden checks.

## 14. Immediate Execution Plan (First 2-3 Weeks)

Week 1:

1. M0 baseline harness hardening.
2. Expand fixtures for known fragile semantics.
3. M1 extraction of remaining large static runtime blocks.

Week 2:

1. Create `compiler/ir` model and AST lowering skeleton.
2. Implement IR validation for top-level and route constraints.
3. Add dual-backend flag plumbing in pipeline.

Week 3:

1. Implement expression emission in `backend/goemit`.
2. Wire first end-to-end fixture path through IR backend.
3. Run differential tests and close first parity gaps.

## 15. Done Definition

Rewrite is complete when:

1. IR backend is default and stable.
2. Legacy backend is removed or archived and unused in normal builds.
3. Full test and parity suite passes in CI and local runs.
4. Runtime code and backend code are modular and contributor-documented.
