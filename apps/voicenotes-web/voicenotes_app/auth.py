"""Authentication, session, local user, and Cognito/OIDC helpers."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import asyncio
import base64
import hashlib
import hmac
import json
import logging
import secrets
import time
from typing import Any
from urllib.parse import urlencode

import httpx
import jwt

from .models import User
from .settings import Settings


logger = logging.getLogger("voicenotes.auth")


class AuthError(Exception):
    """Authentication failure safe to surface as a rejected login/session."""

    pass


@dataclass(frozen=True)
class _AccessDecision:
    """Cached Cognito access verdict with an expiry timestamp."""

    allowed: bool
    expires_at: float


@dataclass(frozen=True)
class LocalCredential:
    """Configured local credential entry parsed from environment."""

    email: str
    password_or_hash: str
    display_name: str


class LocalUserDirectory:
    """Local email/password directory for development or small deployments."""

    def __init__(self, users_config: str, allowed_domains: tuple[str, ...] = ()) -> None:
        self._users = self._parse(users_config)
        self._allowed_domains = allowed_domains

    def authenticate(self, email: str, password: str) -> User:
        normalized = email.strip().lower()
        credential = self._users.get(normalized)
        if credential is None or not self._domain_allowed(normalized):
            raise AuthError("invalid credentials")
        if not _verify_password(password, credential.password_or_hash):
            raise AuthError("invalid credentials")
        return User.from_email(credential.email, credential.display_name)

    def _domain_allowed(self, email: str) -> bool:
        if not self._allowed_domains:
            return True
        domain = email.rsplit("@", 1)[-1].lower()
        return domain in self._allowed_domains

    @staticmethod
    def _parse(users_config: str) -> dict[str, LocalCredential]:
        users: dict[str, LocalCredential] = {}
        for raw_entry in users_config.split(","):
            entry = raw_entry.strip()
            if not entry:
                continue
            parts = entry.split(":", 2)
            if len(parts) < 2:
                continue
            email = parts[0].strip().lower()
            password_or_hash = parts[1].strip()
            display_name = parts[2].strip() if len(parts) == 3 and parts[2].strip() else email
            users[email] = LocalCredential(email=email, password_or_hash=password_or_hash, display_name=display_name)
        return users


class SessionCodec:
    """HMAC session cookie codec with embedded expiry and user identity."""

    def __init__(self, secret: str, ttl_seconds: int) -> None:
        if not secret:
            raise ValueError("session secret must be configured")
        self._secret = secret.encode("utf-8")
        self._ttl_seconds = ttl_seconds

    def encode(self, user: User) -> str:
        now = datetime.now(timezone.utc)
        payload = {
            "sub": user.user_id,
            "email": user.email,
            "display_name": user.display_name,
            "role": user.role,
            "iat": int(now.timestamp()),
            "exp": int((now + timedelta(seconds=self._ttl_seconds)).timestamp()),
            "nonce": secrets.token_urlsafe(12),
        }
        if user.auth_subject:
            payload["auth_subject"] = user.auth_subject
        body = _b64url(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8"))
        signature = _b64url(hmac.new(self._secret, body.encode("ascii"), hashlib.sha256).digest())
        return f"{body}.{signature}"

    def decode(self, token: str) -> User:
        try:
            body, signature = token.split(".", 1)
        except ValueError as exc:
            raise AuthError("invalid session") from exc
        expected = _b64url(hmac.new(self._secret, body.encode("ascii"), hashlib.sha256).digest())
        if not hmac.compare_digest(signature, expected):
            raise AuthError("invalid session")
        try:
            payload = json.loads(_b64url_decode(body))
        except (ValueError, json.JSONDecodeError) as exc:
            raise AuthError("invalid session") from exc
        exp = int(payload.get("exp", 0))
        if exp < int(datetime.now(timezone.utc).timestamp()):
            raise AuthError("expired session")
        return User(
            user_id=str(payload["sub"]),
            email=str(payload["email"]),
            display_name=str(payload.get("display_name") or payload["email"]),
            role=str(payload.get("role") or "user"),
            auth_subject=str(payload["auth_subject"]) if payload.get("auth_subject") else None,
        )


class CognitoUserAccessValidator:
    """Checks that an OIDC-authenticated user still exists and is confirmed."""

    def __init__(self, settings: Settings, cognito_client: Any | None = None) -> None:
        self._settings = settings
        self._client = cognito_client
        self._enabled = settings.auth_mode == "oidc" and settings.oidc_session_validation_enabled
        self._user_pool_id = _oidc_user_pool_id(settings)
        self._cache_ttl_seconds = max(0, settings.oidc_session_validation_ttl_seconds)
        self._cache: dict[str, _AccessDecision] = {}

    async def validate(self, user: User, *, use_cache: bool = True) -> None:
        if not self._enabled:
            return
        if not self._user_pool_id:
            raise AuthError("OIDC user pool is not configured")

        email = user.email.strip().lower()
        if not email:
            raise AuthError("missing user email")
        subject = (user.auth_subject or "").strip()
        if not subject:
            raise AuthError("missing user subject")
        cache_key = f"{email}\0{subject}"

        now = time.monotonic()
        cached = self._cache.get(cache_key)
        if use_cache and cached and cached.expires_at > now:
            if cached.allowed:
                return
            raise AuthError("user no longer has VoiceNotes access")

        try:
            allowed = await asyncio.wait_for(
                asyncio.to_thread(self._user_has_access, email, subject),
                timeout=max(1, self._settings.oidc_session_validation_request_timeout_seconds),
            )
        except TimeoutError as exc:
            raise AuthError("Cognito user access validation timed out") from exc
        self._cache[cache_key] = _AccessDecision(allowed=allowed, expires_at=now + self._cache_ttl_seconds)
        if not allowed:
            raise AuthError("user no longer has VoiceNotes access")

    def _user_has_access(self, email: str, subject: str) -> bool:
        try:
            response = self._cognito_client().list_users(
                UserPoolId=self._user_pool_id,
                Filter=_cognito_email_filter(email),
                Limit=5,
            )
        except Exception as exc:
            logger.warning(
                "cognito_user_access_validation_failed email_domain=%s subject_present=%s error_type=%s",
                email.rsplit("@", 1)[-1] if "@" in email else "",
                bool(subject),
                type(exc).__name__,
                exc_info=True,
            )
            raise AuthError("Cognito user access validation failed") from exc

        logger.info(
            "cognito_user_access_validation_result email_domain=%s subject_present=%s candidates=%d",
            email.rsplit("@", 1)[-1] if "@" in email else "",
            bool(subject),
            len(response.get("Users", [])),
        )
        for candidate in response.get("Users", []):
            attributes = {
                str(attribute.get("Name")): str(attribute.get("Value"))
                for attribute in candidate.get("Attributes", [])
            }
            if attributes.get("email", "").strip().lower() != email:
                continue
            if attributes.get("sub", "").strip() != subject:
                continue
            if not bool(candidate.get("Enabled", False)):
                continue
            if str(candidate.get("UserStatus") or "").upper() != "CONFIRMED":
                continue
            return True
        return False

    def _cognito_client(self) -> Any:
        if self._client is None:
            import boto3
            from botocore.config import Config

            request_timeout = max(1, self._settings.oidc_session_validation_request_timeout_seconds)
            connect_timeout = min(5, max(1, request_timeout - 1))
            read_timeout = max(1, request_timeout - connect_timeout)
            client_kwargs = {
                "region_name": self._settings.aws_region,
                "config": Config(
                    connect_timeout=connect_timeout,
                    read_timeout=read_timeout,
                    retries={"max_attempts": 1},
                ),
            }
            if self._settings.oidc_cognito_idp_endpoint_url:
                client_kwargs["endpoint_url"] = self._settings.oidc_cognito_idp_endpoint_url
            self._client = boto3.client("cognito-idp", **client_kwargs)
        return self._client


class OIDCClient:
    """Minimal OIDC authorization/token/logout client for the web app."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    def authorization_url(self, state: str, nonce: str) -> str:
        endpoint = self._settings.oidc_authorization_endpoint
        if not endpoint:
            raise AuthError("OIDC authorization endpoint is not configured")
        query = urlencode(
            {
                "client_id": self._settings.oidc_client_id,
                "redirect_uri": self._settings.oidc_redirect_uri,
                "response_type": "code",
                "scope": "openid email profile",
                "state": state,
                "nonce": nonce,
            }
        )
        return f"{endpoint}?{query}"

    def logout_url(self) -> str:
        endpoint = self._settings.oidc_logout_url
        if not endpoint or not self._settings.oidc_client_id:
            return ""
        logout_uri = _oidc_logout_redirect_uri(self._settings)
        if not logout_uri:
            return ""
        query = urlencode(
            {
                "client_id": self._settings.oidc_client_id,
                "logout_uri": logout_uri,
            }
        )
        return f"{endpoint}?{query}"

    async def exchange_code(self, code: str, expected_nonce: str) -> User:
        if not self._settings.oidc_token_endpoint:
            raise AuthError("OIDC token endpoint is not configured")
        try:
            async with httpx.AsyncClient(timeout=10.0) as client:
                response = await client.post(
                    self._settings.oidc_token_endpoint,
                    data={
                        "grant_type": "authorization_code",
                        "code": code,
                        "redirect_uri": self._settings.oidc_redirect_uri,
                        "client_id": self._settings.oidc_client_id,
                    },
                    auth=(self._settings.oidc_client_id, self._settings.oidc_client_secret)
                    if self._settings.oidc_client_secret
                    else None,
                )
        except httpx.HTTPError as exc:
            raise AuthError("OIDC token exchange failed") from exc
        if response.status_code >= 400:
            raise AuthError("OIDC token exchange failed")
        tokens = response.json()
        id_token = tokens.get("id_token")
        access_token = tokens.get("access_token")
        if not id_token and not access_token:
            raise AuthError("OIDC token response missing identity")
        nonce_checked = False
        try:
            claims = await self._decode_id_token(id_token) if id_token else await self._fetch_userinfo(access_token)
        except httpx.RequestError as exc:
            if not access_token:
                raise AuthError("OIDC userinfo unavailable") from exc
            if id_token:
                self._validate_unverified_nonce(id_token, expected_nonce)
                nonce_checked = bool(expected_nonce)
            claims = await self._fetch_userinfo(access_token)
        if expected_nonce and not nonce_checked and claims.get("nonce") != expected_nonce:
            raise AuthError("OIDC nonce mismatch")
        email = str(claims.get("email") or claims.get("preferred_username") or claims.get("sub"))
        display_name = str(claims.get("name") or email)
        auth_subject = str(claims.get("sub") or "").strip() or None
        return User.from_email(email, display_name, auth_subject=auth_subject)

    async def _decode_id_token(self, id_token: str) -> dict[str, Any]:
        if not self._settings.oidc_jwks_uri:
            raise AuthError("OIDC JWKS URI is not configured")
        async with httpx.AsyncClient(timeout=10.0) as client:
            response = await client.get(self._settings.oidc_jwks_uri)
        response.raise_for_status()
        jwks = response.json()
        header = jwt.get_unverified_header(id_token)
        key = next((candidate for candidate in jwks.get("keys", []) if candidate.get("kid") == header.get("kid")), None)
        if key is None:
            raise AuthError("OIDC signing key not found")
        public_key = jwt.algorithms.RSAAlgorithm.from_jwk(json.dumps(key))
        return jwt.decode(
            id_token,
            public_key,
            algorithms=[header.get("alg", "RS256")],
            audience=self._settings.oidc_client_id,
            issuer=self._settings.oidc_issuer or None,
            options={"verify_iss": bool(self._settings.oidc_issuer)},
        )

    async def _fetch_userinfo(self, access_token: str | None) -> dict[str, Any]:
        if not access_token:
            raise AuthError("OIDC access_token missing")
        endpoint = self._userinfo_endpoint()
        try:
            async with httpx.AsyncClient(timeout=10.0) as client:
                response = await client.get(endpoint, headers={"Authorization": f"Bearer {access_token}"})
        except httpx.HTTPError as exc:
            raise AuthError("OIDC userinfo request failed") from exc
        if response.status_code >= 400:
            raise AuthError("OIDC userinfo request failed")
        claims = response.json()
        if not isinstance(claims, dict):
            raise AuthError("OIDC userinfo response invalid")
        return claims

    def _userinfo_endpoint(self) -> str:
        if not self._settings.oidc_token_endpoint:
            raise AuthError("OIDC token endpoint is not configured")
        base, _, _ = self._settings.oidc_token_endpoint.rpartition("/")
        if not base:
            raise AuthError("OIDC token endpoint is invalid")
        return f"{base}/userInfo"

    def _validate_unverified_nonce(self, id_token: str, expected_nonce: str) -> None:
        if not expected_nonce:
            return
        claims = _decode_unverified_jwt_claims(id_token)
        if claims.get("nonce") != expected_nonce:
            raise AuthError("OIDC nonce mismatch")


