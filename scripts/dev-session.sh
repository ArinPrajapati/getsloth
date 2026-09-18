#!/usr/bin/env bash

set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf '%s\n' \
    "Usage: ./scripts/dev-session.sh [--remote|--group] [command [args...]]" \
    "" \
    "Starts the relay, web viewer, secure phone-ready tunnel, and PTY." \
    "Without an explicit mode, an interactive prompt asks you to choose." \
    "With no command, getsloth opens your default shell." \
    "" \
    "Examples:" \
    "  ./scripts/dev-session.sh" \
    "  ./scripts/dev-session.sh claude" \
    "  ./scripts/dev-session.sh codex" \
    "  ./scripts/dev-session.sh --remote claude" \
    "  ./scripts/dev-session.sh --group claude"
  exit 0
fi

session_args=("$@")
if [ "${session_args[0]:-}" != "--remote" ] && [ "${session_args[0]:-}" != "--group" ]; then
  if [ -t 0 ]; then
    printf '%s\n' \
      "Choose session mode:" \
      "  1) Remote — one viewer can watch and take control" \
      "  2) Group  — multiple viewers can watch and chat; host keeps control" >&2
    printf 'Mode [1]: ' >&2
    IFS= read -r mode_choice
    case "$mode_choice" in
      2|g|G|group|Group)
        if [ "$#" -gt 0 ]; then
          session_args=(--group "$@")
        else
          session_args=(--group)
        fi
        ;;
      *)
        if [ "$#" -gt 0 ]; then
          session_args=(--remote "$@")
        else
          session_args=(--remote)
        fi
        ;;
    esac
  else
    if [ "$#" -gt 0 ]; then
      session_args=(--remote "$@")
    else
      session_args=(--remote)
    fi
  fi
fi

relay_port="${GETSLOTH_DEV_RELAY_PORT:-18080}"
web_port="${GETSLOTH_DEV_WEB_PORT:-5174}"
state_dir="$(mktemp -d "${TMPDIR:-/tmp}/getsloth-dev.XXXXXX")"
pids=()

cleanup() {
  local pid

  for pid in "${pids[@]:-}"; do
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
    fi
  done

  for pid in "${pids[@]:-}"; do
    if [ -n "$pid" ]; then
      wait "$pid" 2>/dev/null || true
    fi
  done

  rm -rf -- "$state_dir"
}
trap cleanup EXIT INT TERM

fail_with_log() {
  local message="$1"
  local log_file="$2"

  echo "getsloth dev: $message" >&2
  if [ -s "$log_file" ]; then
    echo "--- $log_file ---" >&2
    tail -n 30 "$log_file" >&2
  fi
  exit 1
}

ensure_http_port_free() {
  local port="$1"

  if curl --silent --output /dev/null --max-time 1 "http://127.0.0.1:$port/" 2>/dev/null; then
    echo "getsloth dev: port $port is already serving HTTP; stop the existing dev session or choose another port" >&2
    echo "getsloth dev: override with GETSLOTH_DEV_RELAY_PORT or GETSLOTH_DEV_WEB_PORT" >&2
    exit 1
  fi
}

wait_for_http() {
  local pid="$1"
  local url="$2"
  local log_file="$3"
  local attempt
  local status

  for attempt in $(seq 1 120); do
    if ! kill -0 "$pid" 2>/dev/null; then
      fail_with_log "a local service stopped before becoming ready" "$log_file"
    fi
    status="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 "$url" 2>/dev/null || true)"
    if [[ "$status" =~ ^[234] ]]; then
      return
    fi
    sleep 0.25
  done

  fail_with_log "timed out waiting for $url" "$log_file"
}

wait_for_tunnel() {
  local pid="$1"
  local log_file="$2"
  local attempt
  local public_url

  for attempt in $(seq 1 120); do
    if ! kill -0 "$pid" 2>/dev/null; then
      fail_with_log "Cloudflare Tunnel stopped before publishing a URL" "$log_file"
    fi
    public_url="$(grep -Eo 'https://[-a-z0-9]+\.trycloudflare\.com' "$log_file" | head -n 1 || true)"
    if [ -n "$public_url" ]; then
      printf '%s\n' "$public_url"
      return
    fi
    sleep 0.25
  done

  fail_with_log "timed out waiting for Cloudflare Tunnel" "$log_file"
}

for command in go npm cloudflared curl jq; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "getsloth dev: required command not found: $command" >&2
    exit 1
  fi
