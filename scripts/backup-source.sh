#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_ROOT="${BACKUP_ROOT:-$REPO_ROOT/backups/source-handoff}"
TIMESTAMP="${BACKUP_TIMESTAMP:-$(date +%Y%m%d-%H%M%S)}"
DEST_DIR="$BACKUP_ROOT/$TIMESTAMP"
ARCHIVE_NAME="${ARCHIVE_NAME:-cubicle-source-$TIMESTAMP.tar.gz}"
ARCHIVE_PATH="$DEST_DIR/$ARCHIVE_NAME"
MANIFEST_PATH="$DEST_DIR/MANIFEST.txt"
SHA_PATH="$DEST_DIR/$ARCHIVE_NAME.sha256"

usage() {
  cat <<'EOF'
Usage:
  scripts/backup-source.sh

Creates a source-code handoff archive under:
  backups/source-handoff/<timestamp>/cubicle-source-<timestamp>.tar.gz

The archive intentionally excludes local build outputs, runtime caches, old
backups, Terraform state, Terraform plans, local tfvars, Python virtualenvs,
compiled bytecode, screenshots, result dumps, local app data, and secret-like
environment files.

This is a code backup for transfer to another AWS account. It is not a backup
of live AWS data, Terraform state, Cognito users, DynamoDB rows, S3 objects, or
Secrets Manager values.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

mkdir -p "$DEST_DIR"

EXCLUDES=(
  "./.build"
  "./.cache"
  "./.claude"
  "./.playwright-cli"
  "./.pytest_cache"
  "./.voicenotes-data"
  "./__pycache__"
  "./backups"
  "./output"
  "./apps/voicenotes-web/.venv"
  "./apps/voicenotes-web/.pytest_cache"
  "./apps/voicenotes-web/voicenotes.egg-info"
  "./services/transcription/.pytest_cache"
  "./infra/aws/transcription/.terraform"
  "./infra/aws/transcription/terraform.tfstate.d"
  "./apps/voicenotes-web/infra/aws/.terraform"
  "./apps/voicenotes-web/infra/aws/terraform.tfstate.d"
  "./results-*.txt"
  "./*.pyc"
  "./*.pyo"
  "*.pyc"
  "*.pyo"
  "*.tfstate"
  "*.tfstate.backup"
  "terraform.tfstate.d"
  "*/terraform.tfstate.d"
  "*/*/terraform.tfstate.d"
  "*/*/*/terraform.tfstate.d"
  "*.tfplan*"
  "terraform.tfvars"
  "*/terraform.tfvars"
  ".env"
  ".env.*"
  "*/.env"
  "*/.env.*"
  "./cubicle-admin-src/deploy/current-*.json"
  "./cubicle-admin-src/deploy/*task-definition*.json"
)

TAR_ARGS=()
for item in "${EXCLUDES[@]}"; do
  TAR_ARGS+=(--exclude="$item")
done

cat > "$MANIFEST_PATH" <<EOF
Cubicle source handoff backup
Created: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Source: $REPO_ROOT
Archive: $ARCHIVE_PATH

Purpose:
  Transfer source code, Terraform modules, deployment helpers, and runbooks to
  a new operator. The receiving operator should provision fresh AWS resources
  in their own account instead of reusing this machine's Terraform state.

Excluded by design:
  - build outputs and caches
  - prior backups and screenshots
  - Python virtualenvs and bytecode
  - Terraform state, plans, and tfvars
  - local app/runtime data
  - secret-like env files
  - account-bound ECS task-definition snapshots

Restore:
  mkdir cubicle-handoff
  tar -xzf $ARCHIVE_NAME -C cubicle-handoff

Validate:
  shasum -a 256 -c $ARCHIVE_NAME.sha256
EOF

(
  cd "$REPO_ROOT"
  tar "${TAR_ARGS[@]}" -czf "$ARCHIVE_PATH" .
)

(
  cd "$DEST_DIR"
  shasum -a 256 "$ARCHIVE_NAME" > "$SHA_PATH"
)

echo "Source backup archive: $ARCHIVE_PATH"
echo "Backup manifest: $MANIFEST_PATH"
echo "Checksum: $SHA_PATH"
