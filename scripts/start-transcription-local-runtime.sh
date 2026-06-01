#!/usr/bin/env zsh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SELF_SCRIPT="$REPO_ROOT/scripts/start-transcription-local-runtime.sh"

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
SSM_TARGET_ID="${SSM_TARGET_ID:-${TRANSCRIPTION_EC2_INSTANCE_ID:-i-02b84c39f9912a77a}}"
VLLM_REMOTE_PORT="${VLLM_REMOTE_PORT:-8000}"
VLLM_LOCAL_PORT="${VLLM_LOCAL_PORT:-8000}"
ADAPTER_HOST="${ADAPTER_HOST:-127.0.0.1}"
ADAPTER_PORT="${ADAPTER_PORT:-18080}"
SSM_TMUX_SESSION="${SSM_TMUX_SESSION:-cubicle-transcription-ssm}"
ADAPTER_TMUX_SESSION="${ADAPTER_TMUX_SESSION:-cubicle-transcription-adapter}"
ADAPTER_SCRIPT="${ADAPTER_SCRIPT:-$REPO_ROOT/.build/run-transcription-adapter.sh}"
LAUNCH_AGENT_LABEL="${LAUNCH_AGENT_LABEL:-local.cubicle.transcription-runtime}"
LAUNCH_AGENT_PATH="$HOME/Library/LaunchAgents/$LAUNCH_AGENT_LABEL.plist"
LOG_DIR="$HOME/Library/Logs/Cubicle"
SUPPORT_DIR="$HOME/Library/Application Support/Cubicle"
LAUNCH_WRAPPER_PATH="$SUPPORT_DIR/start-transcription-local-runtime.sh"
START_CUBICLE="${START_CUBICLE:-false}"

usage() {
  cat <<EOF
Usage: $(basename "$0") <start|stop|restart|status|logs|install-launch-agent|uninstall-launch-agent>

Starts the current local Cubicle transcription runtime:
  Cubicle -> ws://127.0.0.1:18080/v1/transcription
          -> local transcription adapter
          -> SSM port forward localhost:8000
          -> EC2 vLLM Voxtral runtime

Environment overrides:
  AWS_PROFILE_NAME=$AWS_PROFILE_NAME
  AWS_REGION_NAME=$AWS_REGION_NAME
  SSM_TARGET_ID=$SSM_TARGET_ID
  VLLM_LOCAL_PORT=$VLLM_LOCAL_PORT
  ADAPTER_PORT=$ADAPTER_PORT
  START_CUBICLE=true   # optionally open Cubicle after startup
EOF
}

need_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 2
  fi
}

tmux_has_session() {
  tmux has-session -t "$1" >/dev/null 2>&1
}

health_ok() {
  curl -fsS --max-time 2 "$1" >/dev/null 2>&1
}

wait_for_health() {
  local label="$1"
  local url="$2"
  local attempts="${3:-30}"
  local delay="${4:-1}"

  for _ in {1..$attempts}; do
    if health_ok "$url"; then
      echo "$label is healthy: $url"
      return 0
    fi
    sleep "$delay"
  done

  echo "$label did not become healthy: $url" >&2
  return 1
}

start_ssm() {
  need_command aws
  need_command tmux
  need_command curl

  if health_ok "http://127.0.0.1:$VLLM_LOCAL_PORT/health"; then
    echo "vLLM is already reachable on localhost:$VLLM_LOCAL_PORT"
    return 0
  fi

  if tmux_has_session "$SSM_TMUX_SESSION"; then
    echo "SSM tmux session already exists: $SSM_TMUX_SESSION"
  else
    echo "Starting SSM port forward in tmux session: $SSM_TMUX_SESSION"
    local parameters="{\"portNumber\":[\"$VLLM_REMOTE_PORT\"],\"localPortNumber\":[\"$VLLM_LOCAL_PORT\"]}"
    tmux new-session -d -s "$SSM_TMUX_SESSION" \
      aws ssm start-session \
        --profile "$AWS_PROFILE_NAME" \
        --region "$AWS_REGION_NAME" \
        --target "$SSM_TARGET_ID" \
        --document-name AWS-StartPortForwardingSession \
        --parameters "$parameters"
  fi

  wait_for_health "vLLM" "http://127.0.0.1:$VLLM_LOCAL_PORT/health" 45 1
}

