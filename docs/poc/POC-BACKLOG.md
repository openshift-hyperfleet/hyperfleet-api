# CLM PoC Backlog — coreshift vertical slice

Status: active
Revision: 2 (2026-11-05) — re-sliced to one task per agent invocation, atomic commits
Related: [PROPOSAL.md](PROPOSAL.md) · [STORIES.md](STORIES.md)
Repo: `cluster-lifecycle-manager/` — clone of `hyperfleet-api` @ `f9e159a` (branch `poc`)

---

## The PoC question

> Can I create a cluster object in the store that a coreshift adapter can use to
> create a coreshift control plane?

Acceptance, end to end:

```
POST /clusters (topology=hosted, platform=coreshift)   → 201, generation 1, stored
  → orchestrator (in server) detects drift             → runs chain [coreshift]
  → coreshift adapter: real control plane via CLI      → status upsert
  → GET /clusters/{id}                                 → Reconciled=True
  → oc get ns against minted kubeconfig                → works
  → DELETE                                             → 202 soft-delete
  → adapter: coreshift delete cluster                  → Finalized=True
  → hard delete                                        → 404
```

## Working model

One task per `.opencode` agent invocation. The preferred form passes the task's
full text in the invocation (the orchestrator parses the section out of this
file; a user may paste it after `/clm`). A bare ID (`/clm PC-x`) also works —
the agent extracts exactly that task's section from this file and treats the
rest as out of scope. Tasks are self-contained: everything needed is in the
task text. Each invocation lands small atomic commits — subject + ≤3 plain
sentences, readable by a sleep-deprived reviewer. Never push. Blocked tasks
stop and report instead of improvising scope.

## Findings that shaped the scope (research, 2026-11-05)

1. **The v2.5 API already has the store-side slice.** Cluster/NodePool CRUD
   (registry-driven, config-defined kinds), spec blob validated against an
   OpenAPI component, `generation`, UUIDv7, `PUT /clusters/{id}/statuses`
   adapter upsert with stale-generation discard, full aggregation to
   `Reconciled`/`LastKnownReconciled` (`pkg/services/aggregation.go`),
   soft-delete 202 gated on `required_adapters` reporting `Finalized=True`,
   force-delete, TSL search incl. `observed_generation` comparisons.
   The PoC is "wire a loop and one adapter on top of a working store," not
   "build a store."
2. **Two coreshift repos exist.** The PoC pins the **rewrite**
   (`~/Projects/vkareh/coreshift`, active, kind-style grammar
   `create/get/delete cluster`, podman-based, verified 4.22 line). The original
   (`openshift-hyperfleet/coreshift`, paused Aug 2026, `up`/`status`/`down`)
   is what E12-S1 currently references — stale; PC-11 fixes it.
3. **Sentinel harvest is concrete.** Port the 4-way drift predicate (below,
   PC-6a) as plain Go — no CEL. Critical contract: adapters must upsert a
   status row on every pass or the debounce never advances.
4. **The repo already ships dev conveniences**: `make db/setup` (local
   PostgreSQL container) and `make run-no-auth` (server without auth), per its
   AGENTS.md. PC-1/PC-2 verify these work before writing anything new.

## Conventions

- **Packaging**: orchestrator + adapter compiled into the server binary
  (`cmd/hyperfleet-api`), started alongside HTTP serving. The drift loop
  queries the store via the dao layer — no HTTP self-calls, no auth needed for
  the loop. In-process adapters report status through the service layer behind
  a `StatusReporter` interface, so an external REST-speaking adapter remains a
  drop-in (the REST status plane is verified via curl in PC-8).
- **Axes for PoC**: `topology`/`platform` live in the spec blob
  (`spec.topology`, `spec.platform`); chain selection reads them. First-class
  API fields are E1-S5 work, not PoC.
