# ADR 0031: Day-0 bootstrap shape

**Status:** Proposed — pending FM-96 refinement
**Date:** 2026-11-03
**Deciders:** Victor Kareh (CLM architect/owner); FM-96 owner (Jay Balunas); FM-96 architect (Joshua Packer); OME core architect (name TBD)
**Consulted:** CLM squad leads (lead-engineer conversation, week of Nov 2, 2026)
**Supersedes (on acceptance):** PROPOSAL.md decision #25 (partially); amends decision #24

---

## Context

FM-96 (OpenShift First-Touch Bootstrap & Pivot) requires a day-0 experience that
runs on a customer machine with minimal ceremony and provisions the first
OpenShift cluster, which becomes the first MCE hub. Its hard constraints:

- FR-1: one-command bootstrap; functional console in < 5 minutes
- FR-15: eval runs with **zero external dependencies** (embedded store)
- FR-18: working authentication with zero setup
- Provisioning-initiated within < 30 minutes on bare metal (vSphere follows
  later via the same agent path — no vSphere test infra exists today)
- FR-2/FR-16: recursive pivot to a production MCE-NEXT on Kubernetes with an
  external relational store, **no eval→prod data migration**
- Air-gapped deployment is out of committed scope but must not be precluded
  (origin design: pre-baked minimal ISO + local mirror registry)

FM-96's own boundary text: the Feature owns bootstrap-and-pivot orchestration
and *consumes* cluster lifecycle's provisioning capability (FR-3/4) as a
dependency. Its dependency on FM-95 (Core MCE) applies to the **production
pivot and zero-trust-gated flows** — the eval bootstrap itself is not gated on
the platform.

The CLM block architecture (see `HPSTRAT-831-epic-breakdown.md` §3) makes the
day-0 shape a *packaging* choice, not an architectural one: one Go binary,
store-port (postgres / facade / [proposed] SQLite), adapter runtime in-process
by default, an ABI executor port in bm-abi (native / host-podman / pod), and a
transport-port with a `none` implementation for day-0 (no management clusters
exist; delivery is the ISO itself plus call-home).

The open question is what the eval instance **is**: a CLM bootstrapper that
creates the product (Option A), a miniature of the product itself (Option B),
or the product's core linked into A's packaging (Option C).