start_adapter() {
  need_command tmux
  need_command curl

  if [[ ! -x "$ADAPTER_SCRIPT" ]]; then
    echo "Missing executable adapter script: $ADAPTER_SCRIPT" >&2
    echo "Run the repo setup/build flow that creates .build/run-transcription-adapter.sh first." >&2
    exit 2
  fi

  if health_ok "http://$ADAPTER_HOST:$ADAPTER_PORT/healthz"; then
    echo "Transcription adapter is already reachable on $ADAPTER_HOST:$ADAPTER_PORT"
    return 0
  fi

  if tmux_has_session "$ADAPTER_TMUX_SESSION"; then
    echo "Adapter tmux session exists but health is not ready; restarting: $ADAPTER_TMUX_SESSION"
    tmux kill-session -t "$ADAPTER_TMUX_SESSION" || true
  fi

  echo "Starting transcription adapter in tmux session: $ADAPTER_TMUX_SESSION"
  tmux new-session -d -s "$ADAPTER_TMUX_SESSION" \
    /bin/zsh -lc "cd '$REPO_ROOT' && '$ADAPTER_SCRIPT'"

  wait_for_health "transcription adapter" "http://$ADAPTER_HOST:$ADAPTER_PORT/healthz" 30 1
}

start_all() {
  start_ssm
  start_adapter
  echo
  echo "Cubicle transcription local runtime is ready."
  echo "Cubicle endpoint should be: ws://$ADAPTER_HOST:$ADAPTER_PORT/v1/transcription"
  echo "Use '$SELF_SCRIPT logs' to inspect background sessions."

  if [[ "$START_CUBICLE" == "true" ]]; then
    open -a Cubicle
  fi
}

stop_all() {
  need_command tmux
  if tmux_has_session "$ADAPTER_TMUX_SESSION"; then
    tmux kill-session -t "$ADAPTER_TMUX_SESSION"
    echo "Stopped $ADAPTER_TMUX_SESSION"
  else
    echo "Adapter session not running: $ADAPTER_TMUX_SESSION"
  fi

  if tmux_has_session "$SSM_TMUX_SESSION"; then
    tmux kill-session -t "$SSM_TMUX_SESSION"
    echo "Stopped $SSM_TMUX_SESSION"
  else
    echo "SSM session not running: $SSM_TMUX_SESSION"
  fi
}

status_all() {
  need_command tmux
  echo "tmux sessions:"
  tmux list-sessions 2>/dev/null || echo "  none"
  echo

  if health_ok "http://127.0.0.1:$VLLM_LOCAL_PORT/health"; then
    echo "vLLM: ok http://127.0.0.1:$VLLM_LOCAL_PORT/health"
  else
    echo "vLLM: not reachable http://127.0.0.1:$VLLM_LOCAL_PORT/health"
  fi

  if health_ok "http://$ADAPTER_HOST:$ADAPTER_PORT/healthz"; then
    echo "adapter: ok http://$ADAPTER_HOST:$ADAPTER_PORT/healthz"
  else
    echo "adapter: not reachable http://$ADAPTER_HOST:$ADAPTER_PORT/healthz"
  fi
}

show_logs() {
  need_command tmux
  echo "== $SSM_TMUX_SESSION =="
  tmux capture-pane -pt "$SSM_TMUX_SESSION" -S -120 2>/dev/null || echo "not running"
  echo
  echo "== $ADAPTER_TMUX_SESSION =="
  tmux capture-pane -pt "$ADAPTER_TMUX_SESSION" -S -120 2>/dev/null || echo "not running"
}

