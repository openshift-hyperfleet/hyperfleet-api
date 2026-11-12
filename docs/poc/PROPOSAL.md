# Cluster Lifecycle Manager (CLM) — Design Proposal

Status: draft for discussion
Date: 2026-10-30

---

## 1. Summary

The Cluster Lifecycle Manager (CLM) is the core of **HyperFleet v3** — the
productization of HyperFleet into the ACM fold ("enfleetification"). HyperFleet v1/v2
proved its API shape, status contract, and adapter model across three PoC generations;
v3 promotes the proven runtime to product standard — `hyperfleet-api` continues as the
core repo, renamed at productization — keeps the project in the `openshift-hyperfleet`
upstream org where it lives today, and integrates via stolostron.

CLM exposes a REST API that accepts a high-level cluster spec — with platform
specifics "blobbed" so the API stays consistent across platforms and topologies —
stores requests in an inventory, delegates provisioning and lifecycle to adapters, and
receives status back from those adapters to keep the inventory current.

It ships in **two deployment modes**, one codebase:

- **`standalone`** — psql storage, JWT-claims tenancy, ManifestWork transport. Zero
  platform dependency; ships first (Oracle/ORO, HPSTRAT-831).
- **`fleet`** — ocm-fleet (the MCE-NEXT core) provides storage, authz, and transport;
  CLM runs as an addon with its CRUD hosted by the platform. The ACM product path
  (FM-97).

Initial modes of cluster provisioning:

- **Bare metal standalone** — agent-based installer (ABI) run in a pod; config and
  call-home manifests baked into the generated ISO.
