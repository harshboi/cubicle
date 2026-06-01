#!/usr/bin/env python3
"""Mint a signed transcription user token for local/manual testing."""

from __future__ import annotations

import argparse
import os
import sys
import uuid
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO_ROOT))

from transcription_service.auth import mint_signed_user_token  # noqa: E402


def _secret_from_args(args: argparse.Namespace) -> str:
    if args.secret_file:
        return Path(args.secret_file).read_text(encoding="utf-8").strip()
    env_value = os.environ.get(args.secret_env, "").strip()
    if env_value:
        return env_value
    raise SystemExit(
        f"Missing signing secret. Set {args.secret_env} or pass --secret-file /path/to/0600-secret."
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Mint a short-lived Cubicle transcription signed user token."
    )
    parser.add_argument("--subject", required=True, help="Stable user id or email for the token subject.")
    parser.add_argument("--email", default="", help="Optional user email claim.")
    parser.add_argument(
        "--scope",
        action="append",
        default=[],
        help="Token scope. May be repeated. Defaults to transcription:stream.",
    )
    parser.add_argument("--ttl-seconds", type=int, default=3600, help="Token lifetime in seconds.")
    parser.add_argument("--issuer", default="cubicle-transcription")
    parser.add_argument("--audience", default="cubicle-macos")
    parser.add_argument("--token-id", default="", help="Optional jti. Defaults to a UUID.")
    parser.add_argument(
        "--secret-env",
        default="TRANSCRIPTION_TOKEN_SIGNING_SECRET",
        help="Environment variable containing the HMAC signing secret.",
    )
    parser.add_argument(
        "--secret-file",
        default="",
        help="File containing the HMAC signing secret. Prefer this over putting secrets in shell history.",
    )
    args = parser.parse_args()

    if args.ttl_seconds <= 0:
        raise SystemExit("--ttl-seconds must be positive")

    token = mint_signed_user_token(
        signing_secret=_secret_from_args(args),
        subject=args.subject,
        email=args.email or None,
        scopes=tuple(args.scope or ["transcription:stream"]),
        ttl_seconds=args.ttl_seconds,
        issuer=args.issuer,
        audience=args.audience,
        token_id=args.token_id or str(uuid.uuid4()),
    )
    print(token)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
