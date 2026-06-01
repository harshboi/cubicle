# syntax=docker/dockerfile:1.7
FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV HOME=/tmp
ENV XDG_CACHE_HOME=/tmp/app-cache
ENV MPLCONFIGDIR=/tmp/app-cache/matplotlib
ENV TMPDIR=/tmp
ENV TRANSCRIPTION_SERVICE_HOST=0.0.0.0
ENV TRANSCRIPTION_SERVICE_PORT=8080
ENV PYANNOTE_METRICS_ENABLED=0
ENV HF_HOME=/models/huggingface
ENV TRANSFORMERS_CACHE=/models/huggingface
ENV HF_HUB_ENABLE_HF_TRANSFER=1

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      ffmpeg \
      libsndfile1 && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

COPY requirements-asr.txt /app/requirements-asr.txt
ARG INSTALL_REAL_ASR=false
RUN if [ "$INSTALL_REAL_ASR" = "true" ]; then \
      pip install --no-cache-dir -r /app/requirements-asr.txt; \
    fi

COPY requirements-voxtral.txt /app/requirements-voxtral.txt
ARG INSTALL_VOXTRAL_REALTIME=false
RUN if [ "$INSTALL_VOXTRAL_REALTIME" = "true" ]; then \
      pip install --no-cache-dir -r /app/requirements-voxtral.txt; \
    fi

COPY requirements-diarization.txt /app/requirements-diarization.txt
ARG INSTALL_DIARIZATION=false
RUN if [ "$INSTALL_DIARIZATION" = "true" ]; then \
      pip install --no-cache-dir -r /app/requirements-diarization.txt; \
    fi

COPY requirements-selfhost.txt /app/requirements-selfhost.txt
ARG INSTALL_SELF_HOSTED_MODELS=false
RUN if [ "$INSTALL_SELF_HOSTED_MODELS" = "true" ]; then \
      pip install --no-cache-dir \
        -r /app/requirements-asr.txt \
        -r /app/requirements-diarization.txt \
        -r /app/requirements-selfhost.txt; \
    fi

RUN mkdir -p /models/huggingface /tmp/app-cache/matplotlib

ARG PRELOAD_SERVICE_MODELS=false
ARG WHISPER_MODEL_ID=h2oai/faster-whisper-large-v3-turbo
ARG PYANNOTE_MODEL_ID=pyannote/speaker-diarization-community-1
RUN --mount=type=secret,id=hf_token,required=false \
    if [ "$PRELOAD_SERVICE_MODELS" = "true" ]; then \
      if [ -f /run/secrets/hf_token ]; then export HF_TOKEN="$(cat /run/secrets/hf_token)"; fi; \
      huggingface-cli download "$WHISPER_MODEL_ID" >/tmp/whisper-download.log; \
      huggingface-cli download "$PYANNOTE_MODEL_ID" >/tmp/pyannote-download.log; \
      rm -f /tmp/whisper-download.log /tmp/pyannote-download.log; \
    fi

COPY transcription_service /app/transcription_service
RUN groupadd --system app && \
    useradd --system --gid app --home-dir /nonexistent --shell /usr/sbin/nologin app && \
    chown -R app:app /app /models /tmp/app-cache

EXPOSE 8080

USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD python -c "import json, urllib.request; data=json.load(urllib.request.urlopen('http://127.0.0.1:8080/healthz', timeout=2)); raise SystemExit(0 if data.get('status') == 'ok' else 1)"

CMD ["python", "-m", "transcription_service.main"]