- **Hosted control planes on OCI** — HostedCluster/NodePool on management clusters
  running HyperShift, starting at `platform: None` and adopting `platform: External`
  for new clusters when it GAs upstream ([enhancement
  proposal](https://github.com/openshift/enhancements/pull/2168), provisional as of
  Oct 2026). Platform type is immutable, so External is a cutover for new clusters,
  not an in-place migration. The OCI adapter is the first **offering adapter** (ORO:
  HCP on OCI managed by Oracle), built out-of-tree against the published CLM contract.

Later adapters: kubevirt, vSphere, ROSA, ARO; hosted control planes on bare metal
(assisted-service + `platform: Agent`).

A primary design goal beyond the two provisioning modes: **day-0 bootstrap**. On a
laptop, CLM runs as the cluster-lifecycle addon inside the ocm-fleet AIO (embedded
SQLite, bundled eval OIDC) — no hub, no management clusters, no klusterlet — and the
standalone bare metal path is the mechanism by which a customer provisions their first
cluster, which becomes their first MCE hub.

## 2. Motivation

Today, cluster lifecycle in MCE is fragmented across separate repos, release trains, and
API surfaces: `assisted-service` (bare metal), `hive` (cluster claims/standalone
provisioning), `hypershift` (hosted control planes), the `cluster-api` provider family.
Consequences:

- No single consistent cross-platform cluster API. Every provider has its own CR shapes,
  status models, and deployment story.
- Integration burden lands on MCE assembly and on every offering built on top —
  each pins and composes components independently.
- The day-0 problem is unsolved: every existing flow assumes a hub already exists.
  Policy/ZTP-based day-0 requires a hub; agent-based install exists but nothing operates
  it as a service with inventory and status.
- HyperFleet itself proved the contracts — API shape, status model, JSONB blobbed
  specs, adapter pipeline — across three PoC generations, but lived as a self-declared
  POC meta-repo with deprecated-by-design components. Without productization that
  investment orphans; with it, MCE gets the consolidated lifecycle service the first
  three bullets demand.

CLM replaces that fragmentation with one service: one API, one inventory, one status
contract, one adapter framework — with adapters as the only per-platform code.

## 3. Goals and non-goals

### Goals

- One REST API for cluster lifecycle across platforms and topologies
  (`topology: standalone | hosted` × `platform: agent-bm | oci | kubevirt | ...`).
- **Two deployment modes from one codebase**: standalone (psql, JWT tenancy,
  ManifestWork) and fleet (ocm-fleet storage/authz/transport).
- Inventory of desired state (cluster requests) and observed state (conditions), in a
  store whose identity model converges with ocm-fleet's.
- Adapter framework: capability adapters and the core in one repo; per-team and
  offering adapters in their own repos, all against a published contract.
- Day-0 bootstrap: CLM as addon in the ocm-fleet AIO on a laptop provisions the
  first standalone cluster (which becomes the first MCE hub) with zero supporting
  infrastructure.
- Hosted-topology machinery (placement, transport, control-plane orchestration) with the
  OCI offering adapter as the first consumer — and an in-core etcd capability giving
  HCPs resilient, fleet-spread etcd behind the `UnmanagedEtcdSpec` seam (see §13).
- **Consolidation of HyperFleet (enfleetification)**: promote the proven runtime to
  product standard in the `openshift-hyperfleet` org — `hyperfleet-api` promoted in
  place as the core repo (renamed at productization), components folded in as
  needed — archive the POC generation, integrate via `stolostron/hyperfleet` as the
  MCE component.
- Eventual home as an MCE component: successor slot to `cluster-lifecycle-mce`,
  `hypershift`, `assisted-service`, and `hive` in `MCEComponents`.

### Non-goals (v1)

- Management-cluster fleet management — MCE + klusterlet already do this.
- Fleet-scale observed-state indexing and search — ocm-fleet's inventory.
- Host-level management (inventory, approval, progress telemetry) for standalone bare
  metal clusters — ABI path is fire-and-forget until call-home.
- Day-2 (upgrades, scale) for standalone clusters — post-import via ACM governance.
- Hosted control planes on bare metal (needs assisted-service; deferred).
- Hive — gone. Standalone BM replaces it via the ABI path.
- Migration of records between instances — never (see §10).
- Shared etcd across HCPs — never (see §13).
- Drop-in compatibility with the HyperFleet POC runtime — contracts carry, runtime does
  not (see §4).
- Standalone-mode full zero-trust (signed/verified intent) — fleet mode inherits it
  from the platform; standalone ships JWT-claims tenancy plus pluggable audit
  (§5).

## 4. Positioning

- **Upstream/midstream split**: engineering repos live in the `openshift-hyperfleet`
  org; `stolostron/hyperfleet` is the thin midstream assembly (fleet-mode deployment
  manifests, image pinning/overrides, the `hyperfleet` MCE component entry). This
  mirrors the OCM → stolostron upstream/midstream split, and the thin-assembly precedent
  already set by `stolostron/multiclusterhub-operator`'s per-component override model.
- **Relationship to ocm-fleet (MCE-NEXT core)**: two modes, one convergence point.
  Standalone mode has **zero ocm-fleet dependency** and ships first (HPSTRAT-831,
  Oracle). Fleet mode runs CLM as an ocm-fleet addon — storage via the platform
  (extension resources + inventory: the identity model CLM's psql schema was
  deliberately shaped to match), authz via the platform, delivery via fleetlet. The
  ocm-fleet facade store implementation (§7) *is* the convergence; FM-97 tracks the
  fleetification.
- **Relationship to MCE + klusterlet**: in standalone mode, CLM uses MCE-classic as its
  transport and fleet substrate: writes to management clusters go through ManifestWork
  on the hub; reads through `managedserviceaccount`-mediated short-lived credentials. In
  fleet mode, the platform's delivery agents (fleetlet) replace both. No central
  component holds long-lived admin credentials on management clusters.
- **Relationship to HyperShift**: CLM deploys and operates HyperShift artifacts
  (HostedCluster, NodePool, and eventually External platform templates) on management
  clusters; CLM does not fork HyperShift. The "deploy CPO, don't reimplement it" lesson
  from the HyperFleet provision-engine PoC applies.

### Lineage and succession — HyperFleet v3

CLM is HyperFleet's successor, not a new service inspired by it:

- **v1/v2** (POC generations, `openshift-hyperfleet` org): proved the REST/TypeSpec API
  shape, the status contract, JSONB blobbed specs, and the adapter pipeline; ended with
  the adapter DSL, broker, and desire-store deprecated by the project's own evolution
docs. Those repos are **archived in place** — except `hyperfleet-api`, promoted to
the core repo per decision #29 — history, ADRs, and reference configs stay visible
where they were written. ADRs 0001-0030 remain authoritative for the
  decisions they record; **new ADRs continue the numbering in the core repo**.
- **v3** (this proposal): the same contracts carried forward verbatim, with the proven
  runtime promoted in place — `hyperfleet-api` continues as the core repo (renamed at
  productization; git history continuous), gaining the two-mode substrate (§5),
  MCE-native transports, and compiled Go adapters instead of the CEL/YAML DSL.
  Successor via promotion, not drop-in.
- **Ownership**: the Cluster Lifecycle / HyperFleet team inside the Fleet Engineering
  org owns the core repo — including the hosted machinery (hcp adapter, placement,
  transport port; decision #33) and the etcd capability (decision #35) — and the
  overall system; Track A owns `bm-abi`; offering teams own offering adapters. The OME
  team is building the new
  MCE core (ocm-fleet / MCE-NEXT) — HyperFleet integrates with it in fleet mode and
  ships as an MCE component; it is not owned by the OME team.
- **Evolution-workstream dispositions** (from the HyperFleet evolution plan):
  placement operator → subsumed into core (decision #33); FleetShift delivery agents →
  replaced by ManifestWork/klusterlet (standalone) and fleetlet (fleet); MC reconciler
  (never started) → subsumed; fleet observer → ocm-fleet's inventory.
- **Strategy-pillar alignment** (per the Fleet Engineering architecture strategy):
  HyperFleet v3 absorbs the **HyperFleet pillar** (placement over an arbitrary target
  pool, ancillary-resource orchestration, nodepool lifecycle) and most of the **Fleet
  Bootstrapper pillar** (day-0 AIO). **Universal Installer** and the **standalone
  installer CLI** remain sibling workstreams; the adapter SPI is the seam where
  Universal Installer drivers plug in later (bm-abi and the hcp adapter become UI-driver
  wrappers when UI exists).
- **Jira ladder**: HPSTRAT-831 (standalone CLM, ORO first consumer) → FM-95 + FM-97
  (fleetification on the MCE-NEXT core) → FM-96 (first-touch laptop/pivot). FM-97 keeps
  its FM-95 dependency — reworded from co-development blocker to staged consumption:
  the capabilities already ship standalone; FM-97 owns the platform integration.
- **Repo consolidation map**:

| HyperFleet repo | v3 destination | Disposition |
|---|---|---|
| hyperfleet-api | **core repo — promoted in place** (renamed at productization) | Runtime promoted with continuous git history; two-mode substrate + de-ROSA'd tenancy added in place (hardening) |
| hyperfleet-api-spec + api-spec-template | folds into core `api/` | TypeSpec source of truth moves into the core repo; overlays become publishable packages |
| hyperfleet-e2e | folds into core `e2e/` | Black-box suite carried; coreshift adapter reshapes CI; remote-transport profile switches to ManifestWork with the v2.5 cutover |
| hyperfleet-adapter | — | Framework retired per its own evolution conclusion; task configs + executor refactor (HYPERFLEET-1683) archived as porting reference |
| hyperfleet-sentinel | core `orchestrator/` | Pattern survives (drift poll → drive adapters, label sharding); CEL decision engine dies; repo EOLs after pattern harvest |
| hyperfleet-broker, desire-store, applier | — | **v2.5 line — live as of Oct 2026** (Desire Store on Postgres, Applier, gateway in active delivery); EOL by ManifestWork delivery cutover, extending the Maestro-decommission pattern (HYPERFLEET-1502) |
| hyperfleet-infra | modes | Collapses into the standalone/fleet deployment stories |
| hyperfleet-credential-provider | standalone | Moves as-is (candidate for the `openshift-hyperfleet` org) |
| kubevirt-poc, provision-engine-poc, oci-infra, hypershift clone | — | Archived as reference; learnings carried in this proposal |
| coreshift | separate project | Unchanged (thin CLM test adapter) |

## 5. Architecture

Black-box flow:

```
1. user calls the API to create a cluster (high-level spec, platform details blobbed)
2. CLM stores the request in the inventory (generation 1, conditions empty)
3. orchestrator detects the new/drifted generation, runs placement (hosted topology)
   and drives the adapter chain for the cluster's topology × platform
4. adapters report status back (observed_generation, conditions, data blob);
   CLM aggregates and updates the inventory
```

Repo and component layout:

```
openshift-hyperfleet org — upstream engineering home (old hyperfleet-* repos archived)

hyperfleet-api (core repo — promoted in place; renamed at productization)
├── api/          TypeSpec source of truth → OpenAPI (standalone REST) + proto
│                 generation (fleet-mode Connect schemas); provider schema registry
├── store/        repository-port SPI, two implementations: postgres (CLM schema,
│                 goose migrations, JSONB) and ocm-fleet facade
│                 (extension-resource + inventory APIs)
├── orchestrator/ drift detection (generation mismatch), per-cluster workflow
│                 state machine, adapter chaining
├── status/       adapter status upsert endpoint + condition aggregation
├── adapters/     Go SPI + registry + chain units — capabilities, translators,
│   │             mode machinery (decision #33)
│   ├── pullsecret/   pull secret / credentials handling (capability)
│   ├── dns/          DNS records for cluster endpoints (capability; DNSProvider
│   │                 impls: RFC2136 / external-dns / embedded)
│   ├── lb/           endpoint/LB provisioning (capability; LBProvider impls:
│   │                 OCI-NLB / keepalived-VIP / MetalLB)
│   ├── coreshift/    test/dev adapter: real OpenShift control plane via the
│   │                 coreshift CLI (external project) — CI + solo-dev fast-mint
│   ├── placement/    MC selection at hosted create (capability; read-through
│   │                 fleet view, no MC tracking)
│   ├── hcp/          translator: cluster spec → HostedCluster/NodePool CRs,
│   │                 publishing, kubeconfig handoff — HyperShift api-types
│   │                 scoped to this package (decision #33)
│   └── etcd/         capability: fleet-spread etcd — member minting,
│                     membership sequencing, multi-endpoint publication
│                     (decision #35)
├── providers/    provider interfaces (DNSProvider, LBProvider; HostProvider later:
│                 Metal3/BMC, kubevirt, vSphere) — implementation strategies that
│                 adapters load, not chain units
├── transport/    transport port (local | manifestwork | fleetlet) — a mechanism
│                 below the chain, not a chain unit: CR delivery to MCs,
│                 generation-tracked; MSA-mediated deep-status reads
├── e2e/          black-box suite, folded in from hyperfleet-e2e: in-tree chains gate
│                 presubmit; full-stack profiles run digest-pinned adapters (§8)
├── contract/     published contract-test module — own module path, decoupled from
│                 the repo name; offering CI imports it without the runtime
└── (later)       kubevirt/ vSphere/ ROSA/ ARO/ hcp-agent chains as repos warrant

platform adapter repos (per team/offering — the platform axis; decision #33)
├── bm-abi        Track A (BM team): standalone BM — openshift-install agent in a
│                 pod, baked-ISO generation, call-home manifests, ISO hosting
└── (offerings)   oci (ORO), rosa, aro, ... — out-of-tree, offering teams

stolostron/hyperfleet (midstream): deploys CLM in fleet mode only — see its
companion proposal
```

### Deployment modes

Two modes, one binary; each mode bundles three planes (storage, authz, transport):

| Plane | `standalone` | `fleet` |
|---|---|---|
| Storage | psql (CLM-owned schema) | ocm-fleet platform (its own psql or sqlite — platform's choice, not CLM's concern) |
| Authz | JWT claims → tenancy scoping | ocm-fleet platform (RBAC, zero-trust) |
| Audit | OTel/stdout events, customer plugs in their own journal | platform |
| Transport | ManifestWork + managedserviceaccount (MCE-classic hub) | fleetlet / platform delivery agents |
| CRUD hosting | CLM's own REST | platform dynamic mux; CLM keeps the operations surface (status upsert, chains, placement) |

- **Sequencing**: standalone ships first — it is the mode with zero platform
  dependency, unblocking Oracle (HPSTRAT-831) now. Fleet mode is the ACM product
  path, gated on ocm-fleet (MCE-NEXT core) maturity — not on FM-95's FR-14 alone.
- **Deployment**: standalone ships via a forthcoming upstream helm + operator repo
  under `openshift-hyperfleet` (deferred for now; first consumer Oracle/ORO). Fleet
  mode is deployed by `stolostron/hyperfleet` (midstream) only.
- In fleet mode, the platform's `Resume` on extension resources maps to CLM's
  soft-delete reversal; platform Disconnect semantics (CRUD stays live, delivery
  pauses) match CLM's status-first design.

## 6. API design

REST, TypeSpec-first (TypeSpec → OpenAPI → server + clients), carrying forward the
contract proven across three HyperFleet PoC generations. **TypeSpec is the source of
truth; a generator emits proto schemas for fleet-mode Connect-time registration** (the
generator is a first-party P0 deliverable).

- **Resource model**: `Cluster`, `NodePool` with `id` (UUIDv7), `kind`, `name`
  (DNS-1123), `labels`, `spec: Record<unknown>` (blob), `status.conditions[]`,
  `generation`, `owner_references`, server-populated tenancy.
- **Topology and platform are first-class axes** in the high-level spec:
  `topology: standalone | hosted`, `platform: agent-bm | oci | ...`. The combination
  selects the adapter chain (§9).
- **Blobbed-but-validated spec**: the core `ClusterSpec` is intentionally empty
  (`{}`); provider overlays (separate TypeSpec packages, publishable artifacts) define
  concrete fields per platform. The server validates the spec blob at runtime against
  the registered provider OpenAPI component. This keeps the core API stable while
  platform schemas evolve independently.
- **Capability opt-outs and options** in the high-level spec, e.g.
  `dns: {managed: false, userRecords: [...]}`, `etcd: {managed: clm | cpo}` — these
  shape the adapter chain without changing the API surface.
- **Status contract**:
  - `generation` (desired) vs `observed_generation` (per adapter).
  - Adapters upsert `AdapterStatus`: mandatory conditions `Available`, `Applied`,
    `Health`, `Finalized` (deletion), plus free-form `data` blob.
  - Aggregation to `Reconciled` and sticky `LastKnownReconciled` (cross-generation).
- **Deletion**: soft delete (202), gated on all required adapters reporting
  `Finalized=True`; `force-delete` as a store-only escape hatch. In fleet mode, the
  platform's `Resume` reverses a soft delete.
- **Errors**: RFC 9457 problem+json; **IDs**: UUIDv7.
- **Search**: label/field selectors over JSONB (TSL-style tree search optional later).
- **Mode split**: standalone mode serves the full REST surface from CLM's own server.
  Fleet mode hosts CRUD on the platform's dynamic mux (Create/Delete/Resume + labels,
  via Connect-registered schemas); CLM keeps the operations surface — status upsert,
  chain invocation, placement-informed creates.
- **External adapter surface**: the same status-upsert API plus list/watch (or efficient
  polling) is the entire contract an out-of-tree adapter needs — any process that
  consumes events and PUTs statuses is a valid adapter.

## 7. Store

The store is a **repository-port SPI** (hexagonal ports, not a SQL-driver abstraction),
with two implementations:

- **`postgres`** — CLM-owned schema, goose migrations, JSONB specs, conditions keyed by
  type, generation-indexed drift queries, SQL transactions. Used by standalone mode.
- **`ocm-fleet facade`** — persistence through the platform's extension-resource +
  inventory APIs (never the platform's database directly — bypassing would break
  schema ownership, migration ownership, and the authz model). Optimistic concurrency
  via etag/generation instead of SQL transactions; schema evolution via Connect-time
  re-registration (content-hashed, atomic swap) instead of SQL migrations; drift
  detection via platform list-with-generation-filter queries. Used by fleet mode.

Rules:

- **Embedded sqlite is not a CLM store impl** — it is a property of ocm-fleet's own
  deployment (its AIO); when the platform runs sqlite, CLM rides it through the facade.
- The identity model is shared across both impls so the mapping is natural:
  - `platform_resources` — canonical identities (virtual rows, exist while referenced)
  - `extension_resources` — per-adapter representations (uid, aliases)
  - `extension_resource_inventory` — latest observation, labels, conditions
  - alias claims/contributions — correlate representations of the same cluster
    (kube-system ns uid, cloud provider id, cluster-id)
- In the postgres impl this is CLM's schema (shaped to match ocm-fleet's tables); in
  the facade impl it *is* the platform's model.
- **No record migration between instances, ever.** Records live where they were
  created (see §10 for the local/hub relationship).

## 8. Adapter framework

- Adapters are **compiled Go controllers** implementing a narrow SPI
  (`Controller`, and provider interfaces as needed: `DNSProvider`, `LBProvider`; later
  `HostProvider` with `CreateHost/DeleteHost/GetHostStatus`). No CEL/YAML DSL —
  HyperFleet's own evolution rejected it; hardcoded lifecycle logic, YAML-configurable
  parameters, and pre/post Jobs for partner extension.
- Idempotent, status-first: adapters upsert status on every pass; every pass is safe to
  repeat; ordering dependencies between adapters are expressed as preconditions in the
  workflow state machine (not adapter-internal magic).
- **Repo strategy — four classes** (amended by decisions #33/#35):
  - **Core repo** (`openshift-hyperfleet/hyperfleet-api`, promoted in place —
     renamed at productization): runtime + SPI/contract + published contract-test
     module + all platform-agnostic chain units — capability adapters (dns, lb,
     pullsecret, coreshift-test, placement, etcd) and the hcp translator — plus the
     transport port, the TypeSpec→proto generator, and the folded-in e2e suite.
     One team, one train. Chain units are of three kinds (decision #33):
     capabilities and translators (core), platform adapters (per-substrate, own
     repos), mode machinery (core). HyperShift api-types are scoped to the hcp
     adapter's package — the adapter is the abstraction layer, so it versions with
     the thing it abstracts.
  - **Platform adapter repos** — the platform axis, the only per-substrate chain
    units: `openshift-hyperfleet/bm-abi` (Track A) plus offering adapters
    (OCI/ORO, ROSA, ARO, ...) in offering teams' repos against the published
    contract: adapter SPI Go module (semver), REST status API spec, TypeSpec
    overlay packages, and contract tests. Core maintains N-2 compatibility with
    the adapter contract. **Invariant: at least one first-party platform adapter
    stays digest-pinned in core e2e** (bm-abi) as the live self-test of the
    contract — if it chafes, the contract isn't clean enough for Oracle.
  - **Fleet infra services** *(latent — #34, superseded by #35 for etcd)*:
    reserved for future services whose availability bar genuinely exceeds
    CLM's own; none today — etcd folded back into core once multi-endpoint
    clients deleted its endpoint layer (see #35).
  - **`stolostron/hyperfleet`**: thin midstream assembly — fleet-mode deployment
    manifests + image pinning; the `hyperfleet` MCE component entry. No product logic.
  - Offerings and teams pin **image digests** (the existing MCH/MCE per-component
     override pattern), not repo versions.
  - **CI layering** (train separation, testing domain): core presubmits test
     in-tree chains only; all platform adapters — first-party per-team and
     offering alike — enter core e2e as digest-pinned profiles moved by
     Renovate-style bump PRs; digest-tracking integration jobs are periodic,
     non-blocking, and attribute failures to the owning team; conformance runs in
     offering CI against the published contract-test module. A broken offering
     build surfaces as a red pin-bump PR (close it, pin stays) or an attributed
     periodic ticket — never as a blocked core merge.
  - **The ORO test** (design gate): could a partner build and operate their adapter
     out-of-tree using only the published contract and public docs? Every SPI/API change
     must keep the answer yes. The OCI adapter is the live proof.
- Packaging: in-process by default (compiles core + adapter Go modules — this is how
  the laptop AIO and both modes run); external adapters run as separate processes
  against the REST contract. Same code path, different packaging.

## 9. Adapter chains and shared capabilities

Common lifecycle concerns (DNS, load balancers/endpoints, pull secrets, credentials,
etcd) are **not core code** — the core runtime is capability-free (orchestration,
contract, aggregation only). They are **capability adapters**: chain units with
lifecycle responsibility, living in the core repo because every chain consumes them,
but still adapters behind the SPI.

Two-layer distinction:

- **Adapter** — a chain unit: participates in create/update/delete, reports
  conditions, gates Finalized/deletion. Composed into per-`topology×platform` chains.
  Adapters may live in the core repo (capabilities), per-team repos (bm-abi, hcp), or
  offering repos (oci) — repo placement is ownership, chain placement is the
  topology×platform combo.
- **Provider interface** — a pluggable implementation strategy *inside* an adapter:
  `DNSProvider` (RFC2136 / external-dns / embedded), `LBProvider` (OCI-NLB /
  keepalived-VIP / MetalLB). Environment pluggability without multiplying chain slots
  or repos.

Chain composition — each combo declares an ordered adapter list with preconditions;
skipping or adding a capability is chain composition plus spec knobs, never code:

```
standalone/agent-bm:    [ pullsecret, dns?, bm-abi ]              # dns optional:
                                                         # user-managed records
hosted/oci:             [ pullsecret, dns, placement, hcp, oci ]   # oci =
                                                          # offering adapter
hosted/oci + etcd:      [ pullsecret, dns, placement, etcd, hcp, oci ]
                                                          # etcd = core capability
                                                          # adapter over the
                                                          # standalone etcd-manager
                                                          # service (§13)
hosted/test (CI):       [ pullsecret, coreshift ]          # real control plane,
                                                         # no cloud creds
hosted/agent-bm (P3):   [ pullsecret, dns, placement, assisted, hcp,
                          agent ]
```

- Opt-outs at spec level (`dns: {managed: false, userRecords: [...]}`) — the adapter is
  excluded or runs verify-only, decided by precondition.
- `etcd: {managed: clm | cpo}` opts a hosted cluster into the etcd manager (§13).
- Deletion/Finalized gating aggregates over whatever chain the combo declared.
- Precedent: the POC's entity registry `required_adapters` (Cluster:
  `validation, dns, pullsecret, hypershift`) — same concept, generalized.
- Why LB/endpoint is a shared capability: both the hcp api-ingress and the etcd client
  endpoint need one; burying it in a single adapter would duplicate it in others.

The **coreshift adapter** is a first-party test/dev adapter: it invokes the coreshift
CLI (separate project, not part of HyperFleet v3) to run a *real* OpenShift control
plane — real kube-apiserver, real conditions, real kubeconfig handoff — with no cloud
credentials. It serves two purposes: in-tree CI for the hosted chain (placement,
transport, the hcp adapter, the etcd adapter, status aggregation, deletion gating), and the
solo-dev fast-mint path (§10).

## 10. Deployment modes, day-0, and the local/hub relationship

### Day-0 (fleet mode, laptop)

Day-0 is a v1 requirement. There is never HCP at day-0 (HCP presupposes 2+ clusters of
infrastructure), so the laptop runs the standalone-cluster path only:

1. Customer runs the **ocm-fleet AIO** (single container via podman/compose — its most
   working artifact: embedded SQLite, bundled eval OIDC) with the **CLM addon enabled**.
   This is a declared dependency: day-0 rides ocm-fleet's AIO; the CLM addon packaging
   for it is an upstream deliverable.
2. Customer calls the API (platform-hosted, routed to CLM) with a standalone BM spec.
3. bm-abi adapter generates the ISO with cluster config **and call-home manifests**
   baked in (first-boot hook calls the laptop's CLM endpoint with cluster identity +
   admin kubeconfig — call-home replaces import-success as the status signal, since no
   hub exists yet).
4. Hosts boot the ISO (network-reachable from the laptop), agents self-install
   (SNO is a valid target for a first hub).
5. Call-home completes the inventory record; CLM (or the operator) drives MCE install on
   the new cluster.

### Local/hub relationship — proxy, never migration

There is **no migration of records from a local instance to a hub instance, ever**.
Records live where they were created. Instead:

- Once a hub exists (the local instance can mint 2+ hubs), the local instance may
  **point its lifecycle API endpoint at a hub's CLM**: new creates and lifecycle
  operations are proxied to the hub, so future clusters are fully hub-managed.
- **Prior clusters remain fully manageable locally** — full CRUD including delete — so
  locally-minted clusters are never orphaned. The local store is a live system of
  record, not read-only history.
- Personas:
  - **Solo dev** — runs the laptop AIO indefinitely, minting clusters for exploration,
    discovery, testing, validation. The coreshift adapter gives the same persona
    instant control planes with no BM hosts at all.
  - **Production/enterprise** — uses local exactly once: mint a proper hub, then retire
    the local instance (the hub self-manages via MCE).
- **Pivot handshake (FM-96)**: the first-touch bootstrap-and-pivot feature pivots the
  eval instance into a production MCE-NEXT; CLM's contribution is the proxy re-point
  above. The exact handoff between FM-96's pivot orchestration and CLM's
  proxy/no-migration model is an explicit refinement item on the FM-96↔FM-97 boundary.
  The production pivot target (production MCE-NEXT with CLM registered) is deployed by
  `stolostron/hyperfleet`.

## 11. Bare metal standalone (v1)

- **ABI in a pod**: `openshift-install agent create image` executed in a container with
  the release payload + pull secret. Stateless; config baked at ISO-generation time.
- ISO served from the embedded HTTP endpoint (`isoDownloadURL` in adapter status, as
  the POC infraenv adapter did).
- Call-home manifests baked into the ISO provide: completion signal, cluster identity,
  admin kubeconfig handoff, and (post-hub) self-import.
- DNS: the `dns` capability adapter (DNSProvider impls); on the laptop, embedded
  trivial DNS or user-supplied records; standalone clusters need api/ingress VIPs +
  names.
- Honest scope: no host inventory, no approval gate, no host-level progress. Coarse
  status only. Day-2 via ACM post-import. If host management is demanded later, the
  answer is the assisted-based standalone path (requires replacing Hive's role —
  explicit non-goal for v1).
- The agent flow generalizes to cloud ("an OCI/AWS instance is just a VM that boots a
  discovery ISO") — relevant later for BYO-infrastructure variants.

## 12. HCP mode (hosted topology; first consumer: OCI offering adapter)

First-party machinery (in core — decision #33): the hcp adapter
(HostedCluster/NodePool orchestration, publishing, kubeconfig), the placement
adapter, and the transport port. Platform specifics (worker lifecycle, platform
templates): the **OCI offering adapter**, built out-of-tree by the ORO team against
the published contract — its successful construction against that contract is the
live ORO-test proof and the P2 acceptance criterion. Etcd for hosted clusters is
operated by the in-core etcd adapter (§13). In-tree CI of the hosted chain uses the coreshift adapter (§9).

- **Platform path**: start `platform: None`; adopt **`platform: External`** for new
  clusters when it GAs upstream ([enhancement
  proposal](https://github.com/openshift/enhancements/pull/2168) — provisional Oct
  2026, `TechPreviewNoUpgrade` at introduction, GA gated on two independent
  integrators passing its conformance suite; realistically 2–3 releases out). Platform
  type is immutable: adoption is a cutover for new HostedClusters — None-provisioned
  clusters stay None. The OCI adapter's worker lifecycle must remain cleanly
  separable: it is the production path until External GA, and the fallback after.
- **External integration model** (per the enhancement): the integrator — Oracle —
  ships a fleet-wide controller on the management cluster plus a template/instance
  CRD pair (`OCIHostedClusterTemplate` → `OCIHostedCluster`, `OCINodePoolTemplate` →
  `OCINodePool`); HyperShift instantiates the templates into the control-plane
  namespace and reads status only (`status.infrastructure`, `status.platform`,
  `status.machineTemplate` — a machineTemplate name change is the node-roll trigger).
  The MC admin registers the integrator
  (`hypershift install --external-platform-provider=<group>/<kind>=<ns>/<sa>`);
  contract version rides a CRD label (N/N-1); conformance runs in Oracle CI via the
  upstream `externalplatform` module. Oracle's controller may wrap CAPOCI — the
  contract accepts genuine CAPI infrastructure objects on both halves. Under External,
  HyperShift reclaims machinery the None-hacks adapter hand-rolls today (bootstrap,
  MachineHealthCheck, drain, label/taint propagation, autoscaling); Oracle keeps infra
  provisioning, machine-template rendering, the rollout-name decision, and providerID
  wiring. The CLM-side oci adapter survives but shrinks — it renders
  `spec.platform.external` template references and the platform blob. The None-hacks
  doc splits into HyperShift-owned vs integrator-owned requirements to become the
  External template spec. Post-External chain re-composition (dns/lb platform
  plumbing shifts to the integrator; the user-managed-records opt-out stays) is a P3
  design item.
- **Worker lifecycle on `platform: None`** lives in the OCI adapter and is specified by
  `~/Projects/oro/ORO-Platform-None-Hacks.md` (requirements, not suggestions):
  node-init MachineConfigs (providerID/zone/instance-type from instance metadata),
  surge-limited worker replace, PDB-respecting drain with pause, node health from guest
  Node objects, auto-repair, narrow CSR approver (provisioner-launched node names only,
  newest-only, backlog alerting), config change = full worker replacement, label/taint
  add and remove, per-cluster reconcile workers.
- **Persistent gotchas** (apply under any platform type): OVN
  `internalMasqueradeSubnet` collision with OCI iSCSI (patch to 169.254.64.0/18),
  Oracle udev device rules, ingress must be LB Service at creation, RWO registry
  volume, per-resource-kind tagging, `allowedCIDRBlocks` via direct patch, delete
  sequencing, `Available=True` with zero workers on 4.22.
- **Transport** (first-party, per mode): standalone — HostedCluster/NodePool/InfraEnv
  CRs delivered as ManifestWorks in per-MC hub namespaces (klusterlet applies;
  generation-tracked), deep status (HostedCluster conditions, NodePool replicas) read
  via `managedserviceaccount` short-lived MC credentials. Fleet — delivery rides the
  platform's delivery agents (fleetlet); the ManifestWork/managedserviceaccount pair is
  the transitional mechanism where MCs are still MCE-classic-managed. The
  adapter-status push API remains in the contract for a future on-MC status reporter
  without API change.
- **Placement** (first-party): MC selection at HCP create (capacity/geo/affinity;
  round-robin default), MC list from hub `ManagedCluster` CRs. MCs must be MCE-managed
  before placement can target them (bootstrap chicken-egg solved by day-0 flow, §10).
- **MC prerequisites** (OCI/OKE, from the POC; carried into adapter deploy manifests
  from `oci-infra/TROUBLESHOOTING.md` learnings): port 12250 security list,
  Prometheus/Route CRD shims + cert-manager on vanilla K8s, PSA annotations,
  control-plane affinity for stateful components.

## 13. Etcd capability (in-CLM; fleet-spread etcd for hosted clusters)

Etcd for hosted clusters opting in via `etcd: {managed: clm}` (default `cpo`) is
operated by a **fat capability adapter in core** (`adapters/etcd`, decision #35) —
behind the same SPI as the other capabilities, runnable as an adapter-worker
deployment where isolation warrants. It takes a chain slot for any hosted
cluster that enables it, on any hosted platform. Etcd remains **dedicated per
HCP, never shared** (blast radius and isolation; per the ORO analysis, etcd is
per-customer even in shared control plane models). **Not in the laptop
profile** — there is no HCP at day-0.

**Why it exists — fleet spread.** HyperShift's CPO manages per-HCP etcd within
a single MC; CLM re-hosts it at fleet scope so fault tolerance becomes a
property of the service rather than of any one MC — 3 members on 3 MCs, 2/3
quorum survives single-MC loss; HCP recovery = redeploy the control plane on a
surviving MC — minutes, no snapshot restore. A within-single-MC CLM etcd
manager would be NIH (CPO already does that; decision #18 fallback covers it).
Serial MC upgrades across quorums sharing the MC pool are coordinated by CLM —
a fleet vantage a per-chain view cannot have. The peering + security lists for
2379/2380 are our infra, not the MC provider's.

**Topology — co-location + spread.** Member-0 is placed on the control plane's
MC at mint (sibling namespace — never the CPO-owned HCP namespace); members 1/2
spread across two other MCs from the same read-through fleet view as placement
(§12, its second consumer). Co-location is a mint-time preference, never a
maintained invariant. Full design: ETCD-SERVICE-DESIGN.md (rev 2).

**Client path — multi-endpoint, no LB.** E7 carries an upstream ask for
`UnmanagedEtcdSpec` to accept `endpoints []string` (wired to `--etcd-servers`,
which is natively a list). Member addresses are per-MC Services behind
slot-stable fabric DNS names — kube tracks pod restarts natively; only member
replacement (rare) touches fabric DNS; no load balancer exists in the client
path. Fallback if upstream declines: a single fabric-DNS endpoint re-pointed by
the adapter (controller-adjacent, documented). The seam's verified facts
(immutability, mutable-secret rotation, the HO secret-sync handoff,
assumed-True availability — health truth is ours to publish) are in the design
doc §1.

**Critical-path discipline.** CLM never appears in the data path (KAS →
members), the client path (multi-endpoint failover is the apiserver's own), or
restart-class healing (reset-member init containers re-join the quorum before
etcd starts — controller-free). CLM touches only topology changes (MC loss,
member migration) — which have hours of slack while the quorum serves on 2/3.
Hard rule: **CLM provisions infrastructure, never proxies it.**

**State — level-based, no CRD.** Membership truth is etcd's own member list
plus the observed member pods; the published surface is adapter status data
(slot endpoints, member health, quorum), which the hcp adapter consumes as its
chain precondition when rendering `UnmanagedEtcdSpec`. The mTLS handoff is the
HC-namespace three-key secret; HO's existing sync does the rest.

**v1 acceptance**: the kill test — one MC lost (including the control plane's
MC) → quorum holds → control plane re-homes → client keeps writing, minutes
not restore. Serial-MC-upgrade fault-tolerance drill lands in P3. Fallback per
decision #18 if E7 says the seam is insufficient.

Managed-K8s MC constraints (what "cannot change OKE" actually blocks, and workarounds;
representative for EKS/AKS/GKE too — input to the etcd service design doc):

| Constraint | Impact | Workaround |
|---|---|---|
| OKE resets zone labels (~1s) | No reliable zone anti-affinity; single-AD regions worst | Per-subnet node pools + custom labels + nodeSelector/anti-affinity on those |
| OKE-managed node upgrades | Uncoordinated member eviction | PDB + quorum spread; cross-MC spread + serial MC upgrades |
| OKE control-plane nodes untouchable | Irrelevant (workers only) | — |
| Cross-MC quorum traffic | Not an OKE limit | VCN/DRG peering + security lists (ours) |
| Storage | — | OCI block volumes via CSI; PVs survive node loss (POC ran etcd on OKE PVs) |

Self-managed MCs (OpenShift on BM/kubevirt) get full placement fidelity (real zone
labels, upgrade pausing). Managed-K8s MCs are all the same class: workarounds, fidelity
loss only — nothing fundamental blocks Oracle.

## 14. Phasing

Jira ladder: **HPSTRAT-831** (standalone CLM; ORO the first consumer) → **FM-95 +
FM-97** (fleetification on the MCE-NEXT core) → **FM-96** (first-touch laptop/pivot).
Standalone mode is the delivery that unblocks now; fleet mode is gated on ocm-fleet
readiness, not on FM-95's FR-14 alone.

P1 (bare metal) and P2 (hosted/OCI) run **in parallel**, owned by two adjacent teams
with their own adapter repos. Both consume the P0 contract — which is why the contract
freeze is a P0 exit criterion.

- **P0 — foundation (shared, core repo = `hyperfleet-api` promoted in place)**: repo
  promotion (api-spec, spec-template, e2e fold in; generations retrospective ADR) +
  API (TypeSpec → OpenAPI) + **store SPI with
  both implementations (postgres, ocm-fleet facade)** + **TypeSpec→proto generator** +
  status contract + orchestrator skeleton + SPI. **Two exit criteria: (1) adapter
  contract v1 frozen** (SPI + REST status API + TypeSpec overlay packaging — both
  parallel teams and the OCI offering team build against it); **(2)
  `UnmanagedEtcdSpec` support level verified with the HyperShift team** (supported
  release? caveats?) — decides etcd-manager sequencing.
- **P1 — bare metal standalone E2E (Track A, `openshift-hyperfleet/bm-abi`)**:
  standalone chain, bm-abi adapter, ISO generation + hosting, call-home manifests,
  laptop AIO addon packaging, local proxy re-point. Demo: laptop → first cluster →
  first hub.
- **P2 — hosted topology E2E (hosted machinery in core; parallel with P1)**: the hcp
   adapter, placement adapter, transport port, coreshift test adapter for in-tree CI —
   plus contract co-development with the **OCI offering adapter** (adjacent team,
   out-of-tree, `platform: None` worker lifecycle per None-hacks) and MC prerequisite
   manifests. Etcd adapter v1 (in core): co-located + fleet-spread members,
   multi-endpoint publication (upstream ask via E7, fallback per §13),
   `UnmanagedEtcdSpec` wiring; v1 acceptance includes the kill test (one MC
   lost → quorum holds → control plane re-homes). Standalone-mode deployment (helm + operator repo)
   ships here for Oracle.
- **Fleet mode (after/with FM-95)**: ocm-fleet facade hardening, Connect-registration
  via generated proto schemas, fleetlet transport, platform authz/audit consumption;
  midstream assembly in `stolostron/hyperfleet` (see its companion proposal).
- **P3 — hardening + pivot**: External platform adoption for new clusters, gated on
   upstream GA (enhancement provisional Oct 2026, `TechPreviewNoUpgrade` at
   introduction): Oracle ships the MC-local integrator controller + template CRD pair
   and passes the upstream conformance suite, MCs carry the registration flag, the
   chain re-composes; etcd serial-MC-upgrade fault-tolerance drill (cross-MC spread
   itself is v1 per §13);
   HCP-on-BM (assisted-service + `platform: Agent`, per kubevirt-poc reference);
   placement hardening; recovery drill (stale-status detect → re-place → restore).
- **Later**: kubevirt, vSphere, ROSA, ARO adapters; HostProvider/BMC (Metal3);
  ocm-fleet convergence beyond the facade (inventory reporting).

## 15. Risks and open questions

| # | Risk / question | Mitigation / action |
|---|---|---|
| 1 | `platform: External` timing: enhancement provisional (Oct 2026), `TechPreviewNoUpgrade` at introduction, GA needs two independent integrators passing conformance (realistically 2–3 releases); platform type immutable → adoption is new-clusters-only | None-hacks worker lifecycle is the **production path** until External GA (not just fallback), kept cleanly separable in the OCI adapter; ORO is a candidate GA-enabling integrator — engage upstream now while the contract is shapeable; None-hacks doc splits HyperShift-owned vs integrator-owned to become the External template spec |
| 2 | `UnmanagedEtcdSpec` maturity/support level unknown (tech-preview?) | P0 exit criterion: verify with HyperShift team. Fallback: CPO-managed etcd + EtcdBackup (§13) |
| 3 | Etcd client endpoint strategy across member/MC churn | Multi-endpoint clients (upstream ask, §13) remove single-URL stability; residual: slot-DNS churn on member replacement (rare) + the ask landing; fallback: single fabric-DNS endpoint re-pointed by the adapter |
| 4 | Standalone day-2 gap (upgrades/scale post-import) | Stated non-goal; ACM governance covers some; revisit post-v1 (FM-97 requires single-cluster scale/upgrade — see open question) |
| 5 | ABI status coarseness (call-home only) | Accepted for v1; assisted-based standalone is the future upgrade path |
| 6 | OKE placement fidelity (zone labels reset, single-AD regions) | Per-subnet node pools + custom labels; enhancement OQ-3 (relax zone anti-affinity for External) — our answer: yes, relax (OKE breaks hard zone anti-affinity anyway; fidelity stays ours) |
| 7 | MC bootstrapping chicken-egg | Solved by day-0 flow (§10): MCs are MCE-managed before placement targets them |
| 8 | Contract governance across offering teams (breaking-change arbitration between offering needs) | N-2 compat policy + published contract tests; escalation path TBD |
| 9 | Two parallel teams (P1/P2) consuming the contract simultaneously | Contract freeze is a P0 exit criterion; separate adapter repos give independent trains; post-freeze changes require both teams' sign-off |
| 10 | Coreshift adapter depends on an external project's cadence | Thin glue (CLI invocation) only; coreshift is independently useful to its own track |
| 11 | Managed↔unmanaged etcd immutability | Migration = recreate HCP + snapshot restore; drill in P3 |
| 12 | Security: laptop instance holds admin kubeconfigs from call-home | Secrets at rest on laptop; document threat model; kubeconfig handoff scoped/short-lived where possible |
| 13 | Successor, not drop-in: teams that previously engaged with HyperFleet may expect runtime compatibility | Contracts carry verbatim (status-upsert, TypeSpec overlays); transport, tenancy, and runtime deliberately do not — stated up front in §4 |
| 14 | Day-0 depends on ocm-fleet's AIO (prototype stability) | Declared dependency (§10); the AIO is ocm-fleet's most working artifact; the bm-abi path itself is mode-agnostic |
| 15 | FM-96 (Q2 2027 target) is blocked by FM-97 (Q3 2027) — milestone inversion | Flagged for refinement; either FM-96's target moves, its Q2 preview drops the provisioning path, or FM-97's fleet-mode subset needed by FM-96 lands early |
| 16 | TypeSpec→proto generator is new first-party code | P0 deliverable; small, well-bounded (TypeSpec → proto schema emission) |
| 17 | FM-97 requires single-cluster scale/upgrade for BM/VM topologies; v1 defers standalone day-2 | Upgrade = orchestrate existing CVO/MCO machinery; scale = agent-flow node addition; both need promotion from non-goal if FM-97 scope holds |
| 18 | Repo rename at productization breaks import paths for SPI/contract consumers | Stable module paths decoupled from the repo name at contract publication (SPI + contract-test modules, decisions #30/#31); the rename is a deliberate, coordinated event |
| 19 | `UnmanagedEtcdSpec` × `platform: External` combination unaddressed upstream (enhancement excludes etcd secrets from the integrator Role but is otherwise silent); etcd client-endpoint LB ownership vs integrator-provisioned network/LB/DNS unclear | P0 spike verifies; raise upstream during the enhancement review window |
| 20 | External contract compat window is N/N-1 (CRD label), tighter than CLM's N-2 adapter policy — couples Oracle's upgrade cadence to HyperShift's | Upstream compatibility matrix + conformance in Oracle CI; plan MC upgrade coordination around the N/N-1 window |

## 16. Decision log

| # | Decision | Status |
|---|---|---|
| 1 | Standalone service, own store; POC-proven patterns, ocm-fleet identity concepts; converge via the fleet-mode facade | locked |
| 2 | REST API, TypeSpec-first; `topology × platform` first-class axes; blobbed spec + provider overlay validation; capability opt-out knobs | locked |
| 3 | Status contract carried forward verbatim from HyperFleet (generation/observed_generation, Available/Applied/Health/Finalized, aggregation, soft-delete + Finalized, UUIDv7, RFC 9457) | locked |
| 4 | Adapters = compiled Go controllers + narrow provider interfaces; no CEL/YAML DSL; pre/post Jobs for extension | locked |
| 5 | BM v1 = standalone only via ABI-in-pod (baked ISO + call-home); HCP-on-BM deferred; Hive gone | locked |
| 6 | OCI path = HCP `platform: None` → `External` adoption for new clusters (platform type immutable — None clusters stay None; External owns workers via the integrator template/controller contract) | locked (refined 2026-10-30 per openshift/enhancements#2168) |
| 7 | Unmanaged etcd in v1; etcd = per-cluster opt-in of hosted topology (chain slot), operated by the in-core etcd adapter per #35; per-HCP dedicated, never shared; not in laptop profile | locked (amended by #34, #35) |
| 8 | Standalone transport = ManifestWork (write) + managedserviceaccount (read); fleet transport = fleetlet/platform delivery agents; Maestro gone; status-push API reserved | locked |
| 9 | Placement component; MC list from hub ManagedCluster CRs | locked |
| 10 | ~~Repo = monorepo (core + contract + first-party adapters)~~ → superseded by #20 (four-class repo topology) | superseded |
| 11 | **OCI adapter is an offering adapter** (ORO = HCP on OCI managed by Oracle), built out-of-tree; its construction against the contract is P2 acceptance | locked |
| 12 | Common lifecycle concerns (dns, lb, pullsecret, coreshift-test) = capability adapters in the core repo, still adapters, never core code; provider interfaces are impl strategies inside adapters | locked |
| 13 | Day-0 laptop = v1 requirement (hub-less standalone, call-home) — delivery vehicle refined by #25 | locked (refined) |
| 14 | **No record migration ever**; local instance proxies new ops to a hub endpoint (switchable, 2+ hubs OK); prior clusters stay fully manageable locally; personas: solo dev (indefinite) vs enterprise (one-shot hub, then retire) | locked |
| 15 | Coreshift stays a separate project; a thin first-party **coreshift test adapter** provides hosted-chain CI + solo-dev fast-mint | locked |
| 16 | P1 (BM) and P2 (hosted/OCI) run in parallel, two adjacent teams; adapter contract v1 freeze is a P0 exit criterion | locked |
| 17 | ~~Name: cluster-lifecycle-manager (CLM)~~ → refined by #19/#20/#29: core repo = `hyperfleet-api` promoted in place (rename at productization); integrated product = HyperFleet v3, assembled by `stolostron/hyperfleet` | superseded (twice-refined) |
| 18 | Etcd fallback (CPO-managed + EtcdBackup) if UnmanagedEtcdSpec support insufficient | conditional |
| 19 | **Full succession framing**: HyperFleet v3 = productization/enfleetification of HyperFleet into the ACM fold; old repos archived in place; ADRs 0001-0030 preserved, numbering continues in the core repo; successor-not-drop-in stated up front | locked |
| 20 | **Repo topology, four classes**: `openshift-hyperfleet` org kept as upstream engineering home — core repo (promoted `hyperfleet-api`, renamed at productization per #29) + per-team adapter repos (`bm-abi`, `hcp`); offering adapters out-of-tree; `stolostron/hyperfleet` = thin midstream assembly. Mirrors OCM → stolostron split | locked (core identity amended by #29; classes amended by #33/#35) |
| 21 | Ownership: Cluster Lifecycle/HyperFleet team (Fleet Engineering org) owns the system; adjacent teams own the adapter repos; the OME team builds the new MCE core — HyperFleet integrates with it, not owned by it | locked (amended by #33/#35) |
| 22 | Universal Installer + standalone installer CLI remain sibling workstreams; the adapter SPI is the seam where UI drivers plug in later | locked |
| 23 | **Two deployment modes, one codebase**: standalone (psql, JWT-claims tenancy, ManifestWork+managedserviceaccount, own REST, OTel/stdout pluggable audit) and fleet (ocm-fleet storage/authz/transport, platform-hosted CRUD, CLM keeps operations surface) | locked |
| 24 | Store SPI at repository-port level; two impls: postgres + ocm-fleet API facade (never the platform's DB); CLM-owned sqlite store impl dropped — embedded sqlite is ocm-fleet's own concern | locked |
| 25 | Day-0 laptop rides the ocm-fleet AIO (embedded sqlite, bundled eval OIDC) with CLM as addon — declared dependency | locked |
| 26 | TypeSpec = source of truth; proto schemas generated for fleet-mode Connect registration (generator is a P0 deliverable) | locked |
| 27 | Standalone deployment ships via a forthcoming upstream helm + operator repo (deferred; first consumer Oracle/HPSTRAT-831); `stolostron/hyperfleet` deploys fleet mode only | locked |
| 28 | Jira ladder: HPSTRAT-831 (standalone, ORO) → FM-95/FM-97 (fleetification) → FM-96 (laptop fleet mode); FM-97 keeps its FM-95 dependency, reworded to staged consumption | locked |
| 29 | **Promote in place**: `hyperfleet-api` continues as the CLM core repo — components fold in as needed (api-spec, spec-template, e2e first); git history continuous by construction; rename to `cluster-lifecycle-manager` deferred to the productization phase | locked |
| 30 | **e2e folds into the core repo**; three-surface testing model: in-tree presubmit CI (coreshift chains), digest-pinned full-stack profiles, and a published contract-test module (own module path) run in offering CI | locked |
| 31 | **CI layering**: core presubmits test in-tree chains only; all adapters (per-team and offering) enter core e2e via digest-pinned profiles moved by bump PRs; digest-tracking jobs are non-blocking with attributed failures | locked |
| 32 | **External integration re-home** (per openshift/enhancements#2168): under `platform: External`, Oracle ships an MC-local fleet-wide controller + `OCIHostedClusterTemplate`/`OCINodePoolTemplate` CRD pair against the upstream contract (admin registration flag, CRD contract label N/N-1, conformance via the `externalplatform` module in Oracle CI); HyperShift reclaims bootstrap/MHC/drain/label-taint/autoscaling; CLM-side oci adapter shrinks to `spec.platform.external` template-ref rendering; adoption new-clusters-only, gated on upstream GA | locked |
| 33 | **hcp dissolution — hosted machinery is core** (2026-11-06): the hcp repo folds into core as three things — the **hcp adapter** (translator chain unit: cluster spec → HostedCluster/NodePool CRs), the **placement adapter** (chain unit), and the **transport port** (a mechanism below the chain, not a chain unit: local | manifestwork | fleetlet). HyperShift api-types are scoped to the hcp adapter's package — the adapter is the abstraction layer, so it versions with the thing it abstracts (that's CLM's job). Chain units are of three kinds: capabilities + translators + mode machinery (core), **platform adapters** (per-substrate, own per-team/offering repos — bm-abi and oci are the scent), and a latent class for services with their own availability bar (currently empty — etcd folded back per #35). Invariant: at least one first-party platform adapter stays digest-pinned in core e2e (bm-abi) as the live self-test of the adapter contract. Keep the hcp adapter on a clean module/package boundary so extraction stays cheap if the org ever demands it | locked |
| 34 | **etcd-manager service split** (2026-11-06; superseded by #35): standalone fleet-spanning service — own repo, own team, no CLM imports; fleet-spread is the deliverable (within-single-MC is NIH: CPO already does it, decision #18 fallback covers it), v1 acceptance includes the kill test (MC lost → quorum holds → control plane re-homes). Contract: `EtcdCluster` CRD + published api-types module; N/N-1 CRD compat owned by the service team; core owns the thin capability adapter (no `EtcdProvider` interface for n=1; escape hatch: tiny CLM-owned adapter repo if CRD churn demands — documented, not built). Greenfield API = accepted risk. Survives CLM outages; hard rule: CLM provisions infrastructure, never proxies it. Enters core e2e as digest-pinned infrastructure (postgres pattern). Internal decomposition (fleet-spanning control + MC-local HyperShift-shaped components) → etcd service design doc (TBD) | superseded (by #35) |

## 17. References

- `~/Projects/fleet-engineering-strategy.md` — Fleet Engineering architecture strategy:
  the four target-architecture pillars (ACM/MCE, Fleet Bootstrapper, HyperFleet,
  Universal Installer) this proposal aligns with
- `~/Projects/stolostron/hyperfleet/PROPOSAL.md` — companion midstream integration
  proposal for `stolostron/hyperfleet` (fleet-mode deployment, MCE component
  registration, image pinning, FM-96/97 traceability, coexistence/cutover)
- Jira: HPSTRAT-831 (ORO cluster lifecycle and provisioning — standalone CLM's first
  consumer), FM-95 (Core MCE and OCM.io Management Capabilities — the MCE-NEXT core),
  FM-97 (Cluster Lifecycle Management for Supported OpenShift Platforms — the
  fleetification), FM-96 (OpenShift First-Touch Bootstrap & Pivot Experience)
- `~/Projects/openshift-hyperfleet/hyperfleet-vm-bm-design-6pager.md` — closest prior
  design: opinionated provisioner, on-prem/cloud bundles, provider interfaces
- `~/Projects/openshift-hyperfleet/hyperfleet-provisioning-engine.md` — no-HyperShift-
  Operator design, agent-binder, placement, resilience phases (Phase 4 = re-place +
  restore; cross-MC etcd replication was explicitly out of scope there)
- `~/Projects/openshift-hyperfleet/kubevirt-poc/` — working reference for the full
  assisted agent flow (`DESIGN.md`, six adapter configs); assisted-on-vanilla-K8s
  gotchas
- `~/Projects/oro/ORO-Platform-None-Hacks.md` — OCI worker lifecycle requirements spec
  (requirements for the OCI offering adapter and for the External templates)
- `~/Projects/oro/ORO-HyperShift-OCI-Upstream-Gaps.md` — upstream asks incl. zone
  anti-affinity (OKE label reset), OVN masquerade subnet
- `~/Projects/oro/ORO-Hosted-Control-Plane-isolation.md` — isolation models; etcd
  always per-customer
- `~/Projects/openshift-hyperfleet/OCI-IMPLEMENTATION-PLAN.md`,
  `oci-infra/TROUBLESHOOTING.md` — OKE/MC prerequisites, RHCOS image pipeline
- `~/Projects/openshift-hyperfleet/hypershift/api/hypershift/v1beta1/
  hostedcluster_types.go` — `EtcdManagement`/`UnmanagedEtcdSpec` (external etcd
  endpoint + mTLS, immutable), `ManagedEtcdSpec` (restoreSnapshotURL),
  EtcdSharding gate
- https://github.com/openshift/enhancements/pull/2168 — HyperShift External Platform
  enhancement (provisional, Oct 2026): the template/instance contract, registration
  model, and conformance suite the OCI External integration builds against
- `~/Projects/vkareh/coreshift/` — real OpenShift control plane as processes/containers
  (separate project; basis for the CLM test/dev adapter)
- `~/Projects/stolostron/ocm-fleet/docs/design/architecture/` — resource indexing,
  resource identity and API, addon integration (Enable/Connect contract, dynamic mux,
  Disconnect-keeps-CRUD semantics), platform hierarchy; SQLite/Postgres duality; AIO
  deployment precedent
- `~/Projects/openshift-hyperfleet/architecture/hyperfleet/adrs/` — ADRs 0001-0030
  (0007 conditions status model, 0008 dynamic status aggregation, 0009 UUIDv7, 0010
  JSONB provider-agnostic spec); preserved in the archived repos, numbering continues
  in the core repo
- `~/Projects/stolostron/multiclusterhub-operator/api/v1/multiclusterhub_methods.go`
  (lines 41-108) — current `MCEComponents` list; HyperFleet v3's eventual successor
  slots
- HyperShift DR docs (upstream): OADP snapshot backup/restore, `HCPEtcdBackup`,
  `restoreSnapshotURL`, etcd member recovery, cross-cluster migration scenario

---

Security note for anyone touching `openshift-hyperfleet` while researching this
proposal: `hypershift.sh:4` contains a live quay.io robot password in plaintext, and the
repo root holds a `pull-secret`, a committed 34MB binary, and real OCIDs/compartment
IDs. Rotate the credential and scrub before sharing.
| 35 | **etcd management folds into CLM** (2026-11-09, supersedes #34): a fat capability adapter in core (`adapters/etcd`), runnable as an adapter-worker for isolation. Topology: member-0 co-located with the control plane's MC at mint (sibling namespace), members 1/2 spread via the E9 read-through fleet view — a mint-time preference, never an invariant. Client path: **multi-endpoint** via an upstream ask (`UnmanagedEtcdSpec` `endpoints []string` → `--etcd-servers` list, carried by E7; fallback: single fabric-DNS endpoint re-pointed by the adapter); member addresses are per-MC Services behind slot-stable fabric DNS — no LB anywhere. Critical-path discipline: CLM never in the data, client, or restart-healing paths (reset-member init containers are controller-free); CLM touches only slack-tolerant topology changes. State: level-based, no CRD — membership truth is etcd's own member list + observed pods; published surface is adapter status data. Survives CLM outages for restart-class failures; degraded-but-serving (2/3) otherwise | locked |
