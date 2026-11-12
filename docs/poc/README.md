# CLM PoC

This branch is the Cluster Lifecycle Manager proof of concept. It answers two
questions, both demonstrated live.

## Can a cluster object drive a real control plane?

Yes. A cluster object in the store drives a coreshift adapter that mints a real
OpenShift control plane. Create reaches `Reconciled` in about 90 seconds and the
full lifecycle works end to end.

## Can an in-core etcd adapter survive management cluster loss?

Yes. The in-core etcd adapter mints a co-located plus fleet-spread quorum that
survives management cluster loss, with CLM nowhere in the data or client paths.
Measured results:

- 499 ms client-stand-in failover gap
- 1092 ms real-apiserver gap through the kill
- 9 s controller-free member rejoin with CLM stopped
- 146 s full recovery: the management cluster hosting the control plane and
  member-0 were destroyed, quorum held, the control plane was re-homed
  co-located, and serving resumed with 52/52 namespaces and the created
  configmap intact
- 19 ms snapshot restore

## Demo runbook

```bash
hack/demo-setup.sh
hack/demo-populate.sh
hack/demo-kill-mc.sh mc-0
hack/demo-teardown.sh
```

`demo-populate.sh` fills the fleet map and prints the controls card.
`demo-kill-mc.sh mc-0` is the on-stage kill.

The hcp demo additionally needs `CORESHIFT_BIN` and
`HYPERFLEET_CORESHIFT_PULL_SECRET_FILE`.

## Prerequisites

- `go` 1.26+
- `podman` (`kind` is pinned in `tools/go.mod` and never installed separately)

The etcd-only demo is fully reproducible: it uses a public etcd image and needs
no pull secret.

The hcp demo additionally needs coreshift, a separate and currently unpublished
project, plus a Red Hat pull secret.

## Documents

- [PROPOSAL.md](PROPOSAL.md)
- [EPICS.md](EPICS.md)
- [STORIES.md](STORIES.md)
- [POC-BACKLOG.md](POC-BACKLOG.md)
- [ETCD-SERVICE-DESIGN.md](ETCD-SERVICE-DESIGN.md)
- [adr-0031-day-0-bootstrap-shape.md](adr-0031-day-0-bootstrap-shape.md)
- [adr-0033-deployment-shape.md](adr-0033-deployment-shape.md)

## Notes

Provenance: some references inside these documents point to internal or local
research material, such as `~/Projects/...` paths and internal repositories.
They are kept deliberately as research provenance, not as dead links to fix.

ADR numbering: ADRs 0001-0030 are preserved in the archived HyperFleet
repositories, and numbering continues here. ADR 0032 is reserved for the
generations retrospective, the promotion epic's first story.