done

ensure_http_port_free "$relay_port"
ensure_http_port_free "$web_port"

if [ ! -x "$repo_dir/web/node_modules/.bin/vite" ]; then
  echo "getsloth dev: installing frontend dependencies..." >&2
  npm install --prefix "$repo_dir/web"
fi

echo "getsloth dev: building local binaries..." >&2
(
  cd "$repo_dir"
  go build -o "$state_dir/getsloth-relay" ./cmd/getsloth-relay
  go build -o "$state_dir/getsloth" ./cmd/getsloth
)

relay_log="$state_dir/relay.log"
GETSLOTH_RELAY_ADDR="127.0.0.1:$relay_port" "$state_dir/getsloth-relay" >"$relay_log" 2>&1 &
relay_pid=$!
pids+=("$relay_pid")
wait_for_http "$relay_pid" "http://127.0.0.1:$relay_port/" "$relay_log"

public_host="${GETSLOTH_DEV_PUBLIC_HOST:-sloth.arin.work}"
tunnel_name="${GETSLOTH_DEV_TUNNEL_NAME:-getsloth-dev}"
tunnel_id="$(cloudflared tunnel list --output json 2>/dev/null | jq -r --arg name "$tunnel_name" '.[] | select(.name == $name) | .id' | head -n 1 || true)"
credentials_file="${HOME}/.cloudflared/${tunnel_id}.json"
use_named_tunnel=false

if [ -n "$tunnel_id" ] && [ -r "$credentials_file" ]; then
  use_named_tunnel=true
  relay_websocket_url="wss://$public_host"
else
  relay_tunnel_log="$state_dir/relay-tunnel.log"
  cloudflared tunnel --no-autoupdate --url "http://127.0.0.1:$relay_port" >"$relay_tunnel_log" 2>&1 &
  relay_tunnel_pid=$!
  pids+=("$relay_tunnel_pid")
  relay_public_url="$(wait_for_tunnel "$relay_tunnel_pid" "$relay_tunnel_log")"
  wait_for_http "$relay_tunnel_pid" "$relay_public_url/" "$relay_tunnel_log"
  relay_websocket_url="wss://${relay_public_url#https://}"
fi

web_log="$state_dir/web.log"
(
  cd "$repo_dir/web"
  exec env VITE_RELAY_BASE_URL="$relay_websocket_url" ./node_modules/.bin/vite --host 127.0.0.1 --port "$web_port"
) >"$web_log" 2>&1 &
web_pid=$!
pids+=("$web_pid")
wait_for_http "$web_pid" "http://127.0.0.1:$web_port/" "$web_log"

if [ "$use_named_tunnel" = true ]; then
  tunnel_config="$state_dir/cloudflared.yml"
  printf '%s\n' \
    "tunnel: $tunnel_id" \
    "credentials-file: $credentials_file" \
    "" \
    "ingress:" \
    "  - hostname: $public_host" \
    "    path: /ws/.*" \
    "    service: http://127.0.0.1:$relay_port" \
    "  - hostname: $public_host" \
    "    service: http://127.0.0.1:$web_port" \
    "  - service: http_status:404" >"$tunnel_config"

  tunnel_log="$state_dir/tunnel.log"
  cloudflared tunnel --no-autoupdate --config "$tunnel_config" run "$tunnel_id" >"$tunnel_log" 2>&1 &
  tunnel_pid=$!
  pids+=("$tunnel_pid")
  web_public_url="https://$public_host"
  wait_for_http "$tunnel_pid" "$web_public_url/" "$tunnel_log"
else
  web_tunnel_log="$state_dir/web-tunnel.log"
  cloudflared tunnel --no-autoupdate --url "http://127.0.0.1:$web_port" >"$web_tunnel_log" 2>&1 &
  web_tunnel_pid=$!
  pids+=("$web_tunnel_pid")
  web_public_url="$(wait_for_tunnel "$web_tunnel_pid" "$web_tunnel_log")"
  wait_for_http "$web_tunnel_pid" "$web_public_url/" "$web_tunnel_log"
fi

echo >&2
echo "getsloth dev: phone-ready HTTPS tunnel is live" >&2
echo "getsloth dev: starting session; press Ctrl-D or run 'exit' to stop everything" >&2
echo >&2

GETSLOTH_RELAY_URL="$relay_websocket_url" \
GETSLOTH_WEB_URL="$web_public_url" \
  "$state_dir/getsloth" "${session_args[@]}"
