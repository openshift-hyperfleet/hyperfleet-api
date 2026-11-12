# ADR 0033: Deployment shape — one binary, roles for scale, ports for mode

**Status:** Proposed
**Date:** 2026-11-06
**Deciders:** Victor Kareh (CLM architect/owner)
**Consulted:** CLM PoC sessions 2026-11-05/06 — the trade analysis and assembly model below originated there
**Related:** ADR 0031 (day-0 bootstrap shape — this ADR dissolves its Option A/B tension) · PROPOSAL.md decisions #20 (four-class repo topology), #23 (two modes, one codebase), #24 (store SPI), #29 (promote in place) · [POC-BACKLOG.md](POC-BACKLOG.md) (evidence)

> Numbering note: ADR 0032 is reserved for the generations retrospective (first story of E1, per STORIES.md E1-S1). The skip here is deliberate.

---

## Context

The PoC (POC-BACKLOG.md, all tasks done 2026-11-05) proved the vertical slice as **one process**: HTTP server + orchestrator goroutine + in-process adapters, dao-direct store access, adapters reporting through a `StatusReporter` that converges with the external REST status plane (PC-8: identical aggregation, in-process vs curl). Live evidence: create → `Reconciled` in ~90s warm against a real coreshift control plane; DELETE → `Finalized` → 404.

Bundling traded away real properties relative to the v2.5 multi-process topology:

| # | Lost property | v2.5 mechanism |
|---|---|---|
| 1 | Scale-plane decoupling | N api replicas + 1 sentinel; bundled: N replicas = N reconcilers racing |
| 2 | Blast-radius isolation | sentinel/adapter crash left the API serving |
| 3 | Release-train decoupling | five processes shipped on five cadences |
| 4 | Push semantics for external adapters | broker pushed CloudEvents; externals would poll |
| 5 | Buffering between detection and execution | event fabric absorbed bursts |
| 6 | Deploy-time tunability of decision logic | CEL rules were config (deliberately dead — decision #4) |
| 7 | Credential isolation | adapter processes held their own secrets |
| 8 | Per-process attribution | separate metrics per component |

Three requirements now converge:

- **FM-96 (laptop day-0):** bm-abi on a laptop with zero supporting infrastructure
- **HPSTRAT-831 (hub standalone):** scale, HA, credential-bearing adapters
- **FM-97 (fleet mode):** platform-hosted CRUD, facade store, fleetlet transport

Architect directive (2026-11-06): laptop and fleet must deploy **roughly the same shape** — one artifact at different replication, differentiated by configuration, never by reconciler code.

Key enabling fact: standalone bare metal needs **no transport at all** — the ISO *is* the delivery, call-home *is* the status return (PROPOSAL §11). The bm-abi path is mode-portable by nature: it needs only the server role's own HTTP endpoints (ISO serving, call-home) and an executor port for `openshift-install agent create image`.

## Decision

**One binary. Roles select which loops a process runs (scale knob). Ports select which implementation each plane gets (mode knob). Every inter-component conversation is config-targeted: at itself (in-process) or at another replica of the same binary (HTTP).**

### Roles (scale knob)

- **`server`** — REST/mux CRUD, status plane, ISO serving, call-home endpoint
- **`orchestrator`** — drift loop → chains; label-shardable
- **`adapter-worker`** — runs capability adapters against the status plane exactly like an external adapter (list/watch + PUT)

Laptop: one process, all roles, conversations in-process — dao-direct, reporter-direct, not even loopback HTTP (this is the PoC as built). Hub: role-split deployments of the same image. Fleet: the same image as an addon.

### Ports (mode knob)

| Port | Implementations |
|---|---|
| store | sqlite (bootstrap profile, amended #24) · postgres · ocm-fleet facade |
| authz | none-dev · jwt-claims · platform |
| transport | local · manifestwork · fleetlet |
| abi executor | native · host-podman · pod |
| crud hosting | own REST · platform mux |

### Binary assembly (not repo merging)

Repo classes per decisions #20 (amended) / #33 / #35: core, platform adapter repos (`bm-abi` + offerings), midstream. What varies is **which modules a given image compiles in** behind the SPI:

- laptop/bootstrap image = core + bm-abi compiled in, SPI-pinned (the hosted
  machinery — hcp adapter, placement, transport port — already lives in core
  per decision #33)
- hub capability image = core (all platform-agnostic chain units, including
  the hcp adapter)
- bm-abi ships its own image for role-split deployments (adapter-worker shape)
- etcd is an in-core capability adapter (decision #35), runnable as an
  adapter-worker where isolation warrants — no separate artifact
- offering adapters (oci) never compile in — external processes against the
  published contract

The enabling proof is PoC PC-8: **in-process and REST are the same contract**, so "run the adapter out-of-process" is a deployment fact, not an architecture change.

### How the lost properties return

| Lost | Returned by |
|---|---|
| 1 scale-plane coupling | role flags: orchestrator deployments separate from server deployments |
| 2 blast-radius | roles are process boundaries when wanted, goroutines when not |
| 3 trains + 7 credentials | adapter-worker role: secret-bearing adapters run as their own deployments — same binary, driving the API like any external adapter |
| 4 push | watch on the status plane (resource-version long-poll/SSE) — broker-like drive, no broker |
| 5 buffering | store-as-queue: generation + debounce over the store is the durable queue; v2.5's broker-in-front-of-a-database was redundant |
| 6 CEL tunability | stays dead (decision #4); parameters and chains are config, rule shape is code |
| 8 attribution | in-process instrumentation labeled by role and adapter |

### Deployment shapes

| | Laptop (FM-96) | Hub standalone (HPSTRAT-831) | Fleet (FM-97) |
|---|---|---|---|
| Processes | 1 (all roles) | N server + M sharded orchestrator + adapter-workers | addon in ocm-fleet |
| store | sqlite | postgres | facade |
| authz | none-dev | jwt-claims | platform |
| transport | local (unexercised at day-0) | manifestwork | fleetlet |
| abi executor | native | pod | pod |
| crud hosting | own REST | own REST | platform mux |
| chains | standalone/agent-bm (+ hosted/test for dev) | all | all |
| ISO / call-home | localhost endpoints | hub ingress | platform endpoint |

Same chains, same loop, same status plane in every column. The one surviving code fork is fleet-mode CRUD hosting on the platform mux (already planned, PROPOSAL §6 mode split); everything behind the front door is mode-blind.

## Relationship to ADR 0031

This dissolves 0031's Option A/B tension: the "bootstrap binary" and "the product" become the same artifact at different replication. The fidelity argument reduces to "store impl behind one conformance suite" (E3-S4) plus the front door. 0031's Option A, read under this ADR, is "the product binary with a laptop profile." 0031's falsifiers (F1–F5) still govern the *packaging* question (container vs binary, cold-start); this ADR governs the *internal shape*, which is packaging-independent.

## Precedent

Grafana Mimir/Cortex/Tempo run this exact model: one binary, `-target=reader|writer|compactor` roles, monolithic mode (all components in-process, config points at self) and microservices mode (per-role deployments) from the same artifact. CLM adds the adapter-contract twist they do not have: components that can be compiled in **or** run externally against the same contract.

## Trade-offs accepted

- Core capability adapters (dns, lb, pullsecret, coreshift) couple to the core release train when compiled in (deliberate — they are every-chain concerns)
- No event fabric: watch + store-as-queue replace it; detection re-fires under debounce when execution lags
- External adapters drive by watch, not push — watch must be good (E5-S3) or out-of-tree ergonomics regress vs v2.5
- Decision-rule shape is code; only parameters are config

## Reversal criteria

Flip toward split binaries or reintroduced messaging only if ALL hold for a given plane:

- watch semantics prove insufficient to drive external adapters at fleet scale (latency or load), AND
- store-as-queue debounce churn under sustained execution lag becomes measurable overhead, AND
- role-split deployments of the same image fail to contain a real blast radius

## Story impacts

- **E5-S3**: list/watch must land early, not late — it is the drive mechanism for adapter-workers and external adapters
- **E5-S4**: label sharding is the multi-replica prerequisite, before any N>1 orchestrator deployment
- **E13 / etcd**: in-core fat capability adapter per decision #35 — multi-endpoint clients (upstream ask via E7) mean no LB and no re-pointing; reset-member init containers make restart healing controller-free; CLM appears only in slack-tolerant topology changes. Hard rule: CLM provisions infrastructure, never proxies it
- **E16**: role-split deployment shapes; digest pinning per image
- **P1 / bm-abi slicing**: laptop-profile assembly (core + bm-abi compiled in) and the abi executor port are first-class stories
- **FM-96**: this ADR + 0031 together answer the eval-instance question

## References

- [PROPOSAL.md](PROPOSAL.md) — §5 (modes), §6 (mode split), §8 (repo strategy/packaging), §10–11 (day-0, bm-abi); decisions #20/#23/#24/#29
- [POC-BACKLOG.md](POC-BACKLOG.md) — status table and appendix (PC-8 convergence proof, live timings, retrospective)
- [STORIES.md](STORIES.md) — E3-S4, E5-S3, E5-S4, E12, E16
- [adr-0031-day-0-bootstrap-shape.md](adr-0031-day-0-bootstrap-shape.md)