def user_from_alb_headers(headers: dict[str, str] | Any) -> User:
    email = (
        headers.get("x-amzn-oidc-accesstoken-email")
        or headers.get("x-amzn-oidc-email")
        or headers.get("x-forwarded-user")
        or headers.get("x-amzn-oidc-identity")
    )
    if not email:
        raise AuthError("missing ALB/Cognito identity")
    return User.from_email(str(email), str(email))


def _verify_password(password: str, password_or_hash: str) -> bool:
    if password_or_hash.startswith("pbkdf2_sha256$"):
        try:
            _, iterations_raw, salt, expected = password_or_hash.split("$", 3)
            iterations = int(iterations_raw)
        except ValueError:
            return False
        actual = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt.encode("utf-8"), iterations)
        return hmac.compare_digest(_b64url(actual), expected)
    return hmac.compare_digest(password, password_or_hash)


def hash_password(password: str, *, iterations: int = 210_000) -> str:
    salt = secrets.token_urlsafe(16)
    digest = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt.encode("utf-8"), iterations)
    return f"pbkdf2_sha256${iterations}${salt}${_b64url(digest)}"


def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode("ascii").rstrip("=")


def _b64url_decode(data: str) -> bytes:
    padding = "=" * (-len(data) % 4)
    return base64.urlsafe_b64decode(data + padding)


def _oidc_user_pool_id(settings: Settings) -> str:
    if settings.oidc_user_pool_id:
        return settings.oidc_user_pool_id
    issuer = settings.oidc_issuer.rstrip("/")
    if not issuer or "/" not in issuer:
        return ""
    return issuer.rsplit("/", 1)[-1]


def _oidc_logout_redirect_uri(settings: Settings) -> str:
    redirect_uri = settings.oidc_redirect_uri.strip()
    if not redirect_uri:
        return ""
    callback_suffix = "/auth/callback"
    if redirect_uri.endswith(callback_suffix):
        return f"{redirect_uri[: -len(callback_suffix)]}/login"
    return redirect_uri


def _cognito_email_filter(email: str) -> str:
    escaped = email.replace("\\", "\\\\").replace('"', '\\"')
    return f'email = "{escaped}"'


def _decode_unverified_jwt_claims(token: str) -> dict[str, Any]:
    parts = token.split(".")
    if len(parts) != 3:
        raise AuthError("OIDC id_token invalid")
    try:
        claims = json.loads(_b64url_decode(parts[1]))
    except (ValueError, json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise AuthError("OIDC id_token invalid") from exc
    return claims if isinstance(claims, dict) else {}
