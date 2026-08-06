# Implementation Plan

## Task Overview
- **Source**: TL-562
- **Title**: Prevent exclusive-label conflicts and improve label/error output
- **Description**: Detect mutually exclusive child-label selections locally before issue create/update mutations, report the conflicting labels and parent group clearly, remove duplicated/raw GraphQL error output, and make both default label listings communicate parent-child exclusivity while preserving JSON compatibility.

## Requirements Analysis
- **Core Functionality**:
  - Resolve labels by case-insensitive name or UUID while preserving canonical label metadata (`ID`, `Name`, and optional `Parent`).
  - Treat two or more distinct child labels with the same non-nil parent as an exclusive-group conflict. Parent/group labels (`Parent == nil`) and standalone labels do not conflict.
  - Validate create selections before `CreateIssue` and validate the final label set for update replace/add operations. Additive updates must include labels already attached to the issue; remove-only updates must not reject a set merely because it reduces or preserves an existing selection.
  - Report every conflicting sibling deterministically, including three-way conflicts, multiple groups, duplicate inputs, case variants, UUID inputs, and names containing punctuation.
  - Render grouped text output for both `linear teams labels` and `linear labels list`; retain the existing machine-readable label JSON structure and parent metadata.
  - Ensure normal CLI errors contain one operation context and never expose raw GraphQL query/mutation previews.
- **Acceptance Criteria**:
  - Invalid create/replace/add selections fail locally and make zero relevant mutation calls.
  - Conflict errors identify the shared group and all conflicting child labels in stable order with actionable wording.
  - Duplicate/case-variant inputs do not create duplicate conflict entries; valid selections from different groups and standalone labels continue to work.
  - Add/remove update behavior accounts for the final set, and removing the final label still sends an explicit empty label list.
  - UUID and existing name resolution contracts work, including parent/group-label resolution and unknown-label guidance.
  - Text listings clearly distinguish group headers and child alternatives; JSON output remains structurally usable for automation.
  - GraphQL errors retain useful structured/type information but contain no query text; create/update output has no duplicated operation prefix.
  - Automated tests cover resolver/cache behavior, conflict validation, mutation suppression, update modes, grouped output, JSON compatibility, and unrelated API errors.
- **Technical Constraints**:
  - Keep public `ResolveLabel`/`ResolveLabelIdentifier` string-returning APIs compatible where possible; add a metadata-returning resolver API for service validation rather than forcing metadata through existing callers.
  - The parent relationship returned by Linear is the authoritative exclusivity signal for this ticket.
  - Preserve nil-versus-non-nil label slices so `nil` means no label field supplied while a non-nil empty slice means clear all labels.
  - Do not run mutating live Linear commands. Verification is repository build/unit testing only; live read-only checks are optional.
- **Integration Points**:
  - `internal/service/issue.go` and `internal/cli/issues.go` issue flows.
  - `pkg/linear/resolver.go`, `pkg/linear/resolver_cache.go`, and the service-facing client interface.
  - `pkg/linear/core/types.go` label metadata and `pkg/linear/core/errors.go` GraphQL errors.
  - `pkg/linear/issues/client.go` issue mutation wrappers and update input serialization.
  - `internal/service/team.go`, `internal/service/label.go`, and a shared `internal/format` label renderer.

## Codebase Analysis
- **Existing Patterns**:
  - `Resolver.ResolveLabel` currently fetches team labels and returns only an ID; `resolverCache` caches only `teamID:labelName -> labelID` and does not normalize keys or retain parent metadata.
  - `core.Label` already contains `Parent *LabelRef`, and `teams.Client.ListLabels` and issue reads already query parent IDs/names.
  - `IssueService.Create` resolves labels before its single create mutation. `IssueService.Update` fetches the issue, then builds replace/add/remove label sets, but currently loses ordering and skips an explicit empty final set.
  - Team and label services marshal raw labels for JSON but duplicate flat text formatting logic.
  - `BaseClient.ExecuteRequest` currently adds a truncated query preview to `GraphQLError`; issue client, service, and CLI each add overlapping create/update context.
  - Existing test infrastructure includes service mocks and GraphQL mock transport/server helpers, but lacks conflict, formatter, and GraphQL error regression coverage.
