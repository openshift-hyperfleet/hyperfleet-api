# HPSTRAT-831 Epic Breakdown — Cluster Lifecycle Manager (CLM)

Status: draft for review — not yet filed in Jira
Revision: 3 (2026-10-30) — External platform model aligned to openshift/enhancements#2168 (re-home, immutability, GA gating); rev 2 (2026-10-28): promote-in-place, fold-in map, CI layering, testing model
Parent: [HPSTRAT-831](https://redhat.atlassian.net/browse/HPSTRAT-831) — ORO cluster lifecycle and provisioning (Feature)

---

## 1. Scope framing

This breakdown covers the **P0 (foundation) + P2 (hosted topology / OCI)** phases of
[PROPOSAL.md](PROPOSAL.md) §14 — the work that unblocks ORO in standalone mode.

**Path: promote in place.** `hyperfleet-api` is promoted to the CLM core repo. It
keeps its git history, test suite (95 files), CI (Prow/Konflux), Renovate, and OWNERS
by construction — nothing is extracted, nothing that already works is rebuilt. New
subsystems (orchestrator, adapter SPI, capability adapters, store SPI) are added in
place; components fold in as needed (api-spec, api-spec-template, e2e first). The
rename to `cluster-lifecycle-manager` is deferred to the productization phase.
Contracts carry verbatim; the orchestration/delivery layer is **new capability, not
re-work**.

**Jira mechanics**: all engineering work lives in the **HYPERFLEET** project. Epics
created from this draft get Parent Link → HPSTRAT-831 (cross-project, Advanced
Roadmaps). Proposed label convention: `hf-clm` plus `hf-clm-p0` / `hf-clm-p2` per
phase (mirrors existing `hf-s19-*` sprint-label style).

**Teams**: squad reorg in flight — drafts are team-agnostic. Team fields read "CLM
squad (post-reorg)" until squads land.

**Transport direction (decided)**: Desire Store line is EOL, superseded by
ManifestWork (MCE) as the delivery transport. See §6.

### Transition narrative (for the engineering conversations)

The "third rewrite before a single customer" reading is wrong, and correctable with
facts. Guardrails for how this work is discussed:

- **Language**: promote, fold, converge, consolidate. Never rewrite, replace, retire.
- **Fact base**: v1/v2 were PoC generations by design — throwaway was the intent.
  v2.5 is the first real system. v3 is its promotion, not its replacement. Zero
  rewrites of the real system: two planned throwaways, one consolidation triggered by
  the first customer.
- **What survived three generations is the team's**: API shape, status contract,
  entity model — carried verbatim. The plumbing iterated (event push → Redis →
  Desire Store → ManifestWork) because remote delivery was the open question; the
  contracts never moved.
- **ManifestWork is buy-vs-build convergence**, not failure: the Desire Store solved
  remote delivery; MCE already ships that layer battle-tested at fleet scale. Org
  precedent: the Maestro decommission (HYPERFLEET-1502) landed the same way.
- **ORO is the first customer — the search is over.** v1 proved contracts, v2 proved
  the pipeline, v2.5 proved operability (Postgres, gateway, operator patterns, CI).
  You consolidate when you finally know what the product is.
- **Comms sequencing**: lead engineers first (week of Nov 2, led by Victor); then the
  full engineering group after the director announces the reorg.
- **Sprint-19 redirect destinations**: desire-store tickets triage openly inside E17
  — redirect to E17 (cutover) or E10 (transport), close with rationale, or fold. No
  quiet closures. Desire-delivery expertise maps directly to both epics.

### In scope
- P0: core promotion + API/TypeSpec, store SPI + both impls, proto generator,
  orchestrator skeleton, adapter SPI, contract v1 freeze, etcd verification spike
- P2: hosted machinery in core (hcp adapter, placement adapter, transport port),
  coreshift CI adapter, capability adapters, the etcd adapter (in core),
  MC prerequisites, OCI contract co-development, standalone deployment, v2.5
  delivery cutover
- OCI offering adapter included as **Oracle-owned external** (visibility only)

### Out of scope (tracked elsewhere)
- **P1 / bm-abi** (bare metal standalone, day-0 laptop) — feeds FM-96
- **Fleet mode** (ocm-fleet facade consumption, fleetlet, Connect registration,
  stolostron/hyperfleet midstream) — FM-95 / FM-97
- **P3 hardening** — `platform: External` adoption for new clusters (upstream
  enhancement provisional Oct 2026, GA-gated; type immutable), cross-MC etcd spread,
  HCP-on-BM (assisted + `platform: Agent`), placement hardening, recovery drill
- kubevirt / vSphere / ROSA / ARO adapters — later

---

## 2. Fold-in map

| Source (Oct 2026 state) | Disposition |
|---|---|
| `hyperfleet-api` — 58.9k LOC, 95 test files, active | **Promoted to core repo (E1).** Keeps history, CI, tests. Hardened in place (E2): store SPI, de-ROSA tenancy, ocm-fleet identity model |
| `hyperfleet-api-spec` + `hyperfleet-api-spec-template` | **Folds into core `api/` (E1).** TypeSpec source of truth moves in; overlays become publishable packages |
| `hyperfleet-e2e` — 17.3k LOC, 18 test files, active | **Folds into core `e2e/` (E1).** Black-box suite carried; coreshift adapter reshapes CI (E12); remote-transport profile switches to ManifestWork with the cutover (E17) |
| `hyperfleet-sentinel` — 11.2k LOC, active | **Harvest-later.** Drift poll → drive adapters pattern + label sharding inform the orchestrator (E5); CEL engine dies; repo EOLs after harvest |
| `hyperfleet-adapter` — 51k LOC, active (executor refactor 1683) | **Harvest-later (reference).** Task configs + executor refactor as porting reference for the adapter SPI; framework retired |
| `hyperfleet-credential-provider` — 12.5k LOC, active | **Stays separate.** Moves as-is per PROPOSAL.md repo map |
| `hyperfleet-broker` / desire-store / applier / gateway | **v2.5 line — EOL by cutover (E17, §6)** |
| OCI/HyperShift epics in HYPERFLEET (1539–1543, 1630) | **P2 track seeds.** Absorbed / referenced by E14 |

---

## 3. Epic drafts

Condensed template (full template at fill time). Sizes: XS–XXL.

---

### E1 — Promote hyperfleet-api to CLM core + TypeSpec API v1
**Phase:** P0 · **Repo:** `hyperfleet-api` (promoted; rename at productization) · **Size:** L · **Label:** `hf-clm-p0`

**Goal:** hyperfleet-api graduates from "the API service" to the CLM core repo:
TypeSpec source of truth and e2e folded in, target layout established for the new
subsystems, and the v1 API contract assembled — with git history continuous by
construction.

**Context:** Promotion, not bootstrap. The repo keeps its history, CI, and test suite
and stays green throughout. First story is the generations retrospective ADR — the
through-line (contracts → pipeline → operability → product) becomes a written
artifact before any restructuring. Most API contract content carries from api-spec;
new work is overlay packaging, runtime validation, and the capability knobs.

**Acceptance criteria:**
- [ ] Generations retrospective ADR filed first (what each generation proved, what
      carries, what EOLs and why)
- [ ] api-spec + api-spec-template folded in; TypeSpec source of truth lives in
      core; provider overlays publishable as versioned packages
- [ ] e2e folded in; suite green in core CI post-fold
- [ ] Resource model carried: `Cluster`, `NodePool` — `id` (UUIDv7), `kind`, `name`
      (DNS-1123), `labels`, `spec: Record<unknown>` blob, `status.conditions[]`,
      `generation`, `owner_references`, server-populated tenancy
- [ ] `topology: standalone | hosted` × `platform` first-class axes; capability
      opt-out knobs (`dns: {managed: false, ...}`, `etcd: {managed: clm | cpo}`)
- [ ] Blobbed-but-validated spec: core `ClusterSpec` intentionally empty; server
      validates blob against registered provider OpenAPI component at runtime
- [ ] Status contract carried verbatim: generation/observed_generation;
      AdapterStatus conditions `Available`, `Applied`, `Health`, `Finalized` +
      free-form `data`; aggregation to `Reconciled`; sticky `LastKnownReconciled`
- [ ] Deletion: soft delete (202) gated on all required adapters `Finalized=True`;
      `force-delete` store-only escape hatch
- [ ] Errors RFC 9457 problem+json; label/field selectors over JSONB
- [ ] Target layout established: `api/ store/ orchestrator/ status/ adapters/
      providers/ e2e/ contract/`; README + OWNERS declare core status, rename
      pending productization
- [ ] ADR numbering continues (0001–0030 preserved)
- [ ] Module-path strategy decided (OQ-8)

**Work items (one-liners):**
- Generations retrospective ADR (first story)
- Fold api-spec + spec-template; fold e2e
- Overlay packaging + provider schema registry + runtime validation
- Status contract types + RFC 9457 error model
- Layout restructure + README/OWNERS identity update
- Module-path decision (runtime, SPI, contract-test)

**Dependencies:** none (entry point). Blocks E2, E4.

---

### E2 — Core runtime hardening: store SPI + de-ROSA tenancy
**Phase:** P0 · **Repo:** core (`hyperfleet-api` promoted) · **Size:** L · **Label:** `hf-clm-p0`

**Goal:** Harden the promoted runtime into the two-mode product substrate in place:
repository-port SPI over the existing dao, tenancy de-ROSA'd to JWT claims, identity
model matched to ocm-fleet (PROPOSAL.md §7, decisions #23/#24).

**Context:** No extraction — the runtime never leaves its repo and CI never goes red
between commits. The store identity model is deliberately shaped to match ocm-fleet's
(`platform_resources`, `extension_resources`, `extension_resource_inventory`, alias
claims/contributions) so the facade impl (E3) maps naturally. Existing tests are the
acceptance bar.

**Acceptance criteria:**
- [ ] Repository-port SPI defined (hexagonal port, not SQL-driver abstraction); dao
      behind it
- [ ] Postgres impl: CLM-owned schema, goose migrations, JSONB specs, conditions
      keyed by type, generation-indexed drift queries, SQL transactions
- [ ] Identity model matches ocm-fleet table shapes
- [ ] Tenancy: JWT claims → scoping; no ROSA-specific assumptions remain
- [ ] Standalone REST surface serves full CRUD + status sub-resources from CLM's
      own server
- [ ] Test coverage not regressed vs pre-promotion baseline

**Work items:**
- Repository-port SPI + dao refactor
- Tenancy de-ROSA (JWT claims scoping)
- Identity model re-shape to ocm-fleet tables
- Drift queries (generation mismatch)

**Dependencies:** E1. Blocks E5.

---

### E3 — Store: ocm-fleet facade implementation
**Phase:** P0 (build) · **Repo:** core · **Size:** M/L · **Label:** `hf-clm-p0`

**Goal:** Second store implementation: persistence through the platform's
extension-resource + inventory APIs — never the platform's database directly
(PROPOSAL.md §7, decision #24).

**Context:** Fleet-mode consumer, but built in P0 per §14 ("store SPI with both
implementations"). Optimistic concurrency via etag/generation instead of SQL
transactions; schema evolution via Connect-time re-registration (content-hashed,
atomic swap) instead of migrations; drift detection via platform
list-with-generation-filter queries. See OQ-5 on timing.

**Acceptance criteria:**
- [ ] Facade implements the repository-port SPI against extension-resource +
      inventory APIs
- [ ] No direct platform-database access anywhere in the impl
- [ ] Optimistic concurrency (etag/generation) verified by tests
- [ ] Re-registration path (content-hash, atomic swap) demonstrated
- [ ] Both impls pass a shared store conformance suite

**Work items:**
- Facade adapter over extension-resource + inventory APIs
- Conformance test suite shared by postgres + facade
- Etag/generation concurrency handling

**Dependencies:** E2 (SPI). Parallel-safe with E5.

---

### E4 — TypeSpec→proto generator
**Phase:** P0 · **Repo:** core · **Size:** S/M · **Label:** `hf-clm-p0`

**Goal:** First-party generator emitting proto schemas from the TypeSpec source for
fleet-mode Connect-time registration (PROPOSAL.md §6, decision #26; risk #16 —
small, well-bounded).

**Context:** Sits alongside the folded-in TypeSpec source (E1).

**Acceptance criteria:**
- [ ] Generator consumes core TypeSpec (+ overlay packages) and emits proto schemas
- [ ] Round-trip check: generated schemas match the OpenAPI contract semantics
- [ ] CI-gated on TypeSpec changes; drift between OpenAPI and proto fails builds

**Work items:**
- Generator implementation
- Round-trip verification harness
- CI wiring

**Dependencies:** E1. Independent of E2/E3/E5.

---

### E5 — Orchestrator skeleton + adapter SPI + status aggregation
**Phase:** P0 · **Repo:** core · **Size:** L · **Label:** `hf-clm-p0`

**Goal:** The new orchestration core: drift detection, per-cluster workflow state
machine, adapter chaining with preconditions, and the status-upsert endpoint with
condition aggregation (PROPOSAL.md §5, §8).

**Context:** New capability, informed by the sentinel pattern (drift poll → drive
adapters; label sharding) — the CEL decision engine dies, no CEL/YAML DSL (decision
#4). Adapters are compiled Go controllers implementing a narrow SPI; ordering
dependencies are preconditions in the workflow state machine, not adapter-internal
magic. In-process packaging default; external adapters run as separate processes
against the REST contract — same code path.

**Acceptance criteria:**
- [ ] Drift detection on generation mismatch drives reconciliation
- [ ] Per-cluster workflow state machine: ordered adapter chains per
      `topology × platform`, preconditions, retry-safe passes
- [ ] Adapter SPI (Go): `Controller` + provider interfaces (`DNSProvider`,
      `LBProvider`; `HostProvider` later)
- [ ] Status upsert endpoint: adapters upsert `AdapterStatus`; aggregation to
      `Reconciled` + sticky `LastKnownReconciled`; deletion gated on `Finalized`
      across the declared chain
- [ ] External adapter surface: status-upsert + list/watch (or efficient polling) —
      any process that consumes events and PUTs statuses is a valid adapter
- [ ] Chain composition is data (ordered list + preconditions), not code

**Work items:**
- Workflow state machine + precondition evaluation
- Adapter SPI + registry + chain composition model
- Status upsert endpoint + aggregation
- Drift detector (sentinel-pattern)
- Pre/post Jobs extension hooks

**Dependencies:** E2 (store). Blocks E6.

---

### E6 — Adapter contract v1 freeze — P0 EXIT CRITERION 1
**Phase:** P0 · **Repo:** core · **Size:** M · **Label:** `hf-clm-p0`

**Goal:** Freeze the published adapter contract: SPI Go module (semver), REST status
API spec, TypeSpec overlay packaging, contract tests — the surface both parallel
tracks and the OCI offering team build against (PROPOSAL.md §8, §14; risks #8, #9).

**Context:** The ORO test is the design gate: could a partner build and operate
their adapter out-of-tree using only the published contract and public docs? Every
SPI/API change must keep the answer yes. Core maintains N-2 compatibility with the
adapter contract. Post-freeze changes require both consuming teams' sign-off. The
contract freeze is the payoff moment for three generations of contract stability —
the team's contract is good enough to freeze for a partner.

**Acceptance criteria:**
- [ ] SPI Go module published with semver + compatibility policy (N-2)
- [ ] REST status API spec published
- [ ] TypeSpec overlay packaging published as versioned artifacts
- [ ] Contract test suite published as a **separately-importable Go module** (own
      module path — offering CI imports conformance without the runtime)
- [ ] Offerings wired to run contract tests in their CI
- [ ] N-2 policy + breaking-change arbitration documented (escalation path may be
      TBD — OQ-2)
- [ ] Both track teams + OCI offering team sign off on v1

**Work items:**
- Contract assembly + publication pipeline
- Contract-test module (own module path)
- Governance/compat policy doc
- Sign-off reviews

**Dependencies:** E5. **Gates E8–E15.**

---

### E7 — Spike: UnmanagedEtcdSpec support verification — P0 EXIT CRITERION 2
**Phase:** P0 · **Size:** S · **Label:** `hf-clm-p0`

**Goal:** Verify `UnmanagedEtcdSpec` support level with the HyperShift team —
supported release? caveats? tech-preview? — and whether it combines with
`platform: External` (PROPOSAL.md §13, §14; risks #2, #19). Decides etcd-manager
sequencing.

**Context:** Mechanism exists upstream
(`hypershift/api/hypershift/v1beta1/hostedcluster_types.go`): single HTTPS client
endpoint + mTLS client secret, immutable after creation. Fallback if insufficient:
CPO-managed etcd + anti-affinity + `EtcdBackup` default-on; etcd manager slips to
the release where support lands (decision #18, conditional). **Research findings
(2026-11-06, see `ETCD-SERVICE-DESIGN.md` §1–2):** `UnmanagedEtcdSpec` is NOT
feature-gated (Default feature set) but has zero upstream e2e coverage, thin docs,
and assumed-True availability — CPO sets `EtcdAvailable=True`, "assumed to be
running in unmanaged state", with no connectivity check ever. CLM is the seam's
first serious consumer; the spike must confirm ungated = supported, raise the
availability-truth gap upstream, and verify the × External combination against
upstream sources — the local hypershift clone predates `platform: External`
entirely (not in the enum).

**Acceptance criteria:**
- [ ] Written findings: support level, target release, caveats, immutability
      constraints
- [ ] `UnmanagedEtcdSpec` × `platform: External` combination verified (the upstream
      enhancement is silent beyond excluding etcd secrets from the integrator Role)
      — raised upstream if needed
- [ ] Upstream ask carried: `UnmanagedEtcdSpec` accepts `endpoints []string`,
      wired to `--etcd-servers` as a list (multi-endpoint clients — the
      load-bearing piece of the in-CLM etcd design, §13; fallback documented)
- [ ] Go/no-go decision recorded for E13 with fallback plan if no-go

**Work items:**
- HyperShift team engagement + findings doc
- External-combination questions (etcd × External, endpoint LB ownership)
- Decision record (ADR)

**Dependencies:** none (start immediately). **Gates E13.**

---

### E8 — hcp adapter in core: HostedCluster orchestration
**Phase:** P2 · **Repo:** core · **Size:** L · **Label:** `hf-clm-p2`

**Goal:** The hcp adapter in the core repo — the translator chain unit: cluster
spec → HostedCluster/NodePool CRs, publishing, kubeconfig handoff (PROPOSAL.md
§12; decision #33).

**Context:** CLM deploys and operates HyperShift artifacts on management clusters;
it does not fork HyperShift — "deploy CPO, don't reimplement it" (§4). The hosted
machinery is core functionality (decision #33): the hcp adapter (this epic), the
placement adapter (E9), and the transport port (E10) are separate concerns landing
in core; HyperShift api-types are scoped to the hcp adapter's package — the
adapter is the abstraction layer, so it versions with the thing it abstracts.
Platform path: start `platform: None`; External adoption in P3, new clusters only
(platform type immutable per openshift/enhancements#2168), gated on upstream GA.

**Acceptance criteria:**
- [ ] hcp adapter in core on a clean module/package boundary (extraction stays
      cheap if the org ever demands it)
- [ ] hcp adapter drives HostedCluster/NodePool lifecycle through the chain:
      create, update, delete with Finalized gating
- [ ] Publishing + kubeconfig handoff surfaced in adapter status
- [ ] Runs against `platform: None` HostedClusters on a management cluster

**Work items:**
- hcp adapter lands in core (package bootstrap, api-types scoped)
- HostedCluster/NodePool reconciliation
- Publishing + kubeconfig status plumbing
- Deletion sequencing (§12 persistent gotchas)

**Dependencies:** E6 (contract). Related: HYPERFLEET 1539–1543 (OCI HyperShift
groundwork).

---

### E9 — Placement
**Phase:** P2 · **Repo:** core · **Size:** M · **Label:** `hf-clm-p2`

**Goal:** Management-cluster selection at HCP create — capacity/geo/affinity aware,
round-robin default; MC list from hub `ManagedCluster` CRs (PROPOSAL.md §12,
decision #9).

**Context:** MCs must be MCE-managed before placement can target them (bootstrap
chicken-egg solved by day-0 flow, §10 — out of scope here). Evolution-workstream
disposition: placement operator subsumed into core (§4, decision #33). The fleet
view has two consumers: control-plane placement (this epic) and etcd member
spread (E13, decision #35). **No MC tracking**:
MCs are never CLM-managed objects — the fleet list is a read-through view of
whoever owns the fleet (MCE-classic hub in standalone mode; ocm-fleet in fleet
mode; no MC records, no per-MC controllers, no lifecycle). The only persisted
artifact is the placement decision on the cluster record. Fleet mode swaps the
source to the platform inventory — FM-97 refinement, same principle; day-0 (FM-96)
runs no placement (no placement slot in its chains).

**Acceptance criteria:**
- [ ] Placement runs as a chain unit for hosted creates
- [ ] Round-robin default + capacity/geo/affinity inputs
- [ ] MC list sourced from hub `ManagedCluster` CRs
- [ ] Placement decision recorded in inventory/status
- [ ] Zero MC records in CLM's store — fleet view is read-through, MCE remains
      system of record

**Work items:**
- Placement chain unit
- Read-through MC view from hub (no MC tracking)
- Selection policies

**Dependencies:** E6, E8.

---

### E10 — Standalone transport: ManifestWork + managedserviceaccount
**Phase:** P2 · **Repo:** core · **Size:** L · **Label:** `hf-clm-p2`

**Goal:** The transport port in core — a mechanism below the chain, not a chain
unit (decision #33). Standalone-mode delivery: HostedCluster/NodePool/InfraEnv CRs
delivered as ManifestWorks in per-MC hub namespaces (klusterlet applies,
generation-tracked); deep status read via `managedserviceaccount`-mediated
short-lived credentials (PROPOSAL.md §12, decision #8).

**Context:** This is the CLM-side transport — also the declared replacement
direction for the v2.5 Desire Store line (see E17, §6). No central component holds
long-lived admin credentials on management clusters. Maestro gone. The
adapter-status push API stays in the contract for a future on-MC status reporter
without API change. maestro-cli decommission context: HYPERFLEET-1502.

**Acceptance criteria:**
- [ ] CR delivery via ManifestWork in per-MC namespaces, generation-tracked
- [ ] Deep status (HostedCluster conditions, NodePool replicas) read via MSA
      short-lived credentials
- [ ] No long-lived admin credentials held centrally
- [ ] Transport works for the hcp-adapter chain E2E against a real MC
- [ ] Status-push API path preserved for future on-MC reporter

**Work items:**
- ManifestWork writer + generation tracking
- MSA credential mediation + deep-status reader
- Transport conformance against kind/OKE MC

**Dependencies:** E6, E8. **Parallel with E17** (same transport, different
consumers).

---

### E11 — Capability adapters: pullsecret, dns, lb
**Phase:** P2 · **Repo:** core · **Size:** M · **Label:** `hf-clm-p2`

**Goal:** Shared capability adapters in the core repo — chain units behind the SPI
with pluggable provider interfaces (PROPOSAL.md §9, decision #12).

**Context:** Provider interfaces are implementation strategies *inside* adapters,
not chain slots: `DNSProvider` (RFC2136 / external-dns / embedded), `LBProvider`
(OCI-NLB / keepalived-VIP / MetalLB). LB is shared because both the hcp api-ingress
and the etcd client endpoint need one. Spec-level opt-outs (`dns: {managed: false,
userRecords: [...]}`) exclude the adapter or run verify-only, decided by
precondition.

**Acceptance criteria:**
- [ ] pullsecret adapter operational in chains
- [ ] dns adapter with ≥1 DNSProvider impl; opt-out path verified
- [ ] lb adapter with OCI-NLB impl (ORO path) + ≥1 dev-friendly impl
- [ ] All three participate in create/update/delete + Finalized gating
- [ ] Precedent check: POC entity registry `required_adapters` concept generalized

**Work items:**
- pullsecret adapter
- dns adapter + provider impls
- lb adapter + OCI-NLB + one dev impl
- Opt-out/verify-only precondition handling

**Dependencies:** E6. Feeds E13 (lb → etcd client endpoint).

---

### E12 — Coreshift test adapter + hosted-chain CI (layered)
**Phase:** P2 · **Repo:** core (`e2e/`) · **Size:** M · **Label:** `hf-clm-p2`

**Goal:** First-party test/dev adapter invoking the coreshift CLI (separate project)
to run a *real* OpenShift control plane — real kube-apiserver, real conditions, real
kubeconfig handoff — with no cloud credentials; plus the layered CI for the hosted
chain (PROPOSAL.md §9, decision #15; decision #31).

**Context:** Thin glue — CLI invocation only (risk #10). Coreshift `container`
target proven against OCP 4.22.8 (HANDOFF.md); project paused Aug 2026 pending this
decision. CI layering per §4: core presubmits test in-tree chains only; external
adapters enter as digest-pinned profiles via bump PRs; digest-tracking jobs are
non-blocking with attribution.

**Acceptance criteria:**
- [ ] `hosted/test` chain `[pullsecret, coreshift]` runs end-to-end as a blocking
      presubmit — real control plane, no cloud credentials
- [ ] **Layer-1 blocking gates contain zero external dependencies** (enforced in CI
      config review)
- [ ] Layer-2 digest-pinned full-stack profiles (blocking or notify) wired
- [ ] Layer-3 periodic digest-tracking jobs wired — non-blocking, failures file
      attributed tickets
- [ ] Full hosted-chain coverage: placement, transport, hcp adapter, etcd adapter,
      status aggregation, deletion gating
- [ ] CI runtime + flake budget documented

**Work items:**
- Coreshift adapter (CLI glue)
- hosted/test chain definition
- CI layering wiring (presubmit / pinned / periodic)
- Bump-PR automation (Renovate digest updates)

**Dependencies:** E6, E8, E11 (pullsecret). Unblocks CI for E13.

---

### E13 — Etcd capability v1 (conditional on E7)
**Phase:** P2 · **Repo:** core · **Size:** XL · **Label:** `hf-clm-p2`

**Goal:** The in-core etcd adapter: fleet-spread etcd for hosted clusters
opting in via `etcd: {managed: clm}` (default `cpo`) — a chain slot for any
hosted platform (decisions #7/#35).

**Context:** Co-location + spread: member-0 on the control plane's MC at mint
(sibling namespace), members 1/2 across two other MCs from the same
read-through fleet view as placement (E9 — its second consumer); co-location
is a mint-time preference, never an invariant. Client path: multi-endpoint —
E7 carries the upstream ask for `UnmanagedEtcdSpec` `endpoints []string`
(wired to `--etcd-servers`); member addresses are per-MC Services behind
slot-stable fabric DNS; no LB; fallback is a single fabric-DNS endpoint
re-pointed by the adapter. Critical-path discipline: CLM never in the data,
client, or restart-healing paths (reset-member init containers re-join the
quorum controller-free); CLM touches only slack-tolerant topology changes —
degraded-but-serving on 2/3 meanwhile. State: level-based, no CRD —
membership truth is etcd's own member list + observed pods; published surface
is adapter status data, which the hcp adapter renders `UnmanagedEtcdSpec`
from (chain precondition). Survives CLM outages for restart-class failures.
Greenfield consumer of an ungated-but-untested seam. Full design:
ETCD-SERVICE-DESIGN.md (rev 2).

**Fallback (if E7 says insufficient support):** hosted clusters ship with CPO-managed
etcd + anti-affinity + `EtcdBackup` default-on; this epic slips to the first release
where `UnmanagedEtcdSpec` support lands (decision #18).

**Acceptance criteria (v1):**
- [ ] Member pods minted with self-heal: STS-per-member, reset-member init
      container, per-MC Service (slot-stable addressing), PVC
- [ ] Co-located + spread placement: member-0 on the placement MC, 1/2 across
      the fleet view; 3 members / 3 MCs
- [ ] Multi-endpoint publication (or the documented single-endpoint fallback);
      `UnmanagedEtcdSpec` rendered by the hcp adapter from etcd status data;
      three-key secret in HC-namespace shape
- [ ] Membership sequencing: learner add/promote/remove, single-failure
      replacement, MC-loss topology repair (slack-tolerant)
- [ ] **Kill test**: MC lost (including the control plane's MC) → quorum
      holds → control plane re-homes → client keeps writing
- [ ] Healing continues through a CLM outage for restart-class failures
      (degraded-mode demonstration)
- [ ] `EtcdBackup`/restore semantics wired

**Work items:**
- Adapter foundations (membership model, mTLS, member minting with self-heal)
- Co-located placement + spread
- Multi-endpoint wiring + seam (hcp-side rendering, secret handoff)
- Membership sequencing
- Kill test + degraded mode + backup/restore

**Dependencies:** E7 (go/no-go + upstream asks: support level, `endpoints
[]string`), E6, E9 (fleet view). No lb dependency — there is no LB.

---

### E14 — MC prerequisites + OCI contract co-development
**Phase:** P2 · **Repo:** core · **Size:** M · **Label:** `hf-clm-p2`

**Goal:** Our side of the ORO partnership: make OKE-class management clusters
runnable for HyperShift, and support the OCI offering team building against the
published contract (PROPOSAL.md §12).

**Context:** MC prerequisite manifests carried into adapter deploy manifests from
`oci-infra/TROUBLESHOOTING.md` learnings: port 12250 security list, Prometheus/Route
CRD shims + cert-manager on vanilla K8s, PSA annotations, control-plane affinity
for stateful components. In-flight HYPERFLEET OCI epics are the seed bed: 1540
(HyperShift on non-OpenShift MCs), 1539 (CPO integration OCI), 1541–1543 (OCI E2E +
CI), 1630 (sweeper image pipeline).

**Acceptance criteria:**
- [ ] MC prerequisite manifests shipped with adapter deployment
- [ ] External registration included for the adoption path (P3): MCs carry
      `hypershift install --external-platform-provider=<group>/<kind>=<ns>/<sa>`
      for the OCI integrator
- [ ] Contract co-development support: reviews, contract-test runs, issue triage
      for the OCI offering team
- [ ] In-flight OCI epics dispositioned: absorbed, linked, or closed with rationale
- [ ] OKE gotcha runbook carried forward (§12 persistent gotchas list)

**Work items:**
- MC prerequisite manifests (shims, security lists, PSA, affinity)
- External registration manifest (P3 adoption path)
- OCI contract support process
- Epic absorption/disposition

**Dependencies:** E6. Related: HYPERFLEET 1539–1543, 1630.

---

### E15 — OCI offering adapter (Oracle-owned, external)
**Phase:** P2 · **Owner:** ORO team (Oracle) · **Size:** XL · **Label:** `hf-clm-p2`, external

**Goal:** The offering adapter — ORO = HCP on OCI managed by Oracle — built
out-of-tree against the published CLM contract. Its successful construction is the
live ORO-test proof and a **P2 acceptance criterion** (PROPOSAL.md §8, §12,
decision #11).

**Context:** Tracked here for visibility; Oracle owns delivery. Blocked-by E6.
Enters core e2e as a digest-pinned profile (§4). Worker lifecycle on
`platform: None` is specified by `~/Projects/oro/ORO-Platform-None-Hacks.md`
(requirements, not suggestions): node-init MachineConfigs (providerID/zone/
instance-type from instance metadata), surge-limited worker replace, PDB-respecting
drain with pause, node health from guest Node objects, auto-repair, narrow CSR
approver (provisioner-launched node names only, newest-only, backlog alerting),
config change = full worker replacement, label/taint add/remove, per-cluster
reconcile workers. Must remain cleanly separable — it is the production path until
`platform: External` GAs and the fallback after (risk #1).

**External end-state** (per
[openshift/enhancements#2168](https://github.com/openshift/enhancements/pull/2168),
provisional Oct 2026): the Oracle deliverable re-homes from a CLM chain unit to a
fleet-wide controller on each MC plus an `OCIHostedClusterTemplate`/
`OCINodePoolTemplate` CRD pair (template/instance contract, instances resolved by
plural naming). The MC admin registers the integrator; contract version rides a CRD
label (N/N-1); validation is the upstream conformance suite (`externalplatform`
module) run in Oracle CI. HyperShift instantiates the templates and reads status
only (`status.infrastructure`, `status.platform`, `status.machineTemplate` — a
machineTemplate name change is the node-roll trigger). HyperShift reclaims bootstrap,
MachineHealthCheck, drain, label/taint propagation, and autoscaling — those
None-hacks requirements dissolve; Oracle keeps infra provisioning, machine-template
rendering (may wrap CAPOCI — the contract accepts genuine CAPI objects on both
halves), the rollout-name decision, and providerID wiring. The CLM-side oci adapter
shrinks to rendering `spec.platform.external` template refs + the platform blob.
Adoption is new-clusters-only (platform type immutable), gated on upstream GA
(TechPreviewNoUpgrade at introduction; GA needs two independent integrators — ORO is
a candidate, which makes early upstream engagement strategic). None-hacks splits
into HyperShift-owned vs integrator-owned requirements to become the External
template spec.

Persistent gotchas (apply under any platform type): OVN `internalMasqueradeSubnet`
collision with OCI iSCSI (patch to 169.254.64.0/18), Oracle udev device rules,
ingress must be LB Service at creation, RWO registry volume, per-resource-kind
tagging, `allowedCIDRBlocks` via direct patch, delete sequencing, `Available=True`
with zero workers on 4.22.

**Acceptance criteria (our visibility criteria, not Oracle's):**
- [ ] OCI adapter consumes only the published contract (SPI module, REST status
      API, overlay packages) — no private surfaces
- [ ] Passes published contract tests in their CI
- [ ] hosted/oci chain `[pullsecret, dns, placement, hcp, oci]` E2E
      demonstrated (digest-pinned profile)
- [ ] Worker lifecycle per None-hacks verified against chain semantics (Finalized
      gating, status conditions)
- [ ] External end-state scoped and tracked: MC-local controller + template CRD
      pair + upstream conformance pass; None-hacks split HyperShift-owned vs
      integrator-owned (template spec input)

**Dependencies:** **Blocked-by E6.** Supported by E14.

---

### E16 — Standalone deployment: helm + operator repo
**Phase:** P2 · **Repo:** new upstream helm + operator repo · **Size:** M · **Label:** `hf-clm-p2`

**Goal:** The Oracle delivery vehicle: standalone-mode deployment via a forthcoming
upstream helm + operator repo under `openshift-hyperfleet` (PROPOSAL.md §5
deployment, decision #27).

**Context:** Standalone ships here for Oracle; fleet mode is deployed by
`stolostron/hyperfleet` (midstream) only — this repo never deploys fleet mode.
Follows the operator-managed component pattern already proven in HYPERFLEET (epics
1530/1665/1666/1667).

**Acceptance criteria:**
- [ ] Helm chart + operator deploy the standalone CLM stack (api, store postgres,
      orchestrator, capability adapters) in the MCE hub — single CLM instance
      driving many MCs (ManifestWork write, `ManagedCluster` reads, MSA status
      reads are all hub-side)
- [ ] Offers digest pinning (MCH/MCE per-component override pattern)
- [ ] Install/upgrade/rollback demonstrated against an OKE-class MCE hub
- [ ] Deployment docs + Day-1 runbook

**Work items:**
- Chart + operator
- Digest pinning/override support
- E2E deployment validation

**Dependencies:** E8–E14 (runnable hosted chain).

---

### E17 — Desire Store → ManifestWork delivery cutover (v2.5 EOL)
**Phase:** parallel line · **Project:** HYPERFLEET · **Size:** M/L · **Label:** `hf-clm` (cutover)

**Goal:** Retire the Desire Store / Applier delivery line: switch existing
consumers' desire-delivery wiring to ManifestWork transport, mirroring the
HYPERFLEET-1502 (Maestro decommission) cutover pattern.

**Context:** The v2.5 line is live and productized — Desire Store on Postgres
(1518), Adapter Desire Transport (1419), Redis interim already retired from
production topology (1870), Applier operator-ization (1667), Envoy/Authorino
gateway (1476/1754). Direction is now supersession: CLM's standalone transport
(E10) is ManifestWork + managedserviceaccount. This epic serves *existing
consumers*, not ORO — see OQ-4 on whether it parents under HPSTRAT-831 or stands
as its own workstream. Owned by the desire-delivery engineers — decommissioning
your own system cleanly, with the cutover to a simpler transport, is senior
portfolio work.

**Acceptance criteria:**
- [ ] Cutover plan per consumer: desire-delivery path → ManifestWork, sequencing,
      rollback
- [ ] Desire Store API, Applier, remote-adapter desire clients, Redis interim
      wiring (1870 pattern) retired or reduced to EOL maintenance
- [ ] e2e remote-transport suite delivery-path switch Desire Store → ManifestWork
      (HYPERFLEET-1870 AC)
- [ ] Broker disposition settled (1655 PostgreSQL backend = terminal state; no CLM
      successor)
- [ ] Gateway (1476/1754) disposition settled: carry pattern to CLM front-door or
      EOL (OQ-6)
- [ ] Desire-store retrospective ADR: what the line proved, why ManifestWork wins
- [ ] Sprint-19 desire-store tickets (~22) dispositioned: redirect, close, or fold

**Work items:**
- Consumer inventory + cutover plans
- Transport switch + e2e remote-transport topology update
- Component EOLs (desire store, applier, broker terminal, gateway)
- Desire-store retrospective ADR
- Sprint-19 ticket triage

**Dependencies:** **Parallel with E10** — same transport destination, different
consumers. Direction decided; timing coordinates with squad capacity post-reorg.

---

## 4. Testing model

Core CI never goes red for someone else's build. Train separation (PROPOSAL.md §8,
decision #31) enforced in the testing domain:

| Layer | Gate | Content | Who can turn it red |
|---|---|---|---|
| 1. Presubmit | **Blocking** | In-tree chains only: coreshift `hosted/test`, standalone CRUD, capability adapters, hcp machinery | Core committers only — zero external dependencies |
| 2. Presubmit, pinned | Blocking (or notify) | Full-stack profiles against **digest-pinned** adapters | Only core changes breaking compat with a known-good consumer — our bug, we want it caught |
| 3. Periodic, digest-tracking | **Non-blocking**, files attributed tickets | Same profiles against latest adapter builds | Nobody blocks — ROSA's breakage is a ticket to ROSA |
| 4. Offering CI | Theirs | Published contract-test module | Their red stays theirs |

Mechanics:
- Digests move via **Renovate-style bump PRs**, never tracking. A broken offering
  build = a red pin-bump PR (close it; pin stays at last-known-good; nobody's merge
  blocked). Integration happens only when deliberately attempted, in an
  attributable, revertible PR.
- The **contract-test module is a separately-importable Go module** (own module
  path, decoupled from the repo name) — offering CI imports conformance without the
  runtime. That path also survives the productization rename (OQ-8).
- Contract tests are the first line — bad digests are caught in offering CI before
  reaching a pin-bump. Layer 3 is the backstop, not the first line.
- **Symmetry rule**: first-party platform adapters (bm-abi) enter core e2e exactly
  like the OCI adapter — pinned digest, bump PR; one mechanism for all platform
  adapters, and live proof of the ORO-test seam (if bm-abi chafes under the digest
  pattern, the contract isn't clean enough for Oracle either). Invariant: at least
  one first-party platform adapter stays digest-pinned (decision #33).
  Per-source bump cadence is squad-tunable.
- Rejected: a separate integration repo — moves the problem without solving
  attribution. Revisit only if e2e signal-noise becomes real.

---

## 5. Dependency graph

```
E1 (promotion) ─→ E2 (hardening) ─→ E5 (orchestrator) ─→ E6 (contract freeze)  [P0 EXIT 1]
│                                        │
│                                        ├──→ E8 (hcp) ──→ E9 (placement)
└──→ E4 (generator)                      │        │        ↘ E10 (transport) ──↔ E17 (cutover)
                                         │        │           E12 (coreshift CI)
E3 (facade, after E2; parallel-safe)     │        ↘
                                         │
E7 (spike) ──────────────────────────────┼──────────→ E13 (etcd mgr, conditional)
                                         ├──→ E11 (capabilities) ──→ E13 (lb endpoint)
                                         ├──→ E14 (MC prereqs + OCI co-dev) ──→ E15 (Oracle, ext)
                                         └──→ E15 (OCI adapter, blocked-by E6)
                                                  E16 (deployment) ⟵ needs E8–E14
```

- **E6 is the gate**: both tracks (P2 machinery) and the Oracle offering adapter
  build against the frozen contract (risk #9).
- **E7 is the second gate** for E13 only; fallback path defined in §13.
- **E17 runs parallel** to E10 — same transport (ManifestWork), different consumers
  (existing v2.5 line vs CLM hosted chain).
- E3 (facade) is P0-built per PROPOSAL.md §14 but fleet-consumed (FM-97) — OQ-5.

---

## 6. v2.5 EOL boundary (corrected dispositions)

PROPOSAL.md's original disposition framing was written against a stale snapshot
("dead in v2"). Reality as of Oct 2026 — the v2.5 line is live, active, and
productized in HYPERFLEET. Corrected table (now mirrored in PROPOSAL.md §4):

| Component | Original framing | Reality (Oct 2026) | Disposition |
|---|---|---|---|
| Desire Store | Dead (deprecated in v2) | Production delivery path; Postgres (1518), transport (1419), ~22 sprint-19 tickets | EOL via E17 cutover → ManifestWork |
| Applier | Dead | Operator-managed epic (1667, New) | EOL with Desire Store line |
| Broker | Dead | Active; PostgreSQL backend in progress (1655) | Terminal: last v2.5 backend; no CLM successor |
| Envoy/Authorino gateway | (not addressed) | Deployed (1476), hardening (1754) | TBD — OQ-6: authz pattern carry vs EOL |
| Sentinel | Pattern survives, CEL dies | Active, operator-managed (1665), CEL + sharding | Pattern → E5 orchestrator; component EOL after harvest |
| Adapter executor | Framework retired | Refactor in progress (1683) | Porting reference only |
| Maestro | Maestro gone | Decommission epic 1502 in flight | Confirmed dead — pattern for E17 |
| hyperfleet-api | Contracts carried, runtime rebuilt | 58.9k LOC, 95 test files, active | **Promoted to core repo** (E1) — history continuous |
| OCI/HyperShift epics | (not addressed) | 1539–1543, 1630 in flight | Seeds for E14 / P2 track |

Sprint-19 note: desire-store tickets were planned the same day this direction
landed. They are redirect cost, not sunk cost — triage in E17, destinations in §1
transition narrative.

---

## 7. Related in-flight work (HYPERFLEET)

| Epic | Summary | Relationship |
|---|---|---|
| 1502 | Maestro Decommission + Delivery Cutover | Pattern for E17; transport lineage |
| 1539 | [HO] Control Plane Operator Integration OCI | Feeds E14/E8 |
| 1540 | [HO] HyperShift on non-OpenShift mgmt cluster | Feeds E14 (MC prerequisites) |
| 1541–1543 | OCI E2E + CI, deployment infra | Absorb into E14 |
| 1630 | OCI CI sweeper image pipeline | Absorb into E14 |
| 1655 | PostgreSQL support for Broker | Terminal state under E17 |
| 1665/1666/1667 | Sentinel/Adapter/Applier operator-managed | Operator pattern → E16; applier EOL |
| 1683 | Reusable Adapter Executor | Porting reference for E5 SPI |
| 1754 | Gateway Hardening | OQ-6 disposition |

---

## 8. Housekeeping

1. **HPSTRAT-831 description** still says workers provisioned via CAPOCI.
   Proposal now reads: HCP `platform: None` (OCI offering adapter owns worker
   lifecycle — the production path) → `platform: External` adoption for new
   clusters when it GAs upstream (openshift/enhancements#2168; Oracle re-homes to
   an MC-local integrator controller + template CRD pair). Update description
   before filing children. *(open — Jira edit)*
2. **PROPOSAL.md** — revised 2026-10-28 (promote-in-place topology, corrected v2.5
   dispositions, CI layering + testing model) and 2026-10-30 (External platform model
   aligned to openshift/enhancements#2168: re-home §12, immutability, GA gating,
   risks #19–#20, decision #32). *(done)*
3. **Root CLAUDE.md** (`~/Projects/openshift-hyperfleet/`) — describes a POC
   meta-repo with v2 event-driven architecture; stale. Rewrite for v3 reality
   (promotion path, ManifestWork transport, repo topology). *(open)*
4. **Upstream engagement** — review openshift/enhancements#2168 from the OCI
   integrator seat while it is provisional: etcd × External question, ingress/LB
   strategy (platform-switch audit flags it unresolved), CAPOCI wrapping,
   release-image staleness on the once-only template copy, zone anti-affinity
   relaxation (enhancement OQ-3 — our answer: yes, per OQ-3 above), ORO gotchas
   (ingress LB at creation, RWO registry volume). Link the enhancement from
   HPSTRAT-831 when its description is updated (housekeeping 1). *(open —
   upstream PR)*

---

## 9. Open questions

| # | Question | Source | Needed by |
|---|---|---|---|
| OQ-1 | `platform: External` GA timing — enhancement provisional Oct 2026, `TechPreviewNoUpgrade` at introduction, GA = two independent integrators pass conformance, realistically 2–3 releases; platform type immutable → new-clusters-only adoption. None-hacks stays the production path until GA; ORO is a candidate GA-enabling integrator | §15 risk 1; openshift/enhancements#2168 | E15 / P3 |
| OQ-2 | Contract governance across offering teams — breaking-change arbitration path | §15 risk 8 | E6 |
| OQ-3 | OKE placement fidelity — zone labels reset ~1s, single-AD regions; upstream ask filed. Enhancement OQ-3 (relax zone anti-affinity for External): **our answer is yes, relax** — OKE-class managed-K8s MCs break hard zone anti-affinity anyway (label reset), OpenStack/KubeVirt precedent fits, fidelity stays ours via per-subnet pools + custom labels | §15 risk 6; openshift/enhancements#2168 OQ-3 | E9/E13 |
| OQ-4 | E17 parentage: under HPSTRAT-831 (supersession work) vs standalone HYPERFLEET workstream (serves existing consumers) | this doc | E17 filing |
| OQ-5 | E3 facade timing: built P0 per §14 but consumed only in fleet mode (FM-97) — confirm P0 build vs defer to fleet-mode phase | §14 vs §7 | E3 filing |
| OQ-6 | Gateway disposition: Authorino/Envoy authz pattern carries to CLM standalone front-door, or EOLs with v2.5 | §6 above | E17 |
| OQ-7 | Squad mapping post-reorg — which squad owns core vs hcp vs cutover | reorg | all filings |
| OQ-8 | Module path strategy: decouple SPI + contract-test module paths from the repo name (recommended — survives productization rename, risk #18) vs one accepted rename event | this doc + §15 risk 18 | E1/E6 filing |
| OQ-9 | External chain re-composition: dns/lb capability-adapter ownership vs integrator-provisioned network/LB/DNS under External; etcd client-endpoint LB ownership (feeds E7 scope) | openshift/enhancements#2168 (platform-switch audit lists ingress strategy as unresolved) | P3 / E7 |

---

## Filing checklist (when ready)

- [ ] Epics E1–E17 → HYPERFLEET project, Parent Link → HPSTRAT-831
- [ ] Labels: `hf-clm` + `hf-clm-p0` / `hf-clm-p2`; E15 additionally external-owner
      flag; E17 `hf-clm` cutover
- [ ] E15 assignee = Oracle counterpart (name TBD); our side = watcher
- [ ] E1 and E7 can file immediately (no dependencies; E1 is the sprint-19
      redirect destination)
- [ ] OQ-8 resolved before E1 module moves / E6 filing
- [ ] Housekeeping items 1 and 3 resolved before filing
- [ ] Sprint-19 desire-store ticket triage coordinated with E17 — after the
      lead-engineer conversation (week of Nov 2), before the all-hands
