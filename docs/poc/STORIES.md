# HPSTRAT-831 Story Breakdown — Cluster Lifecycle Manager (CLM)

Status: draft for review — not yet filed in Jira
Revision: 1 (2026-11-04)
Parent: [HPSTRAT-831](https://redhat.atlassian.net/browse/HPSTRAT-831) — ORO cluster lifecycle and provisioning (Feature)
Sources: [HPSTRAT-831-epics.md](EPICS.md) (epics E1–E17) · [PROPOSAL.md](PROPOSAL.md) (design, decisions #1–#32) · [adr-0031-day-0-bootstrap-shape.md](adr-0031-day-0-bootstrap-shape.md)

---

## 1. How to read this document

Stories are the implementation units under the epics of the epic breakdown. Read in
dependency order, they are the build sequence for the P0 + P2 (+ cutover line) scope of
HPSTRAT-831: each story lands one mergeable, tested increment; completing all of them
delivers standalone CLM with the hosted/OCI path, ready for ORO.

Out of scope here (tracked elsewhere, per breakdown §1): P1 / bm-abi (bare metal
standalone, day-0 laptop — feeds FM-96), fleet mode (ocm-fleet consumption — FM-95 /
FM-97), P3 hardening (External adoption, cross-MC etcd, HCP-on-BM).

Conventions:

- **ID scheme**: `E{n}-S{m}` — story m under epic n. Story order within an epic is
  dependency-first, not priority-first.
- **Size**: XS (≤1 day) · S (1–2 days) · M (3–5 days) · L (1–2 weeks). Points assigned
  at refinement.
- **Depends on**: story-level dependencies (cross-epic allowed), consistent with the
  epic-level graph in breakdown §5.
- **Carry vs build**: "carry" = exists in the v2.5 runtime (`hyperfleet-api`, 58.9k
  LOC) or the archived POC repos and moves forward verbatim/near-verbatim; "build" =
  new capability. The promotion invariant applies throughout: the repo never leaves
  its CI, CI never goes red between commits.
- **Labels at filing**: `hf-clm` plus `hf-clm-p0` / `hf-clm-p2` per phase; E17 carries
  `hf-clm` (cutover); E15 additionally the external-owner flag. All stories file in
  the HYPERFLEET project with Parent Link → their epic (epics parent to HPSTRAT-831).
- **Teams**: drafts are team-agnostic (squad reorg in flight, OQ-7).
- E15 stories are **our-side support/visibility criteria only** — Oracle owns
  delivery. E13 is **conditional** on E7's go decision (fallback per decision #18).
  E17 is a **parallel line** serving existing v2.5 consumers, not ORO.

Story format: Goal (what), Context (why + pointers), Tasks (how), Acceptance criteria
(definition of done), Verification (how it is proven).

---

## 2. Execution order

```
E1-S1 (retrospective ADR — first story, before any restructuring)
  │
  ├──→ E1-S2..S8 (folds, resource model, contract, layout)
  │        │
  │        ├──→ E2 (store hardening) ──→ E3 (facade, parallel-safe)
  │        │        │
  │        │        └──→ E5 (orchestrator + SPI + status) ──→ E6-S4  ◄ P0 EXIT 1
  │        │
  │        └──→ E4 (proto generator, independent of E2/E3/E5)
  │
E7 (etcd spike — start immediately, no deps) ──→ E7-S2  ◄ P0 EXIT 2
                                                   │
E6-S4 (contract v1 frozen) ── gates all of P2 ──┤
  │                                             │
  ├──→ E8 (hcp adapter in core) ──→ E9 (placement)              E17 (parallel line:
  │        │                        E10 (transport) ──↔ E17-S2   same transport,
  │        │                        (in-tree hosted chain)       different consumers)
  │        └──→ E8-S6 ──────────────────→ E16 (deployment)
  ├──→ E11 (capability adapters) ──→ E13 (etcd mgr; needs E7-S2 go + E11-S4)
  ├──→ E12 (coreshift + CI layering)
  ├──→ E14 (MC prereqs + OCI co-dev) ──→ E15 (Oracle, external)
  └──→ E15-S1 (onboarding package)
```

Gates:

| Gate | Story | Meaning |
|---|---|---|
| P0 exit criterion 1 | E6-S4 | Adapter contract v1 frozen — unblocks E8–E15 |
| P0 exit criterion 2 | E7-S2 | Etcd go/no-go — gates E13 (fallback: CPO-managed etcd + EtcdBackup, decision #18) |
| P2 acceptance | E15-S2 | hosted/oci chain E2E demonstrated — the live ORO-test proof |
| Delivery | E16-S3 | Standalone deployment validated — Oracle unblocked |

---

## 3. Stories by epic

### E1 — Promote hyperfleet-api to CLM core + TypeSpec API v1

**Phase:** P0 · **Repo:** `hyperfleet-api` (promoted; rename at productization) · **Epic size:** L · **Label:** `hf-clm-p0`
**Epic goal:** hyperfleet-api graduates to the CLM core repo — TypeSpec source of truth and e2e folded in, target layout established, v1 API contract assembled — git history continuous by construction.

#### E1-S1 — Generations retrospective ADR
**Size:** S · **Depends on:** none · **Blocks:** all of E1 (filed first, before any restructuring)

**Goal:** Written record of what each HyperFleet generation proved, what carries to v3, what EOLs and why — the through-line (contracts → pipeline → operability → product) becomes an artifact before restructuring begins.

**Context:** Breakdown E1 names this the first story. ADR numbering continues from the preserved 0001–0030 series; ADR 0031 (day-0 bootstrap shape) is already drafted in this directory — this is ADR 0032. Language discipline per breakdown §1: promote, fold, converge, consolidate — never rewrite, replace, retire.

**Tasks:**
- Draft ADR 0032 in the core repo's ADR location: v1 (event push), v2 (Redis/broker, desire-store), v2.5 (Postgres, gateway, operator patterns, CI), v3 (promotion)
- What carries verbatim: API shape, status contract, entity model
- What EOLs: CEL decision engine, adapter DSL, broker; the Desire Store line via E17
- Dispositions consistent with the fold-in map (breakdown §2) and PROPOSAL.md §4 lineage/succession

**Acceptance criteria:**
- [ ] ADR filed in core repo, numbered 0032, continuing the series
- [ ] Covers every generation with what it proved and what carries/EOLs
- [ ] Dispositions match breakdown §2 fold-in map exactly
- [ ] Reviewed by squad leads (lead-engineer conversation audience)

**Verification:** doc review; consistency check against PROPOSAL.md §4.

---

#### E1-S2 — Fold api-spec + api-spec-template into core `api/`
**Size:** M · **Depends on:** E1-S1

**Goal:** TypeSpec source of truth moves into the core repo; the standalone spec repos archive in place with pointers.

**Context:** `hyperfleet-api-spec` (TypeSpec, `build-schema.sh core|gcp`) and `hyperfleet-api-spec-template` fold in per breakdown §2. Fold = move + build wiring; the promotion invariant (CI green throughout) applies. Overlay *publication mechanics* are E1-S6 — this story is the fold and the build.

**Tasks:**
- Move TypeSpec sources + build scripts into core `api/`
- Wire TypeSpec → OpenAPI build into the core Makefile and presubmit CI
- Archive `hyperfleet-api-spec` and `hyperfleet-api-spec-template` in place with pointer READMEs
- Carry spec CI (lint, format) into core

**Acceptance criteria:**
- [ ] TypeSpec lives in core `api/`; standalone repos archived with pointers
- [ ] Core CI builds OpenAPI from TypeSpec on every PR
- [ ] Generated OpenAPI is byte-identical to the last pre-fold build (golden diff)

**Verification:** golden-file comparison pre/post fold; CI green.

---

#### E1-S3 — Fold hyperfleet-e2e into core `e2e/`
**Size:** M · **Depends on:** E1-S1

**Goal:** Black-box e2e suite carried into the core repo, green in core CI post-fold.

**Context:** 17.3k LOC, 18 test files, active (breakdown §2). The coreshift adapter reshapes CI later (E12); the remote-transport profile switches to ManifestWork with the E17 cutover — until then it keeps its current delivery path behind a profile flag.

**Tasks:**
- Move the suite into core `e2e/`; wire as presubmit (fast profiles) + periodic (full)
- Keep the remote-transport profile working on its current path, flagged
- Document profiles and their CI placement

**Acceptance criteria:**
- [ ] Suite runs green in core CI post-fold
- [ ] Test-file inventory parity (nothing dropped in the move)
- [ ] Profiles documented, remote-transport path explicitly flagged for E17

**Verification:** CI green; test inventory diff = 0.

---

#### E1-S4 — Resource model v1: `Cluster` + `NodePool`
**Size:** M · **Depends on:** E1-S2

**Goal:** The v1 resource model assembled in TypeSpec: `Cluster`, `NodePool` with `id` (UUIDv7), `kind`, `name` (DNS-1123), `labels`, `spec: Record<unknown>` blob, `status.conditions[]`, `generation`, `owner_references`, server-populated tenancy.

**Context:** Mostly carry: `pkg/api/resource.go`, `resource_id.go` (UUIDv7, ADR 0009), `resource_label.go`, `resource_reference.go`, `metadata_types.go` hold the proven shapes (ADR 0007 conditions, 0010 JSONB blob). `NodePool` becomes a first-class resource with `owner_references` to its Cluster if not already present. **PoC carry evidence (PC-11):** the Cluster half survived the slice unchanged — PC-1 exercised CRUD, UUIDv7 IDs, generation, labels and search; PC-3 added a config-defined kind without core edits.

**Tasks:**
- Audit existing `pkg/api` types against PROPOSAL.md §6 resource model; record deltas
- Extend TypeSpec with the full `Cluster`/`NodePool` field set
- Server-side: UUIDv7 generation, DNS-1123 name validation, tenancy populated server-side (client-set tenancy rejected)

**Acceptance criteria:**
- [ ] TypeSpec defines both resources with the complete field set
- [ ] IDs are UUIDv7, generated server-side
- [ ] Names validated DNS-1123; tenancy server-populated only
- [ ] Delta audit recorded (what carried vs what was added)

**Verification:** TypeSpec → OpenAPI build; unit tests for ID/name/tenancy rules.

---

#### E1-S5 — `topology` × `platform` axes + capability knobs
**Size:** S · **Depends on:** E1-S4

**Goal:** First-class axes in the high-level spec — `topology: standalone | hosted`, `platform: agent-bm | oci | ...` — plus capability opt-out knobs (`dns: {managed: false, userRecords: [...]}`, `etcd: {managed: clm | cpo}`).

**Context:** The combination selects the adapter chain (PROPOSAL.md §9); knobs shape chains without changing the API surface (decision #2). Knob *behavior* (exclude vs verify-only) is implemented by preconditions in E11-S3 — this story is the spec surface and validation. **PoC drift (PC-11):** the PoC carries `topology`/`platform` in the spec blob (`spec.topology`, `spec.platform`) for chain selection; the first-class typed fields here are the product shape.

**Tasks:**
- TypeSpec fields for the axes and knobs on the cluster spec wrapper
- Enum validation of `topology`/`platform` against registered platforms
- Document knob semantics and their chain effects

**Acceptance criteria:**
- [ ] Axes present, typed, validated; unknown values rejected with RFC 9457 errors
- [ ] Knobs typed and documented with their chain-shaping meaning
- [ ] Adding a platform value requires only an overlay/registry entry, not core edits

**Verification:** TypeSpec + API validation unit tests.

---

#### E1-S6 — Overlay packaging + provider schema registry + runtime validation
**Size:** M · **Depends on:** E1-S4, E1-S5

**Goal:** Core `ClusterSpec` intentionally empty (`{}`); provider overlays (separate TypeSpec packages, versioned publishable artifacts) define per-platform fields; the server validates the spec blob at runtime against the registered provider OpenAPI component.

**Context:** Blobbed-but-validated (PROPOSAL.md §6). Starting points in the runtime: `pkg/validators`, `pkg/registry`. Overlays become the publishable artifacts consumed by offering teams (E6 publishes them). **PoC carry evidence (PC-11):** registry-driven kinds and runtime spec validation already ship — PC-1 created clusters against validated specs and PC-3 added a kind through config only. Overlay packaging (separate versioned TypeSpec packages) remains new.

**Tasks:**
- Overlay package format + versioning scheme (publishable, independently versioned)
- Provider schema registry: register an OpenAPI component per `platform` value
- Runtime validation of the spec blob on create/update against the registered component; errors mapped to RFC 9457 with pointers into the overlay schema
- Reject specs for unregistered platforms

**Acceptance criteria:**
- [ ] Overlays build as versioned packages consumable outside the repo
- [ ] Server validates blobs against the registered component at runtime
- [ ] Validation errors carry schema pointers; unregistered platform rejected
- [ ] A sample overlay (e.g. `agent-bm` or `oci` skeleton) demonstrates the flow

**Verification:** unit tests with the sample overlay; e2e create with valid/invalid blob.

---

#### E1-S7 — Status contract + deletion + error model, carried verbatim
**Size:** M · **Depends on:** E1-S4

**Goal:** The status contract carried forward verbatim: `generation` / `observed_generation`; `AdapterStatus` conditions `Available`, `Applied`, `Health`, `Finalized` + free-form `data`; aggregation to `Reconciled`; sticky `LastKnownReconciled`; soft delete (202) gated on all required adapters `Finalized=True`; `force-delete` store-only escape hatch; RFC 9457 problem+json; label/field selectors.

**Context:** Types carry from `pkg/api/adapter_status_types.go`, `status_types.go`, `error.go` (ADR 0007 conditions model, 0008 aggregation — proven three generations). Deletion *enforcement* lives in the orchestrator (E5-S5); this story carries the API semantics and types. Selector *query execution* wires into SQL in E2-S2. **PoC carry evidence (PC-11):** PC-3 drove a fake status to `CoreshiftSuccessful` + `Reconciled` and DELETE 202 → `Finalized=True` → hard delete; PC-8 proved fresh PUT 201, stale PUT 204 with aggregation unchanged; PC-9's live run returned DELETE 202 → 404 in 20–25 s.

**Tasks:**
- Audit status/error types against PROPOSAL.md §6; carry, do not redesign
- Soft-delete API semantics: 202 response, `Finalized`-gated completion state
- `force-delete` escape hatch (store-only, documented as last resort)
- RFC 9457 error envelope (carry `pkg/api/error.go`); selector syntax spec

**Acceptance criteria:**
- [ ] Status types match the contract exactly (field-by-field audit recorded)
- [ ] Soft delete returns 202 and completes only on all-required `Finalized=True`
- [ ] `force-delete` bypasses gating at the store layer only
- [ ] All errors are RFC 9457; selector syntax documented

**Verification:** API unit tests; contract examples in docs.

---

#### E1-S8 — Layout restructure + core identity + module-path decision
**Size:** M · **Depends on:** E1-S2, E1-S3

**Goal:** Target layout established — `api/ store/ orchestrator/ status/ adapters/ providers/ e2e/ contract/` — README + OWNERS declare core status (rename pending productization), and the module-path strategy is decided (OQ-8).

**Context:** Restructure happens in place with CI green after each move (promotion invariant). Module-path decision de-risks the productization rename (risk #18): SPI and contract-test module paths decoupled from the repo name.

**Tasks:**
- Create the target layout; move existing code into it in CI-green increments
- README + OWNERS: core-repo status, rename deferred (decision #29), component map
- ADR for module-path strategy: runtime module, SPI module, contract-test module (recommendation: decoupled paths — resolves OQ-8)
- Update CONTRIBUTING/AGENTS docs for the new layout

**Acceptance criteria:**
- [ ] Layout matches PROPOSAL.md §5; CI green after every move commit
- [ ] README/OWNERS updated with core status + rename-pending note
- [ ] Module-path ADR filed, OQ-8 resolved
- [ ] ADR numbering continues in-repo (0001–0031 preserved)

**Verification:** `make verify`-equivalent green at each step; layout review.

### E2 — Core runtime hardening: store SPI + de-ROSA tenancy

**Phase:** P0 · **Repo:** core · **Epic size:** L · **Label:** `hf-clm-p0`
**Epic goal:** The promoted runtime hardened in place into the two-mode substrate: repository-port SPI over the existing dao, tenancy de-ROSA'd to JWT claims, identity model matched to ocm-fleet (decisions #23/#24).

#### E2-S1 — Repository-port SPI over the dao
**Size:** M · **Depends on:** E1-S8

**Goal:** Hexagonal repository-port SPI defined at repository semantics (not a SQL-driver abstraction); the existing `pkg/dao` moves behind it unchanged in behavior.

**Context:** Two implementations target the port: postgres (this epic) and the ocm-fleet facade (E3). The port covers cluster/NodePool CRUD, status upsert, drift queries, tenancy scoping, soft delete. Existing tests are the acceptance bar — no behavior change.

**Tasks:**
- Define port interfaces in `store/` (Go): CRUD, status upsert, drift query, scoping
- Adapt `pkg/dao` to implement the port; inject via constructors (no globals)
- Handlers/services compile against the port, not the dao

**Acceptance criteria:**
- [ ] Port SPI reviewed, documented, godoc examples
- [ ] Dao behind the port; full existing test suite passes unchanged
- [ ] No handler/service imports dao directly

**Verification:** full suite green; import-lint rule for the port boundary.

---

#### E2-S2 — Postgres implementation hardening
**Size:** M · **Depends on:** E2-S1

**Goal:** Postgres impl as the CLM-owned schema: goose migrations, JSONB specs, conditions keyed by type, generation-indexed drift queries, SQL transactions.

**Context:** v2.5 already runs Postgres (HYPERFLEET-1518) — carry migration tooling and extend. Drift queries feed E5-S4's detector. Conditions keyed by type = upsert semantics per condition type.

**Tasks:**
- Goose migration chain under `store/postgres/`
- Generation-indexed drift query (clusters where desired `generation` ≠ aggregated observed state, or status stale)
- Condition upsert keyed by type; transactions for multi-row operations
- Selector queries over JSONB (labels/fields)

**Acceptance criteria:**
- [ ] Migrations run forward/back cleanly from the v2.5 baseline
- [ ] Drift query returns exactly the drifted clusters in tests
- [ ] Condition upserts keyed by type; no duplicate condition rows
- [ ] Multi-row ops transactional; JSONB selector queries work

**Verification:** integration tests against a real psql CI service.

---

#### E2-S3 — Identity model reshape to ocm-fleet table shapes
**Size:** M · **Depends on:** E2-S2

**Goal:** Schema reshaped to mirror ocm-fleet's identity model: `platform_resources` (canonical identities, virtual rows, exist while referenced), `extension_resources` (per-adapter representations, uid/aliases), `extension_resource_inventory` (latest observation, labels, conditions), alias claims/contributions.

**Context:** PROPOSAL.md §7 — shaped so the facade impl (E3) maps naturally. Reference: `~/Projects/stolostron/ocm-fleet/docs/design/architecture/` (resource identity and API). Internal reshape only; the public API does not change. Alias examples: kube-system ns uid, cloud provider id, cluster-id.

**Tasks:**
- Study the ocm-fleet resource-identity design docs; write a mapping doc (CLM entity → table shape)
- Migrations + dao updates for the four-table shape
- Record alias claims/contributions on cluster create/observe

**Acceptance criteria:**
- [ ] Schema mirrors the ocm-fleet table shapes; mapping doc included
- [ ] Alias claims/contributions recorded for the canonical examples
- [ ] Existing API tests pass unchanged (public surface untouched)

**Verification:** migration tests; mapping-doc review (OME feedback if available).

---

#### E2-S4 — Tenancy de-ROSA: JWT claims scoping
**Size:** M · **Depends on:** E2-S1

**Goal:** Tenancy derived from configurable JWT claims; scoping enforced at the port layer; zero ROSA-specific assumptions remain.

**Context:** Decision #23: standalone authz = JWT-claims tenancy. `pkg/auth` (`jwt_handler.go`, `identity.go`) and `pkg/tenant` exist — audit for ROSA-isms (hardcoded claim names, issuer assumptions) and generalize.

**Tasks:**
- Claim-mapping configuration (issuer, claim names) — no hardcoded ROSA shapes
- Every store query tenant-scoped at the port layer
- Document the claim contract for operators

**Acceptance criteria:**
- [ ] Tenancy from configurable claims; contract documented
- [ ] Cross-tenant reads/writes denied (404/403 per policy) at the store layer
- [ ] Grep audit: no ROSA-specific code paths remain
- [ ] Multi-tenant test matrix (positive + negative)

**Verification:** auth + store test matrix; negative cross-tenant tests.

---

#### E2-S5 — Standalone REST surface: full CRUD + status sub-resources
**Size:** M · **Depends on:** E2-S4, E1-S7

**Goal:** Standalone mode serves the full v1 REST surface — CRUD + status sub-resources + selectors — from CLM's own server, aligned with the TypeSpec contract assembled in E1.

**Context:** The promoted runtime is already a REST server (`pkg/handlers`, `pkg/services`); this story aligns it with the generated v1 spec rather than rebuilding. The adapter status-upsert *endpoint* lands in E5-S3; resource status *reads* are in scope here.

**Tasks:**
- Align handlers with the TypeSpec-generated server interface (lint-diff handlers vs spec)
- Wire selector query params end-to-end to the store
- Preserve health/metrics/telemetry surfaces

**Acceptance criteria:**
- [ ] Server serves the full v1 OpenAPI surface; spec/handler diff is clean
- [ ] Label/field selector queries work end-to-end
- [ ] Folded e2e CRUD suite (E1-S3) green against the standalone server

**Verification:** e2e suite run; spec-conformance lint.

---

#### E2-S6 — Coverage-parity gate
**Size:** S · **Depends on:** E2-S2, E2-S3, E2-S4, E2-S5

**Goal:** Test coverage not regressed vs the pre-promotion baseline (95 test files).

**Context:** Breakdown E2 AC — existing tests are the acceptance bar for the hardening. This story closes gaps the reshape opened and locks the bar in CI.

**Tasks:**
- Record per-package baseline coverage at the promotion commit
- Backfill tests for reshaped paths (identity tables, tenancy scoping, drift query)
- CI coverage gate at baseline-or-better

**Acceptance criteria:**
- [ ] Per-package coverage ≥ baseline
- [ ] Coverage gate wired into CI

**Verification:** CI coverage report before/after.

---

### E3 — Store: ocm-fleet facade implementation

**Phase:** P0 (build) · **Repo:** core · **Epic size:** M/L · **Label:** `hf-clm-p0`
**Epic goal:** Second store implementation: persistence through the platform's extension-resource + inventory APIs — never the platform's database directly (decision #24). Fleet-mode consumer, but built in P0 per PROPOSAL.md §14 (OQ-5 on timing).

#### E3-S1 — Facade adapter over extension-resource + inventory APIs
**Size:** L · **Depends on:** E2-S1 · **Parallel-safe with:** E5

**Goal:** The facade implements the repository-port SPI against ocm-fleet's extension-resource and inventory APIs.

**Context:** PROPOSAL.md §7; references: `~/Projects/stolostron/ocm-fleet/docs/design/architecture/` (resource identity and API, addon integration). The identity-model reshape (E2-S3) makes the mapping natural. Zero SQL anywhere in this impl.

**Tasks:**
- Client for the platform's extension-resource + inventory APIs
- Implement every port SPI method via platform calls
- Mapping doc: port semantics → platform API calls (incl. soft delete ↔ platform `Resume`/Disconnect semantics)
- Import-lint: no database drivers in the facade package

**Acceptance criteria:**
- [ ] Port SPI fully implemented via platform APIs
- [ ] No direct platform-database access (code review + import lint enforce)
- [ ] Mapping doc reviewed

**Verification:** unit tests against platform API fakes; mapping review.

---

#### E3-S2 — Etag/generation optimistic concurrency
**Size:** M · **Depends on:** E3-S1

**Goal:** Concurrency via etag/generation instead of SQL transactions; conflicts detected, surfaced, and retryable.

**Tasks:**
- Conflict detection on write (compare etag/generation; reject stale writes)
- Retry semantics at the port layer (re-read, re-apply)
- Map conflicts to RFC 9457 409/412 responses

**Acceptance criteria:**
- [ ] Concurrent-write conflicts detected and surfaced as 409/412
- [ ] Retry after re-read succeeds; no lost updates in racing-writer tests
- [ ] Semantics documented (differs from postgres txn behavior by design)

**Verification:** concurrency test suite with simulated racing writers.

---

#### E3-S3 — Connect-time re-registration (content-hash, atomic swap)
**Size:** M · **Depends on:** E3-S1, E4-S1

**Goal:** Schema evolution via Connect-time re-registration — content-hashed schemas, atomic swap — instead of SQL migrations.

**Context:** PROPOSAL.md §7. Registration consumes the proto schemas generated by E4. Failure must leave the previous schema serving.

**Tasks:**
- Registration path on Connect: compute content hash over schemas, register, swap
- Atomic swap (no window serving mixed old/new)
- Failure/rollback behavior; hash-mismatch triggers re-register

**Acceptance criteria:**
- [ ] Schema change → re-registration → atomic swap demonstrated
- [ ] Failed registration leaves the previous schema fully serving
- [ ] Hash mismatch detected and handled

**Verification:** integration test against the platform registrar (or a faithful fake).

---

#### E3-S4 — Shared store conformance suite
**Size:** M · **Depends on:** E3-S1, E3-S2, E3-S3, E2-S2

**Goal:** One behavioral conformance suite both store implementations pass.

**Context:** The assurance mechanism ADR 0031 relies on ("store divergence bounded by conformance"). Runs in core CI: postgres against a CI service, facade against a platform fake. This suite is the store contract bar going forward.

**Tasks:**
- Extract port-level behavioral tests: CRUD, tenancy scoping, drift query, concurrency, soft delete/force-delete, selectors
- Wire per-impl execution into CI
- Document as the store contract bar (new impls must pass it)

**Acceptance criteria:**
- [ ] Suite passes on postgres and on the facade
- [ ] Suite runs in CI for both impls
- [ ] Suite documented as the store contract bar

**Verification:** CI green on both impls; deliberate-break test fails both.

---

### E4 — TypeSpec→proto generator

**Phase:** P0 · **Repo:** core · **Epic size:** S/M · **Label:** `hf-clm-p0`
**Epic goal:** First-party generator emitting proto schemas from the TypeSpec source for fleet-mode Connect-time registration (decision #26; risk #16 — small, well-bounded). Independent of E2/E3/E5.

#### E4-S1 — Generator implementation
**Size:** M · **Depends on:** E1-S2

**Goal:** Generator consumes core TypeSpec (+ overlay packages) and emits proto schemas.

**Context:** TypeSpec is the source of truth; proto schemas are generated artifacts for Connect-time registration (E3-S3 consumes them). Use the TypeSpec compiler's program API; map models/enums/unions to proto messages/enums.

**Tasks:**
- Emitter over the TypeSpec IR → proto schema files
- Include overlay packages in emission
- CLI entry wired into the Makefile

**Acceptance criteria:**
- [ ] Runs over core spec + at least one overlay
- [ ] Output compiles as valid proto (protoc check)
- [ ] Deterministic output (same input → identical bytes)

**Verification:** golden-file tests; protoc compile in CI.

---

#### E4-S2 — Round-trip verification harness
**Size:** S · **Depends on:** E4-S1

**Goal:** Generated proto schemas verified to match the OpenAPI contract semantics.

**Tasks:**
- Compare OpenAPI components vs proto messages: field presence, types, required, enums
- Drift report; documented allowlist for known acceptable divergences

**Acceptance criteria:**
- [ ] Harness compares the full surface and reports drift
- [ ] Known-divergence allowlist documented and reviewed

**Verification:** intentional-drift test (break the TypeSpec, harness fails).

---

#### E4-S3 — CI wiring
**Size:** S · **Depends on:** E4-S2

**Goal:** TypeSpec changes are CI-gated; OpenAPI/proto drift fails builds.

**Tasks:**
- Presubmit: build OpenAPI + proto, run the harness, check golden files
- Actionable failure messages; regeneration instructions in CONTRIBUTING

**Acceptance criteria:**
- [ ] Presubmit blocks on drift (red/green exercised on a scratch PR)
- [ ] Regeneration instructions documented

**Verification:** red/green exercise recorded.

### E5 — Orchestrator skeleton + adapter SPI + status aggregation

**Phase:** P0 · **Repo:** core · **Epic size:** L · **Label:** `hf-clm-p0`
**Epic goal:** The new orchestration core: drift detection, per-cluster workflow state machine, adapter chaining with preconditions, and the status-upsert plane with condition aggregation (PROPOSAL.md §5, §8). No CEL/YAML DSL (decision #4).

#### E5-S1 — Adapter SPI + registry
**Size:** M · **Depends on:** E2-S1, E1-S7

**Goal:** Narrow Go SPI: `Controller` interface + provider interfaces (`DNSProvider`, `LBProvider`; `HostProvider` later); adapter registry.

**Context:** Decision #4 — compiled Go controllers. Porting reference: the hyperfleet-adapter executor refactor (HYPERFLEET-1683, archived). The SPI is what E6 freezes; keep it minimal — orchestration stays in the state machine, not the SPI. Provider interfaces are implementation strategies *inside* adapters, not chain slots (§9 two-layer distinction).

**Tasks:**
- `Controller` SPI: idempotent, status-first reconcile pass (input: cluster view; output: `AdapterStatus` upsert)
- Provider interfaces (`DNSProvider`, `LBProvider`); `HostProvider` stubbed for later
- Registry (name → factory); in-process loading; docs

**Acceptance criteria:**
- [ ] SPI compiles at its own module path (per E1-S8 decision)
- [ ] Registry resolves adapters by name; unknown names fail fast
- [ ] Sample no-op adapter passes; no chain logic lives in the SPI

**Verification:** SPI unit tests; sample adapter; godoc review.

---

#### E5-S2 — Chain composition as data
**Size:** M · **Depends on:** E5-S1

**Goal:** Per-`topology × platform` chains declared as an ordered adapter list with preconditions — data, not code.

**Context:** §9. The three v1 chains must load: `standalone/agent-bm` `[pullsecret, dns?, bm-abi]`; `hosted/oci` `[pullsecret, dns, placement, hcp, oci]`; `hosted/test` `[pullsecret, coreshift]`. Precedent: the POC entity registry `required_adapters`, generalized — PC-3/PC-4 confirmed it live and PC-4 validated every chain adapter name against that entity list, so keep one source of truth for valid adapter names rather than duplicating them in chain config. Opt-out/verify-only decided by precondition.

**Tasks:**
- Chain definition format (startup-loaded config), validated: adapter names resolve, preconditions reference declared adapters
- Chain lookup by `topology × platform`
- Hooks for skip/verify-only semantics (behavior completed in E11-S3)

**Acceptance criteria:**
- [ ] Chains validated at startup; invalid chain fails fast with a clear error
- [ ] Lookup by combo works; the three v1 chains load
- [ ] Adding a capability to a chain = config change only (demonstrated in a test)

**Verification:** chain-table unit tests.

---

#### E5-S3 — Status plane: upsert + aggregation + external adapter surface
**Size:** M · **Depends on:** E5-S1

**Goal:** Adapters upsert `AdapterStatus` (conditions `Available`, `Applied`, `Health`, `Finalized` + free-form `data`); aggregation to `Reconciled` + sticky `LastKnownReconciled`; external adapters get the full out-of-tree surface — list/watch (or efficient polling) plus status upsert.

**Context:** ADR 0008 (dynamic status aggregation) carries. §6: "any process that consumes events and PUTs statuses is a valid adapter" — this endpoint pair *is* the offering-team contract surface. `observed_generation` per adapter; stale `observed_generation` never aggregates to `Reconciled`. **PoC carry evidence (PC-11):** PC-8 proved stale-generation PUT discarded (204), sticky `LastKnownReconciled`, and identical aggregation from the in-process reporter and external REST. **Drift:** the PoC's in-process adapters report through a `StatusReporter` interface over the service layer; the REST endpoint remains the external contract, and PC-8 showed both converge.

**Tasks:**
- Status-upsert REST endpoint (adapter identity auth; idempotent)
- Aggregation rules carried from v2.5; sticky `LastKnownReconciled` across generations
- List/watch or efficient-polling endpoint for external adapters to discover work
- `observed_generation` staleness handling

**Acceptance criteria:**
- [ ] Upsert idempotent; aggregation matches the contract (matrix-tested)
- [ ] `LastKnownReconciled` sticky across generation bumps
- [ ] An external process using only REST (discover + upsert) functions as an adapter
- [ ] Stale `observed_generation` never marks a cluster `Reconciled`

**Verification:** aggregation matrix unit tests; black-box external-adapter e2e (separate process, REST only).

---

#### E5-S4 — Drift detector (sentinel pattern)
**Size:** M · **Depends on:** E5-S2, E2-S2

**Goal:** Generation-mismatch drift detection drives reconciliation: poll the store's drift query, enqueue drifted clusters, drive workflows; label sharding for horizontal scale.

**Context:** Pattern harvested from hyperfleet-sentinel (drift poll → drive adapters; label sharding) — the CEL decision engine explicitly dies. Shard via labels so multiple replicas partition work without overlap.

**Tasks:**
- Poll loop with configurable interval + backoff; enqueue drifted clusters
- Label-based shard assignment (multi-replica safe)
- Metrics: drift lag, queue depth, reconciliation pass duration

**Acceptance criteria:**
- [ ] New/incremented generation triggers reconciliation within one poll interval
- [ ] Two replicas shard work without overlap or loss
- [ ] No CEL anywhere (grep audit); metrics emitted

**Verification:** integration test (bump generation → workflow starts); two-replica sharding test.

---

#### E5-S5 — Per-cluster workflow state machine
**Size:** L · **Depends on:** E5-S2, E5-S3, E5-S4

**Goal:** Ordered adapter execution per cluster with preconditions, retry-safe passes, persisted workflow state; deletion gated on `Finalized` across the declared chain; `force-delete` bypass.

**Context:** Ordering dependencies are preconditions in the state machine, not adapter-internal magic (decision #4). Every pass retry-safe; workflow state survives restart. Update = generation bump → chain re-run. Delete = chain delete path → all required adapters `Finalized=True` → soft-delete completes.

**Tasks:**
- State machine with persisted per-cluster workflow records
- Precondition evaluation before each adapter slot; failed adapter blocks dependents and surfaces its condition
- Retry with backoff; restart-resume from persisted state
- Delete orchestration + `Finalized` aggregation + `force-delete` store-only path

**Acceptance criteria:**
- [ ] Chains execute in order with preconditions enforced
- [ ] Mid-chain failure blocks dependents; the failing adapter's condition is visible
- [ ] Retries idempotent; state survives process restart
- [ ] Delete completes only on all-required `Finalized=True`; `force-delete` works
- [ ] Table-driven scenario tests: success, mid-chain failure, retry, delete gating

**Verification:** state-machine scenario suite; restart-resume test.

---

#### E5-S6 — Pre/post Jobs extension hooks
**Size:** S · **Depends on:** E5-S5

**Goal:** Partner extension via pre/post Jobs configured around adapter slots.

**Context:** Decision #4: "hardcoded lifecycle logic, YAML-configurable parameters, and pre/post Jobs for partner extension."

**Tasks:**
- Job template config per chain slot; Kubernetes Job execution before/after the pass
- Job failure blocks/flags per configuration; conditions surface job outcomes
- Jobs safe to re-run on retry; docs

**Acceptance criteria:**
- [ ] Pre/post jobs configurable per slot; failure handling per config
- [ ] Job outcomes visible in conditions; re-runs safe

**Verification:** e2e with a sample job.

---

### E6 — Adapter contract v1 freeze — P0 EXIT CRITERION 1

**Phase:** P0 · **Repo:** core · **Epic size:** M · **Label:** `hf-clm-p0`
**Epic goal:** Freeze the published adapter contract: SPI Go module (semver), REST status API spec, TypeSpec overlay packaging, contract tests — the surface both parallel tracks and the OCI offering team build against (risks #8, #9). **Gates E8–E15.**

#### E6-S1 — Contract assembly + publication pipeline
**Size:** M · **Depends on:** E5-S1, E1-S6, E1-S8

**Goal:** The three contract surfaces assembled and published as versioned artifacts via a tag-driven pipeline: SPI Go module (semver), REST status API spec, TypeSpec overlay packages.

**Context:** The ORO test is the design gate — a partner must be able to build and operate an adapter out-of-tree using only the published contract and public docs. Module paths per the E1-S8 decision (decoupled from repo name).

**Tasks:**
- Module split finalized: SPI module + contract-test module at their own paths
- Publication pipeline: tag → module release + spec artifacts + overlay packages
- Versioning docs + consumer upgrade guide skeleton

**Acceptance criteria:**
- [ ] Pipeline publishes all three surfaces on tag
- [ ] Semver policy stated; artifacts importable from an external repo (dry-run)
- [ ] Upgrade guide skeleton published

**Verification:** dry-run publish; import test from a throwaway module.

---

#### E6-S2 — Contract-test module (own module path)
**Size:** M · **Depends on:** E6-S1

**Goal:** Separately-importable contract-test Go module — offering CI imports conformance without the runtime.

**Context:** Decision #30; survives the productization rename (risk #18). Two modes: SPI-level (in-process) and REST-level (against any endpoint implementing the status API).

**Tasks:**
- Extract conformance tests to the standalone module (no runtime deps beyond SPI)
- SPI-level + REST-level modes
- Docs + example wiring for offering CI

**Acceptance criteria:**
- [ ] Module importable standalone; both modes runnable
- [ ] Conformance passes against core (reference impl) and fails against a deliberately-broken fake
- [ ] Example consumer wiring documented

**Verification:** import from a throwaway module; pass/fail exercise.

---

#### E6-S3 — N-2 policy + governance doc
**Size:** S · **Depends on:** E6-S1

**Goal:** Compatibility and breaking-change arbitration documented: N-2 window, deprecation flow, escalation path (may be TBD — OQ-2), emergency-break process.

**Context:** Risks #8/#9; post-freeze changes require both consuming teams' sign-off.

**Tasks:**
- Policy doc: N-2 window, deprecation flow, arbitration/escalation, emergency breaks
- Review with both track leads + the Oracle counterpart

**Acceptance criteria:**
- [ ] Policy published with the contract artifacts
- [ ] Escalation path recorded (or explicitly TBD with owner + date)
- [ ] Track-lead review recorded

**Verification:** doc review.

---

#### E6-S4 — v1 sign-off reviews + freeze — P0 EXIT 1
**Size:** S · **Depends on:** E6-S2, E6-S3, E4-S3, E5-S5

**Goal:** Both track teams + the OCI offering team sign off on v1; the contract is frozen and tagged.

**Context:** The payoff moment for three generations of contract stability — the team's contract is good enough to freeze for a partner. This story is the P0 exit criterion that gates all of P2.

**Tasks:**
- Review sessions per consuming team; issue log with dispositions
- Tag + publish v1.0.0; announce
- Post-freeze change process goes live (sign-off required)

**Acceptance criteria:**
- [ ] Sign-offs from both track teams + the OCI offering team recorded
- [ ] v1.0.0 tagged and published
- [ ] Post-freeze change process active and documented

**Verification:** tagged artifacts; sign-off notes; gate check for E8+.

---

### E7 — Spike: UnmanagedEtcdSpec support verification — P0 EXIT CRITERION 2

**Phase:** P0 · **Epic size:** S · **Label:** `hf-clm-p0`
**Epic goal:** Verify `UnmanagedEtcdSpec` support level with the HyperShift team and its combination with `platform: External` (risks #2, #19). Decides etcd-manager sequencing. **Start immediately — no dependencies.**

#### E7-S1 — HyperShift engagement + findings doc
**Size:** S · **Depends on:** none

**Goal:** Written findings: `UnmanagedEtcdSpec` support level, target release, caveats, immutability constraints.

**Context:** Mechanism exists upstream: `hypershift/api/hypershift/v1beta1/hostedcluster_types.go` — `EtcdManagement`/`UnmanagedEtcdSpec` (single HTTPS client endpoint + mTLS client secret, immutable after creation). Fallback if insufficient: CPO-managed etcd + anti-affinity + `EtcdBackup` default-on (decision #18, conditional). **Research findings (2026-11-06, see `ETCD-SERVICE-DESIGN.md` §1–2):** the field is ungated (Default feature set) but untested upstream — zero e2e coverage, assumed-True availability, no connectivity check; and the local hypershift clone predates `platform: External` (not in the enum), so the × External question must be verified upstream. The findings doc must confirm ungated = supported and raise the availability-truth gap.

**Tasks:**
- Engage the HyperShift team; compile the findings doc
- Verify claims against the upstream source
- Circulate for accuracy confirmation

**Acceptance criteria:**
- [ ] Findings doc filed: support level, release, caveats, immutability constraints
- [ ] HyperShift team confirms accuracy

**Verification:** doc + confirmation thread.

---

#### E7-S2 — External-combination questions + go/no-go ADR — P0 EXIT 2
**Size:** S · **Depends on:** E7-S1

**Goal:** `UnmanagedEtcdSpec` × `platform: External` verified; the multi-endpoint upstream ask (`endpoints []string` → `--etcd-servers` list) filed; go/no-go decision recorded for E13.

**Context:** The upstream enhancement (openshift/enhancements#2168) is silent beyond excluding etcd secrets from the integrator Role (risk #19). Raise upstream during the review window if needed. Gates E13 explicitly.

**Tasks:**
- Question list to the upstream enhancement (etcd × External; endpoint LB ownership)
- Analysis + upstream thread
- ADR: go (E13 proceeds) or no-go (fallback per decision #18; E13 slips to the release where support lands)

**Acceptance criteria:**
- [ ] ADR filed with the decision + fallback plan
- [ ] Upstream questions raised and linked
- [ ] E13's conditionality resolved in writing

**Verification:** ADR review; upstream thread linked.

### E8 — hcp adapter in core: HostedCluster orchestration

**Phase:** P2 · **Repo:** core · **Epic size:** L · **Label:** `hf-clm-p2`
**Epic goal:** The hcp adapter in the core repo — the translator chain unit: cluster spec → HostedCluster/NodePool CRs, publishing, kubeconfig handoff (PROPOSAL.md §12; decision #33). CLM deploys HyperShift artifacts; it never forks HyperShift ("deploy CPO, don't reimplement it"). The hosted machinery is core functionality: hcp adapter (this epic), placement adapter (E9), transport port (E10) — three separate concerns. HyperShift api-types are scoped to the hcp adapter's package: the adapter is the abstraction layer, so it versions with the thing it abstracts.

#### E8-S1 — hcp adapter lands in core
**Size:** M · **Depends on:** E6-S4

**Goal:** The hcp adapter in core on a clean module/package boundary, HyperShift api-types scoped to its package.

**Context:** Decision #33: hosted machinery is core functionality — the repo/train separation was machinery for a translator. Keep the package boundary clean so extraction stays cheap if the org ever demands it (fold is easy, unfold is expensive). Seeds: HYPERFLEET 1539–1543 groundwork.

**Tasks:**
- Package/module bootstrap in core (adapters/hcp), HyperShift api-types dependency scoped to it
- Register in the chain registry; hosted/oci chain slot wired
- CI: unit + integration in the core presubmit (in-tree — no digest pinning needed)

**Acceptance criteria:**
- [ ] Adapter package in core, building and testing green in core CI
- [ ] HyperShift api-types do not leak beyond the hcp adapter package (import check)
- [ ] Clean module boundary documented

**Verification:** core CI green; import-lint for the api-types boundary.

---

#### E8-S2 — HostedCluster reconciliation adapter
**Size:** L · **Depends on:** E8-S1

**Goal:** The hcp adapter drives HostedCluster lifecycle through the chain: create, update, delete with `Finalized` gating — against `platform: None`.

**Context:** Renders the HostedCluster CR from the cluster spec blob (overlay) plus chain context (placement decision, etcd wiring). Writes go through the transport port (E10 implements ManifestWork; a local-apply impl suffices for tests until then).

**Tasks:**
- HostedCluster render/apply via the transport port
- Condition mapping: HostedCluster conditions → `AdapterStatus` `Available`/`Applied`/`Health`
- Update flow: generation bump → re-reconcile; `observed_generation` tracking
- Delete flow with sequencing + `Finalized` contribution

**Acceptance criteria:**
- [ ] Create → HostedCluster appears on the MC; conditions mapped to adapter status
- [ ] Spec update → re-reconcile at the new generation
- [ ] Delete → `Finalized` only when the HostedCluster is gone
- [ ] Runs against `platform: None` on a test MC

**Verification:** integration vs a kind MC + HyperShift operator; condition-mapping table tests.

---

#### E8-S3 — NodePool reconciliation
**Size:** M · **Depends on:** E8-S2

**Goal:** NodePool lifecycle in the hcp adapter: replicas, upgrade rolls, per-NodePool status; `Finalized` gating includes NodePools.

**Context:** NodePool is a first-class resource (E1-S4). Design decision in-story: per-NodePool `AdapterStatus` vs sub-conditions on the cluster. The machineTemplate name-change roll trigger (relevant under External later) is surfaced now.

**Tasks:**
- NodePool render/apply; replica + condition status surfacing
- Node-roll trigger detection and surfacing
- Delete gating contribution

**Acceptance criteria:**
- [ ] NodePool CRUD via the chain; replicas + conditions surfaced
- [ ] Roll triggers visible in status
- [ ] `Finalized` gating includes all NodePools

**Verification:** integration tests.

---

#### E8-S4 — Publishing + kubeconfig handoff
**Size:** M · **Depends on:** E8-S2

**Goal:** Publishing (hosted kubeconfig) surfaced in the adapter status `data` blob.

**Context:** §12 hcp adapter scope. Kubeconfig is stored as a secret *reference* in the status data — never inline in status.

**Tasks:**
- Retrieve the hosted kubeconfig once the HostedCluster is Available
- Store as a secret ref; expose the ref in status `data`
- Handle kubeconfig rotation; document access control on handoff

**Acceptance criteria:**
- [ ] Kubeconfig ref available in status `data` after `Available`
- [ ] Secret material never appears in status/logs
- [ ] Rotation picked up without manual intervention

**Verification:** e2e — a consumer fetches the kubeconfig via the API and talks to the hosted cluster.

---

#### E8-S5 — Deletion sequencing + persistent gotchas
**Size:** M · **Depends on:** E8-S3

**Goal:** Delete path hardened against the §12 persistent gotchas: delete sequencing, ingress as LB Service at creation, `allowedCIDRBlocks` via direct patch, RWO registry volume, per-resource-kind tagging.

**Context:** Gotchas apply under any platform type (PROPOSAL.md §12 list). Where testable, encode as tests; the rest goes to the runbook (E14-S4).

**Tasks:**
- Ordered teardown: NodePools before HostedCluster; platform resources before finalizers
- Timeouts + retry; orphan audit after delete
- Encode testable gotchas as regression tests

**Acceptance criteria:**
- [ ] Delete completes cleanly on the test MC; repeated-delete e2e stable
- [ ] No orphaned resources in the post-delete audit
- [ ] Testable gotchas covered by regression tests

**Verification:** repeated delete e2e + resource audit.

---

#### E8-S6 — `platform: None` E2E on a real MC
**Size:** M · **Depends on:** E8-S2, E8-S3, E8-S4

**Goal:** The hcp adapter runs the hosted chain end to end against `platform: None` HostedClusters on a management cluster.

**Context:** E8's capstone AC. On None, worker lifecycle is the offering adapter's job (Oracle) — for this test, workers are manual/optional; the control-plane path is what is proven.

**Tasks:**
- Test MC setup (kind with HyperShift operator; OKE-class via E14-S1 manifests)
- Hosted chain run minus the platform adapter; green-path status verification

**Acceptance criteria:**
- [ ] HostedCluster reaches `Available` on the test MC via the CLM chain
- [ ] Conditions visible in inventory end to end

**Verification:** e2e run in core CI (in-tree hosted chain).


### E9 — Placement

**Phase:** P2 · **Repo:** core · **Epic size:** M · **Label:** `hf-clm-p2`
**Epic goal:** Management-cluster selection at HCP create — capacity/geo/affinity aware, round-robin default; MC list from hub `ManagedCluster` CRs (decision #9). MCs are **never CLM-managed objects**: the fleet list is a read-through view of whoever owns the fleet (MCE-classic hub in standalone mode; ocm-fleet in fleet mode); the only persisted artifact is the placement decision on the cluster record.

#### E9-S1 — Read-through MC view from hub `ManagedCluster` CRs
**Size:** M · **Depends on:** E8-S1

**Goal:** Placement's candidate list sourced from the hub's `ManagedCluster` CRs as a read-through view — never as CLM-managed objects.

**Context:** Bootstrap chicken-egg (MCs must be MCE-managed before placement targets them) is solved by the day-0 flow — out of scope here; placement simply never targets a non-Managed cluster. **No MC tracking**: MCs get no records in CLM's store, no conditions, no drift, no lifecycle, no per-MC controllers — the fleet platform owns the fleet; CLM reads its view at selection time (an informer/TTL cache is an implementation detail, not ownership). The only persisted placement artifact is the decision on the *cluster* record (E9-S2). Mirrors the non-goal "fleet-scale observed-state indexing — ocm-fleet's inventory": fleet membership is read, not owned.

**Mode split:** this story's source is the standalone-mode one — the MCE-classic hub's `ManagedCluster` CRs. Fleet mode swaps the source to the platform inventory (extension-resource/inventory APIs), a FM-97 refinement under the same read-through principle. Day-0 (FM-96) runs no placement at all — its chains (`standalone/agent-bm`, `hosted/test`) carry no placement slot; placement becomes live once the first hub exists and MCs are MCE-managed.

**Tasks:**
- Hub client (kube/dynamic API): list `ManagedCluster` at selection time (informer or TTL cache acceptable — no reconcile loop, no store writes)
- Candidate filter: clusterSet/label eligibility; exclude absent/unhealthy MCs
- Read capacity/geo hints from MC labels/claims where present

**Acceptance criteria:**
- [ ] Candidate list reflects the hub's view; an MC removed from the hub is no longer a candidate
- [ ] Unhealthy/absent MCs never targeted
- [ ] Zero MC records in CLM's store; no per-MC controller exists (code review + store-schema audit)

**Verification:** integration against a fake hub (CR mocks); store-schema audit shows no MC kinds/tables.

---

#### E9-S2 — Placement chain unit
**Size:** M · **Depends on:** E9-S1, E5-S2

**Goal:** Placement runs as a chain unit for hosted creates; the decision is recorded on the cluster record (status + inventory) — the only persisted placement artifact.

**Context:** The evolution-workstream placement operator is subsumed into core (§4 dispositions, decision #33). v1 places at create only; re-placement hardening is P3. Stickiness: the decision is pinned once at create; the read-through fleet view (E9-S1) is not re-evaluated against placed clusters.

**Tasks:**
- Placement adapter implementing the SPI
- Write the decision (target MC + rationale) to the cluster record's status `data`
- Chain precondition: the hcp adapter waits for placement

**Acceptance criteria:**
- [ ] Hosted create → placement selects an MC before the hcp adapter runs
- [ ] Decision visible in status/inventory; precondition enforced

**Verification:** chain e2e with a multi-MC fake inventory.

---

#### E9-S3 — Selection policies
**Size:** S · **Depends on:** E9-S2

**Goal:** Round-robin default plus capacity/geo/affinity inputs.

**Tasks:**
- Policy interface; round-robin implementation (default)
- Capacity/geo/affinity filters (best-effort inputs from MC labels + spec knobs)
- Policy selection via configuration; rationale recorded in the decision

**Acceptance criteria:**
- [ ] Round-robin distributes across MCs deterministically
- [ ] Filters exclude per inputs; rationale recorded
- [ ] Policy selection configurable without code changes

**Verification:** deterministic policy unit tests.

---

### E10 — Standalone transport: ManifestWork + managedserviceaccount

**Phase:** P2 · **Repo:** core · **Epic size:** L · **Label:** `hf-clm-p2`
**Epic goal:** The transport port in core — a mechanism below the chain, not a chain unit (decision #33). Standalone-mode delivery: CRs delivered as ManifestWorks in per-MC hub namespaces (klusterlet applies, generation-tracked); deep status read via `managedserviceaccount`-mediated short-lived credentials (decision #8). No central component holds long-lived admin credentials. Maestro gone.

#### E10-S1 — ManifestWork writer + generation tracking
**Size:** L · **Depends on:** E8-S1, E6-S4

**Goal:** HostedCluster/NodePool/InfraEnv CRs delivered as ManifestWorks in per-MC hub namespaces; klusterlet applies; generation tracked desired-vs-applied.

**Context:** Decision #8. Also the declared replacement destination for the v2.5 Desire Store line (E17 — same transport, different consumers). Maestro decommission (HYPERFLEET-1502) is the org precedent. Implement behind the transport port so a `none` impl (day-0) and fleetlet (fleet mode) can slot later.

**Tasks:**
- Transport port interface (write CRs to an MC; report applied generation; delete)
- ManifestWork rendering: per-MC namespace, manifest bundles
- Generation tracking from ManifestWork status; update rolls via MW update
- Delete → ManifestWork deletion + klusterlet cleanup

**Acceptance criteria:**
- [ ] CR applied on the MC via klusterlet; desired vs applied generation visible
- [ ] Update rolls via ManifestWork update; delete cleans up
- [ ] Transport port interface published with a local-apply test impl

**Verification:** integration against a hub + klusterlet (kind-based).

---

#### E10-S2 — MSA credential mediation + deep-status reader
**Size:** L · **Depends on:** E10-S1

**Goal:** Deep status (HostedCluster conditions, NodePool replicas) read via `managedserviceaccount`-mediated short-lived credentials; the adapter-status push API path stays preserved in the contract for a future on-MC reporter.

**Context:** Decision #8 — read path is MSA-mediated, read-only, resource-scoped. No central long-lived admin credentials; the status-push reservation means a future on-MC reporter needs no API change.

**Tasks:**
- MSA provisioning per MC (hub-side) — for MCs that host placed clusters, derived from placement decisions, never from fleet tracking
- Token acquisition + refresh
- Deep-status reader querying MC APIs with scoped RBAC (read-only)
- RBAC scope doc; credential audit proving nothing long-lived is held centrally
- Regression test asserting the status-push contract surface is unchanged

**Acceptance criteria:**
- [ ] Deep status read via MSA tokens; tokens short-lived + auto-refresh
- [ ] Central credential audit clean (no long-lived admin creds)
- [ ] Read-only resource-scoped RBAC enforced
- [ ] Status-push API path preserved (contract test)

**Verification:** integration test; security review checklist.

---

#### E10-S3 — Transport conformance against real MCs
**Size:** M · **Depends on:** E10-S2, E8-S2

**Goal:** Transport works for the hcp-adapter chain E2E against a real MC (kind + OKE-class).

**Context:** E10's capstone AC. The OKE profile exercises the E14-S1 prerequisite manifests; kind is the always-available default.

**Tasks:**
- Conformance profile: full hosted chain on a kind MC via ManifestWork transport
- OKE-class profile (CI job or runbook); flake hardening

**Acceptance criteria:**
- [ ] Chain e2e green on kind MC via ManifestWork transport
- [ ] OKE-class profile documented and runnable

**Verification:** CI runs (kind) + OKE runbook/job.

### E11 — Capability adapters: pullsecret, dns, lb

**Phase:** P2 · **Repo:** core · **Epic size:** M · **Label:** `hf-clm-p2`
**Epic goal:** Shared capability adapters in the core repo — chain units behind the SPI with pluggable provider interfaces (decision #12). The core runtime stays capability-free; these are adapters, never core code.

#### E11-S1 — pullsecret adapter
**Size:** S · **Depends on:** E6-S4, E5-S5

**Goal:** Pull secret / credentials handling as the first chain unit — present in every v1 chain.

**Context:** §9 chains all start with pullsecret. Carries the POC pullsecret adapter's behavior. Handles the pull secret source (secret ref in spec or central config), distribution to the MC via transport, and cleanup on delete where owned.

**Tasks:**
- Pull secret source resolution (spec secret ref or central config)
- Distribution to the MC / injection into rendered CRs
- Status conditions; `Finalized` on delete (cleanup of owned secrets)

**Acceptance criteria:**
- [ ] Runs as the first slot in every v1 chain
- [ ] Missing secret → clear condition, chain blocked (not silently skipped)
- [ ] Delete cleans up owned resources; `Finalized` gating participates

**Verification:** chain tests (presence, missing-secret, delete paths).

---

#### E11-S2 — dns adapter + DNSProvider implementations
**Size:** M · **Depends on:** E11-S1

**Goal:** dns capability adapter with at least one DNSProvider implementation (embedded + RFC2136 or external-dns).

**Context:** `DNSProvider` is an implementation strategy *inside* the adapter, not a chain slot (§9 two-layer distinction). Provider set: RFC2136 / external-dns / embedded. Record lifecycle: api/ingress names per cluster spec.

**Tasks:**
- `DNSProvider` interface (create/verify/delete records)
- Embedded provider (trivial DNS — laptop/dev); RFC2136 provider
- Record lifecycle driven by cluster spec; status conditions

**Acceptance criteria:**
- [ ] Records created/verified/deleted per provider
- [ ] Provider selected by config; adding a provider requires no chain change
- [ ] Status conditions reflect record state

**Verification:** provider unit tests; integration (embedded against a real resolver).

---

#### E11-S3 — dns opt-out / verify-only preconditions
**Size:** S · **Depends on:** E11-S2

**Goal:** `dns: {managed: false, userRecords: [...]}` excludes the adapter or runs it verify-only, decided by precondition.

**Context:** §9 opt-outs — the adapter is excluded or runs verify-only, decided by precondition; this completes the precondition semantics stubbed in E5-S2.

**Tasks:**
- Precondition evaluation on the spec knob
- Verify-only mode: check user records resolve, report condition
- Exclusion mode: chain skips the slot entirely

**Acceptance criteria:**
- [ ] `managed: false` + userRecords → verify-only; passes when records resolve, flags when not
- [ ] Explicit exclusion config skips the slot
- [ ] Both paths produce correct status

**Verification:** unit + e2e variation tests.

---

#### E11-S4 — lb adapter + OCI-NLB provider
**Size:** M · **Depends on:** E11-S1

**Goal:** lb capability adapter with the `LBProvider` interface and the OCI-NLB implementation (ORO path).

**Context:** LB is shared infrastructure (§9). The etcd client endpoint is no longer an lb consumer — etcd went multi-endpoint with no LB (decision #35) — so the core lb capability's consumer is the hcp api-ingress; the provider set stays general. OCI-NLB per the §5 provider set.

**Tasks:**
- `LBProvider` interface: provision endpoint LB → endpoint addr; delete; health
- lb adapter chain unit; endpoint addr surfaced in status `data`
- OCI-NLB implementation behind OCI credentials config

**Acceptance criteria:**
- [ ] LB provisioned/deleted via the chain; endpoint addr in status `data`
- [ ] OCI-NLB impl tested (mocked OCI API; optional real profile)
- [ ] Provider contract documented for future impls

**Verification:** unit + mocked integration; provider docs.

---

#### E11-S5 — lb dev-friendly provider + shared-use validation
**Size:** S · **Depends on:** E11-S4

**Goal:** At least one dev-friendly `LBProvider` (keepalived-VIP or MetalLB) plus validation that two consumers use the lb adapter concurrently.

**Context:** E11 AC: "lb adapter with OCI-NLB impl + ≥1 dev-friendly impl." Primary consumer: hcp api-ingress (E8) — per-cluster isolation, one adapter. (The etcd client endpoint was once the second consumer; etcd went multi-endpoint with no LB — decision #35.)

**Tasks:**
- Dev provider (MetalLB or keepalived-VIP) — no cloud credentials
- Dual-consumer validation scenario; docs

**Acceptance criteria:**
- [ ] Dev provider works in CI without cloud credentials
- [ ] Two consumers use the lb adapter concurrently with per-cluster isolation

**Verification:** CI chain using the dev provider; dual-consumer test.

---

### E12 — Coreshift test adapter + hosted-chain CI (layered)

**Phase:** P2 · **Repo:** core (`e2e/`) · **Epic size:** M · **Label:** `hf-clm-p2`
**Epic goal:** First-party test/dev adapter invoking the coreshift CLI (separate project) to run a *real* OpenShift control plane — real kube-apiserver, real conditions, real kubeconfig handoff — with no cloud credentials; plus the layered CI for the hosted chain (decisions #15, #30, #31). Taxonomy: `hosted` means control-plane *placement* — control plane as workloads on a substrate the cluster doesn't own — not HyperShift. `hosted/coreshift` is a control plane in local podman containers; its chain `[pullsecret, coreshift]` encodes the absence of placement/transport/hcp because placement trivially resolves to the podman substrate itself. E8–E10 build the MC-substrate hosted machinery.

#### E12-S1 — Coreshift adapter (thin CLI glue)
**Size:** M · **Depends on:** E6-S4, E1-S3

**Goal:** Adapter invoking the coreshift CLI to mint a real OpenShift control plane with no cloud credentials.

**Context:** Risk #10 — thin glue only; coreshift is independently useful to its own track. The PoC pins the **rewrite** (`~/Projects/vkareh/coreshift`, active, built Nov 2026): podman-based, kind-style grammar `create cluster` / `get clusters` / `get kubeconfig` / `delete cluster`, verified against the OpenShift 4.22 line. The paused original (`openshift-hyperfleet/coreshift`, Aug 2026, `up`/`status`/`down`) is not the target. Coreshift stays a separate project (decision #15).

**Tasks:**
- Adapter invoking the coreshift CLI (`create cluster`, container target); version-pinned
- Lifecycle mapping: `create cluster` / `get clusters` / `get kubeconfig` / `delete cluster` → chain create/health/kubeconfig/delete
- Kubeconfig handoff via `get kubeconfig` into status `data`; cleanup on delete

**Acceptance criteria:**
- [ ] Adapter mints a control plane via the CLI; conditions are real
- [ ] Delete cleans up processes/containers fully
- [ ] No cloud credentials anywhere in the path

**Verification:** adapter e2e in CI (the hosted/test chain).

---

#### E12-S2 — `hosted/test` chain as blocking presubmit
**Size:** M · **Depends on:** E12-S1, E11-S1

**Goal:** The `hosted/test` chain `[pullsecret, coreshift]` runs end-to-end as a blocking presubmit; layer-1 blocking gates contain zero external dependencies.

**Context:** CI layering (decision #31): layer 1 is core committers only. External-dep absence is enforced in CI config review, not by convention.

**Tasks:**
- Presubmit job for the chain; runtime budget negotiated
- Layer-1 external-dep enforcement: lint job configs for external references
- Flake handling: retry policy, quarantine rules

**Acceptance criteria:**
- [ ] Chain green as a blocking presubmit on core PRs
- [ ] External-dep lint passes (zero external dependencies in blocking gates)
- [ ] Runtime within the documented budget

**Verification:** presubmit runs on core PRs; budget report.

---

#### E12-S3 — Layer-2 pinned profiles + Layer-3 digest-tracking
**Size:** M · **Depends on:** E12-S2

**Goal:** Digest-pinned full-stack profiles (blocking or notify) in layer 2; non-blocking periodic digest-tracking jobs in layer 3 with attributed tickets.

**Context:** Testing model (breakdown §4): digests move via Renovate-style bump PRs, never tracking; a broken offering build = a red pin-bump PR (close it, pin stays) or an attributed periodic ticket — never a blocked core merge. Rejected: a separate integration repo.

**Tasks:**
- Profile definitions per chain (hosted/hcp, hosted/oci)
- Layer-2 wiring: pinned profiles on bump PRs
- Layer-3 wiring: periodic jobs against latest adapter builds; ticket-filing automation with attribution labels

**Acceptance criteria:**
- [ ] Layer-2 profiles run on bump PRs; a red bump PR is closeable without moving the pin
- [ ] Layer-3 periodic files attributed tickets; non-blocking
- [ ] No core merge can be blocked by an external adapter build

**Verification:** bump-flow exercise (green + deliberately red).

---

#### E12-S4 — Bump automation + flake budget
**Size:** S · **Depends on:** E12-S3

**Goal:** Renovate-style digest bump PRs on a per-source cadence (hcp daily, offerings weekly/manual); CI runtime + flake budget documented.

**Tasks:**
- Renovate config: digest sources, per-source cadence, labels
- Flake budget doc: allowances, quarantine rules, escalation
- CI runtime report/dashboard

**Acceptance criteria:**
- [ ] Bumps arrive as PRs on cadence; cadence squad-tunable
- [ ] Flake budget published; flake rate tracked

**Verification:** observe bump PRs over a week; budget doc review.

---

### E13 — Etcd capability v1 (conditional on E7)

**Phase:** P2 · **Repo:** core · **Epic size:** XL · **Label:** `hf-clm-p2`
**Epic goal:** The in-core etcd adapter: fleet-spread etcd for hosted clusters opting in via `etcd: {managed: clm}` (default `cpo`) — a chain slot for any hosted platform (decisions #7/#35). Dedicated per HCP, never shared. Co-location + spread; multi-endpoint clients, no LB; CLM never in the data, client, or restart-healing paths. Design: ETCD-SERVICE-DESIGN.md (rev 2).

> **Conditional:** proceeds only on E7-S2 *go*. On *no-go*: hosted clusters ship with CPO-managed etcd + anti-affinity + `EtcdBackup` default-on, and this epic slips to the first release where `UnmanagedEtcdSpec` support lands (decision #18).

#### E13-S1 — Adapter foundations: membership, mTLS, member minting
**Size:** L · **Depends on:** E6-S4, E5-S5

**Goal:** The etcd adapter's machinery, tested: internal membership model, mTLS minting, and member manifests — one StatefulSet per member slot with the reset-member init container and a per-MC Service for slot-stable addressing — delivered behind a minimal transport interface (direct-kubeconfig impl; ManifestWork is E10's product work).

**Context:** The reset-member init container (HyperShift harvest H5) re-joins the quorum before etcd starts — name-keyed `etcdctl member remove/add`, idempotent, operator-free: this is what keeps CLM out of the restart-healing path. mTLS is mandatory (`--client-cert-auth --peer-cert-auth`); the adapter mints the CA, member server/peer certs, and the client cert whose three-key secret (`etcd-client-ca.crt/crt/key`) is the seam's handoff. No CRD — membership is internal state reconciled from etcd's own member list plus observed pods. Status-data contract: slot endpoints, member health, quorum.

**Tasks:**
- Membership model + status-data contract (slot endpoints, member health, quorum) via the StatusReporter
- mTLS minting: CA, member server/peer certs, client cert + three-key secret shape
- Member manifests: STS-per-member, reset-member init container, per-MC Service, PVC
- Minimal transport interface (apply/list to a target MC) with a direct-kubeconfig impl

**Acceptance criteria:**
- [ ] Member manifests render correctly; init-container rejoin logic unit-tested (fake etcd)
- [ ] mTLS material minted and wired into manifests; three-key secret emitted
- [ ] Status data published through the reporter
- [ ] Fake-client unit tests only — no kind, no live etcd

**Verification:** unit tests with fakes; `make verify` + `make test` green.

---

#### E13-S2 — Co-located placement + spread
**Size:** M · **Depends on:** E13-S1, E9-S2

**Goal:** Member-0 lands on the placement decision's MC (sibling namespace); members 1/2 spread across two other MCs from the read-through fleet view.

**Context:** Co-location is a mint-time preference, never an invariant — after re-homing, nothing moves. 3 members / 3 MCs: any single MC loss leaves 2/3 quorum serving. The fleet view is E9's — its second consumer.

**Tasks:**
- Consume the placement decision for member-0's MC
- Spread members 1/2 across distinct MCs from the fleet view
- Chain wiring: the etcd slot activates on `etcd: {managed: clm}`; hcp waits for etcd (precondition)

**Acceptance criteria:**
- [ ] Members land 1-per-MC with member-0 co-located, verified against a fake fleet
- [ ] Insufficient MCs → clear condition, no partial quorum minted
- [ ] Chain precondition enforced

**Verification:** unit tests with fake fleet view + fake transport.

---

#### E13-S3 — Multi-endpoint wiring + the seam
**Size:** M · **Depends on:** E13-S1, E8-S2

**Goal:** The hcp adapter renders the etcd config from the etcd adapter's status data — the endpoints list (all member slots) once the upstream ask lands, or the single fabric-DNS endpoint via the documented fallback — plus the three-key secret in HC-namespace shape.

**Context:** UnmanagedEtcdSpec-shaped validation without a HostedCluster: endpoint format, secret keys, immutability semantics. The client secret lands in the HC namespace; HO's existing sync does the rest. Health truth is ours — HyperShift assumes True.

**Tasks:**
- hcp-side rendering of the endpoints list from etcd status data
- Three-key secret emitted into HC-namespace shape
- Single-endpoint fallback path (fabric-DNS name re-pointed by the adapter)
- UnmanagedEtcdSpec-shaped contract validation

**Acceptance criteria:**
- [ ] Endpoints list + secret render correctly from status data
- [ ] Fallback path exercised and documented
- [ ] Contract validation (format, keys, immutability) green

**Verification:** unit tests; contract-shape tests.

---

#### E13-S4 — Membership sequencing
**Size:** L · **Depends on:** E13-S1

**Goal:** Slack-tolerant topology work: learner add/promote/remove (idempotent, H12), single-failure replacement (H11 decision logic), MC-loss repair (remove dead member, add replacement on a healthy MC, update slot DNS).

**Context:** All sequencing runs against the live quorum with the fleet client cert — works from anywhere. Conservative fencing: dead-MC members are removed before replacements serve; unfenced promotion out of scope. Quorum serves on 2/3 throughout — hours of slack, never urgent.

**Tasks:**
- Learner-based member add/promote/remove with peer-URL dedupe
- Single-failure replacement: delete pod+PVC, re-add, refuse during rollouts
- MC-loss repair: remove dead member, mint replacement, update slot DNS target

**Acceptance criteria:**
- [ ] Member ops idempotent under retry (fake etcd)
- [ ] Replacement flow matches the recovery decision logic
- [ ] MC-loss repair lands a replacement on a healthy MC

**Verification:** unit tests with a fake etcd membership; sequencing scenario table.

---

#### E13-S5 — Kill test + degraded mode + backup/restore
**Size:** L · **Depends on:** E13-S2, E13-S3, E13-S4

**Goal:** v1 acceptance made real: the kill test (one MC lost — including the control plane's MC — → quorum holds → control plane re-homes → client keeps writing), healing through a CLM outage for restart-class failures, and `EtcdBackup`/restore semantics.

**Context:** The kill test is the proof this is not NIH — the thing CPO structurally cannot do. Degraded mode: with CLM stopped, a member pod restart still re-joins (controller-free self-heal); a second member loss during the outage breaks the quorum — the documented residual risk. Backup: etcd-consistent snapshots only (`etcdctl snapshot save`); restore only into an empty data dir (`--bump-revision --mark-compacted`).

**Tasks:**
- Kill test: destroy one kind MC; verify quorum serves, client keeps writing, replacement lands, slot DNS updated; measure RTO
- Degraded-mode demonstration: CLM stopped, member restart heals
- Snapshot/restore drill; measure RPO/RTO

**Acceptance criteria:**
- [ ] Kill test passes with measured RTO (minutes, not restore)
- [ ] Restart-class healing works with CLM down
- [ ] Backup per policy; restore drill succeeds

**Verification:** recorded kill-test run; degraded-mode run; restore runbook executed.


### E14 — MC prerequisites + OCI contract co-development

**Phase:** P2 · **Repos:** hcp + core · **Epic size:** M · **Label:** `hf-clm-p2`
**Epic goal:** Our side of the ORO partnership: make OKE-class management clusters runnable for HyperShift, and support the OCI offering team building against the published contract (PROPOSAL.md §12).

#### E14-S1 — MC prerequisite manifests
**Size:** M · **Depends on:** E8-S1

**Goal:** OKE-class MCs runnable for HyperShift via a prerequisite manifest bundle shipped with adapter deployment: port 12250 security list, Prometheus/Route CRD shims + cert-manager on vanilla K8s, PSA annotations, control-plane affinity for stateful components.

**Context:** Carried from `oci-infra/TROUBLESHOOTING.md` learnings; feeds from HYPERFLEET-1540 (HyperShift on non-OpenShift MCs).

**Tasks:**
- Manifest bundle: shims, CRDs, cert-manager, PSA, affinity
- Package with the hcp adapter deployment; versioned
- Bring-up docs + prereq check

**Acceptance criteria:**
- [ ] Fresh vanilla-K8s/OKE MC passes the prereq check and runs HyperShift
- [ ] Bundle ships with the adapter deployment, versioned
- [ ] Bring-up runbook published

**Verification:** fresh OKE MC bring-up test.

---

#### E14-S2 — External registration manifest (P3 adoption path)
**Size:** S · **Depends on:** E14-S1

**Goal:** MCs carry `hypershift install --external-platform-provider=<group>/<kind>=<ns>/<sa>` registration for the OCI integrator — the P3 adoption path, shipped now so adoption is a cutover, not a re-deploy.

**Context:** Decision #32 (External integration re-home per openshift/enhancements#2168). Feature-gated off until External adoption (new-clusters-only, GA-gated).

**Tasks:**
- Registration manifest + application docs
- Feature gate (inactive by default)

**Acceptance criteria:**
- [ ] Manifest applies cleanly on a test MC
- [ ] Gated off by default; documented

**Verification:** apply on test MC; gate check.

---

#### E14-S3 — OCI contract co-development support process
**Size:** M (ongoing) · **Depends on:** E6-S4

**Goal:** Our side of the partnership operational: reviews, contract-test runs in Oracle CI, issue triage for the OCI offering team.

**Context:** The ORO test is the design gate — every SPI/API change must keep "a partner can build out-of-tree from public docs" true. Pre-freeze feedback flows into the contract; post-freeze it goes through the E6-S3 governance process.

**Tasks:**
- Support process doc: channels, SLA, triage rotation
- Assist wiring the contract-test module into Oracle CI
- Feedback log feeding contract issues

**Acceptance criteria:**
- [ ] Process doc + channels live
- [ ] Oracle CI runs the published contract tests (green run recorded)
- [ ] Triage turnaround tracked

**Verification:** Oracle CI green run; support log.

---

#### E14-S4 — In-flight epic disposition + OKE gotcha runbook
**Size:** S · **Depends on:** E14-S1

**Goal:** In-flight HYPERFLEET OCI epics dispositioned (1539, 1540, 1541–1543, 1630 — absorbed, linked, or closed with rationale); the OKE gotcha runbook carried forward.

**Context:** Breakdown §2/§7 — these epics are the P2 track's seed bed. The runbook is the §12 persistent-gotchas list made operational: OVN `internalMasqueradeSubnet` collision with OCI iSCSI (patch to 169.254.64.0/18), Oracle udev device rules, ingress must be LB Service at creation, RWO registry volume, per-resource-kind tagging, `allowedCIDRBlocks` via direct patch, delete sequencing, `Available=True` with zero workers on 4.22.

**Tasks:**
- Per-epic disposition in Jira with links into E8/E14 stories
- Gotcha runbook in repo docs, linked from E8-S5 tests

**Acceptance criteria:**
- [ ] All listed epics dispositioned (none quietly closed)
- [ ] Runbook published covering the full §12 list

**Verification:** Jira audit; runbook review.

---

### E15 — OCI offering adapter (Oracle-owned, external)

**Phase:** P2 · **Owner:** ORO team (Oracle) · **Epic size:** XL · **Labels:** `hf-clm-p2`, external
**Epic goal (visibility):** The offering adapter — ORO = HCP on OCI managed by Oracle — built out-of-tree against the published CLM contract. Its successful construction is the live ORO-test proof and a **P2 acceptance criterion** (decision #11). **Oracle owns delivery; the stories below are our-side support and visibility criteria only.**

#### E15-S1 — Contract onboarding package for Oracle
**Size:** S · **Depends on:** E6-S4

**Goal:** Oracle has everything needed to build out-of-tree: onboarding guide, example skeleton adapter, contract-test CI template.

**Context:** The ORO test — public docs only, no private surfaces. If an engineer outside the squad can build a sample adapter from the package, the seam is real.

**Tasks:**
- Onboarding guide: build an adapter against the published contract, end to end
- Example skeleton adapter repo (or in-guide scaffold)
- Contract-test CI template for offering repos

**Acceptance criteria:**
- [ ] Guide + skeleton + CI template published
- [ ] Skeleton passes the published contract tests
- [ ] Oracle team confirms it is sufficient to start (feedback recorded)

**Verification:** dry-run — an engineer outside the squad builds the sample using only the package.

---

#### E15-S2 — `hosted/oci` chain + digest-pinned E2E profile
**Size:** M · **Depends on:** E15-S1, E8-S6, E11-S2, E11-S4

**Goal:** The `hosted/oci` chain `[pullsecret, dns, placement, hcp, oci]` defined and demonstrated E2E as a digest-pinned profile — the joint demo with Oracle.

**Context:** P2 acceptance criterion. The OCI adapter consumes only the published contract (SPI module, REST status API, overlay packages). Worker lifecycle on `platform: None` is specified by `~/Projects/oro/ORO-Platform-None-Hacks.md` (requirements, not suggestions) and must verify against chain semantics: `Finalized` gating, status conditions.

**Tasks:**
- Chain definition in core config (validated per E5-S2)
- Pinned e2e profile including the oci adapter digest
- Joint E2E demo with Oracle; worker-lifecycle behaviors verified against chain semantics

**Acceptance criteria:**
- [ ] Chain defined + validated; pinned profile runs in core CI
- [ ] Joint E2E demonstrated (demo recorded)
- [ ] None-hacks worker lifecycle verified against `Finalized` gating + status conditions
- [ ] OCI adapter consumes only published surfaces (audit)

**Verification:** pinned-profile run; demo artifacts; surface audit.

---

#### E15-S3 — External end-state scoping + tracking
**Size:** M · **Depends on:** E15-S2, E7-S2

**Goal:** The External end-state scoped and tracked: Oracle ships an MC-local fleet-wide controller + `OCIHostedClusterTemplate`/`OCINodePoolTemplate` CRD pair against the upstream contract (admin registration, CRD contract label N/N-1, conformance via the `externalplatform` module in Oracle CI); the None-hacks doc splits into HyperShift-owned vs integrator-owned requirements.

**Context:** Decision #32; adoption is new-clusters-only (platform type immutable), gated on upstream GA (TechPreviewNoUpgrade at introduction; GA needs two independent integrators — ORO is a candidate, which makes early upstream engagement strategic). Under External, HyperShift reclaims bootstrap, MachineHealthCheck, drain, label/taint propagation, autoscaling — those None-hacks requirements dissolve; Oracle keeps infra provisioning, machine-template rendering (may wrap CAPOCI), the rollout-name decision, providerID wiring. The CLM-side oci adapter shrinks to rendering `spec.platform.external` template refs + the platform blob. Post-External chain re-composition (dns/lb ownership shifts — OQ-9) is a P3 design item.

**Tasks:**
- Scoping doc: deliverables, owners, gates, timelines
- None-hacks split table (HyperShift-owned vs integrator-owned) agreed by both sides — input to the External template spec
- P3 tracking tickets (including the CLM-side oci adapter shrink plan and OQ-9 re-composition)
- Upstream engagement linked (enhancement review participation)

**Acceptance criteria:**
- [ ] Scoping doc reviewed by both sides
- [ ] None-hacks split table agreed; feeds the upstream template spec
- [ ] P3 tracking exists for the shrink plan + chain re-composition
- [ ] Upstream thread participation linked

**Verification:** doc review; tracking audit.

---

### E16 — Standalone deployment: helm + operator repo

**Phase:** P2 · **Repo:** new upstream helm + operator repo · **Epic size:** M · **Label:** `hf-clm-p2`
**Epic goal:** The Oracle delivery vehicle: standalone-mode deployment via a forthcoming upstream helm + operator repo under `openshift-hyperfleet` (decision #27). CLM runs as a **single instance in the MCE hub**, driving many management clusters — hub-side by construction: ManifestWork writes, `ManagedCluster` inventory reads, and MSA-mediated status reads are all hub APIs. This repo never deploys fleet mode — that is `stolostron/hyperfleet` (midstream) only.

#### E16-S1 — Helm chart + operator for the standalone stack
**Size:** L · **Depends on:** E8-S6, E10-S3, E11-S5

**Goal:** Helm chart + operator deploy the standalone CLM stack (api, store postgres, orchestrator, capability adapters) in the MCE hub — one CLM instance driving many MCs.

**Context:** CLM is hub-side: transport (ManifestWork in per-MC hub namespaces), placement inputs (hub `ManagedCluster` CRs), and deep-status reads (MSA tokens) are all hub APIs — in-cluster config reaches all three, no central long-lived credentials (decision #8). The chart deploys the core binary role-split (server, orchestrator, adapter-workers — the etcd adapter among them, decision #35) with all images digest-pinned. Operator-managed component pattern already proven in HYPERFLEET (epics 1530/1665/1666/1667). Needs the runnable hosted chain (E8–E14).

**Tasks:**
- New repo scaffold under `openshift-hyperfleet`
- Helm chart: CLM components + postgres, deployed into the hub, role-split
- Operator managing the stack (config CRD, reconciliation); versioning

**Acceptance criteria:**
- [ ] Chart + operator deploy the full stack on an MCE hub (OKE-class)
- [ ] Deployed CLM drives multiple MCs through the hub transport (multi-MC e2e)
- [ ] Uninstall is clean; upgrades managed by the operator

**Verification:** deployment e2e on a fresh hub; multi-MC fan-out test.

---

#### E16-S2 — Digest pinning + per-component overrides
**Size:** S · **Depends on:** E16-S1

**Goal:** All images digest-pinned by default, with per-component override support (the MCH/MCE pattern).

**Context:** Offerings and teams pin image digests, not repo versions (§8).

**Tasks:**
- Image override values in the chart; default digests pinned per release
- Override docs

**Acceptance criteria:**
- [ ] All images digest-pinned by default
- [ ] Per-component overrides work; documented

**Verification:** deploy with an overridden digest.

---

#### E16-S3 — Install/upgrade/rollback E2E + Day-1 runbook
**Size:** M · **Depends on:** E16-S2

**Goal:** Install, upgrade, and rollback demonstrated against an OKE-class MCE hub; deployment docs + Day-1 runbook published.

**Context:** E16's capstone ACs; this is the delivery gate that unblocks Oracle.

**Tasks:**
- E2E: install → provision a cluster → upgrade the stack → rollback
- Day-1 runbook: bring-up, common operations, troubleshooting (links to the E14-S4 runbook)

**Acceptance criteria:**
- [ ] All three flows demonstrated and recorded
- [ ] Deployment docs + runbook published

**Verification:** e2e run recording; doc review.

---

### E17 — Desire Store → ManifestWork delivery cutover (v2.5 EOL)

**Phase:** parallel line · **Project:** HYPERFLEET · **Epic size:** M/L · **Label:** `hf-clm` (cutover)
**Epic goal:** Retire the Desire Store / Applier delivery line: switch existing consumers' desire-delivery wiring to ManifestWork transport, mirroring the HYPERFLEET-1502 (Maestro decommission) cutover pattern. Serves *existing consumers*, not ORO. Owned by the desire-delivery engineers.

#### E17-S1 — Consumer inventory + cutover plans
**Size:** M · **Depends on:** none (parallel line; direction decided)

**Goal:** Per-consumer cutover plan: desire-delivery path → ManifestWork, with sequencing and rollback.

**Context:** The v2.5 line is live and productized (Desire Store on Postgres — 1518; Adapter Desire Transport — 1419; ~22 sprint-19 tickets). Decommissioning your own system cleanly, with the cutover to a simpler transport, is senior portfolio work. OQ-4 (parentage under HPSTRAT-831 vs standalone workstream) is decided at filing.

**Tasks:**
- Consumer census (who rides desire-delivery today)
- Per-consumer plan: sequencing, rollback, comms; risk review

**Acceptance criteria:**
- [ ] Census complete; no unknown consumers
- [ ] Plans reviewed with each consumer; rollback validated per plan

**Verification:** plan review with consumers.

---

#### E17-S2 — Transport switch + e2e remote-transport topology update
**Size:** L · **Depends on:** E17-S1, E10-S1 (same transport destination, different consumers)

**Goal:** Existing consumers switched to ManifestWork delivery; the e2e remote-transport suite's delivery path switched Desire Store → ManifestWork.

**Context:** E10-S1's ManifestWork writer is the shared destination; HYPERFLEET-1870 (Redis interim retirement) set the e2e-topology-update precedent this follows. Parallel with E10 by design.

**Tasks:**
- Per-consumer migration execution against the plans
- e2e remote-transport suite delivery-path switch
- Verification windows + rollback drills

**Acceptance criteria:**
- [ ] Consumers served via ManifestWork; Desire Store out of the critical path
- [ ] e2e remote-transport profile green on the new path
- [ ] Rollback drill executed at least once

**Verification:** e2e green; consumer smoke tests.

---

#### E17-S3 — Component EOLs + dispositions
**Size:** M · **Depends on:** E17-S2

**Goal:** Desire Store API, Applier, remote-adapter desire clients, and Redis interim wiring retired or reduced to EOL maintenance; broker disposition settled (1655 PostgreSQL backend = terminal state, no CLM successor); gateway disposition settled (OQ-6: carry the authz pattern to CLM's front door or EOL); sprint-19 desire-store tickets (~22) dispositioned.

**Context:** Breakdown §6 corrected dispositions. Ticket triage is redirect cost, not sunk cost: destinations are E17 (cutover) or E10 (transport); no quiet closures; coordinate after the lead-engineer conversation (week of Nov 2), before the all-hands.

**Tasks:**
- EOL process per component: deprecation notices, archive-in-place
- OQ-6 decision ADR (gateway: carry pattern vs EOL)
- Sprint-19 ticket triage: redirect, close with rationale, or fold — every ticket dispositioned openly

**Acceptance criteria:**
- [ ] Components EOL'd/archived; broker terminal state recorded
- [ ] OQ-6 ADR filed
- [ ] All ~22 sprint-19 tickets dispositioned with rationale (audit clean)

**Verification:** Jira audit; repo archive states.

---

#### E17-S4 — Desire-store retrospective ADR
**Size:** S · **Depends on:** E17-S2

**Goal:** What the Desire Store line proved, and why ManifestWork wins.

**Context:** The transition narrative's written anchor: buy-vs-build convergence, not failure — the Desire Store solved remote delivery; MCE already ships that layer battle-tested at fleet scale; org precedent is the Maestro decommission (HYPERFLEET-1502). Cross-linked from the E1-S1 generations retrospective.

**Tasks:**
- ADR: lineage (event push → Redis → Desire Store → ManifestWork), what proved out, why superseded
- Share with the engineering group post-reorg announcement

**Acceptance criteria:**
- [ ] ADR filed; consistent with the E1-S1 retrospective
- [ ] Shared with the full engineering group after the director announcement

**Verification:** review; cross-link check.

---

## 4. Coverage matrix

Every work item from the epic breakdown (§3, "Work items" per epic) maps to at least
one story:

| Epic | Work item | Story(ies) |
|---|---|---|
| E1 | Generations retrospective ADR (first story) | E1-S1 |
| E1 | Fold api-spec + spec-template; fold e2e | E1-S2, E1-S3 |
| E1 | Overlay packaging + provider schema registry + runtime validation | E1-S6 |
| E1 | Status contract types + RFC 9457 error model | E1-S7 |
| E1 | Layout restructure + README/OWNERS identity update | E1-S8 |
| E1 | Module-path decision (runtime, SPI, contract-test) | E1-S8 |
| E2 | Repository-port SPI + dao refactor | E2-S1 |
| E2 | Tenancy de-ROSA (JWT claims scoping) | E2-S4 |
| E2 | Identity model re-shape to ocm-fleet tables | E2-S3 |
| E2 | Drift queries (generation mismatch) | E2-S2 |
| E3 | Facade adapter over extension-resource + inventory APIs | E3-S1 |
| E3 | Conformance test suite shared by postgres + facade | E3-S4 |
| E3 | Etag/generation concurrency handling | E3-S2 (+ re-registration: E3-S3) |
| E4 | Generator implementation | E4-S1 |
| E4 | Round-trip verification harness | E4-S2 |
| E4 | CI wiring | E4-S3 |
| E5 | Workflow state machine + precondition evaluation | E5-S5 |
| E5 | Adapter SPI + registry + chain composition model | E5-S1, E5-S2 |
| E5 | Status upsert endpoint + aggregation | E5-S3 |
| E5 | Drift detector (sentinel-pattern) | E5-S4 |
| E5 | Pre/post Jobs extension hooks | E5-S6 |
| E6 | Contract assembly + publication pipeline | E6-S1 |
| E6 | Contract-test module (own module path) | E6-S2 |
| E6 | Governance/compat policy doc | E6-S3 |
| E6 | Sign-off reviews | E6-S4 |
| E7 | HyperShift team engagement + findings doc | E7-S1 |
| E7 | External-combination questions (etcd × External, endpoint LB ownership) | E7-S2 |
| E7 | Decision record (ADR) | E7-S2 |
| E8 | hcp adapter lands in core (package bootstrap, api-types scoped) | E8-S1 |
| E8 | HostedCluster/NodePool reconciliation adapter | E8-S2, E8-S3 |
| E8 | Publishing + kubeconfig status plumbing | E8-S4 |
| E8 | Deletion sequencing (§12 persistent gotchas) | E8-S5 |
| E9 | Placement chain unit | E9-S2 |
| E9 | Read-through MC view from hub (no MC tracking) | E9-S1 |
| E9 | Selection policies | E9-S3 |
| E10 | ManifestWork writer + generation tracking | E10-S1 |
| E10 | MSA credential mediation + deep-status reader | E10-S2 |
| E10 | Transport conformance against kind/OKE MC | E10-S3 |
| E11 | pullsecret adapter | E11-S1 |
| E11 | dns adapter + provider impls | E11-S2 |
| E11 | lb adapter + OCI-NLB + one dev impl | E11-S4, E11-S5 |
| E11 | Opt-out/verify-only precondition handling | E11-S3 |
| E12 | Coreshift adapter (CLI glue) | E12-S1 |
| E12 | hosted/test chain definition | E12-S2 |
| E12 | CI layering wiring (presubmit / pinned / periodic) | E12-S2, E12-S3 |
| E12 | Bump-PR automation (Renovate digest updates) | E12-S4 |
| E13 | Adapter foundations (membership, mTLS, member minting with self-heal) | E13-S1 |
| E13 | Co-located placement + spread | E13-S2 |
| E13 | Multi-endpoint wiring + the seam (hcp-side rendering, secret handoff, fallback) | E13-S3 |
| E13 | Membership sequencing (learner ops, replacement, MC-loss repair) | E13-S4 |
| E13 | UnmanagedEtcdSpec wiring + HCP integration | E13-S3 |
| E13 | Kill test + degraded mode + backup/restore | E13-S5 |
| E14 | MC prerequisite manifests (shims, security lists, PSA, affinity) | E14-S1 |
| E14 | External registration manifest (P3 adoption path) | E14-S2 |
| E14 | OCI contract support process | E14-S3 |
| E14 | Epic absorption/disposition | E14-S4 |
| E15 | (Oracle-owned) contract consumption + chain E2E + External scoping | E15-S1, E15-S2, E15-S3 |
| E16 | Chart + operator | E16-S1 |
| E16 | Digest pinning/override support | E16-S2 |
| E16 | E2E deployment validation (+ docs/runbook) | E16-S3 |
| E17 | Consumer inventory + cutover plans | E17-S1 |
| E17 | Transport switch + e2e remote-transport topology update | E17-S2 |
| E17 | Component EOLs (desire store, applier, broker terminal, gateway) | E17-S3 |
| E17 | Desire-store retrospective ADR | E17-S4 |
| E17 | Sprint-19 ticket triage | E17-S3 |

Story count: 73 (E1: 8 · E2: 6 · E3: 4 · E4: 3 · E5: 6 · E6: 4 · E7: 2 · E8: 6 ·
E9: 3 · E10: 3 · E11: 5 · E12: 4 · E13: 5 · E14: 4 · E15: 3 · E16: 3 · E17: 4).

---

## 5. Filing notes and open items

- File stories in the HYPERFLEET project, Parent Link → their epic; epics parent to
  HPSTRAT-831 (cross-project, Advanced Roadmaps). Labels per §1.
- E1 and E7 stories can file immediately (no dependencies; E1-S1 is the sprint-19
  redirect destination anchor).
- OQ dependencies carried into stories: OQ-2 (escalation path — E6-S3), OQ-4 (E17
  parentage — E17 filing), OQ-5 (facade timing — E3 filing), OQ-6 (gateway —
  E17-S3), OQ-8 (module paths — resolved by E1-S8), OQ-9 (External chain
  re-composition — tracked via E15-S3).
- Squad mapping (OQ-7) applies at filing; drafts are team-agnostic.
- Housekeeping items 1 and 3 from the breakdown (HPSTRAT-831 description update;
  root CLAUDE.md rewrite) remain open prerequisites for filing.
- E13 carries its conditional banner (E7-S2 go/no-go) into the Jira description;
  E15 carries the external-owner note (Oracle owns delivery; our stories are
  support/visibility criteria).
