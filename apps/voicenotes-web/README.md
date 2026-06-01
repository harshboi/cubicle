# VoiceNotes

VoiceNotes is a secure, browser-based transcription app for
`voicenotes.agenticisolation.com`.

It is intentionally implemented as a separate service from the Cubicle macOS app
and the existing AWS transcription backend. The web app talks to the existing
transcription service only through a server-side WebSocket proxy, so browser
JavaScript never receives upstream bearer tokens, AWS credentials, signing
secrets, or model credentials.

## What Is Implemented

- Username/password login for local development.
- Production-ready hooks for managed auth through OIDC/Cognito or ALB/Cognito.
- Secure signed session cookie.
- Otter-style recordings shell and Cubicle-style recording page.
- Browser microphone capture with 16 kHz mono PCM S16LE streaming.
- Same-origin `/ws/record` WebSocket endpoint.
- Server-side proxy to the existing Cubicle transcription WebSocket.
- Local encrypted-at-rest-equivalent development store using filesystem
  ownership boundaries.
- AWS storage adapter for DynamoDB metadata and S3 transcript objects.
- Per-user transcript listing, read, title update, download, and delete.
- Metadata-only audit logging.
- Dockerfile and deployment configuration.
- Unit tests for auth, storage, API ownership, and mock recording.

## Local Development

```bash
cd /Users/prabhat7/Desktop/project/offsite/voicenotes
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -e ".[dev]"
python -m uvicorn voicenotes_app.main:app --host 127.0.0.1 --port 8787
```

Then open:

```text
http://127.0.0.1:8787
```

Default local credentials:

```text
Email:    prabhat7@cisco.com
Password: voicenotes-dev
```

The default local mode uses mock transcription so the UI and storage flow can be
validated without touching the production transcription backend.

## Production Shape

Recommended production path:

```text
Browser
  -> https://voicenotes.agenticisolation.com
  -> VoiceNotes ECS service
  -> /ws/record same-origin WebSocket
  -> server-side upstream connection with Authorization header
  -> wss://dcabsri6ekziv.cloudfront.net/v1/transcription
```

Production settings should use:

```text
VOICENOTES_AUTH_MODE=oidc
VOICENOTES_STORAGE_BACKEND=aws
VOICENOTES_MOCK_TRANSCRIPTION=false
VOICENOTES_UPSTREAM_TRANSCRIPTION_URL=wss://dcabsri6ekziv.cloudfront.net/v1/transcription
VOICENOTES_UPSTREAM_TRANSCRIPTION_SIGNING_SECRET=<server-side signing secret only>
```

The upstream signing secret should be injected from Secrets Manager into the ECS
task. VoiceNotes uses it only on the server to mint short-lived transcription
tokens for authenticated users. Do not place it in frontend code, static files,
query strings, or browser storage.

## Safety Boundary

This project does not modify:

- `Cubicle.app`
- `/Applications/Cubicle.app`
- the original `/Volumes/Webex/getwebexspace-data/GetWebexSpaceMac` checkout
- the existing transcription ECS service
- the existing Voxtral/vLLM runtime
- the existing admin console