- **Available Infrastructure**:
  - `make build` and `make test` invoke Go build and `go test -v ./...`.
  - `pkg/linear/resolver_test.go`, `internal/service/issue_create_test.go`, `internal/service/issue_relation_test.go`, `pkg/linear/issues/client_test.go`, and `pkg/linear/teams/client_test.go` provide nearby test patterns.
  - `pkg/linear/testutil/mock_transport.go`/`mock_server.go` can assert request counts and payloads without a network round trip.
  - Existing `guidance.ErrorWithGuidance`, `core.ValidationError`, and typed `core.GraphQLError` should be reused/extended rather than introducing ad hoc string-only errors.
- **Dependencies**: No new external dependency is expected. Changes to `IssueClientOperations` or resolver return types require updating all service mock implementations and client delegates.
- **Architecture Notes**:
  - Add a resolver method such as `ResolveLabelMetadata(labelName, teamID) (*core.Label, error)` and a corresponding internal/public client delegate. Keep the existing ID-only methods delegating to it. Exact UUID matches must be constrained to the requested team's returned label set; case-insensitive name collisions must produce deterministic ambiguity guidance instead of selecting an arbitrary API-order match. Parent/group labels remain resolvable exactly as they are today; the validator only treats labels with non-nil `Parent` as children.
  - Populate metadata caches for all labels returned by one team listing, keyed by normalized name and ID, so resolving multiple labels or ID inputs does not discard metadata or cause avoidable repeat requests. Normalize at lookup time, extend cleanup to every new index, and return deep copies (including `Parent`) so callers cannot mutate cached records. Because conflict detection depends on a complete team label set, either add pagination to `ListLabels` or document and test the API's complete-result guarantee before relying on it.
  - Add a pure validator/typed conflict error in the service/domain layer. Deduplicate resolved labels by ID for both validation and mutation payloads, group children by `Parent.ID`, sort groups and canonical `Label.Name` values, and quote canonical names/group names safely in user-facing output.
  - For create, resolve all labels to metadata, validate, then pass canonical, deduplicated, sorted IDs to the mutation. For update replace, resolve supplied labels and validate that set. For add/remove, reuse parent metadata already returned on the existing issue when available, hydrate missing metadata from the target team's complete label set, construct a deterministic final set from current issue labels plus additions minus removals, and validate when additions can introduce a conflict. If metadata remains unavailable, fail validation clearly rather than silently treating the label as standalone. Preserve explicit empty output for remove-all. If an update changes teams, permit replacement labels resolved in the target team but reject additive/removal label operations combined with a team change before mutation so source-team IDs cannot be submitted to the target team.
  - Define the error boundary explicitly: the CLI adds no create/update wrapper; `IssueService` adds one `failed to create/update issue` context for local validation failures; `pkg/linear/issues.Client` retains one context for remote create/update mutation failures; the service must propagate those remote errors without wrapping them again. Resolver and unrelated service errors retain their existing operation-specific guidance.
  - Remove query previews from `BaseClient` globally and decode/preserve GraphQL `extensions`; test that typed errors remain useful without implementation internals.
  - Add a shared label text formatter that reconstructs groups independent of API order, renders each exclusive parent header once with indented children, preserves descriptions/IDs appropriate to each command, uses `Parent.Name`/ID as a fallback header when a parent record is absent, and sorts groups/children deterministically. Keep JSON paths as direct `core.Label` marshaling.

