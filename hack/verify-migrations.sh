#!/usr/bin/env bash
# Verifies migration files follow project conventions.
set -euo pipefail

MIGRATION_DIR="pkg/db/migrations"

# Find the base commit to diff against:
# - In Prow: ci-operator merges the PR onto the base, so HEAD is a merge commit
#   and merge-base resolves to the base tip.
# - Locally: resolves to where the current branch diverged from origin/main,
#   which also catches uncommitted changes in the working tree.
BASE=$(git merge-base HEAD origin/main)

# Migration implementation files must not be modified, renamed, or deleted.
# migration_structs.go is excluded — it must change when registering new migrations.
# Schema changes must be additive — add a new migration file instead.
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
