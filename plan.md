# Implementation Plan

## Task Overview
- **Source**: TL-561
- **Title**: Make CLI stdin body handling explicit and regression-tested
- **Description**: `linear issues create/update`, project create/update, and issue comment/reply commands currently describe implicit piped stdin even though stdin is only read when the corresponding flag value is explicitly `-`. The implementation must make that contract unambiguous, preserve the historical non-blocking behavior, and add coverage proving body values reach the service/API inputs.

## Requirements Analysis
- **Core Functionality**:
  - Standardize the documented convention: `--description -` for issue/project descriptions and `--body -` for comments/replies.
  - Keep stdin consumption opt-in only; do not restore automatic TTY/non-TTY detection.
  - Preserve ordinary flag values and current `strings.TrimSpace` handling of explicit stdin.
  - Preserve current empty-input behavior, including the existing update/clearing semantics, unless tests reveal only documentation or helper behavior needs clarification.
  - Ensure explicit stdin content reaches issue create/update, project create/update, comment, and reply inputs.
- **Acceptance Criteria**:
  - Every affected command's help text and examples explicitly show the `-` value and no longer imply that an unadorned pipe is consumed.
  - README and embedded CLI skill documentation contain no stale implicit-pipe examples or wording.
  - Tests cover the shared resolver, whitespace trimming, empty input, explicit propagation through all affected command surfaces, and the non-consuming implicit path.
  - Existing ordinary `--description <text>`/`--body <text>`, attachments, and service-layer behavior remain intact.
- **Technical Constraints**:
  - `readStdin` currently reads `os.Stdin` directly and trims leading/trailing whitespace.
  - A prior fix (commit `2a47758`) intentionally removed automatic pipe detection because open stdin could make updates hang in CI, Claude Code, or scripts; this must not regress.
  - Tests should not depend on mutating process-global stdin or on a live Linear API.
  - No service or GraphQL schema change is required; the defect is at CLI input resolution and documentation.
- **Integration Points**:
  - Shared helper in `internal/cli/helpers.go` is used by issue/project descriptions and issue comment/reply bodies.
  - Cobra command closures in `internal/cli/issues.go` and `internal/cli/projects.go` assemble service inputs.
  - `Dependencies`, `NewCmdWithDeps`, and the service interfaces provide test injection; the raw client path used by comments may require an HTTP test transport or a narrowly scoped test seam.
  - README and `internal/skills/linear/SKILL.md` are user-facing documentation surfaces to audit and update.

## Codebase Analysis
- **Existing Patterns**:
  - `readStdin` uses a `bufio.Reader` over `os.Stdin` and returns `strings.TrimSpace(builder.String())` (`internal/cli/helpers.go:24-41`).
  - `getDescriptionFromFlagOrStdin` reads only for the exact flag value `"-"` (`helpers.go:63-71`); all affected commands call it before building service inputs.
  - Issue create resolves the body before attachment processing and passes it as `CreateIssueInput.Description` (`issues.go:375-394`); issue update does the same for `UpdateIssueInput.Description` (`issues.go:560-582`).
  - Project create/update follow the same resolver pattern (`projects.go:194-205`, `projects.go:285-299`). Reply already uses the injectable `IssueServiceInterface`; comment currently resolves the issue and creates the comment through `deps.Client` (`issues.go:697-725`).
  - Existing tests are table-driven Go tests, but `helpers_test.go` currently covers only date parsing. `NewCmdWithDeps` injects command dependencies, while service interfaces use hand-written test doubles in existing service tests.
- **Available Infrastructure**:
  - `Dependencies` exposes issue/project/user services and the Linear client (`internal/cli/dependencies.go`).
  - `pkg/linear/core` supports test HTTP clients/transports, and `pkg/linear/testutil` contains mock transport helpers for API-level assertions.
  - `make test` is the repository-wide verification command.
- **Dependencies**:
  - Likely changes are confined to CLI helpers, affected Cobra commands, CLI tests, and documentation. Service input types and GraphQL mutations should remain unchanged.
- **Architecture Notes**:
  - The explicit `-` check is the safety boundary: the implementation should make this boundary clearer rather than infer intent from stdin state.
  - Extract a pure `readStdinFrom(io.Reader)` helper and keep `readStdin()` as the production wrapper over `os.Stdin`; this gives tests isolated readers without a mutable package-global override.
  - Use `NewCmdWithDeps` and complete test doubles for the issue/project service interfaces. Tests must pass explicit `--team` values so they do not depend on local config. For comment, preserve the existing output and raw-client path and use a test-local sequential recording `http.RoundTripper` that handles issue resolution and comment mutation responses while capturing the mutation variables; do not broaden public APIs.

