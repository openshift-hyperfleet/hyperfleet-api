#!/usr/bin/env bash
# Verifies migration files follow project conventions.
#
# Migration implementation files must not be modified, renamed, or deleted
# once committed. migration_structs.go is excluded — it must change when
# registering new migrations. Schema changes must be additive: add a new
# migration file instead.
set -euo pipefail

MIGRATION_DIR="pkg/db/migrations"

# Determine the base commit to diff against.
#
# Prow's clonerefs fetches via raw URLs and does NOT add an `origin` remote,
# so `origin/main` only exists in local checkouts. Three possible contexts:
#
#   1. Prow presubmit on this repo: clonerefs runs `git merge --no-ff <pr-sha>`
#      onto the upstream base, so HEAD is a merge commit. HEAD^1 is the
#      upstream main tip the PR was merged onto; the working tree contains
#      all PR changes layered on top. `git diff HEAD^1` therefore yields the
#      cumulative diff of every commit in the PR against upstream main.
#
#   2. Prow rehearsal / extra_refs (e.g. openshift/release PR running this
#      job against hyperfleet-api at main): no PR is merged in, HEAD has a
#      single parent and there is nothing to verify.
#
#   3. Local run: diff against the local origin/main snapshot. May lag
#      behind upstream if not recently fetched — CI is the source of truth.
if git rev-parse --verify --quiet HEAD^2 >/dev/null; then
    BASE=$(git rev-parse HEAD^1)
elif git rev-parse --verify --quiet origin/main >/dev/null; then
    BASE=$(git merge-base HEAD origin/main)
else
    echo "verify-migrations: no PR detected (HEAD is not a merge commit and origin/main is not configured); skipping."
    exit 0
fi

VIOLATIONS=$(git diff --diff-filter=MRCD --name-only "${BASE}" -- \
    "${MIGRATION_DIR}/*.go" \
    ":(exclude)${MIGRATION_DIR}/migration_structs.go")

if [[ -n "${VIOLATIONS}" ]]; then
    echo "FAIL: migration immutability — these files were modified, renamed, or deleted:"
    echo "${VIOLATIONS}" | sed 's/^/  - /'
    echo
    echo "Migrations must not change after they have been applied."
    echo "Create a new migration file with the required changes instead."
    echo
    echo "If the modification is intentional, a root OWNERS approver can merge by"
    echo "commenting: /override ci/prow/verify-migrations"
    exit 1
fi

echo "Migration verification passed."
