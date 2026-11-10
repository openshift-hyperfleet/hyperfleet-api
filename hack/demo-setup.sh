#!/usr/bin/env bash
# One-command PoC demo bring-up: database, API server, kind fleet, dashboard.
# Usage: hack/demo-setup.sh
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO_ROOT"

BASE_URL=${CLM_POC_BASE_URL:-http://localhost:8000}
API="${BASE_URL}/api/hyperfleet/v1"
DASHBOARD="${API}/poc-dashboard"
SERVER_LOG=${CLM_POC_SERVER_LOG:-/tmp/clm-poc-demo-server.log}
SERVER_PID_FILE=${CLM_POC_SERVER_PID_FILE:-/tmp/clm-poc-demo-server.pid}

missing=0
for bin in go podman curl jq; do
	if ! command -v "$bin" >/dev/null 2>&1; then
		echo "error: '$bin' is required for the demo but was not found on PATH" >&2
		missing=1
	fi
done
(( missing == 0 )) || exit 1

if [[ ! -f pkg/api/openapi/openapi.gen.go ]]; then
	echo "Generated code is missing; running make generate-all..."
	make generate-all
fi

if podman container exists psql-hyperfleet; then
	echo "Database container 'psql-hyperfleet' already exists; reusing it."
	podman start psql-hyperfleet >/dev/null 2>&1 || true
else
	echo "Starting the local PostgreSQL container..."
	make db/setup
fi

server_pid=""
if curl -fsS --max-time 2 -o /dev/null "${API}/clusters?size=1"; then
	echo "API server already running at ${BASE_URL}; reusing it."
else
	echo "Starting the API server (log: ${SERVER_LOG})..."
	setsid make run-no-auth >"${SERVER_LOG}" 2>&1 &
	server_pid=$!
	echo "$server_pid" >"${SERVER_PID_FILE}"

	for _ in $(seq 1 180); do
		if curl -fsS --max-time 2 -o /dev/null "${API}/clusters?size=1"; then
			break
		fi
		if ! kill -0 "$server_pid" 2>/dev/null; then
			echo "error: the API server exited during startup; last log lines:" >&2
			tail -n 20 "${SERVER_LOG}" >&2
			exit 1
		fi
		sleep 1
	done
	if ! curl -fsS --max-time 2 -o /dev/null "${API}/clusters?size=1"; then
		echo "error: the API server did not become ready within 180s; last log lines:" >&2
		tail -n 20 "${SERVER_LOG}" >&2
		exit 1
	fi
	echo "API server listening at ${BASE_URL}"
fi

echo "Bringing up the kind fleet..."
hack/fleet.sh up

echo
echo "PoC dashboard: ${DASHBOARD}"
echo "Server log:    ${SERVER_LOG}"
echo
echo "Teardown:"
if [[ -n $server_pid ]]; then
	echo "  kill -- -${server_pid}          # stop the API server"
else
	echo "  stop the already-running API server"
fi
echo "  hack/fleet.sh down              # delete the kind clusters"
echo "  podman stop psql-hyperfleet     # optional: stop the database"
echo

if command -v xdg-open >/dev/null 2>&1; then
	xdg-open "$DASHBOARD" >/dev/null 2>&1 || true
else
	echo "Open ${DASHBOARD} in a browser."
fi
