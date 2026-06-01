"""Signed-token authentication for transcription users."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import base64
import hashlib
import hmac
import json
import os
import time
from typing import Any, Mapping

from .user_registry import UserRegistry, UserRegistryError, UserRegistrySettings


class AuthError(PermissionError):
    """Authentication or authorization failure for a transcription request."""

    pass


def _split_csv(value: str | None) -> frozenset[str]:
    if not value:
        return frozenset()
    return frozenset(part.strip().lower() for part in value.replace("\n", ",").split(",") if part.strip())


def _base64url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _base64url_decode(value: str) -> bytes:
    padding = "=" * (-len(value) % 4)
    try:
        return base64.urlsafe_b64decode((value + padding).encode("ascii"))
    except (ValueError, UnicodeEncodeError) as exc:
        raise AuthError("invalid signed token encoding") from exc


@dataclass(frozen=True)
class AuthContext:
    """Authenticated user context attached to a transcription session."""

    mode: str
    user_id: str | None = None
    email: str | None = None
    scopes: tuple[str, ...] = ()
    token_id: str | None = None

    @property
    def audit_user(self) -> str | None:
        return self.email or self.user_id


@dataclass(frozen=True)
class TokenAuthenticator:
    """Validates shared tokens and signed per-user transcription tokens."""

    expected_token: str | None = None
    auth_mode: str = "shared_token"
    signing_secret: str | None = None
    allowed_users: frozenset[str] = frozenset()
    revoked_token_ids: frozenset[str] = frozenset()
    token_issuer: str | None = "cubicle-transcription"
    token_audience: str | None = "cubicle-macos"
    required_scope: str = "transcription:stream"
    user_registry: UserRegistry | None = None

    @classmethod
    def from_environment(cls) -> "TokenAuthenticator":
        token_file = os.environ.get("TRANSCRIPTION_SERVICE_TOKEN_FILE")
        if token_file and token_file.strip():
            with open(token_file.strip(), encoding="utf-8") as handle:
                token = handle.read()
            expected_token = token.strip() if token and token.strip() else None
        else:
            token = os.environ.get("TRANSCRIPTION_SERVICE_TOKEN")
            expected_token = token.strip() if token and token.strip() else None

        signing_secret_file = os.environ.get("TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE")
        if signing_secret_file and signing_secret_file.strip():
            with open(signing_secret_file.strip(), encoding="utf-8") as handle:
                signing_secret = handle.read().strip()
        else:
            signing_secret = os.environ.get("TRANSCRIPTION_TOKEN_SIGNING_SECRET", "").strip()

        user_registry = UserRegistrySettings.from_environment().create_registry()

        return cls(
            expected_token=expected_token,
            auth_mode=os.environ.get("TRANSCRIPTION_AUTH_MODE", "shared_token").strip().lower()
            or "shared_token",
            signing_secret=signing_secret or None,
            allowed_users=_split_csv(os.environ.get("TRANSCRIPTION_ALLOWED_USERS")),
            revoked_token_ids=_split_csv(os.environ.get("TRANSCRIPTION_REVOKED_TOKEN_IDS")),
            token_issuer=os.environ.get("TRANSCRIPTION_TOKEN_ISSUER", "cubicle-transcription").strip()
            or None,
            token_audience=os.environ.get("TRANSCRIPTION_TOKEN_AUDIENCE", "cubicle-macos").strip()
            or None,
            required_scope=os.environ.get("TRANSCRIPTION_REQUIRED_SCOPE", "transcription:stream").strip()
            or "transcription:stream",
            user_registry=user_registry,
        )

    @property
    def auth_required(self) -> bool:
        if self.auth_mode in {"signed_user_token", "signed_or_shared"}:
            return True
        return bool(self.expected_token)

    def validate_authorization_header(self, header_value: str | None) -> AuthContext:
        if self.auth_mode in {"signed_user_token", "signed_or_shared"}:
            return self._validate_signed_authorization_header(header_value)

        if self.auth_mode != "shared_token":
            raise AuthError(f"unsupported auth mode: {self.auth_mode}")

        if not self.expected_token:
            return AuthContext(mode="none")
        token = self._extract_bearer_token(header_value)
        if not hmac.compare_digest(token, self.expected_token):
            raise AuthError("invalid bearer token")
        return AuthContext(mode="shared_token")

    def _validate_signed_authorization_header(self, header_value: str | None) -> AuthContext:
        token = self._extract_bearer_token(header_value)
        if self.auth_mode == "signed_or_shared" and self.expected_token and hmac.compare_digest(
            token, self.expected_token
        ):
            return AuthContext(mode="shared_token")
        claims = self._decode_signed_token(token)
        token_id = _claim_string(claims, "jti", required=False)
        if token_id and token_id.lower() in self.revoked_token_ids:
            raise AuthError("signed token has been revoked")
        user_id = _claim_string(claims, "sub", required=True)
        email = _claim_string(claims, "email", required=False)
        if self.allowed_users and self.user_registry is None:
            principals = {principal.lower() for principal in [user_id, email] if principal}
            if not principals.intersection(self.allowed_users):
                raise AuthError("user is not allowed to use transcription")
        scopes = _claim_scopes(claims)
        if self.required_scope and self.required_scope not in scopes:
            raise AuthError("signed token does not include required scope")
        if self.user_registry is not None:
            try:
                self.user_registry.validate_user_token(
                    user_id=user_id,
                    email=email,
                    token_id=token_id,
                )
            except UserRegistryError as exc:
                raise AuthError(str(exc)) from exc
        return AuthContext(
            mode="signed_user_token",
            user_id=user_id,
            email=email,
            scopes=tuple(scopes),
            token_id=token_id,
        )

    def _extract_bearer_token(self, header_value: str | None) -> str:
        if not header_value:
            raise AuthError("missing Authorization header")
        scheme, _, token = header_value.partition(" ")
        if scheme.lower() != "bearer" or not token:
            raise AuthError("Authorization header must use Bearer token")
        return token.strip()

    def _decode_signed_token(self, token: str) -> Mapping[str, Any]:
        if not self.signing_secret:
            raise AuthError("signed token auth is not configured")
        parts = token.split(".")
        if len(parts) != 3:
            raise AuthError("signed token must have three JWT parts")
        header_b64, payload_b64, signature_b64 = parts
        signing_input = f"{header_b64}.{payload_b64}".encode("ascii")
        expected_signature = hmac.new(
            self.signing_secret.encode("utf-8"),
            signing_input,
            hashlib.sha256,
        ).digest()
        try:
            supplied_signature = _base64url_decode(signature_b64)
        except AuthError:
            raise
        if not hmac.compare_digest(supplied_signature, expected_signature):
            raise AuthError("signed token signature is invalid")

        try:
            header = json.loads(_base64url_decode(header_b64))
            claims = json.loads(_base64url_decode(payload_b64))
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            raise AuthError("signed token JSON is invalid") from exc
        if not isinstance(header, dict) or not isinstance(claims, dict):
            raise AuthError("signed token parts must be JSON objects")
        if header.get("alg") != "HS256":
            raise AuthError("signed token must use HS256")
        if header.get("typ") not in (None, "JWT"):
            raise AuthError("signed token type is unsupported")

        now = int(time.time())
        exp = _claim_int(claims, "exp", required=True)
        if exp <= now:
            raise AuthError("signed token has expired")
        nbf = _claim_int(claims, "nbf", required=False)
        if nbf is not None and nbf > now:
            raise AuthError("signed token is not active yet")
        iat = _claim_int(claims, "iat", required=False)
        if iat is not None and iat > now + 60:
            raise AuthError("signed token issue time is in the future")
        if self.token_issuer and claims.get("iss") != self.token_issuer:
            raise AuthError("signed token issuer is invalid")
        if self.token_audience:
            aud = claims.get("aud")
            valid_audience = aud == self.token_audience or (
                isinstance(aud, list) and self.token_audience in aud
            )
            if not valid_audience:
                raise AuthError("signed token audience is invalid")
        return claims


def mint_signed_user_token(
    *,
    signing_secret: str,
    subject: str,
    email: str | None = None,
    scopes: tuple[str, ...] = ("transcription:stream",),
    ttl_seconds: int = 3600,
    issuer: str = "cubicle-transcription",
    audience: str = "cubicle-macos",
    token_id: str | None = None,
    now: datetime | None = None,
) -> str:
    if not signing_secret.strip():
        raise ValueError("signing_secret is required")
    if not subject.strip():
        raise ValueError("subject is required")
    issued_at = now or datetime.now(timezone.utc)
    expires_at = issued_at + timedelta(seconds=ttl_seconds)
    claims: dict[str, Any] = {
        "iss": issuer,
        "aud": audience,
        "sub": subject.strip(),
        "iat": int(issued_at.timestamp()),
        "nbf": int(issued_at.timestamp()),
        "exp": int(expires_at.timestamp()),
        "scope": " ".join(scopes),
    }
    if email and email.strip():
        claims["email"] = email.strip()
    if token_id and token_id.strip():
        claims["jti"] = token_id.strip()
    header = {"alg": "HS256", "typ": "JWT"}
    header_b64 = _base64url_encode(json.dumps(header, sort_keys=True, separators=(",", ":")).encode("utf-8"))
    payload_b64 = _base64url_encode(json.dumps(claims, sort_keys=True, separators=(",", ":")).encode("utf-8"))
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")
    signature = hmac.new(signing_secret.encode("utf-8"), signing_input, hashlib.sha256).digest()
    return f"{header_b64}.{payload_b64}.{_base64url_encode(signature)}"


def _claim_string(claims: Mapping[str, Any], key: str, *, required: bool) -> str | None:
    value = claims.get(key)
    if value is None and not required:
        return None
    if not isinstance(value, str) or not value.strip():
        raise AuthError(f"signed token claim {key} must be a non-empty string")
    return value.strip()


def _claim_int(claims: Mapping[str, Any], key: str, *, required: bool) -> int | None:
    value = claims.get(key)
    if value is None and not required:
        return None
    if not isinstance(value, int):
        raise AuthError(f"signed token claim {key} must be an integer")
    return value


def _claim_scopes(claims: Mapping[str, Any]) -> frozenset[str]:
    scope = claims.get("scope")
    if isinstance(scope, str):
        return frozenset(part for part in scope.split() if part)
    scp = claims.get("scp")
    if isinstance(scp, list) and all(isinstance(item, str) for item in scp):
        return frozenset(item for item in scp if item)
    return frozenset()