## Implementation Strategy
- **Problem Complexity**: Complex, cross-layer bugfix with compatibility-sensitive resolver, update, formatter, and shared error behavior.
- **Core Problem**: Label parent metadata is available at the API boundary but discarded before validation, while multiple layers expose the same low-level mutation error and list labels without their group structure.
- **Approach**: Preserve existing public ID-based APIs, add metadata-aware resolution and caching, centralize pure conflict validation before mutations, make update label-set semantics explicit, centralize grouped text formatting, and establish one user-facing error boundary. Implement and verify in gated phases.
- **Testing Approach**: Automated unit and mock-transport tests after each implementation phase, followed by `make build` and `make test`. Do not perform live mutating verification.
- **Phases**:
  1. **Metadata-aware label resolution and conflict domain logic**
     - **Implementation**:
       - Extend resolver/cache storage to retain complete `core.Label` records, normalize name/ID lookup keys at lookup and population time, populate name and UUID indexes from a complete/paginated team-label fetch, and accept UUID inputs while retaining case-insensitive name matching and guidance for unknown or ambiguous labels. Extend cache cleanup for every new index and return deep copies of cached metadata.
       - Add the metadata resolver delegate and update the minimal service interface plus all mocks/delegates.
       - Add a deterministic conflict validator/error that groups distinct child IDs by non-nil parent, reports all siblings/groups in stable order using canonical names, handles duplicates/case variants, and leaves parent labels/standalone labels valid. Define a clear failure when an existing label cannot be hydrated with authoritative parent metadata.
     - **Verification**:
       - Add resolver/cache tests for name, case, UUID, punctuation, duplicate lookups, cache reuse, unknown labels, ambiguous names, cross-team UUID rejection, parent labels, deep-copy safety, and cleanup of all label indexes.
       - Add pure validator tests for two-way/three-way conflicts, multiple groups, duplicate inputs, different groups, standalone labels, unavailable metadata, and deterministic actionable formatting. Run the focused Go tests.
  2. **Create and update validation before mutation**
     - **Implementation**:
       - Change issue create resolution to retain metadata, validate all requested labels, and only then invoke `CreateIssue`. Treat an explicitly supplied empty replacement as a deliberate clear operation rather than as an omitted flag.
       - Update replace/add/remove processing to resolve metadata, combine current issue labels (reusing their fetched parent references) for additive operations, validate the final set when additions/replace can introduce conflicts, and preserve canonical, deduplicated, sorted IDs.
       - Define CLI flag-presence behavior for `--labels ""` and related replace/add/remove exclusivity checks using whether the flag changed, not only whether the parsed slice is non-empty.
       - Fix nil versus non-nil label slice handling in service/client update-field checks and GraphQL input construction so removing the last label sends `labelIds: []`; use one shared field-presence helper or audit all service/client checks and serialization sites.
       - If `--team` changes the issue's team in the same update, allow replacement labels from the target team but reject additive/removal label modes before mutation to prevent mixing source-team existing IDs with target-team IDs.
       - Keep remove-only operations from producing false conflict failures and ensure invalid selections result in zero create/update mutation calls.
       - Remove the CLI's generic create/update wrappers. Add exactly one issue-operation context for local validation in `IssueService`, retain exactly one remote mutation context in `pkg/linear/issues.Client`, and propagate remote errors through the service unchanged.
     - **Verification**:
       - Extend service/CLI tests to assert zero mutation calls for invalid create, replace, and add selections; cover valid cross-group/standalone selections, three-way and UUID conflicts, additive updates using existing labels, remove-only updates, remove-all payloads, explicit empty `--labels`, and simultaneous team-change/label-mode behavior.
       - Assert operation context occurs exactly once for both local validation and remote mutation failures, validation errors do not include raw query text, duplicate inputs are deduplicated in the mutation payload, and all label-field presence checks still distinguish nil from empty. Run focused service, CLI, and issue-client tests.
  3. **Grouped label-list text output with JSON compatibility**
     - **Implementation**:
       - Add a reusable formatter in `internal/format` for grouped labels, reconstructing parent groups from `Parent.ID` regardless of response order.
       - Render exclusive group headers and indented child alternatives deterministically, retain parent/group-label and standalone records without making JSON changes, and preserve command-specific useful fields (colors, descriptions, IDs).
       - Replace duplicated text formatting in `TeamService.GetLabels`/`GetLabelsWithOutput` and `LabelService.List`; keep empty and JSON behavior intentional and compatible.
     - **Verification**:
       - Add formatter/service tests for group ordering, child ordering, three or more siblings, parent records arriving after children, missing parent records (fallback header), standalone labels, descriptions/IDs, and both commands.
       - Assert JSON output still contains the raw parent metadata fields and remains valid machine-readable JSON. Run focused formatter/service tests.
  4. **GraphQL error sanitization and regression coverage**
     - **Implementation**:
       - Change `BaseClient.ExecuteRequest` GraphQL decoding to add `extensions` to the response error shape, retain it in `core.GraphQLError`, and return the error without embedding any query/mutation preview.
       - Ensure the explicit issue-client/service/CLI boundary yields one operation context for local and remote create/update failures while unrelated GraphQL operations retain clear typed errors.
       - Preserve existing error classification behavior and avoid changing unrelated HTTP/retry handling.
     - **Verification**:
       - Add mock-response tests for GraphQL messages with extensions, assert no query fragment leaks, and verify typed/code details remain available.
       - Test create/update wrapping for exactly one operation prefix and representative unrelated API errors for non-regressed diagnostics. Run focused core/client tests.
  5. **Full regression and release-quality verification**
     - **Implementation**:
       - Review all changed interfaces/mocks and formatting paths for compatibility, update comments/help text only where behavior changed, and add any missing edge-case tests discovered by prior phases.
       - Keep implementation within the existing package architecture and avoid live mutation checks.
     - **Verification**:
       - Run `make build` and `make test` (equivalent to `go test -v ./...`). If the Go toolchain is unavailable, report that limitation rather than substituting an unverified claim.
       - Optionally run read-only label listing commands against a configured team to inspect grouped output, but do not run issue create/update mutations.

