# Cubicle Transcription EC2 Voxtral vLLM Rebuild Runbook

This runbook recreates the currently working AWS GPU inference path for Cubicle
Live Transcription. It is intentionally scoped to the verified Docker + SSM
runtime path, not the older native Python/Conda path and not the paused ECS
production cutover path.

## Current Known-good Runtime

| Field | Value |
| --- | --- |
| AWS account | `562304353751` |
| AWS profile | `strln` |
| Region | `us-west-2` |
| Auto Scaling group | `cubicle-transcription-gpu` |
| Launch template | `lt-022538ed1a03a0f16` |
| Launch template name | `cubicle-transcription-gpu-20260518022539248300000001` |
| AMI | `ami-004bec71e0ff4f3b4` |
| AMI name | `amzn2-ami-ecs-gpu-hvm-2.0.20260514-x86_64-ebs` |
| Instance type | `g5.xlarge` |
| GPU | NVIDIA A10G |
| Root volume | 200 GiB encrypted gp3, delete on termination |
| Subnets | `subnet-0ef920598c83852d8`, `subnet-06345153613c5045a` |
| Current security group | `sg-0cfa10a79225905d0` / `cubicle-transcription-task` |
| Instance profile | `cubicle-transcription-ecs-gpu-instance` |
| Access method | AWS SSM only |
| SSH key pair | none |
| Container | `voxtral-vllm` |
| Container image | `vllm/vllm-openai:v0.21.0-ubuntu2404` |
| Model | `mistralai/Voxtral-Mini-4B-Realtime-2602` |
| EC2 listen address | `127.0.0.1:8000` |
| Mac forwarded URL | `http://localhost:8000` |
| Realtime route | `ws://localhost:8000/v1/realtime` |

The launch template user data is:

```bash
#!/bin/bash
echo "ECS_CLUSTER=cubicle-transcription-cluster" >> /etc/ecs/ecs.config
echo "ECS_ENABLE_GPU_SUPPORT=true" >> /etc/ecs/ecs.config
```

## What This Rebuild Does

The runnable helper at
`infra/transcription/rebuild-voxtral-vllm-ec2.sh` does the repeatable parts:

1. Verifies the AWS caller is in account `562304353751`.
2. Refuses to run if ambient AWS credential environment variables could shadow
   the `strln` profile.
3. Grants the EC2 instance role least-privilege read access to the existing
   Hugging Face token secret `cubicle-transcription/huggingface-token`.
4. Ensures one GPU instance exists in the `cubicle-transcription-gpu` ASG.
5. Waits for SSM to report the instance online.
6. Uses SSM Run Command to start the Dockerized vLLM Voxtral runtime.
7. Verifies `/health` and `/v1/models` on the EC2-local port.
8. Opens an SSM port forward from the Mac to EC2-local port `8000`.

It does not create a public SSH path and does not open port `8000` to the
Internet.

## Prerequisites

On the Mac:

```bash
aws --version
aws sts get-caller-identity --profile strln --region us-west-2
```

The account must be:

```text
562304353751
```

For port forwarding, the AWS Session Manager plugin must also be installed:

```bash
session-manager-plugin --version
```

If it is missing, install it before using the `port-forward` command.

The Hugging Face token secret must exist in Secrets Manager:

```bash
aws secretsmanager describe-secret \
  --profile strln \
  --region us-west-2 \
  --secret-id cubicle-transcription/huggingface-token
```

The script does not print or store that token locally. The EC2 instance reads it
directly from Secrets Manager after the script attaches a narrow role policy.

## Rebuild From An Existing Or Recreated ASG Instance

From the repo root:

```bash
chmod +x infra/transcription/rebuild-voxtral-vllm-ec2.sh
infra/transcription/rebuild-voxtral-vllm-ec2.sh full
```

`full` performs:

```text
ensure-secret-access
ensure-instance
start-vllm
```

Expected successful output includes:

```text
Granted cubicle-transcription-ecs-gpu-instance read access to cubicle-transcription/huggingface-token.
SSM Online: i-...
[success] Voxtral vLLM runtime is ready
```

## Replace The EC2 Instance And Rebuild

This is the closest match to "tear down the EC2 instance and do it again".
It is destructive, so the helper requires explicit confirmation:

```bash
CONFIRM_TERMINATE=replace-i-understand \
  infra/transcription/rebuild-voxtral-vllm-ec2.sh replace-instance

infra/transcription/rebuild-voxtral-vllm-ec2.sh full
```

The first command terminates the current ASG instance and waits for the ASG to
produce another `InService` instance from the same launch template. The second
command starts the working Docker runtime on that new instance.

## Open The Port Forward

Keep this terminal open on the Mac where the transcription adapter or app runs:

```bash
infra/transcription/rebuild-voxtral-vllm-ec2.sh port-forward
```

That command is equivalent to:

```bash
aws ssm start-session \
  --profile strln \
  --region us-west-2 \
  --target <CURRENT_GPU_INSTANCE_ID> \
  --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["8000"],"localPortNumber":["8000"]}'
```

Then verify from another Mac terminal:

```bash
curl -fsS http://localhost:8000/health
curl -fsS http://localhost:8000/v1/models | python3 -m json.tool
```

Expected model:

```text
mistralai/Voxtral-Mini-4B-Realtime-2602
```

## Start Or Verify The Local Cubicle Adapter

The app should not point directly to vLLM. The working path is:

```text
Cubicle.app
  -> ws://127.0.0.1:18080/v1/transcription
  -> local transcription-service adapter
  -> ws://localhost:8000/v1/realtime through SSM
  -> EC2 Docker vLLM
  -> Voxtral Mini Realtime on A10G
```

