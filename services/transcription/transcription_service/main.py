from __future__ import annotations

import logging
import os


def main() -> None:
    try:
        import uvicorn
    except ImportError as exc:
        raise RuntimeError("Install requirements.txt before running the service.") from exc

    host = os.environ.get("TRANSCRIPTION_SERVICE_HOST", "0.0.0.0")
    port = int(os.environ.get("TRANSCRIPTION_SERVICE_PORT", "8080"))
    log_level_name = os.environ.get("TRANSCRIPTION_SERVICE_LOG_LEVEL", "info").upper()
    log_level = getattr(logging, log_level_name, logging.INFO)
    logging.basicConfig(
        level=log_level,
        format="%(levelname)s:%(name)s:%(message)s",
    )
    logging.getLogger("transcription_service").setLevel(log_level)
    role = os.environ.get("TRANSCRIPTION_SERVICE_ROLE", "websocket").strip().lower()
    if role in {"diarization_worker", "worker", "diarization"}:
        app_target = "transcription_service.diarization_worker:app"
    elif role in {"text_intelligence_worker", "text_intelligence", "translation_worker", "translation"}:
        app_target = "transcription_service.text_intelligence_worker:app"
    else:
        app_target = "transcription_service.server:app"
    uvicorn.run(
        app_target,
        host=host,
        port=port,
        log_level=os.environ.get("TRANSCRIPTION_SERVICE_LOG_LEVEL", "info").lower(),
    )


if __name__ == "__main__":
    main()