**Phase Verification Approach**: Each phase must pass its focused automated tests before the next phase begins. The final phase requires the repository build and full unit suite; no manual GUI work is applicable. Live API mutation verification is explicitly out of scope.

## Quality Assurance Plan
- **Testing Strategy**: Pure unit tests for resolution/cache/validation/formatting, service tests with mutation-call spies, GraphQL mock transport tests for serialization/error boundaries, then full Go build and unit suite.
- **Edge Cases**:
  - Two, three, or more siblings in one group; conflicts in multiple groups; duplicate IDs and case variants.
  - UUID inputs, names with punctuation, unknown or ambiguous labels, cross-team UUIDs, parent/group labels, standalone labels, and labels from different groups.
  - Existing conflicting labels during additive updates; replace versus add/remove semantics; explicit empty replacement flags; simultaneous team changes; remove-only and remove-all updates.
  - API responses whose parent records appear after children or are absent; complete/paginated label retrieval; deep-copy/cache expiry; JSON parent references; GraphQL errors with and without extensions.
  - Exact operation-context count, deduplicated/sorted mutation IDs, and absence of raw query/mutation text.
- **Regression Prevention**: Preserve public ID-only resolver methods and raw label JSON schema, retain typed GraphQL errors and error classification, keep valid issue mutations unchanged, and update every interface mock in the repository.
- **Success Verification**: All TL-562 acceptance criteria are represented by focused tests, `make build` succeeds, and `make test` passes without live mutation calls.

## Development Environment
- **Setup Requirements**: Go toolchain and repository dependencies from `go.mod`; no new dependency expected. Use existing mock transport/server helpers and current branch `tl-562`.
- **Debugging Strategy**: Use typed errors, focused mock response/request assertions, and deterministic output snapshots/string assertions. Never add query text to user-facing errors.
- **Iteration Approach**: Run focused package tests after each phase, then `make build`/`make test`; inspect `git diff` for interface and JSON compatibility changes.

