#!/usr/bin/env bash
set -euo pipefail

MODEL_ID="${VOXTRAL_MODEL_ID:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
MODEL_PATH="${VOXTRAL_MODEL_PATH:-/models/voxtral/${MODEL_ID##*/}}"
SERVED_MODEL_NAME="${VOXTRAL_SERVED_MODEL_NAME:-$MODEL_ID}"
HOST="${VOXTRAL_HOST:-0.0.0.0}"
PORT="${VOXTRAL_PORT:-8000}"
MAX_MODEL_LEN="${VOXTRAL_MAX_MODEL_LEN:-45000}"
MAX_BATCHED_TOKENS="${VOXTRAL_MAX_NUM_BATCHED_TOKENS:-4096}"
TOKENIZER_MODE="${VOXTRAL_TOKENIZER_MODE:-mistral}"
GPU_MEMORY_UTILIZATION="${VOXTRAL_GPU_MEMORY_UTILIZATION:-0.90}"
ENFORCE_EAGER="${VOXTRAL_ENFORCE_EAGER:-false}"
COMPILATION_CONFIG="${VOXTRAL_COMPILATION_CONFIG:-{\"cudagraph_mode\":\"PIECEWISE\"}}"

if [[ -d "$MODEL_PATH" ]] && find "$MODEL_PATH" -type f -print -quit | grep -q .; then
  SERVE_TARGET="$MODEL_PATH"
else
  SERVE_TARGET="$MODEL_ID"
fi

VLLM_ARGS=(
  serve "$SERVE_TARGET"
  --served-model-name "$SERVED_MODEL_NAME"
  --host "$HOST"
  --port "$PORT"
  --tokenizer-mode "$TOKENIZER_MODE"
  --max-model-len "$MAX_MODEL_LEN"
  --max-num-batched-tokens "$MAX_BATCHED_TOKENS"
  --gpu-memory-utilization "$GPU_MEMORY_UTILIZATION"
)

if [[ "$ENFORCE_EAGER" == "true" ]]; then
  VLLM_ARGS+=(--enforce-eager)
else
  VLLM_ARGS+=(--compilation_config "$COMPILATION_CONFIG")
fi

exec vllm "${VLLM_ARGS[@]}"
