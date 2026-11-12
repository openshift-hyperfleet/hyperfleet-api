#!/usr/bin/env bash
# Stops demo-setup's host server so a demo script can publish its own API
# server on the same port. Fails when something else holds the port.
#
# Idempotent: no holder is a no-op; a stale demo-setup pid file is removed.
# Callers must set API_PORT before sourcing this file.

DEMO_SERVER_PID_FILE=${CLM_POC_SERVER_PID_FILE:-/tmp/clm-poc-demo-server.pid}

port_holder() {
	if command -v ss >/dev/null 2>&1; then
		ss -ltnp "sport = :$API_PORT" 2>/dev/null |
			sed -n 's/.*users:(("\([^"]*\)",pid=\([0-9]*\).*/\1 (pid \2)/p' | head -n 1
	elif command -v lsof >/dev/null 2>&1; then
		lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null |
			awk 'NR==2 {print $1 " (pid " $2 ")"}'
	elif command -v fuser >/dev/null 2>&1; then
		fuser "$API_PORT/tcp" 2>/dev/null | xargs -r -n 1 sh -c \
			'ps -o comm= -p "$0" 2>/dev/null | sed "s/$/ (pid $0)/"'
	fi
}

# stop_demo_server kills the whole process group: demo-setup starts the
# server with setsid, and the pid file holds the group leader.
stop_demo_server() {
	local pid=""
	[[ -f $DEMO_SERVER_PID_FILE ]] && pid=$(<"$DEMO_SERVER_PID_FILE")
	if [[ -z $pid ]] || ! kill -0 "$pid" 2>/dev/null; then
		rm -f "$DEMO_SERVER_PID_FILE"
		return 0
	fi
	echo "Taking over the API port from demo-setup's server"
	kill -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
	rm -f "$DEMO_SERVER_PID_FILE"
}

# takeover_api_port stops the demo-setup server, then waits for the port to be
# released. Anything still holding it is reported by name.
takeover_api_port() {
	stop_demo_server
	for _ in $(seq 1 50); do
		local holder
		holder=$(port_holder)
		[[ -z $holder ]] && return 0
		sleep 0.2
	done
	holder=$(port_holder)
	[[ -z $holder ]] && return 0
	echo "error: port $API_PORT is held by $holder, not demo-setup's server" >&2
	echo "       stop that process and rerun this script" >&2
	return 1
}
