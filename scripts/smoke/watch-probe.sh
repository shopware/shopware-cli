#!/usr/bin/env bash
# Start a watcher, wait for an HTTP answer on its URL, stop it and wait until the URL is quiet.
# Usage: watch-probe.sh NAME URL TIMEOUT_SECONDS COMMAND [ARGS...]
set -euo pipefail

if (($# < 4)); then
  echo "usage: $0 NAME URL TIMEOUT_SECONDS COMMAND [ARGS...]" >&2
  exit 2
fi
name=$1 url=$2 timeout=$3
shift 3
log=${RUNNER_TEMP:-/tmp}/$name.log

# Print the HTTP status of the URL; 000 means no HTTP answer.
status() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$url" || true
}

# Print the end of the watcher log and fail.
fail() {
  echo "::error::$name: $1"
  echo "Last 40 lines of $log:"
  tail -n 40 "$log" || true
  exit 1
}

# Succeed while any process of the watcher's process group is left.
running() {
  kill -0 -- "-$pid" 2>/dev/null
}

# Send a signal to the process group and wait up to the given seconds for it to end.
stop() {
  local signal=$1 seconds=$2 waited=0
  running || return 0
  echo "Sending SIG$signal to $name"
  kill -s "$signal" -- "-$pid" 2>/dev/null || true
  while running && ((waited < seconds)); do
    sleep 1
    waited=$((waited + 1))
  done
  if ! running; then
    echo "$name ended ${waited}s after SIG$signal"
  fi
}

echo "Starting $name: $*"
# Own session for a group stop; restore SIGINT, which background jobs ignore.
setsid env --default-signal=INT "$@" </dev/null >"$log" 2>&1 &
pid=$!
# Never leave the watcher behind when this script fails.
trap 'kill -s KILL -- "-$pid" 2>/dev/null || true' EXIT

# Only an HTTP status counts: Docker accepts TCP before the container listens.
start=$SECONDS
code=$(status)
while [[ $code == 000 ]]; do
  kill -0 "$pid" 2>/dev/null || fail "exited before $url answered"
  ((SECONDS - start < timeout)) || fail "no HTTP answer from $url within ${timeout}s"
  sleep 5
  code=$(status)
done
echo "$name answered HTTP $code on $url after $((SECONDS - start))s"

stop INT 15
stop TERM 5
stop KILL 5
if running; then
  fail "still running after SIGKILL"
fi

# A quiet URL proves the watcher in the container is gone too.
quiet=$SECONDS
while [[ $(status) != 000 ]]; do
  ((SECONDS - quiet < 30)) || fail "$url still answers 30s after the stop"
  sleep 2
done
echo "$url went quiet $((SECONDS - quiet))s after the stop"
