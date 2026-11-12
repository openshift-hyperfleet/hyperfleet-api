# Etcd Capability (in-CLM) — Design & PoC Backlog

Status: revision 2 — PoC executed (kill test passed 2026-11-09)
Date: 2026-11-09 · rev 1: 2026-11-06 (standalone service — superseded by decision #35)
Related: [PROPOSAL.md](PROPOSAL.md) §13, decisions #7/#33/#35 · [adr-0033-deployment-shape.md](adr-0033-deployment-shape.md) · [STORIES.md](STORIES.md) E13, E7, E9 · prior proposal: `~/Projects/vkareh/etcd-as-a-service/` (2026) · HyperShift reference: `~/Projects/openshift-hyperfleet/hypershift/`

> Revision 2 folds etcd management into CLM as a fat capability adapter
> (decision #35), superseding rev 1's standalone service (#34). The research
> (§3, §5) carries unchanged; the architecture it supports got simpler:
> multi-endpoint etcd clients delete the endpoint LB — the one component whose
> availability bar justified a separate service. Rev 1's CRD contract and
> compat policy are also gone: membership truth is etcd's own member list.

---

## The PoC question

> Can the in-core etcd adapter mint a three-member quorum — one member
> co-located with the hosted control plane's management cluster, two spread
> across other MCs — that keeps a client writing while one MC is destroyed,
> with CLM nowhere in the data or client paths?

**Answered: yes.** Kill test passed 2026-11-09 — evidence and numbers in the
appendix.

## 1. The seam (verified against HyperShift source)

`EtcdManagement: Unmanaged` (`hypershift/api/hypershift/v1beta1/hostedcluster_types.go`,
all CEL, no webhooks):

- `Endpoint string` — required, `^https://`, ≤255 chars. Immutable.
- `TLS.ClientSecret` — required, a Secret with exactly three keys:
  `etcd-client-ca.crt` (CA bundle), `etcd-client.crt`, `etcd-client.key`.
- `managementType`, `endpoint`, and the TLS secret *name* are immutable; the
  secret's **content is mutable and re-synced every reconcile** — cert rotation
  works by updating the secret (CA bundle carries old+new chain; kube-apiserver
  reloads on restart).
- HO copies the HC-namespace secret into the control-plane namespace
  (`etcd-client-tls`) verbatim, every reconcile. CPO validates the three keys
  (that is the *entire* validation) and wires `--etcd-servers` + the client
  cert into kube-apiserver, openshift-apiserver, oauth-apiserver;
  `wait-for-etcd` is removed for unmanaged; the endpoint hostname is added to
  NO_PROXY.
- Nothing ever probes the endpoint. For unmanaged clusters CPO sets
  `EtcdAvailable=True`, reason `EtcdRunning`, message *"Etcd cluster is
  assumed to be running in unmanaged state."* **Availability truth is ours to
  publish** — the adapter's status data and conditions are the health record.

**The upstream ask (carried by E7):** `UnmanagedEtcdSpec` accepts a single URL
today. kube-apiserver's `--etcd-servers` is natively a list — the most proven
HA etcd client configuration that exists. Ask HyperShift for an additive
`Endpoints []string` (parallel to `Endpoint`) wired to `--etcd-servers` as a
list. This is the load-bearing piece of the in-CLM design (§2.3). Fallback if
upstream declines: a single fabric-DNS endpoint re-pointed by the adapter —
controller-adjacent, degraded mode documented, strictly worse.

## 2. The design (decision #35)

### 2.1 Home and shape

Etcd management is a **fat capability adapter in core** (`adapters/etcd`),
behind the same SPI as pullsecret/dns/lb/coreshift, runnable as an
**adapter-worker** deployment where process or credential isolation warrants
(ADR 0033's role model — isolation as a deployment fact, not a repo). No
separate service, no cross-team contract, no CRD.

### 2.2 Topology: co-location + spread

```
                     fabric DNS zone (ours): m0.etcd-h1   m1.etcd-h1   m2.etcd-h1
                        slot-stable names; change only on member REPLACEMENT (rare)
        ┌───────────────┼─────────────────────────────────────────────────┐
        ▼               ▼                                                 ▼
   MC-A (placement)  MC-B                                            MC-C
   ┌──────────────┐  ┌──────────────┐                                ┌──────────────┐
   │ HCP-1        │  │ etcd member 1 │  per-MC Service tracks         │ etcd member 2 │
   │  kube-apisvr │  │ (sibling ns)  │  pod restarts natively         │              │
   │ etcd member 0│  └──────────────┘                                └──────────────┘
   │ (sibling ns) │
   └──────────────┘
        │   KAS --etcd-servers = m0,m1,m2 — all three; the client fails over alone
        └── co-location is a mint-time PREFERENCE: MC-A dies → quorum 2/3 on B,C
            → control plane re-homes to B → member set unchanged → KAS reconnects
```

- Member-0 is placed on the same MC as the control plane **at mint**, in a
  **sibling namespace** (never the CPO-owned HCP namespace); members 1/2 are
  spread across two other MCs from the **same read-through fleet view as
  placement (E9)** — its second consumer. 3 members / 3 MCs: any single MC
  loss leaves 2/3 quorum serving.
- Co-location is **never a maintained invariant** — after re-homing, "the
  local member" is whichever survivor the control plane landed near. Chasing
  co-location across re-homes would be churn for no resilience gain.
- Latency honesty: writes go to the leader wherever it is, and the leader
  moves — every write crosses at least one MC boundary. Same-region fabric is
  the accepted price (already the design). Co-location's real benefit is
  simpler placement (spread only the *other* two) and a local member for the
  KAS's own connection.

### 2.3 Client path: multi-endpoint, no LB

With the upstream ask landed, the HCP's etcd config lists **all three member
slot endpoints**; kube-apiserver's etcd client handles failover natively. No
load balancer exists anywhere in the client path. Member addressing:

- Each member slot has a **per-MC Service** (kube tracks pod restarts
  natively — membership URLs never change on restart) behind a
  **slot-stable fabric DNS name**. Only member *replacement* (rare) touches
  fabric DNS.
- The fabric DNS zone is ours (small, stateless, cache-tolerant — it replaces
  rev 1's LB as the one shared fabric component, with strictly weaker
  availability requirements: stale answers degrade to a working member at
  worst, given multiple endpoints).
- Fabric requirements: slot names resolve from every MC; MC pod/service CIDRs
  must not overlap (joins the MC-prerequisites list, E14, alongside 2379/2380
  security lists and PMTU).

### 2.4 Critical-path discipline

| Path | Contains CLM? | Why |
|---|---|---|
| Data path (KAS → members) | Never | Provision, never proxy — unchanged |
| Client connectivity (KAS finding a live member) | Never | Multi-endpoint: KAS fails over by itself; no component of ours is involved |
| Restart-class healing (pod dies, node reboots) | No | **Reset-member init container** re-joins the quorum before etcd starts (H5) — controller-free; per-MC Services absorb pod churn |
| Topology changes (MC loss, member migration) | Yes — hours of slack | Quorum serves on 2/3, degraded-but-serving; removal + replacement is not urgent. A CLM outage delays it, never breaks it |

The design goal is not a separate high-availability controller — it is
**maximizing controller-free healing** (init containers + per-MC Services) and
**eliminating the re-pointing component** (multi-endpoint). What remains for
CLM is slack-tolerant membership work.

### 2.5 State: level-based, no CRD

No EtcdCluster CRD, no api-types module, no compat policy. Membership truth
is reconstructed every pass from three sources that already exist: **etcd's
own member list** (ground truth, via the fleet client cert), **the member
pods observed on MCs**, and **the adapter's published status data** in the
CLM store (slot endpoints, member health, quorum — the surface the hcp
adapter consumes as its chain precondition when rendering
`UnmanagedEtcdSpec`). Desired-vs-observed reconciliation; no owned state to
drift. mTLS handoff is unchanged: the adapter emits the three-key secret for
the HC namespace; HO's existing sync does the rest.

### 2.6 Member delivery

Member manifests (STS-per-member, init container, Service, PVC) are delivered
**through the transport port** (decision #33) — same port as the hcp
adapter's CRs. Implementations per mode: direct-kubeconfig for the PoC (kind
MCs), ManifestWork for standalone production, fleetlet for fleet mode.

## 3. Findings that shaped the design (research, 2026-11-06)

1. **We are the seam's first serious consumer.** `UnmanagedEtcdSpec` is not
   feature-gated (Default feature set) yet has zero e2e coverage upstream and
   assumed-True availability. API-GA but operationally greenfield. E7 confirms
   supported-status and carries both upstream asks (support level;
   `endpoints []string`).
2. **The local hypershift clone predates `platform: External`** — the
   Unmanaged × External question (risk #19) can only be verified upstream.
3. **The gold pattern — reset-member self-heal.** HyperShift's member pods
   run an init container that, on restart, executes `etcdctl member
   remove/add` against the live quorum *before etcd starts* (name-keyed,
   idempotent, operator-free). The common failure needs no controller in the
   loop. This is what makes in-CLM safe (§2.4).
4. **The prior proposal's quorum caveat stands**: 2+1 across two MCs tolerates
   member loss, not MC loss — 3-on-3 is the default, loudly.
5. **Isolation is invariant** (ORO analysis): per-customer etcd in every
   model; 3 members on 3 distinct fault domains is the expected HA shape.
6. **Storage resilience is proven** (OCI POC: PVs survive node loss; etcd I/O
   is the density limiter; ~8–16 GiB PV per cluster); HA spread never was —
   this capability fills it.
7. **The cross-MC network fabric is greenfield** (no DRG/peering in any POC).
   Known constraints: security lists are the #1 failure source (stateful,
   both directions), ICMP 3/4 (PMTU) required, kubelet port 10250-vs-12250
   discrepancy, OVN `internalMasqueradeSubnet` collision with OCI iSCSI.

## 4. Harvest inventory (HyperShift → this adapter)

Fleet-transferable (cite in implementation PRs):

| ID | Pattern | Source |
|---|---|---|
| H5 | Reset-member init container: stateless rejoin via name-keyed member remove/add before etcd starts | `v2/etcd/etcd-init.sh`, statefulset.yaml |
| H7 | Health model: per-member clientv3 `Get` (linearized voters / serializable learners, 30s timeout), quorum = n/2+1, unstarted = empty ClientURLs | `pkg/etcdcli/health.go` |
| H12 | Idempotent member ops: `MemberAddAsLearner` (peer-URL dedupe), `MemberPromote`, `MemberRemove`, `MemberUpdatePeerURL` | `pkg/etcdcli/etcdcli.go` |
| H11 | Recovery decision logic: recoverable iff quorum present + exactly one failing member; refuse during rollouts; delete pod+PVC then re-add | `etcd-recovery/etcdrecovery.go` |
| H6 | Defrag: 10m ticker, ≥45% fragmented && ≥100MB, leader last, health-gated, HA-only, 3 failures → degrade | `etcd-defrag/etcddefrag_controller.go` |
| H8/H9 | Backup: one-shot, health precondition, serial guard, TTL 600s / deadline 900s, count retention; `etcdctl snapshot save` + object-store upload | `etcd-backup/` |
| H10 | Restore: only when data dir empty; per-member identity + token; `--bump-revision --mark-compacted`; staged dir; etcdutl (3.6+) vs etcdctl (3.5) | `etcd-init.sh` |
| H13 | Probe budgets, PDB minAvailable 1, priorityClass, topology spread | statefulset.yaml, pdb.yaml |
| H14 | Quorum-based availability condition (ready ≥ n/2+1) — publish real health, not assumptions | `hostedcontrolplane_controller.go` |
| H2/H3 | Secret-sync semantics + CA-bundle rotation (old+new chain; reload on restart) | HO controller, pki docs |
| H17 | NO_PROXY: endpoint hostnames reachable without MC egress-proxy interference | oapi/oauth deployments |

Single-MC-bound (replace): in-namespace DNS peer URLs → slot-stable fabric
DNS; StatefulSet-of-3 in one namespace → one STS per member per MC; in-pod
defrag sidecar + KAS-lease leader election → remote defrag via clientv3,
CLM's own sequencing.

Operational specifics: mTLS mandatory (`--client-cert-auth
--peer-cert-auth`); server cert verifiable by the client CA bundle; TLS
1.2/1.3 standard suites; metrics on a separate port/CA; probe shape
`curl --cacert/--cert/--key https://<ep>/health`.

## 5. Disaster recovery

Flow (unchanged from rev 1 / the prior proposal): detect/declare MC loss →
fence failed or partitioned members → quorum check → **quorum remains:** add
replacements on healthy MCs, remove failed members → **quorum lost:** restore
latest validated snapshot into a new cluster → validate health, membership,
certs, data → verify consumer availability (with multi-endpoint, no endpoint
re-pointing is needed — the surviving member endpoints keep serving).

Rules: never point clients at a stale or partially restored member; the old
cluster must not accept writes after migration (split-brain); consistency =
etcd-consistent snapshots only — raw data-dir copies are not backups.

**Targets: deferred to PoC data** — the kill test (EP-6) measures the
quorum-preserved RTO; the restore drill measures the quorum-lost RTO; the
backup cadence experiment sets RPO. Numbers land in the PoC appendix.

## 6. Security and monitoring requirements

Carried unchanged from rev 1 / the prior proposal: TLS on client, peer, and
metrics traffic; separate client and peer CAs; snapshots encrypted at rest
and treated as sensitive copies of the whole keyspace (never committed, never
logged); credential rotation without cluster rebuild; audit provisioning,
credential changes, snapshots, restores, promotions, endpoint changes.
Monitor: member health and quorum, leader changes and proposal latency,
backend fsync/commit latency, DB size and fragmentation, snapshot age and
upload failures, certificate expiration, restore/migration progress. Upgrades
preserve quorum; every release tests restore, not only backup.

## 7. PoC backlog (in-CLM, `poc` branch, driven by the CLM agent)

The PoC runs in the CLM repo — the etcd adapter is core code. Salvaged from
the retired etcd-manager repo: the 3× kind harness (EP-2). The inotify
sysctl fix (`fs.inotify.max_user_instances=1024`) was required before live
kind runs — applied and persisted.

| # | Task | Proves |
|---|---|---|
| EP-1 | Adapter foundations in `pkg/adapters` (etcd flavor): internal membership model, mTLS minting (CA + member server/peer certs + the 3-key client secret contract), member manifests (STS-per-member + reset-member init container + per-MC Service + PVC), status-data contract, member delivery behind a minimal transport interface (direct-kubeconfig impl; ManifestWork is E10's product work). Fake-client unit tests — no kind, no live etcd | the machinery, tested |
| EP-2 | Kind harness (salvaged): 3× kind MCs via podman, `.fleet/` kubeconfigs, wired to CLM's read-through fleet view; live `mcs`-style verification | the substrate |
| EP-3 | Co-located placement: member-0 on the placement decision's MC, spread 1/2 across the fleet view; chain wiring (`hosted/*+etcd`) | §2.2 |
| EP-4 | Multi-endpoint + seam: hcp-side rendering of the endpoints list from etcd status data; three-key secret in HC-namespace shape; UnmanagedEtcdSpec-shaped validation (no HostedCluster needed). Include the single-endpoint fallback path | §2.3 + the seam |
| EP-5 | Membership sequencing: learner add/promote/remove (H12), single-failure replacement (H11), MC-loss topology repair | §2.4 row 4 |
| EP-6 | **Kill test**: client stand-in + member-0 on MC-A, members on B/C → kill MC-A → quorum holds → client keeps writing → replacement lands. Plus degraded-mode (controller stopped, member restart heals) and snapshot/restore drill. Measure RTO/RPO | acceptance |
| EP-7 | Retro: findings, measured numbers, doc corrections | the loop closes |

Sequencing: EP-1 → EP-2 → EP-3 → EP-4 → EP-5 → EP-6 → EP-7.

### Status

| Task | State | Commits |
|---|---|---|
| EP-1 | done (2026-11-09) | `b9bcbfd` `b110e67` `643a7a8` `e936e96` `f4aafca` `7816e1b` — membership, mTLS, member manifests, transport port |
| EP-2 | done (2026-11-09) | `9803b19` `ec0f92c` — kind harness (podman, pinned) + read-through fleet view |
| EP-3 | done (2026-11-09) | `1052fa3` `bafb4a6` `6c8b0cd` — chain-scoped gating, co-located placement, chain wiring |
| EP-4 | done (2026-11-09) | `d582ac0` `2ae1dfb` — seam payload (both shapes) + contract validation |
| EP-5 | done (2026-11-09) | `7bde6ba` `321d26b` `870a5b3` `37e1d91` — member ops, decision table, MC-loss repair |
| EP-6 | done (2026-11-09) | `2deb844` `540977e` `e23c3dd` `7bf778f` — fabric, real clientv3, client stand-in, kill test |
| EP-7 | done (2026-11-09) | this retro (appendix below) |

### Demo tasks (DM-x, added 2026-11-09 — presentation, not product)

| # | Task | State |
|---|---|---|
| DM-1 | coreshift external-etcd flags: `--external-etcd-endpoints` + three cert-file flags on `run/create cluster`; omit internal etcd when configured; KAS/openshift-apiserver/oauth-apiserver rewired to the external endpoints. Runs in the **coreshift repo** via `/coreshift` | done (2026-11-10) — coreshift `17f903c` + `2b2bde7` + `e27a3b9`; live smoke test included; state-dir mode guard |
| DM-2 | `hack/poc-demo-etcd.sh`: mint etcd cluster → write three-key material → `coreshift create cluster --external-etcd-*` (container joined to the kind network via `podman network connect`) → `oc get ns` off the fleet quorum → kill MC-A → kubectl survives → repair lands | done (2026-11-10) — `687bc2c` + race fix `710a63b`/`1206580`. **Live: kubectl gap 1092ms, 6 errors; coreshift ready 49s; repair 59s; 3 members after repair, health matches** |
| DM-3 | PoC dashboard: single-file HTML served at `/poc-dashboard` (public tier, `go:embed` pattern per `/openapi.html`), polling clusters + adapter statuses, rendering condition cards, adapter panels, and the etcd topology from status `data`; plus `hack/demo-setup.sh` | done (2026-11-09) — `6d4c0ed` + `405422e` |
| DM-4 | **hcp stand-in adapter**: deploy the nested coreshift pod (coreshift `docs/nested.md` recipe) onto the preferred MC via the transport — co-located with etcd member-0, wired to the fleet etcd via the external-etcd flags + cert Secret, cold re-home on MC loss. The miniature of the product hcp adapter (E8) | done (2026-11-10) — `c826b04` + `784dbef` + `076ecb0` |
| DM-5 | Demo v2: mint+load the coreshift image, POST one cluster on the `hosted/hcp` chain (`[etcd, hcp]`), port-forward, `oc get ns`, kill MC-0 (control plane + member-0 together) → quorum holds → control plane re-homes → kubectl resumes with data intact | done (2026-11-10) — `42a870d` + PVC-claim fix `1188b45`. **Live: quorum 113s, CP boot 176s, kill→quorum 46s, member replaced 72s, CP re-homed+ serving 272s; 52/52 namespaces, configmap survived; re-home co-located with the replacement member** |
| DM-6 | Dashboard fleet view: a fleet endpoint (service cluster + MCs from the fleet view + per-MC workloads from adapter statuses) and the map rendering — MC boxes with member dots AND control-plane pods, the kill showing both workloads moving | done (2026-11-10) — `2b42b03` + `3c41ba2`; live-verified: the map showed MC-0 lost with red workloads, then removal as member + CP re-landed on MC-3 |
| DM-7 | Interactive demo instruments: `demo-stage.sh` (prepare the stack, stay live, print the typed runbook with actual names), `demo-kill-mc.sh` (the optional MC kill convenience, both kubeconfig dirs), `demo-teardown.sh` (idempotent clean-down); shared `demo-lib.sh`; `poc-demo-hcp.sh` refactored onto the lib | done (2026-11-11) — `0994dd9` + `5527672` + `73ddcd0` + `a443884`; manual flow live-tested (stage → typed create/kill/recovery → teardown) + automated regression |

## 8. Open questions

| # | Question | Needed by |
|---|---|---|
| Q1 | Upstream E7 confirmations: supported-status of ungated `UnmanagedEtcdSpec`; the `endpoints []string` ask; Unmanaged × External (local clone predates it). 2026-11-09: the PM behind OCPSTRAT-3993 is the HyperShift PM and is on-board with upstreaming — the asks have a friendly sponsor | E7 filing |
| Q2 | DR targets — answered from PoC data (appendix): RPO ≤15m proposed (5m feasible), failover <1s observed, repair <5m proposed; confirm against OKE-class fabric | v1 planning |
| Q3 | OKE-class fabric profile — when to graduate from kind (real DRG/peering, security-list runbook, non-overlapping CIDRs) | post-PoC |
| Q4 | Defrag cadence for fleet (adopt H6 constants vs. tune for cross-MC latency) | v1 hardening |
| Q5 | Sharding (`--etcd-servers-overrides`) — out of v1; revisit if per-HCP keyspaces demand it | later |

---

## Appendix: PoC evidence (2026-11-09)

All numbers from `CLM_POC_E2E=1 ./hack/etcd-kill-test.sh` (branch `poc`, repo
`cluster-lifecycle-manager`).

### The kill test (acceptance)

| Moment | Result |
|---|---|
| Quorum formation (3 members, 3 MCs) | 55s |
| MC destroyed → write-stream gap | **499ms, 2 client errors, self-recovered** — the client's own multi-endpoint failover; no LB, no CLM, no re-pointing |
| Repair RTO (replacement voting on spare MC) | 46s |
| Member rejoin with CLM stopped | **9s — controller-free self-heal** |
| Snapshot restore | 19ms, 238 keys, empty-dir guard honored |

Both critical-path claims held live: client failover was the etcd client's own
behavior — the architecture the upstream ask proposes, with numbers behind it —
and restart-class healing worked with the controller down.

### Fabric approach (PoC stand-in)

kind on podman shares the `kind` bridge: members address via node IP + stable
NodePort (2379/2380), cert SANs covering node IPs. The production shape (fabric
DNS + per-MC Services) is unchanged; the PoC substituted routable addresses.

**Deviation (the one TLS relaxation):** NodePort SNAT hides peer source
addresses, so peer TLS skips client-SAN verification (SAN-less member client
certs). PoC-scale artifact — production fabric preserves source addresses, so
peer verification can be strict.

### Repair topology

The harness runs a spare fourth MC (`etcd-mc-3`); the replacement landed there
after `etcd-mc-0` was destroyed — the full repair path exercised, not a
degraded-only demonstration. Deferred-repair (2-member serving until a new MC
joins) remains the documented fallback narrative for fleets without a spare.

### DR targets (from data — answers Q2)

- RPO: 5-minute snapshot cadence feasible at PoC scale; **propose ≤15m for v1**
  (conservative vs. the feasible 5m, pending production-scale I/O)
- Failover: sub-second (499ms observed); **propose <1s**
- Repair RTO: 46s observed; **propose <5m**
- Degraded-mode rejoin: ~9s
- To be confirmed against OKE-class fabric, where cross-MC latencies differ

### In-flight findings

- The kernel inotify instance limit (`fs.inotify.max_user_instances`, was 128)
  was the only machine-level blocker across the whole PoC — raised to 1024,
  persisted. Ambient desktop/container load had exhausted the default budget.
- Initial bootstrap used the H4 static enumeration (peer URLs + token);
  formation in 55s — the HyperShift harvest translated directly.
- 1930 tests green at PoC completion; no secret material committed (live certs
  under gitignored `.fleet/` paths; transient client certs removed with the
  script temp directory).
- (2026-11-09, post-retro) The etcd finalize path was a live-found gap —
  deletion hung in Finalizing forever until `97d9eec` made deletion a teardown
  pass. And the "member-list timeout" flake was root-caused: **NodePort
  collisions between concurrent etcd clusters** on the shared kind bridge —
  the fabric stand-in needs per-cluster port allocation before the demo runs
  two etcd clusters at once (DM-2's clean-start handles it by deleting stale
  records first).
- (2026-11-10, DM-2) **The MC-loss repair race** — a design-relevant invariant
  for the product work: every member's `--initial-cluster` must include the
  current membership's peer URLs (a survivor that later re-bootstraps after
  pod+PVC loss validates against it), so repairs MUST re-apply survivors —
  but **roll survivors first, wait for their rollout, then start the
  replacement** (`1206580`), plus a bounded quorum-wait in the reset-member
  init container before join-vs-fresh (`710a63b`). The un-ordered version let
  the replacement bootstrap a divergent single-member etcd during the gap.
  Real-apiserver failover through the kill: 1092ms gap, 6 kubectl errors —
  the apiserver's own multi-endpoint recovery, no LB, no CLM.
- (2026-11-10, DM-5) **The hosted model, live**: MC-0 died with BOTH the
  control plane pod and etcd member-0 — quorum held on 2/3 (46s), member
  replaced on the spare (72s), control plane re-homed COLD onto the spare
  co-located with the replacement, serving again in 272s total, with 52/52
  namespaces and a created configmap intact (data lives in the fleet etcd,
  not the dead MC's PVC). Findings: bound PVCs are immutable — the hcp
  adapter's per-pass claim rewrite stalled until `1188b45` made claims
  stable; adapter readiness should mean API-serving, not pod+kubeconfig (the
  script works around it); the payload re-pull into the pod's cache emptyDir
  costs ~200s per fresh pod (a cache PV would amortize it — future work); a
  reconcile tick racing the kill can publish a transient empty error report
  (tolerated; robustness item).
