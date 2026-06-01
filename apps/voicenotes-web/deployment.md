# VoiceNotes Deployment Notes

This service is intentionally isolated from the Cubicle macOS app and the
existing Cubicle transcription backend.

## Local Validation

```bash
cd /Users/prabhat7/Desktop/project/offsite/voicenotes
python3 -m venv .venv
.venv/bin/python -m pip install -e ".[dev]"
.venv/bin/python -m pytest -q
.venv/bin/python -m uvicorn voicenotes_app.main:app --host 127.0.0.1 --port 8787
```

Open:

```text
http://127.0.0.1:8787
```

Default local sign-in:

```text
prabhat7@cisco.com
voicenotes-dev
```

Local mode uses `VOICENOTES_MOCK_TRANSCRIPTION=true` by default. This validates
the product, auth, WebSocket, and storage flow without calling the live AWS
transcription service.

## Production Rollout

1. Build and push the Docker image.
2. Create ACM certificate for `voicenotes.agenticisolation.com`.
3. Confirm CAA permits Amazon certificate issuance.
4. Inject the existing transcription token signing secret from Secrets Manager
   into the VoiceNotes ECS task.
5. Apply `infra/aws`.
6. Add the GoDaddy CNAME:

```text
voicenotes -> <terraform output domain_cname_target>
```

7. Create first Cognito user.
8. Verify:

```text
https://voicenotes.agenticisolation.com/healthz
https://voicenotes.agenticisolation.com/login
```

## Production Settings

The ECS task should run with:

```text
VOICENOTES_AUTH_MODE=oidc
VOICENOTES_SECURE_COOKIES=true
VOICENOTES_STORAGE_BACKEND=aws
VOICENOTES_MOCK_TRANSCRIPTION=false
VOICENOTES_UPSTREAM_TRANSCRIPTION_URL=wss://dcabsri6ekziv.cloudfront.net/v1/transcription
```

Secrets Manager injects:

```text
VOICENOTES_SESSION_SECRET
VOICENOTES_UPSTREAM_TRANSCRIPTION_SIGNING_SECRET
```

## Non-Goals

Do not change these during VoiceNotes deployment:

- `/Applications/Cubicle.app`
- `/Volumes/Webex/getwebexspace-data/GetWebexSpaceMac`
- existing Cubicle transcription ECS service
- existing Voxtral/vLLM runtime
- existing Cubicle admin console
