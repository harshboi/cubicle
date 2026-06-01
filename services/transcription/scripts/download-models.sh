#!/usr/bin/env bash
set -euo pipefail

MODEL_ROOT="${TRANSCRIPTION_MODEL_CACHE_DIR:-/models}"
HF_HOME="${HF_HOME:-$MODEL_ROOT/huggingface}"
VOXTRAL_MODEL_ID="${VOXTRAL_MODEL_ID:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
WHISPER_MODEL_ID="${WHISPER_MODEL_ID:-h2oai/faster-whisper-large-v3-turbo}"
PYANNOTE_MODEL_ID="${PYANNOTE_MODEL_ID:-pyannote/speaker-diarization-community-1}"

export HF_HOME
mkdir -p "$HF_HOME" "$MODEL_ROOT/voxtral" "$MODEL_ROOT/whisper" "$MODEL_ROOT/pyannote"

if [[ -n "${EXTRA_CA_CERT_FILE:-}" ]]; then
  if [[ ! -r "$EXTRA_CA_CERT_FILE" ]]; then
    echo "EXTRA_CA_CERT_FILE is set but is not readable: $EXTRA_CA_CERT_FILE" >&2
    exit 2
  fi
  export SSL_CERT_FILE="$EXTRA_CA_CERT_FILE"
  export REQUESTS_CA_BUNDLE="$EXTRA_CA_CERT_FILE"
  export CURL_CA_BUNDLE="$EXTRA_CA_CERT_FILE"
fi

download_model() {
  local repo_id="$1"
  local target_dir="$2"
  echo "Downloading $repo_id into $target_dir"
  hf download "$repo_id" \
    --local-dir "$target_dir" \
    --cache-dir "$HF_HOME" \
    >/tmp/cubicle-model-download.log
  if [[ "$(find "$target_dir" -type f | wc -l)" -eq 0 ]]; then
    echo "Model download produced no files for $repo_id in $target_dir" >&2
    exit 1
  fi
  rm -f /tmp/cubicle-model-download.log
}

download_model "$VOXTRAL_MODEL_ID" "$MODEL_ROOT/voxtral/$(basename "$VOXTRAL_MODEL_ID")"
download_model "$WHISPER_MODEL_ID" "$MODEL_ROOT/whisper/$(basename "$WHISPER_MODEL_ID")"
download_model "$PYANNOTE_MODEL_ID" "$MODEL_ROOT/pyannote/$(basename "$PYANNOTE_MODEL_ID")"

echo "Model cache prepared under $MODEL_ROOT"