## Implementation Strategy
- **Problem Complexity**: Moderate, cross-cutting CLI contract/documentation bug with a small behavior surface and a significant regression-testing gap.
- **Core Problem**: The code already implements safe explicit stdin handling, but help and examples promise a different interface and lack tests proving the safe path is used consistently.
- **Approach**: Preserve the exact-`-` behavior and trimming policy, make the stdin reader testable, update all affected help/docs/error text to state the explicit convention, then add command-level propagation tests through dependency injection and a bounded non-consumption test.
- **Testing Approach**: Automated unit and CLI-level tests, followed by the full Go test suite. No manual GUI verification is relevant.
- **Phases**:
  1. **Stabilize and test the shared stdin contract**
     - **Implementation**:
       - Extract a pure `readStdinFrom(io.Reader)` helper and keep the production `readStdin()` wrapper sourced from `os.Stdin`.
       - Keep `getDescriptionFromFlagOrStdin` exact-match behavior (`"-"` reads; all other values return unchanged) and retain `strings.TrimSpace`.
       - Add helper tests for ordinary flag values, explicit non-empty stdin, leading/trailing whitespace and final newlines, empty stdin, reader errors, and a reader that records whether it was touched.
     - **Verification**:
       - Run the focused CLI helper tests. Confirm explicit `-` returns trimmed content, empty stdin remains empty, ordinary values bypass the reader, and no implicit path attempts a read.
  2. **Cover propagation through affected commands**
     - **Implementation**:
       - Add command-level tests using `NewCmdWithDeps`, complete issue/project service test doubles, and explicit `--team` values to capture `CreateIssueInput.Description`, `UpdateIssueInput.Description`, `CreateProjectInput.Description`, and `UpdateProjectInput.Description` for explicit stdin.
       - Cover reply body propagation through the issue service fake.
       - Cover comment body propagation without changing its current output path: use a test-local sequential recording `http.RoundTripper` that responds to the issue lookup and comment mutation and asserts the exact comment body variable.
       - Add bounded non-blocking coverage that executes commands with ordinary flags and a reader that would fail/block if consumed; verify service calls still occur without reading unrelated stdin. The shared helper test is the primary proof of the no-read branch; command coverage should only be added where it exercises a distinct command path.
       - Assert explicit stdin read errors are returned with the command's existing contextual error wrapping.
       - Define empty-input compatibility precisely: the helper returns `""`; create inputs carry an empty description; update inputs retain the current nil `Description` behavior while `description == "-"` still passes the existing update gate; comment/reply commands reject empty bodies before lookup/mutation. Do not introduce description clearing.
       - Assert ordinary literal flag values continue to pass through unchanged and that stdin-resolved text remains the body before attachment appending.
     - **Verification**:
       - Run all new CLI tests. Confirm each affected command forwards explicit stdin content and the implicit form does not consume stdin or silently claim to have accepted a body.
  3. **Correct user-facing help and documentation**
     - **Implementation**:
       - Update issue create/update, comment, and reply long descriptions, flag descriptions, examples, and required-body error messages in `internal/cli/issues.go` to say stdin is read only with `--description -` or `--body -`; specifically fix the unadorned `comment` and `reply` examples.
       - Update project create/update examples and flag descriptions in `internal/cli/projects.go`, specifically replacing the unadorned project create/update pipe examples with explicit `--description -` examples.
       - Audit `internal/cli/onboard.go` for its stale pipe example and correct it as well.
       - Audit README piping and command examples for the same surfaces. Preserve examples that already use explicit `-` (including the current README issue/comment/reply forms), changing only stale or ambiguous wording and adding the whitespace policy where the stdin contract is documented.
       - Audit `internal/skills/linear/SKILL.md` similarly; retain its already-correct explicit `-` examples unless the audit finds wording that still implies implicit reading. Search for command-shaped unadorned pipelines as well as `pipe to stdin` text before finishing.
     - **Verification**:
       - Run help/documentation-focused tests if added (including assertions against command help text), then inspect targeted grep results to ensure no affected command advertises implicit stdin. Run `make test` after documentation and command changes.
  4. **Final regression verification**
     - **Implementation**:
       - Review the diff for unchanged attachment handling, ordinary flag behavior, empty-input semantics, and the historical no-auto-detection constraint.
       - Add or adjust comments only where they accurately describe explicit `-` behavior and trimming.
     - **Verification**:
       - Run `make test` from a clean working state. Confirm all tests pass and the final search/help checks show a single consistent stdin contract.

