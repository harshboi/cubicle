# Cubicle Local Transcription Startup

The current working transcription path is:

```text
Cubicle.app
  -> ws://127.0.0.1:18080/v1/transcription
  -> local transcription adapter
  -> SSM port forward on localhost:8000
  -> EC2 g5.xlarge Voxtral/vLLM runtime
```

The SSM tunnel only exposes the private AWS vLLM server to the Mac. Cubicle
still needs the local adapter on `127.0.0.1:18080` because the adapter speaks
Cubicle's transcription protocol, sends audio to vLLM, and returns
`partial_transcript` / `final_transcript` events to the app.

## Manual Start

From this repo:

```bash
Scripts/start-transcription-local-runtime.sh start
```

This starts two tmux sessions:

- `cubicle-transcription-ssm`: AWS SSM port forward from Mac `localhost:8000`
  to EC2 `localhost:8000`.
- `cubicle-transcription-adapter`: local transcription service on
  `127.0.0.1:18080`.

Then verify:

```bash
Scripts/start-transcription-local-runtime.sh status
```

Expected:

```text
vLLM: ok http://127.0.0.1:8000/health
adapter: ok http://127.0.0.1:18080/healthz
```

Cubicle Settings should use:

```text
ws://127.0.0.1:18080/v1/transcription
```

## Logs And Stop

```bash
Scripts/start-transcription-local-runtime.sh logs
Scripts/start-transcription-local-runtime.sh stop
Scripts/start-transcription-local-runtime.sh restart
```

## Start At Laptop Login

Install a LaunchAgent:

```bash
Scripts/start-transcription-local-runtime.sh install-launch-agent
```

This creates:

```text
~/Library/LaunchAgents/local.cubicle.transcription-runtime.plist
```

The agent starts the SSM tunnel and adapter at login and rechecks every five
minutes. It does not store tokens or AWS keys. It uses AWS profile `strln`,
region `us-west-2`, and instance `i-02b84c39f9912a77a` by default.

The LaunchAgent points at a small wrapper under:

```text
~/Library/Application Support/Cubicle/start-transcription-local-runtime.sh
```

That wrapper waits for the repo volume to be available, then starts the SSM
tunnel and adapter through tmux.

Remove it with:

```bash
Scripts/start-transcription-local-runtime.sh uninstall-launch-agent
```

LaunchAgent logs are written to:

```text
~/Library/Logs/Cubicle/transcription-runtime.out.log
~/Library/Logs/Cubicle/transcription-runtime.err.log
```

## Common Failure

If Cubicle shows "Could not connect to the server", check:

```bash
Scripts/start-transcription-local-runtime.sh status
```

If `vLLM` is down, the SSM tunnel is not running or AWS credentials are expired.
If `adapter` is down, the local bridge service is not running. Re-run:

```bash
Scripts/start-transcription-local-runtime.sh restart
```

If AWS credentials are expired, refresh the `strln` AWS profile first, then
run the startup script again.