**Provenance note:** the origin design doc ("OpenShift First Touch
Experience") specified a local binary with a web UI; the FM-96 PRD framing
("evaluation MCE-NEXT, recursive pivot") entered during translation into the
ticket. This is noted as context, not authority — the ocm-fleet AIO matured
after the origin doc was written, and circumstances changed.

**Weighting discipline:** the arguments below were audited for inflation. The
dependency-direction argument is stated as the value judgment it is, not a
graph proof; FR-15's key phrase is presented under both readings; the AIO's
current limitations are labeled current-state, not architectural. The intent
is a document the Option B advocate can sign as accurate.

---

## Option A — bootstrap binary

CLM standalone profile compiled as a single binary: SQLite store impl
(amends decision #24, which dropped it on the assumption that day-0 rides the
AIO), native ABI executor (openshift-install extracted from the release
payload and run as a binary — no container runtime assumed), embedded HTTP
server for ISO distribution and call-home, local web UI, minimal local auth
(FR-18 requires *something* even with eval OIDC deferred).

**Case for:**

- Zero external dependencies, literally: no podman, no image pull, no
  platform, no identity provider. The only network needs are the release
  payload + pull secret (or a pre-baked ISO for air-gap).
- Cold-start: a binary starts in milliseconds; a cold container pull of a
  multi-gigabyte image takes minutes. *(Unmeasured — see falsifier F4.)*
- Train independence: day-0 ships on CLM's release train, not gated on
  platform readiness (fleetlet unbuilt, addon model compile-time, no
  private-state resolution).
- None of the AIO's current constraints apply: no loopback-only publishing
  fight (the binary binds the interface it needs for ISO serving and
  call-home), no compile-time addon pinning, no fleetlet dependency.
- Pivot is boring: create the first cluster, deploy MCE on it, proxy re-point
  (decision #14 — records live where created; no migration ever). The origin
  doc's model: "they both will represent the same data."

**Honest costs:**

- **Fidelity**: the eval exercises a different code path from fleet mode —
  own REST instead of platform mux, SQLite instead of facade, local auth
  instead of platform tenancy. The store conformance suite (E3) mitigates
  *store* divergence only; the auth/hosting/tenancy paths are untested at
  day-0. "Works in eval, different in prod" is a real complaint class.
- **UX seam**: two consoles (binary's, then the hub's), joined by proxy
  re-point. Workable, but a handoff discontinuity that B does not have.
- **Forcing function deferred**: day-0 does not pressure the extension
  contract (FR-14) to completion — see B's case; this is A's most serious
  concession.
- Third store impl to maintain (thin, conformance-tested, but real).
- The < 5-minute console requires a UI CLM does not yet have (FM-96 owns
  first-touch UX and can target the CLM API under either option, but under A
  the burden lands on this side).
- **Dependency-direction, stated honestly**: "the platform cannot exist
  before clusters exist" is false in the literal sense — the AIO *is* the
  platform existing pre-clusters. The defensible claim is the value judgment:
  *the platform adds nothing CLM needs at day-0.* That judgment is reasonable
  but contestable, and B's advocate should contest it.

---

## Option B — ocm-fleet AIO (platform + CLM compiled in)

Day-0 rides the ocm-fleet AIO per current PROPOSAL.md decision #25: single
container (podman/compose), embedded SQLite under the store-facade, bundled
Dex eval OIDC, CLM compiled in as an in-process addon, CRUD hosted on the
platform's dynamic mux.

**Case for (steelman, full strength):**

- **The forcing function — arguably the strongest argument in this entire
  discussion**: making day-0 ride the platform means CLM passes through the
  extension contract (Enable/Connect, proto registration, Disconnect-keeps-
  CRUD) from the very first customer touch. CLM's contract discipline is
  load-bearing for Oracle (the ORO test) and for FM-97; day-0 pressure is a
  real enforcement mechanism for finishing and hardening FR-14. Option A
  removes that pressure and lets integration debt accrue quietly until
  fleet-mode work begins.
- **Fidelity**: the eval exercises the production code paths — extension
  resources, dynamic mux, tenancy, inventory — so bugs surface early in the
  exact paths production uses, and "what you evaluated is what you run" is
  literally true.
- **Product narrative**: the eval instance *is* MCE-NEXT and pivots into
  itself. This is the strongest version of the INSTALL → MANAGE → CONSOLIDATE
  story against the Rancher/Proxmox benchmark, and the frame FM-96's PRD
  already commits to.
- **Single console, no handoff seam** — one identity, one surface, from
  first touch through pivot.
- The AIO is the platform's most working artifact — reuse beats new surface.
- One image, one build, versioned together — the coupling CLM calls "pinning"
  is, from the platform side, a feature.

**Honest costs:**

- **Zero-dep strained**: a container runtime is an external dependency under
  the strict reading of FR-15; a multi-gigabyte cold pull pressures the
  5-minute console AC. *(Both readings of FR-15 are legitimate — see
  falsifier F1. Rancher's first touch is `docker run`, and Rancher is the
  benchmark.)*
- **Pivot machinery**: "recursive pivot of the eval instance" means
  reconstituting a container + SQLite + Dex + compile-time in-process addons
  into a production Kubernetes deployment with an external relational store,
  with no data migration. This is genuinely hard, and under B it is owned by
  the platform/FM-96 side. A's pivot (create hub, re-point) is boring by
  comparison — but note A's pivot has its own cost, the UX seam above. Both
  pivots cost; they cost differently.
- **Current constraints** *(current-state, fixable — not architectural
  verdicts)*: loopback-only publishing mandates a fight with call-home from
  booted hosts; addon selection is compile-time, pinning AIO builds to CLM
  versions; fleetlet is design-only, unbuilt; no private addon state is
  resolved. Each is plausibly changed in a pre-1.0 platform; designing day-0
  around today's limitations is tail-wagging-dog — a fair B rebuttal.
- **Timeline coupling**: day-0 readiness inherits the platform's readiness.

---

## Option C — binary linking resource-manager (negotiated middle)

CLM's binary packaging (A) with the ocm-fleet resource-manager linked as a
**library**: the eval instance literally *is* MCE-NEXT — same code,
embedded — delivered with zero container dependency. Storage rides the
platform's SQLite through the facade; CLM is an in-process addon exactly as
in B.

- Preserves B's narrative ("what you evaluate is what you run") at A's
  packaging.
- Cost on the OME side: resource-manager must become embeddable — today it
  is an s6-supervised process in a container, not a library (falsifier F2).
- Cost on the CLM side: compile-time coupling to OME's code layout and
  release cadence — the coupling B has, minus the container.
- Auth at day-0 still needs an answer (Dex embedded, or deferred local auth
  as in A).
- Worth offering only if it actually settles the narrative dispute — it is a
  concession to be traded, not a default.

---

## Falsifiers — the empirical questions that settle this

| # | Question | Owner | Method |
|---|---|---|---|
| F1 | Does FR-15's "zero external dependencies" include the container runtime? | PRD owner (Shawn Purtell), FM-96 owner (Jay) | Direct ruling in refinement. Strict reading → strong for A; "external services only" reading (Rancher precedent) → neutralizes A's headline argument |
| F2 | Can resource-manager be linked as a library (embeddable, no server-context assumptions)? | OME core architect | Time-boxed spike. Yes → C is viable and likely the peace |
| F3 | Are loopback relaxation and dynamic (non-compile-time) addons committed on the platform roadmap? | OME core architect | Roadmap commitment with dates |
| F4 | Cold-start benchmark: binary start vs cold podman pull + AIO start, against the 5-minute console AC | Both teams | Measure on a clean laptop with average broadband |
| F5 | Fleetlet / platform delivery timeline for FM-97 | OME core architect | Roadmap; determines how long B's "current constraints" persist |

---

## Proposed decision

**Option A — bootstrap binary — contingent on the falsifiers:**

1. If **F1 rules the container runtime a dependency** (strict reading) *and*
   **F4 confirms a material cold-start gap** → A stands, unless **F2 shows C
   is cheap** (then C, as the negotiated settlement).
2. If **F1 rules tooling acceptable** (lenient reading) *and* **F4 shows the
   gap is negligible on warm cache** → B's coherence and forcing-function
   case wins on the merits.
3. Absent falsifier resolution by FM-96 refinement, A is the default because
   it is the only option with no unresolved external dependency — but this
   default is explicitly weak, and B's advocate is entitled to call it so.

## Concessions A makes to B

- **Fidelity enforcement moves from runtime-riding to contract**: the store
  conformance suite, the facade implementation, and the published
  contract-test module are already P0 deliverables (E3/E6, HPSTRAT-831
  breakdown). The assurance the platform side actually needs — *CLM will not
  fork* — is delivered by published contract plus conformance, not by
  packaging. Under A, day-0 divergence is bounded to the store
  implementation, which passes the same conformance suite as postgres and
  the facade.
- **Facade-first commitment for fleet mode** stands unchanged: when CLM runs
  on the platform (FM-97), it rides extension resources and the dynamic mux
  with no private tables, per decision #24's surviving intent.
- **Narrative framing**: day-0 is positioned as "the bootstrapper creates
  your MCE-NEXT" — the platform remains the pivot target and the product. A
  changes who delivers the front door, not what the product is.

## Decision amendments triggered (on acceptance)

- **#24 (amended)**: the SQLite store impl is revived, scoped to the
  bootstrap profile only, behind the shared store conformance suite.
- **#25 (superseded)**: the ocm-fleet AIO is *optional* at day-0 — a
  supported packaging, not a dependency. It remains the fleet-mode host.
- **New decision #32**: day-0 shape = bootstrap binary (contingent form
  above).

## Reversal criteria (flip to B or C when ALL hold)

- Resource-manager is linkable (F2 yes) or addons are dynamic (F3 yes), **and**
- Loopback publishing is relaxed for addon endpoints, **and**
- The pivot machinery (eval instance → production) is proven, **and**
- Warm-cache AIO start meets the 5-minute AC.

When B's costs are gone, B's coherence advantage should win. This ADR should
not be read as a permanent property right to the front door.

---

## References

- FM-96 — OpenShift First-Touch Bootstrap & Pivot (Jira; PRD v0.11)
- FM-97 — Cluster Lifecycle Management for Supported OpenShift Platforms (Jira)
- Origin design doc: "OpenShift First Touch Experience" (author's doc,
  pre-FM-96)
- `HPSTRAT-831-epic-breakdown.md` §3 (block architecture), §4 (testing model)
- `PROPOSAL.md` decisions #14 (no migration, proxy re-point), #24 (store SPI),
  #25 (day-0 rides AIO)
- ocm-fleet: `docs/design/architecture/addon_integration.md`,
  `docs/design/managed_resources.md`, `deploy/aio/README.md`,
  `docs/design/architecture/core_model.md` (single-pod viability)
- hyperfleet-api promotion model (ADR 0031 continues numbering from the
  preserved 0001–0030 series)