**Phase Verification Approach**: Each phase ends with automated verification before the next begins. Tests are added after the corresponding implementation within each phase, using injected readers, service fakes, and deterministic HTTP transport rather than a live API. The final phase requires the complete repository test suite to pass.

## Quality Assurance Plan
- **Testing Strategy**:
  - Table-driven helper tests for the resolver and whitespace/empty-input policy.
  - CLI execution tests for issue/project create/update and reply using `NewCmdWithDeps`.
  - Deterministic client/transport coverage for comment request body propagation.
  - Help-text assertions or focused string checks for explicit flag syntax, plus the full `make test` suite.
- **Edge Cases**:
  - Literal body/description values other than `-` must not trigger stdin.
  - Explicit stdin with leading/trailing spaces and final newline is trimmed exactly as today.
  - Explicit stdin read errors are surfaced with the existing helper/command context.
  - Empty stdin returns an empty string; create forwards empty description, update keeps `Description` nil while retaining the `"-"` update gate, and comment/reply fail before issue lookup or mutation. No new clearing behavior is introduced.
  - Commands with unrelated open stdin must not block or consume it.
  - Attachment appending must still work after body resolution, with the resolved body retained before the attachment markdown is added.
  - Issue update's `description == "-"` must still count as an explicitly requested update even when the resulting body is empty, without inventing a new clearing behavior.
- **Regression Prevention**:
  - Keep the exact-`-` condition in one shared helper.
  - Test both the positive read path and the negative no-read path.
  - Search all affected help/docs surfaces for stale implicit-pipe language.
  - Preserve service interfaces and GraphQL inputs unless a narrowly scoped testability refactor is proven necessary.
- **Success Verification**:
  - `make test` passes.
  - Explicit stdin examples work consistently for all six command paths (issue create/update, project create/update, comment, reply).
  - Ordinary values and non-body commands do not read stdin.
  - Help, README, and embedded skill guidance all require the explicit `-` convention.

## Development Environment
- **Setup Requirements**: Go toolchain and repository dependencies already defined by the project; no external service credentials or live Linear workspace are needed.
- **Debugging Strategy**: Capture service inputs and GraphQL request payloads in test doubles/transports; use focused test runs while iterating, then `make test`.
- **Iteration Approach**: Implement one phase at a time, run its focused tests, then proceed only after verification; finish with repository-wide tests and targeted documentation search.

## Risk Assessment
- **Potential Issues**:
  - Process-global stdin can make tests flaky or hang if the seam is incomplete.
  - Comment command's direct client access may make body capture harder than the service-backed paths.
  - Help wording can remain inconsistent in an overlooked README or embedded skill example.
  - Changing empty stdin handling accidentally could alter description-clearing behavior.
- **Mitigation Strategies**:
  - Use the pure reader helper rather than mutable package-global test state; keep command tests serial only where the existing command globals require it.
  - Use a test-local sequential recording `http.RoundTripper` for the comment path so issue lookup and comment mutation receive distinct deterministic responses and the mutation body can be asserted; avoid requiring a live API or changing the production base URL.
  - Complete every method on the issue/project service interfaces in test doubles, and pass explicit team flags to avoid machine-local configuration.
  - Search all source/docs for `pipe`, `stdin`, `--description`, and `--body`, including unadorned command-shaped pipelines and onboarding examples, and add help assertions for affected commands.
  - Treat whitespace, read errors, and empty-input behavior as explicit compatibility cases before editing command assembly.
- **Backup Approaches**:
  - If the recording transport is too brittle, keep production comment code unchanged and isolate the GraphQL request sequence in a focused test helper that still asserts the mutation variables.
  - If complete interface fakes become noisy, define test-only adapters that delegate unneeded methods to zero-value error implementations while keeping the captured methods explicit; do not weaken production interfaces or use a live API.

## Files Likely to Change
- `internal/cli/helpers.go` - Add the internal stdin reader seam and clarify the explicit-`-` helper contract without changing production behavior.
- `internal/cli/helpers_test.go` - Add whitespace, empty-input, ordinary-value, explicit-read, and non-read helper tests.
- `internal/cli/issues.go` - Correct issue description/body help, examples, and errors; retain explicit stdin behavior.
- `internal/cli/projects.go` - Correct project description help and examples.
- `internal/cli/onboard.go` - Correct the stale onboarding pipeline example.
- `internal/cli/*_test.go` - Add command-level fakes/transport tests for issue, project, comment, and reply body propagation, read errors, and bounded non-consumption.
- `README.md` - Remove stale implicit-pipe examples and document explicit `--description -`/`--body -` usage and trimming policy.
- `internal/skills/linear/SKILL.md` - Audit and align embedded CLI stdin guidance.