Use the forwarded environment:

```bash
set -a
. aws/transcription-service/env.vllm-forwarded.example
set +a
```

Important values:

```bash
VLLM_BASE_URL=http://localhost:8000
VLLM_REALTIME_URL=ws://localhost:8000/v1/realtime
VLLM_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted
```

Run the adapter with a token file, not a token in the command line:

```bash
TRANSCRIPTION_SERVICE_TOKEN_FILE=/path/to/0600-token-file \
PYTHONPATH=aws/transcription-service \
python3 -m transcription_service.main
```

Cubicle Settings should use the adapter endpoint:

```text
ws://127.0.0.1:18080/v1/transcription
```

## Remote Verification Commands

To check the GPU host without opening a shell:

```bash
INSTANCE_ID="$(infra/transcription/rebuild-voxtral-vllm-ec2.sh ensure-instance)"

aws ssm send-command \
  --profile strln \
  --region us-west-2 \
  --instance-ids "$INSTANCE_ID" \
  --document-name AWS-RunShellScript \
  --parameters 'commands=[
    "docker ps",
    "docker logs --tail 120 voxtral-vllm",
    "nvidia-smi",
    "curl -fsS http://127.0.0.1:8000/health",
    "curl -fsS http://127.0.0.1:8000/v1/models"
  ]'
```

## Manual Docker Command On EC2

If you are already in an SSM shell on the instance, this is the working runtime
command. It expects `HF_TOKEN` to be set in that shell:

```bash
docker rm -f voxtral-vllm 2>/dev/null || true

docker run -d \
  --name voxtral-vllm \
  --gpus all \
  --ipc=host \
  --restart unless-stopped \
  -p 127.0.0.1:8000:8000 \
  -v /home/ssm-user/.cache/huggingface:/root/.cache/huggingface \
  -e HF_TOKEN \
  -e VLLM_DISABLE_COMPILE_CACHE=1 \
  -e VLLM_NO_USAGE_STATS=1 \
  -e DO_NOT_TRACK=1 \
  --entrypoint /bin/bash \
  vllm/vllm-openai:v0.21.0-ubuntu2404 \
  -lc 'python3 -m pip install --no-cache-dir "mistral-common[soundfile]" soundfile && exec vllm serve mistralai/Voxtral-Mini-4B-Realtime-2602 --host 0.0.0.0 --port 8000 --tokenizer-mode mistral --max-model-len 45000 --gpu-memory-utilization 0.90 --compilation_config '\''{"cudagraph_mode":"PIECEWISE"}'\'''
```

The `mistral-common[soundfile]` install is required because the stock vLLM image
does not include the Voxtral audio dependency.

## Security Notes

- Do not open EC2 port `8000` in a security group.
- Do not use the EC2 public IP for inference traffic.
- Do not add an SSH key unless there is a separate operational need.
- Keep SSM as the admin and port-forwarding path.
- The HF token is read by the EC2 instance from Secrets Manager. Do not pass it
  in a WebSocket URL, app setting, Git file, Docker build arg, or shell history.
- The Docker container receives `HF_TOKEN` as an environment variable because
  vLLM/Hugging Face need it for gated model access. Root on the EC2 host can see
  container environment. For production, prefer the already-published preloaded
  ECR model image or an encrypted model volume so runtime does not need a token.
- Cubicle sends the transcription service token as a Bearer header and stores it
  in Keychain.
- Transcript/audio logging remains disabled/redacted by default.

## Troubleshooting

### SSM Is Not Online

```bash
aws ssm describe-instance-information \
  --profile strln \
  --region us-west-2 \
  --filters Key=InstanceIds,Values=<INSTANCE_ID>
```

If no record appears, wait for the ECS GPU AMI to boot. If it never appears,
check the instance profile includes `AmazonSSMManagedInstanceCore`.

### Session Manager Plugin Missing On The Mac

The `port-forward` command needs the AWS Session Manager plugin. Install it and
retry:

```bash
session-manager-plugin --version
```

### Container Starts But No Audio Route Works

Check that `/v1/models` returns the model and that the local adapter points at
`ws://localhost:8000/v1/realtime`.

```bash
curl -fsS http://localhost:8000/v1/models | python3 -m json.tool
```

### Native Python Setup Fails

Do not retry native vLLM installation on Amazon Linux 2. The known failure modes
were system Python 3.7, pyenv/OpenSSL conflicts, old GCC, and source builds for
NumPy/SciPy/xformers. Docker is the working runtime.

## Rollback

Stop the GPU runtime without destroying the instance:

```bash
INSTANCE_ID="$(infra/transcription/rebuild-voxtral-vllm-ec2.sh ensure-instance)"
aws ssm send-command \
  --profile strln \
  --region us-west-2 \
  --instance-ids "$INSTANCE_ID" \
  --document-name AWS-RunShellScript \
  --parameters 'commands=["docker rm -f voxtral-vllm || true"]'
```

Scale the ASG down only when you are done paying for the GPU instance:

```bash
aws autoscaling update-auto-scaling-group \
  --profile strln \
  --region us-west-2 \
  --auto-scaling-group-name cubicle-transcription-gpu \
  --desired-capacity 0
```

If the instance is protected from scale-in, remove protection first:

```bash
INSTANCE_ID="$(infra/transcription/rebuild-voxtral-vllm-ec2.sh ensure-instance)"
aws autoscaling set-instance-protection \
  --profile strln \
  --region us-west-2 \
  --auto-scaling-group-name cubicle-transcription-gpu \
  --instance-ids "$INSTANCE_ID" \
  --no-protected-from-scale-in
```
