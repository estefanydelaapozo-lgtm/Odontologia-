#!/bin/sh
set -eu

PUBLIC_PORT="${PORT:-10000}"
BACKEND_PORT="${BACKEND_PORT:-3000}"

export PORT="$BACKEND_PORT"
export CLINICAL_STORAGE_DIR="${CLINICAL_STORAGE_DIR:-/app/var/clinical}"
mkdir -p "$CLINICAL_STORAGE_DIR"

/app/sigo-dental &
backend_pid=$!

cleanup() {
  kill "$backend_pid" 2>/dev/null || true
  wait "$backend_pid" 2>/dev/null || true
}
trap cleanup INT TERM EXIT

# nginx's official image expands ${PORT} and ${BACKEND_PORT} in /etc/nginx/templates.
export PORT="$PUBLIC_PORT"
export BACKEND_PORT
nginx -g 'daemon off;' &
nginx_pid=$!

while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$nginx_pid" 2>/dev/null; do
  sleep 2
done

if ! kill -0 "$backend_pid" 2>/dev/null; then
  wait "$backend_pid" || exit $?
fi
wait "$nginx_pid"