install_launch_agent() {
  mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR" "$SUPPORT_DIR"
  cat > "$LAUNCH_WRAPPER_PATH" <<EOF
#!/usr/bin/env zsh
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

REPO_ROOT="$REPO_ROOT"
AWS_PROFILE_NAME="$AWS_PROFILE_NAME"
AWS_REGION_NAME="$AWS_REGION_NAME"
SSM_TARGET_ID="$SSM_TARGET_ID"
VLLM_REMOTE_PORT="$VLLM_REMOTE_PORT"
VLLM_LOCAL_PORT="$VLLM_LOCAL_PORT"
ADAPTER_HOST="$ADAPTER_HOST"
ADAPTER_PORT="$ADAPTER_PORT"
SSM_TMUX_SESSION="$SSM_TMUX_SESSION"
ADAPTER_TMUX_SESSION="$ADAPTER_TMUX_SESSION"
ADAPTER_PYTHON="$REPO_ROOT/.build/transcription-service-venv/bin/python"

health_ok() {
  curl -fsS --max-time 2 "\$1" >/dev/null 2>&1
}

tmux_has_session() {
  tmux has-session -t "\$1" >/dev/null 2>&1
}

for _ in {1..60}; do
  if [[ -x "\$ADAPTER_PYTHON" ]]; then
    break
  fi
  sleep 2
done

if [[ ! -x "\$ADAPTER_PYTHON" ]]; then
  echo "Cubicle transcription adapter Python was not available: \$ADAPTER_PYTHON" >&2
  exit 2
fi

if ! health_ok "http://127.0.0.1:\$VLLM_LOCAL_PORT/health"; then
  if ! tmux_has_session "\$SSM_TMUX_SESSION"; then
    parameters="{\\\"portNumber\\\":[\\\"\$VLLM_REMOTE_PORT\\\"],\\\"localPortNumber\\\":[\\\"\$VLLM_LOCAL_PORT\\\"]}"
    tmux new-session -d -s "\$SSM_TMUX_SESSION" \
      aws ssm start-session \
        --profile "\$AWS_PROFILE_NAME" \
        --region "\$AWS_REGION_NAME" \
        --target "\$SSM_TARGET_ID" \
        --document-name AWS-StartPortForwardingSession \
        --parameters "\$parameters"
  fi

  for _ in {1..45}; do
    health_ok "http://127.0.0.1:\$VLLM_LOCAL_PORT/health" && break
    sleep 1
  done
fi

if ! health_ok "http://\$ADAPTER_HOST:\$ADAPTER_PORT/healthz"; then
  if tmux_has_session "\$ADAPTER_TMUX_SESSION"; then
    tmux kill-session -t "\$ADAPTER_TMUX_SESSION" || true
  fi

  tmux new-session -d -s "\$ADAPTER_TMUX_SESSION" /bin/zsh -lc "
    cd '\$REPO_ROOT' &&
    export TRANSCRIPTION_SERVICE_HOST='\$ADAPTER_HOST' &&
    export TRANSCRIPTION_SERVICE_PORT='\$ADAPTER_PORT' &&
    export TRANSCRIPTION_SERVICE_TOKEN_FILE='\$REPO_ROOT/.build/transcription-service-token' &&
    export TRANSCRIPTION_ASR_PROVIDER='voxtral_self_hosted' &&
    export TRANSCRIPTION_DIARIZATION_PROVIDER='mock' &&
    export TRANSCRIPTION_VOXTRAL_RUNTIME='vllm' &&
    export TRANSCRIPTION_VOXTRAL_MODEL_VERSION='self-hosted-vllm-2602' &&
    export TRANSCRIPTION_REQUIRE_GPU='false' &&
    export TRANSCRIPTION_RETENTION='disabled' &&
    export VLLM_BASE_URL='http://localhost:\$VLLM_LOCAL_PORT' &&
    export VLLM_REALTIME_URL='ws://localhost:\$VLLM_LOCAL_PORT/v1/realtime' &&
    export VLLM_MODEL='mistralai/Voxtral-Mini-4B-Realtime-2602' &&
    export PYTHONPATH='\$REPO_ROOT/services/transcription' &&
    exec '\$ADAPTER_PYTHON' -m transcription_service.main
  "
fi

health_ok "http://127.0.0.1:\$VLLM_LOCAL_PORT/health" || exit 3
health_ok "http://\$ADAPTER_HOST:\$ADAPTER_PORT/healthz" || exit 4
exit 0
EOF
  chmod 700 "$LAUNCH_WRAPPER_PATH"

  cat > "$LAUNCH_AGENT_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LAUNCH_AGENT_LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$LAUNCH_WRAPPER_PATH</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>StartInterval</key>
  <integer>300</integer>
  <key>StandardOutPath</key>
  <string>$LOG_DIR/transcription-runtime.out.log</string>
  <key>StandardErrorPath</key>
  <string>$LOG_DIR/transcription-runtime.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>AWS_PROFILE_NAME</key>
    <string>$AWS_PROFILE_NAME</string>
    <key>AWS_REGION_NAME</key>
    <string>$AWS_REGION_NAME</string>
    <key>SSM_TARGET_ID</key>
    <string>$SSM_TARGET_ID</string>
  </dict>
</dict>
</plist>
EOF
  chmod 600 "$LAUNCH_AGENT_PATH"
  launchctl unload "$LAUNCH_AGENT_PATH" >/dev/null 2>&1 || true
  launchctl load "$LAUNCH_AGENT_PATH"
  echo "Installed and loaded LaunchAgent: $LAUNCH_AGENT_PATH"
  echo "It will start the SSM tunnel and adapter at login while AWS credentials are valid."
}

uninstall_launch_agent() {
  launchctl unload "$LAUNCH_AGENT_PATH" >/dev/null 2>&1 || true
  rm -f "$LAUNCH_AGENT_PATH"
  rm -f "$LAUNCH_WRAPPER_PATH"
  echo "Removed LaunchAgent: $LAUNCH_AGENT_PATH"
}

action="${1:-start}"
case "$action" in
  start)
    start_all
    ;;
  stop)
    stop_all
    ;;
  restart)
    stop_all
    start_all
    ;;
  status)
    status_all
    ;;
  logs)
    show_logs
    ;;
  install-launch-agent)
    install_launch_agent
    ;;
  uninstall-launch-agent)
    uninstall_launch_agent
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