- **Taxonomy (pinned 2026-11-05)**: `topology` = where the control plane
  runs — `standalone` on the cluster's own machines, `hosted` as workloads on
  a substrate the cluster doesn't own. `platform` = the substrate + machinery.
  `hosted` does NOT imply HyperShift: `hosted/coreshift` is a control plane in
  local podman containers, and its chain `[coreshift]` encodes the absence of
  placement/transport/hcp-core because placement trivially resolves to the
  podman substrate itself. `hosted/oci` is HCP-on-MC; E8–E10 build that
  MC-substrate machinery, not "hosted" as such.
- **Module path unchanged** — repo rename is productization-time (decision #29).
- **Commits**: pure ape style, no task prefix. The status table below holds
  the task ↔ commit mapping.

## Prerequisites (confirmed available)

- podman (coreshift container target; postgres container)
- Red Hat pull-secret JSON — path via env/config, **gitignored, never
  committed, never in logs or status payloads**
- Go 1.26 + repo toolchain (`make generate-all` first — generated code is
  gitignored)
- coreshift binary from the rewrite repo on PATH or configured path

---

## Tasks

### PC-0 — Promote-by-clone + green baseline ✅ DONE
**Maps to:** E1 (promotion mechanics, simulated)

Docs relocated to parent (`CLM-*`), `hyperfleet-api` cloned into
`cluster-lifecycle-manager/` as repo root (history from `f9e159a`), branch
`poc`, `make generate-all` + `make build` + `make test` green (1804 tests).

---

### PC-1 — Local run path: postgres + server + CRUD smoke
**Maps to:** — (existing plumbing, verified)

The repo ships `make db/setup` (local PostgreSQL container) and
`make run-no-auth` (server without auth) per its AGENTS.md. Verify both
actually work on this machine; fix what is broken (small, boring fixes only).

**Done when:**
- [ ] PostgreSQL container running via the repo's own target
- [ ] Server starts via the repo's own no-auth target
- [ ] curl smoke passes: POST a cluster (any spec), GET it, PATCH labels,
      DELETE → 202, force-delete → gone, list with `?search=`
- [ ] Whatever needed fixing is committed; nothing else changed

---

### PC-2 — Dev auth documented
**Maps to:** E2-S4 adjacent (minimal)

`make run-no-auth` covers the PoC's needs. Document the story for scripts and
humans: how no-auth mode works, what tenancy defaults to without a JWT (which
tenant do created clusters land in?), and the exact curl incantation including
any required headers.

**Done when:**
- [ ] No-auth behavior verified: curl CRUD works, tenancy default understood
      and written down
- [ ] Notes land in the PoC notes appendix of this file (not the repo docs —
      this is PoC evidence, not product documentation)

---

### PC-3 — Cluster entity: `required_adapters: [coreshift]`
**Maps to:** E5-S2 lite (the POC `required_adapters` precedent, confirmed live)

Entity descriptors in `configs/dev.yaml` declare kinds; the aggregation layer
(`pkg/services/aggregation.go`) computes `Reconciled` only when all
`required_adapters` have reported at the current generation with
`Available=True`, and the soft-delete → hard-delete path waits for
`Finalized=True` from all of them. This is config-only: teach the dev config
that `coreshift` is a required adapter for clusters.

**Done when:**
- [ ] `configs/dev.yaml` Cluster entity carries `required_adapters:
      [coreshift]`
- [ ] curl: PUT a fake coreshift adapter status (Available/Applied/Health
      True, `observed_generation` = current generation) → per-adapter
      condition appears, cluster computes `Reconciled`
- [ ] curl: DELETE cluster → 202; PUT Finalized=True status → cluster hard
      deletes
- [ ] No Go code changed (if code changes turn out to be required, stop and
      report — that is a finding, not this task)

---

### PC-4 — Chain selection: (hosted, coreshift) → [coreshift]
**Maps to:** E5-S2, E1-S5 lite

Chains are data, not code: a config-declared map from
`(spec.topology, spec.platform)` to an ordered adapter list, validated at
startup (unknown adapter name fails fast with a clear error). Declare exactly
one chain: `hosted/coreshift → [coreshift]`. Selection reads the axes from the
spec blob. No executor yet — this task is the config schema, the lookup, and
the validation.

**Done when:**
- [ ] Chain config schema in the server config; startup validation
- [ ] Lookup by topology×platform returns the declared chain; unknown combo
      is a clear "no chain" result, not a crash
- [ ] Unit tests: lookup, validation failure, unknown-adapter rejection
- [ ] Implementation and tests land as separate commits

---

### PC-5a — Adapter SPI + StatusReporter + no-op adapter
**Maps to:** E5-S1

Adapters are compiled Go controllers behind a narrow SPI. PoC packaging is
in-process inside the server binary. The SPI: a `Controller` interface whose
reconcile pass takes a cluster view and returns a status report (conditions
Available/Applied/Health/Finalized + free-form data + `observed_generation`);
idempotent, status-first. The `StatusReporter` interface is the seam adapters
report through — the PoC implementation calls the service layer directly; an
external REST-speaking implementation drops in later without adapter changes.
Prove the shape with a no-op adapter that reports healthy without doing
anything.

**Done when:**
- [ ] `Controller` SPI + `StatusReporter` interface + service-layer
      implementation
- [ ] No-op adapter passes through the SPI and reports via the reporter
- [ ] Unit tests for the reporter path
- [ ] Implementation and tests land as separate commits

---

### PC-5b — Registry + chain executor
**Maps to:** E5-S1, E5-S5 lite

The registry maps adapter names to factories (names come from chain config).
The executor runs an ordered chain against one cluster: invoke each adapter's
reconcile pass in order, report each through the `StatusReporter`, isolate
per-adapter failures (a failing adapter stops the chain and surfaces its
condition; later adapters do not run), and make every pass retry-safe.
Preconditions are trivially "ordered" for the PoC — the hook exists but does
nothing yet.

**Done when:**
- [ ] Registry resolves names → factories; unknown name fails fast
- [ ] Executor runs a two-fake-adapter chain in order, reports both
- [ ] Failing first fake blocks the second; chain is retry-safe (re-run
      produces the same reports)
- [ ] Implementation and tests land as separate commits

---

### PC-6a — Drift predicate + ported test table
**Maps to:** E5-S4

Port the sentinel's drift semantics as one plain Go function — no CEL, no
config-driven decision engine. The four rules (semantics carried verbatim from
`hyperfleet-sentinel` `DefaultMessageDecision`):

1. **new resource** — `generation == 1` and no `Reconciled` condition →
   reconcile immediately (no debounce)
2. **generation mismatch** — `generation > Reconciled.observed_generation` →
   reconcile
3. **unreconciled + debounced** — `Reconciled == False` and
   `now - last_updated_time > 10s` → reconcile
4. **reconciled + stale** — `Reconciled == True` and
   `now - last_updated_time > 30m` → reconcile (periodic health check)

Age is measured from `last_updated_time` (updated on every adapter report),
never `last_transition_time` (only flips on status change). Sentinel's test
scenarios live at
`~/Projects/openshift-hyperfleet/hyperfleet-sentinel/docs/decision-engine.md`
(Tests 1–10) — port that table as the unit test matrix.

**Done when:**
- [ ] `NeedsReconcile(resource, now, debounce, maxAge)` (or equivalent) with
      the four rules above
- [ ] The 10-scenario table ported and green
- [ ] No CEL import anywhere in the new code
- [ ] Implementation and tests land as separate commits

---

### PC-6b — Orchestrator goroutine in the server
**Maps to:** E5-S4, E5-S5 lite

Wire the loop into `cmd/hyperfleet-api`: a goroutine started alongside HTTP
serving, config-gated (enabled flag, poll interval, default 5s), cleanly shut
down on context cancel. Each tick: list clusters through the dao layer (no
HTTP self-calls), apply the drift predicate, look up the chain for each
drifted cluster (PC-4), run it through the executor (PC-5b). Per-cluster
errors are logged and isolated — one bad cluster never stops the tick. The
always-upsert contract holds: every pass writes a status row or the debounce
never advances.

**Done when:**
- [ ] Loop runs in-server behind a config flag; clean start/stop
- [ ] With the no-op adapter registered and chain selected, creating a
      cluster via the API produces a coreshift status row within one tick
      (integration test or live verification, agent's choice)
- [ ] A failing cluster does not block other clusters in the same tick
- [ ] Implementation and tests land as separate commits

---

### PC-7a — Coreshift adapter: create + readiness + kubeconfig
**Maps to:** E12-S1

The real adapter, implementing the SPI against the coreshift rewrite CLI
(`~/Projects/vkareh/coreshift`; binary on PATH or configured path). On
reconcile of a `hosted/coreshift` cluster: check for an existing control plane
(`coreshift get clusters`); if absent, mint one
(`coreshift create cluster --pull-secret <path> --name <cluster.name>`,
port allocation if more than one); poll readiness; fetch the kubeconfig
(`coreshift get kubeconfig --name <cluster.name>`); report
Available/Applied/Health with the kubeconfig path in status `data` and
`observed_generation` = cluster generation. Config: binary path, pull-secret
path (env — **never** in the repo, logs, or payloads), state dir.

**Done when:**
- [ ] A `hosted/coreshift` cluster reaches `Reconciled` with a real minted
      control plane (live verification with podman; record timing)
- [ ] Kubeconfig path lands in status `data`; `oc get ns` works against it
- [ ] CLI invocation wrapped behind a runner interface, unit-tested with a
      fake runner (no podman in unit tests)
- [ ] Implementation and tests land as separate commits

---

### PC-7b — Coreshift adapter: delete + idempotency
**Maps to:** E12-S1, E1-S7 (Finalized gating, exercised)

On reconcile of a soft-deleted (Finalizing) cluster: tear the control plane
down (`coreshift delete cluster --name <cluster.name>`) and report
`Finalized=True` only when it is gone — the existing hard-delete machinery
does the rest. Reconcile against an existing healthy control plane is a
status refresh, not a re-mint.

**Done when:**
- [ ] DELETE cluster → 202 → adapter tears down → Finalized=True → hard
      delete (404 on GET)
- [ ] Re-running reconcile on an existing control plane refreshes status only
- [ ] Fake-runner unit tests for both paths
- [ ] Implementation and tests land as separate commits

---

### PC-8 — Status plane evidence (REST contract)
**Maps to:** E5-S3 (mostly pre-existing; verify)

Prove the external adapter surface works as the contract, from outside the
process, with curl: PUT an adapter status at the current generation →
aggregation updates; PUT at a stale `observed_generation` → silently
discarded (204). Confirm the in-process `StatusReporter` path and the REST
path produce identical aggregation results.

**Done when:**
- [ ] External PUT drives aggregation; stale-generation PUT discarded
- [ ] Evidence (commands + observed responses) recorded in this file's
      appendix

---

### PC-9 — Demo script + timing
**Maps to:** E12-S2 lite

One script proving the acceptance flow: create → poll Reconciled → `oc get ns`
with the minted kubeconfig → DELETE → poll gone → exit code. Env-gated
(`CLM_POC_E2E=1`, podman + pull secret present) so it never runs in default
test paths. Record cold vs warm cache timing (coreshift cold readiness budget
is 30 min; the image cache makes subsequent runs fast).

**Done when:**
- [x] `hack/poc-demo.sh` passes against a local server + podman
- [x] Cold/warm timings recorded in this file's appendix

---

### PC-11 — Retrospective + backlog corrections
**Maps to:** feeds E-story revisions

- Fix the stale coreshift reference in `STORIES.md` E12-S1:
  point at the rewrite repo (`create/get/delete` grammar, active), not the
  paused original
- Record "carry confirmed" evidence for E1-S4/S6/S7, E5-S3 (status plane) and
  the `required_adapters` precedent — v2.5 already ships them; the stories
  say "carry"; the PoC proves it
- Note drifts from the story shape and why: axes in the spec blob,
  `StatusReporter` seam, goroutine packaging
- Append the PoC retro to this file

**Done when:**
- [x] Stories doc corrected; retro appended below

---

## Out of scope (deliberate)

TypeSpec fold (use existing OpenAPI/schema validation) · facade store ·
proto generator · tenancy de-ROSA (no-auth dev mode only) · ManifestWork/MSA
transport (coreshift is local — no hub, no MC) · placement · etcd manager ·
pullsecret adapter (coreshift takes the secret as a flag) · pre/post Jobs ·
digest pinning · CI layering.

## Status

| Task | State | Commits |
|---|---|---|
| PC-0 | done (2026-11-05) | clone @ `f9e159a`, branch `poc`, 1804 tests green |
| PC-1 | done (2026-11-05) | `ae82fb2` — CRUD smoke green: create 201, get/patch 200, delete 202, force-delete 204, get 404, search works |
| PC-2 | done (2026-11-05) | docs only — auth/tenancy evidence appended below |
| PC-3 | done (2026-11-05) | `71e0cce` — config-only: fake status PUT → `CoreshiftSuccessful` + `Reconciled`; DELETE 202 → `Finalized=True` → hard delete |
| PC-4 | done (2026-11-05) | `4f4a0fb` + `aced884` — `pkg/chains`: config-declared chains, startup validation vs required_adapters, no-chain result for unknown combos |
| PC-5a | done (2026-11-05) | `b7b51ee` + `d4a5a7f` — `pkg/adapters`: Controller SPI, StatusReporter through `ProcessAdapterStatus`, no-op adapter |
| PC-5b | done (2026-11-05) | `5c5d8c9` + `cb8a92b` — registry + executor: ordered, per-adapter failure isolation, retry-safe |
| lint toolchain (off-backlog) | done (2026-11-05) | `8e1bf9f` + `1240807` + `dc5bdd3` — Go 1.27 export-data + staticcheck pin bumps, pkg/chains findings fixed; `make lint` 0 issues |
| PC-6a | done (2026-11-05) | `5f21130` + `4d082ae` — `pkg/orchestrator/drift.go`: 4-rule `NeedsReconcile`, no CEL; 8 of 10 sentinel scenarios ported (2 skipped: CEL-validation-only, N/A by design) |
| PC-6b | done (2026-11-05) | `ce65f7e` + `ff09733` — in-server loop: dao poll → drift → chain → executor, closer shutdown, error isolation; integration test proves create-cluster → status row in one tick |
| PC-7a | done (2026-11-05) | `05021ba` + `8f8deed` + `d20fb27` — coreshift adapter: runner interface, background mint, tick polling, kubeconfig in status `data`. **Live: create → ready → Reconciled in ~94s; `oc get ns` green** |
| PC-7b | done (2026-11-05) | `b02cbfd` + `636c4f1` — delete path: Finalizing-only teardown, Finalized after confirmed gone, idempotent refresh. **Live: DELETE 202 → 404 in 24s; listing already surfaced Finalizing clusters** |
| PC-8 | done (2026-11-05) | verification only — fresh PUT 201 → `Reconciled`; stale PUT 204, aggregation unchanged; reporter and REST aggregation identical |
| PC-9 | done (2026-11-05) | `hack/poc-demo.sh` — live create 201 → Reconciled in 176s cold / 91s warm, `oc get ns` green, DELETE 202 → 404 in 25s / 20s; evidence below |
| PC-11 | done (2026-11-05) | docs only — E12-S1 corrected to the rewrite coreshift (grammar + status); carry evidence and drift notes inline; retro appended below |

## Appendix: PoC notes

(verification evidence, timings, auth notes — appended as tasks complete)

### PC-2 — Dev auth documented

Verified against `make run-no-auth` on 2026-11-05 (build `v0.4.1-68-gae82fb2`).

How no-auth mode works:

- `make run-no-auth` first runs `db/migrate` (builds the binary, applies
  migrations), copies `configs/config.yaml.example` to `configs/config.yaml`
  if no local config exists, then starts the server with
  `HYPERFLEET_SERVER_JWT_ENABLED=false` and
  `HYPERFLEET_SERVER_TLS_ENABLED=false`.
- With JWT disabled, no auth middleware is mounted. No `Authorization` header
  is needed, and no token is validated.
- Tenant enforcement is off by default (`server.tenant.enabled: false` in
  `configs/config.yaml.example`), so no tenant headers are needed either.
- With no token and no tenant context, the audit user falls back to
  `system@hyperfleet.local`.
- Created clusters land in the empty tenancy: `"tenancy": {}`. That is an
  unscoped resource; no tenant-scoped query can match it. It is not placed in
  any named tenant.

Exact curl incantation (only required header is `Content-Type` on writes):

```bash
API=http://localhost:8000/api/hyperfleet/v1

# create
curl -sS -X POST "$API/clusters" \
  -H 'Content-Type: application/json' \
  -d '{"kind":"Cluster","name":"pc2-noauth","labels":{"env":"poc"},"spec":{}}'

# read (substitute the returned id)
curl -sS "$API/clusters/$ID"

# patch labels
curl -sS -X PATCH "$API/clusters/$ID" \
  -H 'Content-Type: application/json' \
  -d '{"labels":{"env":"poc-updated"}}'

# request deletion, then force-delete the finalizer
curl -sS -X DELETE "$API/clusters/$ID"
curl -sS -X POST "$API/clusters/$ID/force-delete" \
  -H 'Content-Type: application/json' \
  -d '{"reason":"poc cleanup"}'
```

Observed results: create 201, get 200, patch 200, delete 202, force-delete
204, get-after-force 404. Create response included `"tenancy":{}` and
`"created_by":"system@hyperfleet.local"`; no auth or tenant headers sent.

### PC-8 — Status plane: external PUT and in-process reporter agree

Verified against a local no-auth server on 2026-11-05 (build
`v0.4.1-85-gd20fb27-dirty`). No coreshift or podman needed: a temporary
config (outside the repo) set the Cluster `required_adapters` to `[noop]` and
the `hosted/coreshift` chain to `[noop]`. The orchestrator was toggled on only
for the in-process reporter half of the check.

Commands below use `API=http://localhost:8000/api/hyperfleet/v1`.

#### Current-generation PUT drives aggregation

```bash
# create at generation 1, then bump to generation 2 with a label patch
curl -sS -X POST "$API/clusters" -H 'Content-Type: application/json' \
  -d '{"kind":"Cluster","name":"pc8-rest","spec":{"topology":"hosted","platform":"coreshift"}}'
curl -sS -X PATCH "$API/clusters/$ID" -H 'Content-Type: application/json' \
  -d '{"labels":{"pc8":"gen2"}}'

# PUT a status for the current generation
curl -sS -D - -X PUT "$API/clusters/$ID/statuses" \
  -H 'Content-Type: application/json' --data-binary @fresh.json
```

`fresh.json` was `noop` with `observed_generation: 2`, a current
`observed_time`, and Available/Applied/Health/Finalized all `True`.

Observed:

```text
PATCH labels -> HTTP 200; generation 2
PUT statuses -> HTTP/1.1 201 Created
GET cluster:
  generation 2
  Reconciled           True  observed_generation 2  reason ReconciledAll
  LastKnownReconciled  True  observed_generation 2  reason AllAdaptersReconciled
  NoopSuccessful       True  observed_generation 2  reason NoOp
```

#### Stale-generation PUT is discarded

Same cluster, then a PUT with `observed_generation: 1`, a newer
`observed_time`, and deliberately failing conditions:

```bash
curl -sS -D - -X PUT "$API/clusters/$ID/statuses" \
  -H 'Content-Type: application/json' --data-binary @stale.json
```

Observed:

```text
HTTP/1.1 204 No Content
response bytes: 0
GET cluster conditions before/after: diff empty
GET adapter status before/after: diff empty
adapter row:
  adapter noop
  observed_generation 2
  conditions Available/Applied/Health/Finalized True, reason NoOp
```

The stale report never reached the table; the existing generation-2 row and
its aggregated conditions were unchanged.

#### In-process reporter and REST produce identical aggregation

Phase 1: orchestrator on, chain `[noop]`. Create a cluster and let the in-server
loop report through `adapters.StatusReporter`, then capture the stored adapter
row:

```bash
curl -sS "$API/clusters/$ID/statuses" | jq '.items[0]'
```

Phase 2: restart with the orchestrator off, create a second cluster, and PUT
that captured row back as an `AdapterStatusCreateRequest` (same adapter,
`observed_generation`, `observed_time`, conditions, and data):

```bash
jq '.items[0] | {adapter, observed_generation,
    observed_time: .last_report_time,
    conditions: [.conditions[] | {type,status,reason,message}], data}' \
  reporter-statuses.json > reporter-payload.json

curl -sS -X PUT "$API/clusters/$ID/statuses" \
  -H 'Content-Type: application/json' --data-binary @reporter-payload.json
```

Compared with timestamps dropped, both paths produced byte-identical
aggregation:

```bash
jq -S '[.status.conditions[] |
  {type,status,observed_generation,reason,message}]' cluster.json
diff -u reporter-cluster.norm.json rest-cluster.norm.json
```

```text
REST PUT -> HTTP/1.1 201 Created
aggregation identical
adapter status payload identical

Reconciled           True  observed_generation 1  reason ReconciledAll
LastKnownReconciled  True  observed_generation 1  reason AllAdaptersReconciled
NoopSuccessful       True  observed_generation 1  reason NoOp
```

No status-plane bug surfaced. The external REST contract and the in-process
`StatusReporter` therefore converge on the same persisted status and the same
cluster conditions.

### PC-9 — Demo script + timing

`hack/poc-demo.sh` runs the acceptance flow against an already-running local
no-auth server and podman. It is gated by `CLM_POC_E2E=1`, exits with guidance
when the server, podman, or pull secret is missing, and never prints the
pull-secret path (read from `HYPERFLEET_CORESHIFT_PULL_SECRET_FILE`).

```bash
export CLM_POC_E2E=1
export HYPERFLEET_CORESHIFT_PULL_SECRET_FILE=<adapter path>
./hack/poc-demo.sh
```

Timing on 2026-11-05, build `v0.4.1-87-g636c4f1`:

| Run | Cache | create → Reconciled | DELETE → 404 | Total |
|---|---|---|---|---|
| Cold | cleared with `coreshift cleanup --all --cache` | 176s | 25s | 3m21s |
| Warm | coreshift image cache present | 91s | 20s | 1m50s |

Cold readiness stayed well inside the 30 min budget. The warm run reused the
image cache and was roughly twice as fast.

Observed transitions in both runs:

```text
POST /clusters            HTTP 201, generation 1
GET  /clusters/{id}       HTTP 200, Reconciled=False (ReconciledMissingAdapters)
GET  /clusters/{id}       HTTP 200, Reconciled=True  (ReconciledAll)
GET  /clusters/{id}/statuses  HTTP 200, coreshift data.kubeconfig present
oc get ns                 51 namespaces, exit 0
DELETE /clusters/{id}     HTTP 202
GET  /clusters/{id}       HTTP 200, Finalizing, Reconciled=False
GET  /clusters/{id}       HTTP 404
```

The script is not wired into `make test`, so default test paths skip it; an
unset `CLM_POC_E2E` exits immediately with the export hint.

### PC-11 — PoC retrospective

The PoC answered its question: a cluster created in the v2.5 store reached
`Reconciled` through a coreshift adapter that minted a real OpenShift control
plane, and deleting it returned 404 after confirmed teardown. The store-side
slice the stories propose to carry is real, not aspirational; the new work is
the orchestrator seam above it.

#### Carry confirmed

| Story | Evidence from the PoC |
|---|---|
| E1-S4 resource model | Cluster shape carried unchanged: PC-0 (1804 tests), PC-1 (create 201, get/patch 200, delete 202/404, force-delete 204, search), PC-3 (config-defined entity) |
| E1-S6 overlay packaging + runtime validation | Registry-driven kinds and runtime spec validation already ship: PC-1 created against validated specs, PC-3 added a kind through config only. Overlay *packaging* remains new work |
| E1-S7 status + deletion + errors | PC-3 fake PUT → `CoreshiftSuccessful` + `Reconciled`, DELETE 202 → `Finalized=True` → hard delete; PC-8 fresh PUT 201, stale PUT 204 with aggregation unchanged; PC-9 live DELETE 202 → 404 in 20–25 s |
| E5-S3 status plane | PC-8: stale-generation PUT discarded (204), `LastKnownReconciled` sticky, in-process reporter and external REST produce identical aggregation |
| `required_adapters` precedent (E5-S2) | PC-3/PC-4: `required_adapters: [coreshift]` scopes status gating; startup chain validation checks every chain adapter against it |

#### Drifts from the story shape

- **Axes in the spec blob.** The PoC selects chains from `spec.topology` /
  `spec.platform`; E1-S5's first-class typed fields are the product shape.
  Blob-first kept the PoC to config and avoided a spec/TypeSpec change.
- **`StatusReporter` seam.** In-process adapters report through an interface
  over the v2.5 service layer; external REST adapters use the same upsert
  endpoint. PC-8 proved both paths converge, so the seam adds no second status
  contract.
- **Goroutine packaging.** The drift loop runs inside the API server binary
  (PC-6b), not as a separate sentinel service. One process, dao access without
  HTTP self-calls or auth; E5-S4/E5-S5 still own the product deployment shape.

#### Session findings

- **Lint toolchain.** The repo's pinned golangci-lint toolchain (x/tools +
  staticcheck) was silently incompatible with the local Go 1.27 toolchain:
  `make lint` failed repo-wide on any branch with bogus typecheck errors,
  while CI stayed green because it matched the pinned container. Fixed on
  `poc` in `8e1bf9f` + `1240807` (plus `dc5bdd3`); upstream should pick
  equivalent bumps.
- **Chain validation.** PC-4 validated chain adapter names against the entity
  `required_adapters`, keeping one source of truth for valid adapter names.
  Carry that into E5-S2 rather than duplicating the list in chain config.
- **Sentinel test table.** 8 of 10 scenarios ported as plain-Go equivalents
  (PC-6a); the 2 skipped tests exercise CEL startup validation, machinery this
  design deletes. That is E5-S4's "CEL dies" made concrete.
- **Deployment shape.** The single-process trade analysis (what bundling lost
  across the v2.5 topology, and how roles/ports bring it back for the
  laptop → hub → fleet progression) is now
  [ADR 0033](adr-0033-deployment-shape.md).

#### Loose ends

- The demo is not wired into `make test`; `hack/poc-demo.sh` is opt-in via
  `CLM_POC_E2E=1` (PC-9).
- Follow-on: the etcd capability PoC (decision #35) ran in the same repo and
  branch after this backlog closed — EP-1..EP-7, kill test passed 2026-11-09.
  See [ETCD-SERVICE-DESIGN.md](ETCD-SERVICE-DESIGN.md) (rev 2,
  appendix).
- Timing is local-podman timing: 176 s cold / 91 s warm to `Reconciled`, well
  inside the 30 min budget (PC-9 appendix).