## Risk Assessment
- **Potential Issues**:
  - Adding a metadata resolver method can break multiple mocks and consumers.
  - Existing callers may rely on exact duplicated error strings or query previews.
  - Issue label data may lack parent metadata in some paths, especially fixtures or older responses.
  - The label endpoint may paginate despite the current query exposing only `nodes`.
  - Reordering text output may affect snapshot/string tests and scripts that incorrectly parse human output.
  - Non-nil empty label slices may interact with GraphQL input omission logic in more than one layer.
  - Simultaneous team and label updates can mix source-team and target-team IDs if not rejected or modeled explicitly.
- **Mitigation Strategies**:
  - Keep existing resolver signatures as adapters and update all compile-time mocks in one phase.
  - Preserve typed underlying errors and test only the intended single context; remove query previews deliberately and cover the global behavior with regression tests.
  - Hydrate missing metadata through the complete/paginated team-label resolver only when needed; if authoritative metadata cannot be obtained, return a clear validation error; use parent ID as the sole conflict key.
  - Leave JSON unchanged and make text ordering explicit and deterministic; use parent name/ID fallback headers when group records are absent.
  - Add request-payload tests specifically for `labelIds: []`, use one shared field-presence helper or audit every check, and test CLI flag-changed semantics for empty replacement.
  - Reject additive/removal label modes combined with a team change unless the implementation can prove all final IDs belong to the target team.
- **Backup Approaches**:
  - If changing the service interface proves too invasive, introduce a narrow optional metadata resolver capability while retaining the existing ID-only interface and fail validation only when metadata cannot be obtained.
  - If shared formatter integration causes command-specific regressions, keep one shared grouping primitive with thin team/label renderers rather than duplicating grouping logic or forcing one service to own the other's command behavior.
  - If the three nil/empty checks drift during implementation, centralize label-field presence in a small helper used by service gating, client gating, and GraphQL input construction.

## Files Likely to Change
- `pkg/linear/resolver.go` — metadata-aware, UUID-capable label resolution and compatibility adapter.
- `pkg/linear/resolver_cache.go` — complete label metadata cache and normalized name/ID indexes.
- `pkg/linear/client.go` — public/internal metadata resolver delegate.
- `pkg/linear/core/errors.go` — any typed conflict/error helpers or GraphQL extension handling needed.
- `pkg/linear/core/base_client.go` — remove query previews and preserve GraphQL extensions.
- `pkg/linear/issues/client.go` — update label field presence/empty-array serialization and remote operation error boundary.
- `pkg/linear/teams/client.go` — paginate label retrieval if required by the API contract while preserving parent fields.
- `internal/service/client_interfaces.go` — metadata resolver capability.
- `internal/service/issue.go` — create/update conflict validation, final label-set construction, and wrapper cleanup.
- `internal/cli/issues.go` — remove duplicate create/update error wrapping if still present after service boundary changes.
- `internal/service/team.go` — use grouped text label formatter.
- `internal/service/label.go` — use grouped text label formatter.
- `internal/format/labels.go` — new shared grouped text renderer.
- `pkg/linear/resolver_test.go` — resolution/cache behavior.
- `internal/service/issue_create_test.go` and `internal/service/issue_relation_test.go` (or a focused new update test) — create/update conflict and mutation suppression.
- `internal/cli/issues.go` and its CLI tests/helpers — empty replacement flag presence, label-mode exclusivity, and wrapper behavior.
- `pkg/linear/issues/client_test.go` — update payload and operation-error behavior.
- `pkg/linear/core/base_client_test.go` — GraphQL sanitization/extensions regression tests.
- `internal/format/labels_test.go` — grouped rendering tests.
- `internal/service/team_test.go` and/or `internal/service/label_test.go` — command integration and JSON compatibility tests.
- Other repository service mocks implementing `IssueClientOperations` — compile/test updates for the metadata resolver method.
