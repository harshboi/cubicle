from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import base64
import html
import hmac
import hashlib
import json
import os
import secrets
import uuid
from typing import Any
from urllib.parse import quote

try:
    from fastapi import APIRouter, HTTPException, Request
    from fastapi.responses import HTMLResponse, JSONResponse, RedirectResponse
except ImportError:
    APIRouter = None  # type: ignore[assignment]
    HTTPException = None  # type: ignore[assignment]
    Request = None  # type: ignore[assignment]
    HTMLResponse = None  # type: ignore[assignment]
    JSONResponse = None  # type: ignore[assignment]
    RedirectResponse = None  # type: ignore[assignment]

from .admin_store import (
    AdminAudioTuningConfig,
    AdminStore,
    AdminStoreError,
    AdminHistoryEvent,
    AdminTokenRecord,
    AdminUsageSummary,
    AdminUserRecord,
    DynamoDBAdminStore,
    InMemoryAdminStore,
)
from .auth import mint_signed_user_token


SESSION_COOKIE_NAME = "cubicle_admin_session"
TOKEN_TTL_COOKIE_NAME = "cubicle_admin_token_ttl_seconds"
ALB_AUTH_COOKIE_NAMES = (
    "CubicleAdminAuth",
    "CubicleAdminAuth-0",
    "CubicleAdminAuth-1",
    "CubicleAdminAuth-2",
    "CubicleAdminAuth-3",
    "AWSELBAuthSessionCookie",
    "AWSELBAuthSessionCookie-0",
    "AWSELBAuthSessionCookie-1",
    "AWSELBAuthSessionCookie-2",
    "AWSELBAuthSessionCookie-3",
)
DEFAULT_ADMIN_SESSION_TTL_SECONDS = 900
DEFAULT_USER_TOKEN_TTL_SECONDS = 86_400
VOICENOTES_TEMP_PASSWORD_LENGTH = 18
VOICENOTES_TEMP_PASSWORD_LOWER = "abcdefghijkmnopqrstuvwxyz"
VOICENOTES_TEMP_PASSWORD_UPPER = "ABCDEFGHJKLMNPQRSTUVWXYZ"
VOICENOTES_TEMP_PASSWORD_DIGITS = "23456789"
VOICENOTES_TEMP_PASSWORD_SYMBOLS = "!@#%*-_+="


@dataclass(frozen=True)
class AdminSettings:
    enabled: bool = False
    admin_token: str | None = None
    session_secret: str | None = None
    user_token_signing_secret: str | None = None
    external_auth_provider: str = "private"
    required_admin_group: str | None = None
    cognito_logout_url: str | None = None
    allowed_admin_emails: frozenset[str] = frozenset()
    issuer: str = "cubicle-transcription"
    audience: str = "cubicle-macos"
    required_scope: str = "transcription:stream"
    session_ttl_seconds: int = DEFAULT_ADMIN_SESSION_TTL_SECONDS
    default_user_token_ttl_seconds: int = DEFAULT_USER_TOKEN_TTL_SECONDS
    cookie_secure: bool = True
    store_backend: str = "memory"
    region_name: str | None = None
    user_table_name: str | None = None
    token_ledger_table_name: str | None = None
    audit_table_name: str | None = None
    admin_cognito_user_pool_id: str | None = None
    admin_cognito_client_id: str | None = None
    admin_cognito_region: str | None = None
    voicenotes_cognito_user_pool_id: str | None = None
    voicenotes_cognito_region: str | None = None
    voicenotes_admin_lambda_name: str | None = None
    voicenotes_admin_lambda_region: str | None = None
    voicenotes_notes_table_name: str | None = None
    voicenotes_audit_table_name: str | None = None
    voicenotes_transcript_bucket_name: str | None = None

    @classmethod
    def from_environment(cls) -> "AdminSettings":
        enabled = _env_bool("TRANSCRIPTION_ADMIN_ENABLED", default=False)
        region_name = (
            os.environ.get("TRANSCRIPTION_ADMIN_REGION")
            or os.environ.get("AWS_REGION")
            or os.environ.get("AWS_DEFAULT_REGION")
            or ""
        ).strip() or None
        settings = cls(
            enabled=enabled,
            admin_token=_read_secret("TRANSCRIPTION_ADMIN_TOKEN", "TRANSCRIPTION_ADMIN_TOKEN_FILE"),
            session_secret=_read_secret(
                "TRANSCRIPTION_ADMIN_SESSION_SECRET",
                "TRANSCRIPTION_ADMIN_SESSION_SECRET_FILE",
            ),
            user_token_signing_secret=_read_secret(
                "TRANSCRIPTION_TOKEN_SIGNING_SECRET",
                "TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE",
            ),
            external_auth_provider=os.environ.get(
                "TRANSCRIPTION_ADMIN_EXTERNAL_AUTH_PROVIDER", "private"
            )
            .strip()
            .lower()
            or "private",
            required_admin_group=(
                os.environ.get("TRANSCRIPTION_ADMIN_REQUIRED_GROUP", "").strip() or None
            ),
            cognito_logout_url=(
                os.environ.get("TRANSCRIPTION_ADMIN_COGNITO_LOGOUT_URL", "").strip() or None
            ),
            allowed_admin_emails=_split_csv(os.environ.get("TRANSCRIPTION_ADMIN_ALLOWED_EMAILS")),
            issuer=os.environ.get("TRANSCRIPTION_TOKEN_ISSUER", "cubicle-transcription").strip()
            or "cubicle-transcription",
            audience=os.environ.get("TRANSCRIPTION_TOKEN_AUDIENCE", "cubicle-macos").strip()
            or "cubicle-macos",
            required_scope=os.environ.get("TRANSCRIPTION_REQUIRED_SCOPE", "transcription:stream").strip()
            or "transcription:stream",
            session_ttl_seconds=max(
                60,
                _env_int("TRANSCRIPTION_ADMIN_SESSION_TTL_SECONDS", DEFAULT_ADMIN_SESSION_TTL_SECONDS),
            ),
            default_user_token_ttl_seconds=max(
                60,
                _env_int("TRANSCRIPTION_ADMIN_DEFAULT_USER_TOKEN_TTL_SECONDS", DEFAULT_USER_TOKEN_TTL_SECONDS),
            ),
            cookie_secure=_env_bool("TRANSCRIPTION_ADMIN_COOKIE_SECURE", default=True),
            store_backend=os.environ.get("TRANSCRIPTION_ADMIN_STORE_BACKEND", "memory").strip().lower()
            or "memory",
            region_name=region_name,
            user_table_name=os.environ.get("TRANSCRIPTION_USER_REGISTRY_TABLE", "").strip() or None,
            token_ledger_table_name=os.environ.get("TRANSCRIPTION_TOKEN_LEDGER_TABLE", "").strip() or None,
            audit_table_name=os.environ.get("TRANSCRIPTION_ADMIN_AUDIT_TABLE", "").strip() or None,
            admin_cognito_user_pool_id=(
                os.environ.get("TRANSCRIPTION_ADMIN_COGNITO_USER_POOL_ID", "").strip() or None
            ),
            admin_cognito_client_id=(
                os.environ.get("TRANSCRIPTION_ADMIN_COGNITO_CLIENT_ID", "").strip() or None
            ),
            admin_cognito_region=(
                os.environ.get("TRANSCRIPTION_ADMIN_COGNITO_REGION", "").strip() or region_name
            ),
            voicenotes_cognito_user_pool_id=(
                os.environ.get("VOICENOTES_COGNITO_USER_POOL_ID", "").strip() or None
            ),
            voicenotes_cognito_region=(
                os.environ.get("VOICENOTES_COGNITO_REGION", "").strip() or region_name
            ),
            voicenotes_admin_lambda_name=(
                os.environ.get("VOICENOTES_ADMIN_LAMBDA_NAME", "").strip() or None
            ),
            voicenotes_admin_lambda_region=(
                os.environ.get("VOICENOTES_ADMIN_LAMBDA_REGION", "").strip() or region_name
            ),
            voicenotes_notes_table_name=(
                os.environ.get("VOICENOTES_NOTES_TABLE", "").strip() or None
            ),
            voicenotes_audit_table_name=(
                os.environ.get("VOICENOTES_AUDIT_TABLE", "").strip() or None
            ),
            voicenotes_transcript_bucket_name=(
                os.environ.get("VOICENOTES_TRANSCRIPT_BUCKET", "").strip() or None
            ),
        )
        if settings.enabled:
            settings.validate_enabled()
        return settings

    def validate_enabled(self) -> None:
        if not self.session_secret:
            raise RuntimeError(
                "TRANSCRIPTION_ADMIN_SESSION_SECRET or TRANSCRIPTION_ADMIN_SESSION_SECRET_FILE is required"
            )
        if not self.user_token_signing_secret:
            raise RuntimeError(
                "TRANSCRIPTION_TOKEN_SIGNING_SECRET or TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE is required"
            )

    def create_store(self) -> AdminStore:
        if self.store_backend in {"memory", "inmemory", "local"}:
            return InMemoryAdminStore()
        if self.store_backend != "dynamodb":
            raise RuntimeError(f"unsupported admin store backend: {self.store_backend}")
        if not self.user_table_name or not self.token_ledger_table_name:
            raise RuntimeError(
                "TRANSCRIPTION_USER_REGISTRY_TABLE and TRANSCRIPTION_TOKEN_LEDGER_TABLE are required"
            )
        try:
            import boto3  # type: ignore[import-not-found]
        except ImportError as exc:
            raise RuntimeError("boto3 is required for DynamoDB admin store") from exc
        return DynamoDBAdminStore(
            client=boto3.client("dynamodb", region_name=self.region_name),
            user_table_name=self.user_table_name,
            token_ledger_table_name=self.token_ledger_table_name,
            audit_table_name=self.audit_table_name,
        )

    def runtime_status(self) -> dict[str, Any]:
        return {
            "enabled": self.enabled,
            "store_backend": self.store_backend,
            "user_table_configured": bool(self.user_table_name),
            "token_ledger_table_configured": bool(self.token_ledger_table_name),
            "audit_table_configured": bool(self.audit_table_name),
            "cookie_secure": self.cookie_secure,
            "session_ttl_seconds": self.session_ttl_seconds,
            "external_auth_provider": self.external_auth_provider,
            "required_admin_group": self.required_admin_group,
            "cognito_logout_configured": bool(self.cognito_logout_url),
            "admin_cognito_direct_configured": bool(
                self.admin_cognito_user_pool_id and self.admin_cognito_client_id
            ),
            "voicenotes_user_pool_configured": bool(self.voicenotes_cognito_user_pool_id),
            "voicenotes_admin_lambda_configured": bool(self.voicenotes_admin_lambda_name),
        }


@dataclass(frozen=True)
class AdminSession:
    auth_type: str
    csrf_token: str | None = None
    admin_subject: str | None = None
    admin_email: str | None = None

    @property
    def is_cookie(self) -> bool:
        return self.auth_type == "cookie"


@dataclass(frozen=True)
class VoiceNotesCognitoUser:
    username: str
    email: str
    display_name: str | None
    status: str
    enabled: bool
    created_at: str
    updated_at: str


def create_admin_router(
    *,
    settings: AdminSettings | None = None,
    store: AdminStore | None = None,
):
    if APIRouter is None or HTMLResponse is None or JSONResponse is None or RedirectResponse is None:
        raise RuntimeError("FastAPI runtime dependencies are not installed")
    settings = settings or AdminSettings.from_environment()
    if not settings.enabled:
        raise RuntimeError("admin console is not enabled")
    settings.validate_enabled()
    store = store or settings.create_store()
    router = APIRouter(prefix="/admin", tags=["admin"])

    @router.get("/login")
    async def login_page(request: Request):
        if settings.external_auth_provider == "cognito_alb":
            session_response = _maybe_start_external_session(request, settings, redirect_to="/admin")
            if session_response is not None:
                return session_response
            return RedirectResponse("/admin", status_code=303)
        return HTMLResponse(_layout("Cubicle Admin Login", _login_form()), status_code=200)

    @router.post("/login")
    async def login(request: Request):
        if settings.external_auth_provider == "cognito_alb":
            session_response = _maybe_start_external_session(request, settings, redirect_to="/admin")
            if session_response is not None:
                return session_response
            return RedirectResponse("/admin", status_code=303)
        form = await request.form()
        if settings.external_auth_provider == "cognito_direct":
            email = str(form.get("email") or "").strip().lower()
            password = str(form.get("password") or "")
            if not email or not password:
                return HTMLResponse(
                    _layout("Cubicle Admin Login", _login_form("Email and password are required.")),
                    status_code=401,
                )
            try:
                identity = _authenticate_direct_cognito_admin(
                    settings,
                    email=email,
                    password=password,
                )
            except AdminStoreError as exc:
                return HTMLResponse(
                    _layout("Cubicle Admin Login", _login_form(str(exc), email=email)),
                    status_code=401,
                )
            response = RedirectResponse("/admin", status_code=303)
            _set_admin_session_cookie(
                response,
                settings,
                admin_subject=identity.get("sub") or email,
                admin_email=identity.get("email") or email,
            )
            return response
        supplied_token = str(form.get("admin_token") or "")
        if not settings.admin_token or not hmac.compare_digest(supplied_token, settings.admin_token or ""):
            return HTMLResponse(_layout("Cubicle Admin Login", _login_form("Invalid admin credential.")), status_code=401)
        response = RedirectResponse("/admin", status_code=303)
        _set_admin_session_cookie(response, settings, admin_subject="admin", admin_email=None)
        return response

    @router.post("/logout")
    async def logout():
        redirect_to = (
            settings.cognito_logout_url
            if settings.external_auth_provider == "cognito_alb" and settings.cognito_logout_url
            else "/admin/login"
        )
        response = RedirectResponse(redirect_to, status_code=303)
        _clear_admin_auth_cookies(response)
        return response

    @router.get("/logout")
    async def logout_landing():
        response = RedirectResponse("/admin/login", status_code=303)
        _clear_admin_auth_cookies(response)
        return response

    @router.get("/health")
    async def admin_health(request: Request):
        _require_admin(request, settings)
        return JSONResponse({"status": "ok", "admin": settings.runtime_status()})

    @router.get("")
    async def index(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin")
        if not isinstance(session, AdminSession):
            return session
        return HTMLResponse(
            _render_index(
                users=store.list_users(),
                csrf_token=session.csrf_token or "",
                default_user_token_ttl_seconds=_dashboard_token_ttl_seconds(request, settings),
            )
        )

    @router.post("/token-duration")
    async def update_token_duration(request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        ttl_seconds = _safe_form_int(
            form.get("ttl_seconds"),
            default=settings.default_user_token_ttl_seconds,
            minimum=60,
            maximum=30 * 24 * 60 * 60,
        )
        response = RedirectResponse("/admin", status_code=303)
        response.set_cookie(
            TOKEN_TTL_COOKIE_NAME,
            str(ttl_seconds),
            httponly=True,
            secure=settings.cookie_secure,
            samesite="lax",
            max_age=settings.session_ttl_seconds,
            path="/admin",
        )
        return response

    @router.get("/users")
    async def list_users(request: Request):
        _require_admin(request, settings)
        return JSONResponse({"users": [_user_json(user) for user in store.list_users()]})

    @router.get("/usage")
    async def usage_lookup(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/usage")
        if not isinstance(session, AdminSession):
            return session
        email = str(request.query_params.get("email") or "").strip()
        if not email:
            return HTMLResponse(_layout("Usage Lookup", _render_usage_lookup(csrf_token=session.csrf_token or "")))
        try:
            users_by_email = {user.email: user for user in store.list_users()}
            normalized_email = email.lower()
            user = users_by_email.get(normalized_email)
            summary = store.usage_summary(email=normalized_email)
            tokens = store.list_tokens(email=normalized_email)
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return HTMLResponse(
            _layout(
                "Usage Lookup",
                _render_usage_lookup(
                    csrf_token=session.csrf_token or "",
                    email=normalized_email,
                    user=user,
                    summary=summary,
                    tokens=tokens,
                ),
            )
        )

    @router.get("/history")
    async def history(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/history")
        if not isinstance(session, AdminSession):
            return session
        limit = _safe_form_int(
            request.query_params.get("limit"),
            default=100,
            minimum=25,
            maximum=500,
        )
        try:
            events = store.list_history_events(limit=limit)
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return HTMLResponse(_layout("Admin History", _render_history(events=events, limit=limit)))

    @router.get("/audio-tuning")
    async def audio_tuning(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/audio-tuning")
        if not isinstance(session, AdminSession):
            return session
        try:
            stored_config = store.get_audio_tuning_config()
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return HTMLResponse(
            _layout(
                "Audio Tuning",
                _render_audio_tuning(
                    config=stored_config or AdminAudioTuningConfig(),
                    using_defaults=stored_config is None,
                    csrf_token=session.csrf_token or "",
                    saved=str(request.query_params.get("saved") or "") == "1",
                ),
            )
        )

    @router.post("/audio-tuning")
    async def update_audio_tuning(request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            config = AdminAudioTuningConfig(
                target_rms=_form_percent_fraction(
                    form.get("target_rms_percent"),
                    "target RMS",
                    minimum=1.0,
                    maximum=60.0,
                ),
                rms_floor=_form_percent_fraction(
                    form.get("rms_floor_percent"),
                    "RMS floor",
                    minimum=0.0,
                    maximum=60.0,
                ),
                max_gain=_form_float(
                    form.get("max_gain"),
                    "max server gain",
                    minimum=1.0,
                    maximum=64.0,
                ),
                peak_ceiling=_form_percent_fraction(
                    form.get("peak_ceiling_percent"),
                    "peak ceiling",
                    minimum=10.0,
                    maximum=99.0,
                ),
                updated_at=_format_dt(datetime.now(timezone.utc)),
                updated_by=session.admin_email or session.admin_subject or "admin",
            )
            saved_config = store.save_audio_tuning_config(config)
            _record_history(
                store,
                session=session,
                event_type="audio_tuning_updated",
                email="runtime-config@cubicle.local",
                detail=(
                    "Audio tuning updated: target RMS "
                    f"{_format_percent(saved_config.target_rms)}, floor {_format_percent(saved_config.rms_floor)}, "
                    f"max gain {_format_gain(saved_config.max_gain)}, peak ceiling "
                    f"{_format_percent(saved_config.peak_ceiling)}"
                ),
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/audio-tuning?saved=1", status_code=303)

    @router.get("/voicenotes-users")
    async def voicenotes_users(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        saved = str(request.query_params.get("saved") or "")
        try:
            users = _list_voicenotes_users(settings)
        except AdminStoreError as exc:
            return HTMLResponse(
                _layout(
                    "VoiceNotes Users",
                    _render_voicenotes_users(
                        users=[],
                        csrf_token=session.csrf_token or "",
                        user_pool_id=settings.voicenotes_cognito_user_pool_id,
                        notice=None,
                        error=str(exc),
                    ),
                ),
                status_code=400,
            )
        return HTMLResponse(
            _layout(
                "VoiceNotes Users",
                _render_voicenotes_users(
                    users=users,
                    csrf_token=session.csrf_token or "",
                    user_pool_id=settings.voicenotes_cognito_user_pool_id,
                    notice=_voicenotes_notice(saved),
                    error=None,
                ),
            )
        )

    @router.post("/voicenotes-users")
    async def add_voicenotes_user(request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        email = str(form.get("email") or "").strip().lower()
        display_name = str(form.get("display_name") or "").strip() or None
        if not email:
            return HTMLResponse(_render_error("Email is required."), status_code=400)
        try:
            _create_voicenotes_user(settings, email=email, display_name=display_name)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_user_created",
                email=email,
                detail="VoiceNotes Cognito user created; Cognito delivered the invite email.",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/voicenotes-users?saved=created", status_code=303)

    @router.post("/voicenotes-users/{username}/enable")
    async def enable_voicenotes_user(username: str, request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            user = _get_voicenotes_user(settings, username)
            _run_voicenotes_admin_action(settings, "admin_enable_user", username)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_user_enabled",
                email=user.email,
                detail="VoiceNotes Cognito user enabled.",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/voicenotes-users?saved=enabled", status_code=303)

    @router.post("/voicenotes-users/{username}/disable")
    async def disable_voicenotes_user(username: str, request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            user = _get_voicenotes_user(settings, username)
            _run_voicenotes_admin_action(settings, "admin_disable_user", username)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_user_disabled",
                email=user.email,
                detail="VoiceNotes Cognito user disabled.",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/voicenotes-users?saved=disabled", status_code=303)

    @router.post("/voicenotes-users/{username}/reset-password")
    async def reset_voicenotes_password(username: str, request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            user = _get_voicenotes_user(settings, username)
            was_pending_invite = user.status == "FORCE_CHANGE_PASSWORD"
            _run_voicenotes_admin_action(settings, "admin_reset_user_password", username)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_password_reset",
                email=user.email,
                detail=(
                    "VoiceNotes Cognito invite email resent."
                    if was_pending_invite
                    else "VoiceNotes Cognito password reset email triggered."
                ),
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        saved_code = "invite-resent" if was_pending_invite else "reset"
        return RedirectResponse(f"/admin/voicenotes-users?saved={saved_code}", status_code=303)

    @router.post("/voicenotes-users/{username}/sign-out")
    async def sign_out_voicenotes_user(username: str, request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            user = _get_voicenotes_user(settings, username)
            _run_voicenotes_admin_action(settings, "admin_user_global_sign_out", username)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_user_signed_out",
                email=user.email,
                detail="VoiceNotes Cognito refresh tokens invalidated.",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/voicenotes-users?saved=signed-out", status_code=303)

    @router.post("/voicenotes-users/{username}/delete")
    async def delete_voicenotes_user(username: str, request: Request):
        session = _require_admin_or_redirect(request, settings, redirect_to="/admin/voicenotes-users")
        if not isinstance(session, AdminSession):
            return session
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            user = _get_voicenotes_user(settings, username)
            _run_voicenotes_admin_action(settings, "admin_delete_user", username)
            _record_history(
                store,
                session=session,
                event_type="voicenotes_user_deleted",
                email=user.email,
                detail="VoiceNotes user and stored transcripts deleted.",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin/voicenotes-users?saved=deleted", status_code=303)

    @router.post("/users")
    async def add_user(request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            requested_email = str(form.get("email") or "")
            normalized_email = requested_email.lower().strip()
            previous_users = {user.email: user for user in store.list_users()}
            previous = previous_users.get(normalized_email)
            store.add_user(
                email=requested_email,
                display_name=str(form.get("display_name") or "") or None,
                role=str(form.get("role") or "transcription_user") or "transcription_user",
                notes=str(form.get("notes") or "") or None,
            )
            if previous is None:
                event_type = "user_added"
                detail = "User added from admin console"
            elif previous.status.lower() != "active":
                event_type = "user_activated"
                detail = f"User reactivated from {previous.status}"
            else:
                event_type = "user_updated"
                detail = "User record updated from admin console"
            _record_history(
                store,
                session=session,
                event_type=event_type,
                email=requested_email,
                detail=detail,
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    @router.post("/users/{email}/disable")
    async def disable_user(email: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            revoked = store.revoke_all_tokens(email=email, reason="user disabled")
            store.disable_user(email=email)
            _record_history(
                store,
                session=session,
                event_type="user_disabled",
                email=email,
                reason="user disabled",
                detail=f"Disabled user and revoked {len(revoked)} active token(s)",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    @router.post("/users/{email}/tokens/revoke-all")
    async def revoke_all_user_tokens(email: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            reason = str(form.get("reason") or "") or "revoked from admin console"
            revoked = store.revoke_all_tokens(
                email=email,
                reason=reason,
            )
            _record_history(
                store,
                session=session,
                event_type="tokens_revoked",
                email=email,
                reason=reason,
                detail=f"Revoked {len(revoked)} active token(s)",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    @router.post("/users/{email}/delete")
    async def delete_user(email: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            store.delete_user(email=email, delete_usage=True)
            _record_history(
                store,
                session=session,
                event_type="user_deleted",
                email=email,
                detail="User, tokens, and usage metadata deleted from admin console",
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    @router.post("/users/{email}/tokens")
    async def issue_token(email: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        ttl_seconds = _safe_form_int(
            form.get("ttl_seconds"),
            default=_dashboard_token_ttl_seconds(request, settings),
            minimum=60,
            maximum=30 * 24 * 60 * 60,
        )
        issued_at = datetime.now(timezone.utc)
        expires_at = issued_at + timedelta(seconds=ttl_seconds)
        token_id = str(uuid.uuid4())
        scope = settings.required_scope
        try:
            store.record_issued_token(
                email=email,
                token_id=token_id,
                scope=scope,
                issued_at=_format_dt(issued_at),
                expires_at=_format_dt(expires_at),
            )
            token = mint_signed_user_token(
                signing_secret=settings.user_token_signing_secret or "",
                subject=email,
                email=email,
                scopes=(scope,),
                ttl_seconds=ttl_seconds,
                issuer=settings.issuer,
                audience=settings.audience,
                token_id=token_id,
                now=issued_at,
            )
            _record_history(
                store,
                session=session,
                event_type="token_issued",
                email=email,
                token_id=token_id,
                detail=f"Issued token with TTL {ttl_seconds} seconds",
            )
        except (AdminStoreError, ValueError) as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return HTMLResponse(
            _layout(
                "Issued Token",
                _render_issued_token(
                    email=email,
                    token_id=token_id,
                    expires_at=_format_dt(expires_at),
                    token=token,
                    csrf_token=session.csrf_token or "",
                ),
            )
        )

    @router.post("/users/{email}/tokens/{token_id}/revoke")
    async def revoke_token(email: str, token_id: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        try:
            reason = str(form.get("reason") or "") or None
            store.revoke_token(
                email=email,
                token_id=token_id,
                reason=reason,
            )
            _record_history(
                store,
                session=session,
                event_type="token_revoked",
                email=email,
                token_id=token_id,
                reason=reason,
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    @router.post("/users/{email}/tokens/revoke")
    async def revoke_token_from_form(email: str, request: Request):
        session = _require_admin(request, settings)
        form = await request.form()
        _validate_csrf(session, str(form.get("csrf_token") or ""))
        token_id = str(form.get("token_id") or "")
        if not token_id.strip():
            return HTMLResponse(
                _render_error(
                    "Token ID is required. Use the short UUID shown as Token ID, not the long bearer token."
                ),
                status_code=400,
            )
        try:
            reason = str(form.get("reason") or "") or None
            store.revoke_token(
                email=email,
                token_id=token_id,
                reason=reason,
            )
            _record_history(
                store,
                session=session,
                event_type="token_revoked",
                email=email,
                token_id=token_id,
                reason=reason,
            )
        except AdminStoreError as exc:
            return HTMLResponse(_render_error(str(exc)), status_code=400)
        return RedirectResponse("/admin", status_code=303)

    return router


def _require_admin_or_redirect(request: Request, settings: AdminSettings, *, redirect_to: str = "/admin"):
    try:
        return _require_admin(request, settings)
    except HTTPException:
        session_response = _maybe_start_external_session(request, settings, redirect_to=redirect_to)
        if session_response is not None:
            return session_response
        if settings.external_auth_provider == "cognito_alb":
            return HTMLResponse(_render_external_auth_failure(settings), status_code=403)
        return RedirectResponse("/admin/login", status_code=303)


def _require_admin(request: Request, settings: AdminSettings) -> AdminSession:
    authorization = request.headers.get("authorization", "")
    scheme, _, token = authorization.partition(" ")
    if (
        settings.admin_token
        and scheme.lower() == "bearer"
        and token
        and hmac.compare_digest(token.strip(), settings.admin_token)
    ):
        return AdminSession(auth_type="bearer")
    cookie_value = request.cookies.get(SESSION_COOKIE_NAME)
    if cookie_value:
        session = _decode_admin_session(settings.session_secret or "", cookie_value)
        if session:
            return session
    raise HTTPException(status_code=401, detail="admin authentication required")


def _maybe_start_external_session(
    request: Request,
    settings: AdminSettings,
    *,
    redirect_to: str,
) -> RedirectResponse | None:
    if settings.external_auth_provider != "cognito_alb":
        return None
    identity = _cognito_alb_identity(request, settings)
    if identity is None:
        return None
    response = RedirectResponse(redirect_to, status_code=303)
    _set_admin_session_cookie(
        response,
        settings,
        admin_subject=identity.get("sub") or "cognito-admin",
        admin_email=identity.get("email"),
    )
    return response


def _cognito_alb_identity(request: Request, settings: AdminSettings) -> dict[str, Any] | None:
    claims = _decode_unverified_jwt_claims(request.headers.get("x-amzn-oidc-data"))
    subject = request.headers.get("x-amzn-oidc-identity") or _claim_text(claims, "sub")
    email = _claim_text(claims, "email")
    if not subject and not email:
        return None
    if settings.allowed_admin_emails:
        if not email or email.lower() not in settings.allowed_admin_emails:
            return None
    if settings.required_admin_group:
        groups = _claim_groups(claims)
        if settings.required_admin_group not in groups:
            return None
    return {"sub": subject, "email": email}


def _decode_unverified_jwt_claims(value: str | None) -> dict[str, Any]:
    if not value:
        return {}
    parts = value.split(".")
    if len(parts) != 3:
        return {}
    try:
        payload = json.loads(_base64url_decode(parts[1]))
    except (ValueError, json.JSONDecodeError, UnicodeDecodeError):
        return {}
    return payload if isinstance(payload, dict) else {}


def _claim_text(claims: dict[str, Any], name: str) -> str | None:
    value = claims.get(name)
    return value.strip() if isinstance(value, str) and value.strip() else None


def _claim_groups(claims: dict[str, Any]) -> frozenset[str]:
    value = claims.get("cognito:groups") or claims.get("groups")
    if isinstance(value, str):
        return frozenset(part.strip() for part in value.split(",") if part.strip())
    if isinstance(value, list):
        return frozenset(str(part).strip() for part in value if str(part).strip())
    return frozenset()


def _validate_csrf(session: AdminSession, supplied_token: str) -> None:
    if not session.is_cookie:
        return
    if not session.csrf_token or not hmac.compare_digest(session.csrf_token, supplied_token):
        raise HTTPException(status_code=403, detail="CSRF token is invalid")


def _set_admin_session_cookie(
    response: RedirectResponse,
    settings: AdminSettings,
    *,
    admin_subject: str,
    admin_email: str | None,
) -> None:
    response.set_cookie(
        SESSION_COOKIE_NAME,
        _encode_admin_session(
            settings.session_secret or "",
            ttl_seconds=settings.session_ttl_seconds,
            admin_subject=admin_subject,
            admin_email=admin_email,
        ),
        httponly=True,
        secure=settings.cookie_secure,
        samesite="lax",
        max_age=settings.session_ttl_seconds,
        path="/admin",
    )


def _clear_admin_auth_cookies(response: RedirectResponse) -> None:
    response.delete_cookie(SESSION_COOKIE_NAME, path="/admin")
    response.delete_cookie(TOKEN_TTL_COOKIE_NAME, path="/admin")
    for cookie_name in ALB_AUTH_COOKIE_NAMES:
        response.delete_cookie(cookie_name, path="/")
        response.delete_cookie(cookie_name, path="/admin")


def _encode_admin_session(
    secret: str,
    *,
    ttl_seconds: int,
    admin_subject: str = "admin",
    admin_email: str | None = None,
) -> str:
    if not secret:
        raise ValueError("session secret is required")
    csrf_token = secrets.token_urlsafe(24)
    payload = {
        "sub": admin_subject or "admin",
        "exp": int((datetime.now(timezone.utc) + timedelta(seconds=ttl_seconds)).timestamp()),
        "csrf": csrf_token,
    }
    if admin_email:
        payload["email"] = admin_email
    payload_b64 = _base64url_encode(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8"))
    signature = hmac.new(secret.encode("utf-8"), payload_b64.encode("ascii"), hashlib.sha256).digest()
    return f"{payload_b64}.{_base64url_encode(signature)}"


def _decode_admin_session(secret: str, cookie_value: str) -> AdminSession | None:
    if not secret:
        return None
    parts = cookie_value.split(".")
    if len(parts) != 2:
        return None
    payload_b64, signature_b64 = parts
    expected_signature = hmac.new(secret.encode("utf-8"), payload_b64.encode("ascii"), hashlib.sha256).digest()
    try:
        supplied_signature = _base64url_decode(signature_b64)
    except ValueError:
        return None
    if not hmac.compare_digest(supplied_signature, expected_signature):
        return None
    try:
        payload = json.loads(_base64url_decode(payload_b64))
    except (json.JSONDecodeError, UnicodeDecodeError, ValueError):
        return None
    if not isinstance(payload, dict):
        return None
    exp = payload.get("exp")
    csrf = payload.get("csrf")
    subject = payload.get("sub")
    email = payload.get("email")
    if not isinstance(exp, int) or exp <= int(datetime.now(timezone.utc).timestamp()):
        return None
    if not isinstance(csrf, str) or not csrf:
        return None
    if not isinstance(subject, str) or not subject:
        return None
    return AdminSession(
        auth_type="cookie",
        csrf_token=csrf,
        admin_subject=subject,
        admin_email=email if isinstance(email, str) and email else None,
    )


def _admin_cognito_client(settings: AdminSettings):
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError as exc:
        raise AdminStoreError("boto3 is required for Cognito admin login") from exc
    return boto3.client(
        "cognito-idp",
        region_name=settings.admin_cognito_region or settings.region_name,
    )


def _authenticate_direct_cognito_admin(
    settings: AdminSettings,
    *,
    email: str,
    password: str,
) -> dict[str, str | None]:
    if not settings.admin_cognito_user_pool_id or not settings.admin_cognito_client_id:
        raise AdminStoreError("Admin Cognito direct login is not configured.")
    if settings.allowed_admin_emails and email.lower() not in settings.allowed_admin_emails:
        raise AdminStoreError("This email is not allowed to use the Cubicle admin console.")
    client = _admin_cognito_client(settings)
    try:
        auth = client.admin_initiate_auth(
            UserPoolId=settings.admin_cognito_user_pool_id,
            ClientId=settings.admin_cognito_client_id,
            AuthFlow="ADMIN_USER_PASSWORD_AUTH",
            AuthParameters={
                "USERNAME": email,
                "PASSWORD": password,
            },
        )
        if auth.get("ChallengeName"):
            raise AdminStoreError(f"Cognito login requires {auth.get('ChallengeName')}.")
        user = client.admin_get_user(
            UserPoolId=settings.admin_cognito_user_pool_id,
            Username=email,
        )
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_admin_cognito_error_message(exc)) from exc
    attributes = {
        str(attribute.get("Name")): str(attribute.get("Value") or "")
        for attribute in user.get("UserAttributes", [])
        if isinstance(attribute, dict) and attribute.get("Name")
    }
    user_email = (attributes.get("email") or email).strip().lower()
    if settings.allowed_admin_emails and user_email not in settings.allowed_admin_emails:
        raise AdminStoreError("This Cognito user is not allowed to use the Cubicle admin console.")
    return {
        "sub": attributes.get("sub") or user.get("Username"),
        "email": user_email,
    }


def _admin_cognito_error_message(exc: Exception) -> str:
    response = getattr(exc, "response", None)
    if isinstance(response, dict):
        error = response.get("Error")
        if isinstance(error, dict):
            code = str(error.get("Code") or "Unknown")
            if code in {"NotAuthorizedException", "UserNotFoundException"}:
                return "Invalid admin email or password."
            if code == "PasswordResetRequiredException":
                return "Password reset is required before this admin account can sign in."
            if code == "UserNotConfirmedException":
                return "This admin Cognito account is not confirmed."
            return f"Cognito admin login failed ({code})."
    return "Cognito admin login failed."


def _voicenotes_user_pool_id(settings: AdminSettings) -> str:
    if not settings.voicenotes_cognito_user_pool_id:
        raise AdminStoreError("VOICENOTES_COGNITO_USER_POOL_ID is not configured for this admin service.")
    return settings.voicenotes_cognito_user_pool_id


def _voicenotes_client(settings: AdminSettings):
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError as exc:
        raise AdminStoreError("boto3 is required to manage VoiceNotes Cognito users") from exc
    return boto3.client(
        "cognito-idp",
        region_name=settings.voicenotes_cognito_region or settings.region_name,
    )


def _voicenotes_lambda_client(settings: AdminSettings):
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError as exc:
        raise AdminStoreError("boto3 is required to invoke the VoiceNotes admin Lambda") from exc
    return boto3.client(
        "lambda",
        region_name=settings.voicenotes_admin_lambda_region or settings.region_name,
    )


def _voicenotes_ddb_client(settings: AdminSettings):
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError as exc:
        raise AdminStoreError("boto3 is required to purge VoiceNotes user data") from exc
    return boto3.client(
        "dynamodb",
        region_name=settings.voicenotes_cognito_region or settings.region_name,
    )


def _voicenotes_s3_client(settings: AdminSettings):
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError as exc:
        raise AdminStoreError("boto3 is required to purge VoiceNotes user data") from exc
    return boto3.client(
        "s3",
        region_name=settings.voicenotes_cognito_region or settings.region_name,
    )


def _invoke_voicenotes_admin_lambda(settings: AdminSettings, payload: dict[str, Any]) -> dict[str, Any]:
    if not settings.voicenotes_admin_lambda_name:
        raise AdminStoreError("VOICENOTES_ADMIN_LAMBDA_NAME is not configured.")
    client = _voicenotes_lambda_client(settings)
    try:
        response = client.invoke(
            FunctionName=settings.voicenotes_admin_lambda_name,
            InvocationType="RequestResponse",
            Payload=json.dumps(payload).encode("utf-8"),
        )
        raw_payload = response.get("Payload")
        body_bytes = raw_payload.read() if raw_payload is not None else b"{}"
        result = json.loads(body_bytes.decode("utf-8"))
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_lambda_error_message(exc)) from exc
    except (json.JSONDecodeError, UnicodeDecodeError, OSError) as exc:
        raise AdminStoreError(f"VoiceNotes admin Lambda returned an invalid response: {exc}") from exc
    if not isinstance(result, dict):
        raise AdminStoreError("VoiceNotes admin Lambda returned an invalid response.")
    if response.get("FunctionError"):
        message = str(result.get("errorMessage") or result.get("message") or response.get("FunctionError"))
        raise AdminStoreError(f"VoiceNotes admin Lambda failed: {message}")
    if not result.get("ok"):
        code = str(result.get("error_code") or "Unknown")
        message = str(result.get("message") or "VoiceNotes admin Lambda request failed.")
        raise AdminStoreError(_voicenotes_admin_error_message(code, message))
    return result


def _list_voicenotes_users(settings: AdminSettings) -> list[VoiceNotesCognitoUser]:
    if settings.voicenotes_admin_lambda_name:
        result = _invoke_voicenotes_admin_lambda(settings, {"action": "list_users"})
        raw_users = result.get("users")
        if not isinstance(raw_users, list):
            raise AdminStoreError("VoiceNotes admin Lambda returned an invalid user list.")
        return [_voicenotes_user_from_cognito(user) for user in raw_users if isinstance(user, dict)]
    pool_id = _voicenotes_user_pool_id(settings)
    client = _voicenotes_client(settings)
    users: list[VoiceNotesCognitoUser] = []
    try:
        paginator = client.get_paginator("list_users")
        for page in paginator.paginate(UserPoolId=pool_id):
            users.extend(_voicenotes_user_from_cognito(user) for user in page.get("Users", []))
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_cognito_error_message(exc)) from exc
    return users


def _get_voicenotes_user(settings: AdminSettings, username: str) -> VoiceNotesCognitoUser:
    if settings.voicenotes_admin_lambda_name:
        result = _invoke_voicenotes_admin_lambda(
            settings,
            {"action": "admin_get_user", "username": username},
        )
        raw_user = result.get("user")
        if not isinstance(raw_user, dict):
            raise AdminStoreError("VoiceNotes admin Lambda returned an invalid user record.")
        return _voicenotes_user_from_cognito(raw_user)
    pool_id = _voicenotes_user_pool_id(settings)
    client = _voicenotes_client(settings)
    try:
        response = client.admin_get_user(UserPoolId=pool_id, Username=username)
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_cognito_error_message(exc)) from exc
    return _voicenotes_user_from_cognito(response)


def _create_voicenotes_user(
    settings: AdminSettings,
    *,
    email: str,
    display_name: str | None,
) -> None:
    if settings.voicenotes_admin_lambda_name:
        _invoke_voicenotes_admin_lambda(
            settings,
            {
                "action": "admin_create_user",
                "email": email,
                "display_name": display_name,
            },
        )
        return
    pool_id = _voicenotes_user_pool_id(settings)
    client = _voicenotes_client(settings)
    attributes = [
        {"Name": "email", "Value": email},
        {"Name": "email_verified", "Value": "true"},
    ]
    if display_name:
        attributes.append({"Name": "name", "Value": display_name})
    try:
        client.admin_create_user(
            UserPoolId=pool_id,
            Username=email,
            TemporaryPassword=_generate_voicenotes_temporary_password(),
            UserAttributes=attributes,
            DesiredDeliveryMediums=["EMAIL"],
        )
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_cognito_error_message(exc)) from exc


def _run_voicenotes_admin_action(settings: AdminSettings, action_name: str, username: str) -> None:
    if settings.voicenotes_admin_lambda_name:
        _invoke_voicenotes_admin_lambda(
            settings,
            {"action": action_name, "username": username},
        )
        return
    pool_id = _voicenotes_user_pool_id(settings)
    client = _voicenotes_client(settings)
    try:
        if action_name == "admin_reset_user_password":
            user = client.admin_get_user(UserPoolId=pool_id, Username=username)
            if str(user.get("UserStatus") or "") == "FORCE_CHANGE_PASSWORD":
                client.admin_create_user(
                    UserPoolId=pool_id,
                    Username=_user_email(user) or username,
                    TemporaryPassword=_generate_voicenotes_temporary_password(),
                    UserAttributes=_invite_attributes(user),
                    DesiredDeliveryMediums=["EMAIL"],
                    MessageAction="RESEND",
                )
                return
        if action_name == "admin_delete_user":
            user = client.admin_get_user(UserPoolId=pool_id, Username=username)
            _purge_voicenotes_user_data(settings, _voicenotes_user_id(_user_email(user) or username))
        getattr(client, action_name)(UserPoolId=pool_id, Username=username)
    except _botocore_client_error_type() as exc:
        raise AdminStoreError(_cognito_error_message(exc)) from exc


def _purge_voicenotes_user_data(settings: AdminSettings, user_id: str) -> None:
    if not settings.voicenotes_notes_table_name or not settings.voicenotes_transcript_bucket_name:
        raise AdminStoreError("VoiceNotes storage purge is not configured for this admin service.")
    _delete_voicenotes_s3_prefix(
        settings,
        settings.voicenotes_transcript_bucket_name,
        f"users/{user_id}/",
    )
    _delete_voicenotes_ddb_partition(
        settings,
        settings.voicenotes_notes_table_name,
        f"USER#{user_id}",
    )
    if settings.voicenotes_audit_table_name:
        _delete_voicenotes_ddb_partition(
            settings,
            settings.voicenotes_audit_table_name,
            f"USER#{user_id}",
        )


def _delete_voicenotes_s3_prefix(settings: AdminSettings, bucket: str, prefix: str) -> None:
    client = _voicenotes_s3_client(settings)
    continuation_token: str | None = None
    while True:
        kwargs: dict[str, Any] = {"Bucket": bucket, "Prefix": prefix}
        if continuation_token:
            kwargs["ContinuationToken"] = continuation_token
        response = client.list_objects_v2(**kwargs)
        objects = [{"Key": item["Key"]} for item in response.get("Contents", []) if item.get("Key")]
        for index in range(0, len(objects), 1000):
            chunk = objects[index : index + 1000]
            if chunk:
                client.delete_objects(Bucket=bucket, Delete={"Objects": chunk, "Quiet": True})
        if not response.get("IsTruncated"):
            return
        continuation_token = str(response.get("NextContinuationToken") or "")


def _delete_voicenotes_ddb_partition(settings: AdminSettings, table_name: str, pk: str) -> None:
    client = _voicenotes_ddb_client(settings)
    exclusive_start_key: dict[str, Any] | None = None
    while True:
        kwargs: dict[str, Any] = {
            "TableName": table_name,
            "KeyConditionExpression": "pk = :pk",
            "ExpressionAttributeValues": {":pk": {"S": pk}},
        }
        if exclusive_start_key:
            kwargs["ExclusiveStartKey"] = exclusive_start_key
        response = client.query(**kwargs)
        for item in response.get("Items", []):
            client.delete_item(TableName=table_name, Key={"pk": item["pk"], "sk": item["sk"]})
        exclusive_start_key = response.get("LastEvaluatedKey")
        if not exclusive_start_key:
            return


def _voicenotes_user_id(email: str) -> str:
    return email.strip().lower().replace("@", "_at_").replace(".", "_")


def _generate_voicenotes_temporary_password() -> str:
    characters = (
        VOICENOTES_TEMP_PASSWORD_LOWER
        + VOICENOTES_TEMP_PASSWORD_UPPER
        + VOICENOTES_TEMP_PASSWORD_DIGITS
        + VOICENOTES_TEMP_PASSWORD_SYMBOLS
    )
    password = [
        secrets.choice(VOICENOTES_TEMP_PASSWORD_LOWER),
        secrets.choice(VOICENOTES_TEMP_PASSWORD_UPPER),
        secrets.choice(VOICENOTES_TEMP_PASSWORD_DIGITS),
        secrets.choice(VOICENOTES_TEMP_PASSWORD_SYMBOLS),
    ]
    password.extend(secrets.choice(characters) for _ in range(VOICENOTES_TEMP_PASSWORD_LENGTH - len(password)))
    secrets.SystemRandom().shuffle(password)
    return "".join(password)


def _invite_attributes(user: dict[str, Any]) -> list[dict[str, str]]:
    attributes: list[dict[str, str]] = []
    for attribute in user.get("UserAttributes", user.get("Attributes", [])):
        if not isinstance(attribute, dict):
            continue
        name = str(attribute.get("Name") or "")
        value = str(attribute.get("Value") or "")
        if name == "sub":
            continue
        if name and value:
            attributes.append({"Name": name, "Value": value})
    return attributes


def _user_email(user: dict[str, Any]) -> str | None:
    for attribute in user.get("UserAttributes", user.get("Attributes", [])):
        if not isinstance(attribute, dict):
            continue
        if str(attribute.get("Name") or "") == "email":
            value = str(attribute.get("Value") or "").strip().lower()
            return value or None
    return None


def _voicenotes_user_from_cognito(raw_user: dict[str, Any]) -> VoiceNotesCognitoUser:
    attributes = {
        str(attribute.get("Name")): str(attribute.get("Value") or "")
        for attribute in raw_user.get("Attributes", raw_user.get("UserAttributes", []))
        if isinstance(attribute, dict) and attribute.get("Name")
    }
    username = str(raw_user.get("Username") or attributes.get("email") or "unknown")
    email = attributes.get("email") or username
    return VoiceNotesCognitoUser(
        username=username,
        email=email,
        display_name=attributes.get("name") or None,
        status=str(raw_user.get("UserStatus") or "UNKNOWN"),
        enabled=bool(raw_user.get("Enabled", True)),
        created_at=_format_cognito_datetime(raw_user.get("UserCreateDate")),
        updated_at=_format_cognito_datetime(raw_user.get("UserLastModifiedDate")),
    )


def _format_cognito_datetime(value: Any) -> str:
    if isinstance(value, datetime):
        return _format_dt(value)
    return str(value or "unknown")


def _voicenotes_notice(code: str) -> str | None:
    notices = {
        "created": "VoiceNotes user created. Cognito sent a temporary password. If you resend it, only the newest temporary-password email will work.",
        "enabled": "VoiceNotes user enabled.",
        "disabled": "VoiceNotes user disabled.",
        "invite-resent": "VoiceNotes Cognito temporary-password email resent. Only the newest temporary-password email will work.",
        "reset": "Cognito password reset email sent.",
        "signed-out": "VoiceNotes user sessions signed out.",
        "deleted": "VoiceNotes user deleted.",
    }
    return notices.get(code)


def _voicenotes_status_label(status: str) -> str:
    labels = {
        "FORCE_CHANGE_PASSWORD": "pending first login",
        "RESET_REQUIRED": "reset required",
        "CONFIRMED": "confirmed",
        "UNCONFIRMED": "unconfirmed",
    }
    return labels.get(status, status.replace("_", " ").lower())


def _voicenotes_status_help(status: str) -> str | None:
    if status == "FORCE_CHANGE_PASSWORD":
        return "Temporary password issued; user must use the newest temporary-password email, then create a permanent password before normal login works."
    if status == "RESET_REQUIRED":
        return "Password reset is pending; user must complete the Cognito reset flow."
    if status == "UNCONFIRMED":
        return "Cognito account is not confirmed yet."
    return None


def _botocore_client_error_type():
    try:
        from botocore.exceptions import ClientError  # type: ignore[import-not-found]
    except ImportError:
        return Exception
    return ClientError


def _cognito_error_message(exc: Exception) -> str:
    response = getattr(exc, "response", None)
    if isinstance(response, dict):
        error = response.get("Error")
        if isinstance(error, dict):
            code = str(error.get("Code") or "Unknown")
            message = str(error.get("Message") or exc)
            return _voicenotes_admin_error_message(code, message)
    return f"VoiceNotes Cognito request failed: {exc}"


def _lambda_error_message(exc: Exception) -> str:
    response = getattr(exc, "response", None)
    if isinstance(response, dict):
        error = response.get("Error")
        if isinstance(error, dict):
            code = str(error.get("Code") or "Unknown")
            message = str(error.get("Message") or exc)
            return f"VoiceNotes admin Lambda invoke failed ({code}): {message}"
    return f"VoiceNotes admin Lambda invoke failed: {exc}"


def _voicenotes_admin_error_message(code: str, message: str) -> str:
    if code == "UsernameExistsException":
        return "That VoiceNotes user already exists. Use reset password, enable, or disable from the account row."
    return f"VoiceNotes Cognito request failed ({code}): {message}"


def _render_index(
    *,
    users: list[AdminUserRecord],
    csrf_token: str,
    default_user_token_ttl_seconds: int,
) -> str:
    sorted_users = sorted(users, key=lambda user: (user.status.lower() != "active", user.email))
    active_count = sum(1 for user in users if user.status.lower() == "active")
    disabled_count = sum(1 for user in users if user.status.lower() == "disabled")
    rows = "\n".join(
        _user_row(user, csrf_token, default_user_token_ttl_seconds=default_user_token_ttl_seconds)
        for user in sorted_users
    ) or (
        '<tr><td colspan="6" class="empty-state">No transcription users have been added.</td></tr>'
    )
    duration_options = _duration_select_options(default_user_token_ttl_seconds)
    duration_label = _format_duration_label(default_user_token_ttl_seconds)
    body = f"""
    <header class="page-header">
      <div>
        <p class="eyebrow">Cubicle AWS service</p>
        <h1>Transcription Management</h1>
        <p class="lede">Manage client access for the live transcription service. Admin login is separate from issued client bearer tokens.</p>
      </div>
      <div class="header-actions">
        <a class="button secondary" href="/admin/voicenotes-users">VoiceNotes users</a>
        <a class="button secondary" href="/admin/audio-tuning">Audio tuning</a>
        <a class="button secondary" href="/admin/history">History</a>
        <form method="post" action="/admin/logout" class="header-action">
          <button type="submit" class="button secondary">Sign out</button>
        </form>
      </div>
    </header>

    <section class="metric-grid" aria-label="User overview">
      <article class="metric-card">
        <span class="metric-label">Total users</span>
        <strong>{len(users)}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Active users</span>
        <strong>{active_count}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Disabled users</span>
        <strong>{disabled_count}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Token duration</span>
        <strong>{html.escape(duration_label)}</strong>
      </article>
    </section>

    <section class="split-panel">
      <div class="panel-block">
        <h2>Usage Lookup</h2>
        <p class="section-copy">Review session metadata and token status for one user.</p>
        <form method="get" action="/admin/usage" class="stacked-form">
          <label>User email <input required type="email" name="email" autocomplete="off" placeholder="user@example.com"></label>
          <button type="submit" class="button">View usage and tokens</button>
        </form>
      </div>
      <div class="panel-block">
        <h2>Token Duration</h2>
        <p class="section-copy">Choose how long newly issued client bearer tokens should remain valid.</p>
        <form method="post" action="/admin/token-duration" class="stacked-form">
          <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
          <label>Default duration
            <select name="ttl_seconds">
              {duration_options}
            </select>
          </label>
          <button type="submit" class="button secondary">Use this duration</button>
        </form>
      </div>
      <div class="panel-block">
        <h2>Add or Reactivate User</h2>
        <p class="section-copy">Create a transcription user record before issuing a client token.</p>
        <form method="post" action="/admin/users" class="stacked-form">
        <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
        <label>Email <input required type="email" name="email" autocomplete="off"></label>
        <label>Display name <input name="display_name" autocomplete="off"></label>
        <label>Role <input name="role" value="transcription_user"></label>
        <label>Notes <input name="notes" autocomplete="off"></label>
          <button type="submit" class="button">Add or reactivate user</button>
      </form>
      </div>
    </section>

    <section class="surface">
      <div class="section-header">
        <div>
          <h2>Users</h2>
          <p class="section-copy">Issue, revoke, disable, or delete transcription access.</p>
        </div>
      </div>
      <div class="table-wrap">
        <table class="data-table">
          <thead><tr><th>Account</th><th>Role</th><th>Status</th><th>Updated</th><th>Usage</th><th>Actions</th></tr></thead>
          <tbody>{rows}</tbody>
        </table>
      </div>
    </section>
    """
    return _layout("Cubicle Transcription Admin", body)


def _render_audio_tuning(
    *,
    config: AdminAudioTuningConfig,
    using_defaults: bool,
    csrf_token: str,
    saved: bool = False,
) -> str:
    source_label = "Service defaults" if using_defaults else "Admin override"
    updated_at = config.updated_at or "not saved yet"
    updated_by = config.updated_by or "system default"
    saved_notice = (
        '<div class="notice success-notice">Audio tuning saved. New transcription sessions will use these values without restarting ECS.</div>'
        if saved
        else ""
    )
    return f"""
    <header class="page-header compact-header">
      <div>
        <p class="eyebrow">Runtime controls</p>
        <h1>Transcription Audio Tuning</h1>
        <p class="lede">Tune the server-side normalization applied before audio reaches Voxtral. Changes apply to new transcription sessions only.</p>
      </div>
      <div class="header-actions">
        <a class="button secondary" href="/admin">Users</a>
        <a class="button secondary" href="/admin/voicenotes-users">VoiceNotes users</a>
        <a class="button secondary" href="/admin/history">History</a>
      </div>
    </header>

    {saved_notice}

    <section class="surface tuning-summary">
      <div>
        <span class="metric-label">Current source</span>
        <strong>{html.escape(source_label)}</strong>
      </div>
      <div>
        <span class="metric-label">Updated</span>
        <strong class="small-metric">{html.escape(updated_at)}</strong>
      </div>
      <div>
        <span class="metric-label">Updated by</span>
        <strong class="small-metric">{html.escape(updated_by)}</strong>
      </div>
      <div>
        <span class="metric-label">Applies to</span>
        <strong>New sessions</strong>
      </div>
    </section>

    <section class="surface">
      <div class="section-header">
        <div>
          <h2>Normalization Settings</h2>
          <p class="section-copy">Recommended values are shown beside each field. Use small adjustments and restart only the Cubicle live transcription session after saving.</p>
        </div>
      </div>
      <form method="post" action="/admin/audio-tuning" class="tuning-form">
        <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
        {_tuning_field(
            label="Target RMS (%)",
            name="target_rms_percent",
            value=_percent_value(config.target_rms),
            recommended="20%",
            detail="Lifts quiet speech to the observed Voxtral startup activation range.",
            min_value="1",
            max_value="60",
            step="0.1",
        )}
        {_tuning_field(
            label="RMS floor (%)",
            name="rms_floor_percent",
            value=_percent_value(config.rms_floor),
            recommended="0.8%",
            detail="Audio below this is treated as near-silence and is not amplified.",
            min_value="0",
            max_value="60",
            step="0.1",
        )}
        {_tuning_field(
            label="Max server gain (x)",
            name="max_gain",
            value=_plain_number(config.max_gain),
            recommended="24x",
            detail="Upper bound on server-side amplification for quiet-but-real speech.",
            min_value="1",
            max_value="64",
            step="0.5",
        )}
        {_tuning_field(
            label="Peak ceiling (%)",
            name="peak_ceiling_percent",
            value=_percent_value(config.peak_ceiling),
            recommended="92%",
            detail="Caps gain so boosted samples avoid clipping before vLLM.",
            min_value="10",
            max_value="99",
            step="0.1",
        )}
        <div class="tuning-actions">
          <button type="submit" class="button">Save audio tuning</button>
          <a class="button secondary" href="/admin">Cancel</a>
        </div>
      </form>
    </section>
    """


def _tuning_field(
    *,
    label: str,
    name: str,
    value: str,
    recommended: str,
    detail: str,
    min_value: str,
    max_value: str,
    step: str,
) -> str:
    return f"""
    <label class="tuning-field">
      <span class="tuning-label">{html.escape(label)}</span>
      <input required type="number" name="{html.escape(name)}" value="{html.escape(value)}" min="{min_value}" max="{max_value}" step="{step}">
      <span class="recommended-value">Recommended: {html.escape(recommended)}</span>
      <span class="field-detail">{html.escape(detail)}</span>
    </label>
    """


def _user_row(
    user: AdminUserRecord,
    csrf_token: str,
    *,
    default_user_token_ttl_seconds: int,
) -> str:
    escaped_email = html.escape(user.email)
    email_path = quote(user.email, safe="")
    csrf = html.escape(csrf_token)
    duration_options = _duration_select_options(default_user_token_ttl_seconds)
    is_active = user.status.lower() == "active"
    issue_form = (
        f"""
        <form method="post" action="/admin/users/{email_path}/tokens" class="inline-form">
          <input type="hidden" name="csrf_token" value="{csrf}">
          <label class="compact-field"><span>Duration</span><select name="ttl_seconds">{duration_options}</select></label>
          <button type="submit" class="button">Issue token</button>
        </form>
        """
        if is_active
        else '<p class="hint">Reactivate this user before issuing a token.</p>'
    )
    disable_form = (
        f"""
        <form method="post" action="/admin/users/{email_path}/disable">
          <input type="hidden" name="csrf_token" value="{csrf}">
          <button type="submit" class="button warning-button">Disable + revoke tokens</button>
        </form>
        """
        if is_active
        else ""
    )
    return f"""
    <tr>
      <td>
        <div class="account-cell">
          <strong>{escaped_email}</strong>
          <span>{html.escape(user.display_name or "No display name")}</span>
        </div>
      </td>
      <td>{html.escape(user.role)}</td>
      <td>{_status_badge(user.status)}</td>
      <td><code class="timestamp">{html.escape(user.updated_at or "never")}</code></td>
      <td><a class="text-link" href="/admin/usage?email={email_path}">View usage</a></td>
      <td>
        <div class="action-panel">
          <div class="action-title">Issue token</div>
        {issue_form}
          <div class="action-title">Token operations</div>
          <form method="post" action="/admin/users/{email_path}/tokens/revoke" class="stacked-form compact">
            <input type="hidden" name="csrf_token" value="{csrf}">
            <label>Revoke by token ID <input required name="token_id" placeholder="UUID shown as Token ID"></label>
            <label>Reason <input name="reason" placeholder="Optional"></label>
            <button type="submit" class="button secondary">Revoke token</button>
          </form>
          <form method="post" action="/admin/users/{email_path}/tokens/revoke-all">
            <input type="hidden" name="csrf_token" value="{csrf}">
            <input type="hidden" name="reason" value="revoked all from admin console">
            <button type="submit" class="button secondary">Revoke all active tokens</button>
          </form>
          <div class="danger-zone">
        {disable_form}
            <form method="post" action="/admin/users/{email_path}/delete">
              <input type="hidden" name="csrf_token" value="{csrf}">
              <button type="submit" class="button danger-button">Delete user</button>
            </form>
          </div>
        </div>
      </td>
    </tr>
    """


def _render_voicenotes_users(
    *,
    users: list[VoiceNotesCognitoUser],
    csrf_token: str,
    user_pool_id: str | None,
    notice: str | None,
    error: str | None,
) -> str:
    sorted_users = sorted(users, key=lambda user: (not user.enabled, user.email))
    enabled_count = sum(1 for user in users if user.enabled)
    disabled_count = len(users) - enabled_count
    invite_count = sum(1 for user in users if user.status == "FORCE_CHANGE_PASSWORD")
    rows = "\n".join(_voicenotes_user_row(user, csrf_token) for user in sorted_users) or (
        '<tr><td colspan="6" class="empty-state">No VoiceNotes users have been added.</td></tr>'
    )
    notice_html = (
        f'<div class="notice success-notice">{html.escape(notice)}</div>'
        if notice
        else ""
    )
    error_html = (
        f'<div class="notice warning-notice">{html.escape(error)}</div>'
        if error
        else ""
    )
    pool_label = user_pool_id or "not configured"
    return f"""
    <header class="page-header compact-header">
      <div>
        <p class="eyebrow">VoiceNotes access</p>
        <h1>VoiceNotes User Accounts</h1>
        <p class="lede">Manage accounts for voicenotes.agenticisolation.com through the VoiceNotes Cognito user pool. Passwords and login tokens stay inside Cognito.</p>
      </div>
      <div class="header-actions">
        <a class="button secondary" href="/admin">Cubicle users</a>
        <a class="button secondary" href="/admin/history">History</a>
      </div>
    </header>

    {notice_html}
    {error_html}

    <section class="metric-grid" aria-label="VoiceNotes user overview">
      <article class="metric-card">
        <span class="metric-label">Total users</span>
        <strong>{len(users)}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Enabled</span>
        <strong>{enabled_count}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Disabled</span>
        <strong>{disabled_count}</strong>
      </article>
      <article class="metric-card">
        <span class="metric-label">Pending first login</span>
        <strong>{invite_count}</strong>
      </article>
    </section>

    <section class="split-panel voicenotes-split">
      <div class="panel-block">
        <h2>Add VoiceNotes User</h2>
        <p class="section-copy">Create the Cognito user and send a temporary password by email. The user must set a permanent password before VoiceNotes login is complete.</p>
        <form method="post" action="/admin/voicenotes-users" class="stacked-form">
          <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
          <label>Email <input required type="email" name="email" autocomplete="off" placeholder="user@example.com"></label>
          <label>Display name <input name="display_name" autocomplete="off"></label>
          <button type="submit" class="button">Create and send invite</button>
        </form>
      </div>
      <div class="panel-block">
        <h2>Password Handling</h2>
        <p class="section-copy">Pending first login means Cognito has sent a temporary password. The user signs in at voicenotes.agenticisolation.com with the newest temporary-password email, then creates a permanent password that satisfies the Cognito policy. Resending replaces the earlier temporary password.</p>
        <dl class="detail-list compact-detail-list">
          <dt>User pool</dt><dd><code>{html.escape(pool_label)}</code></dd>
          <dt>Login</dt><dd>Hosted by VoiceNotes Cognito</dd>
          <dt>New password</dt><dd>At least 14 chars with uppercase, lowercase, number, and symbol</dd>
          <dt>Storage</dt><dd>Per-user transcripts remain owned by VoiceNotes</dd>
        </dl>
      </div>
    </section>

    <section class="surface">
      <div class="section-header">
        <div>
          <h2>Accounts</h2>
          <p class="section-copy">Enable, disable, reset password, sign out, or delete VoiceNotes users.</p>
        </div>
      </div>
      <div class="table-wrap">
        <table class="data-table voicenotes-table">
          <thead><tr><th>Account</th><th>Status</th><th>Enabled</th><th>Created</th><th>Updated</th><th>Actions</th></tr></thead>
          <tbody>{rows}</tbody>
        </table>
      </div>
    </section>
    """


def _voicenotes_user_row(user: VoiceNotesCognitoUser, csrf_token: str) -> str:
    username_path = quote(user.username, safe="")
    csrf = html.escape(csrf_token)
    reset_label = "Resend temporary password" if user.status == "FORCE_CHANGE_PASSWORD" else "Send password reset"
    status_help = _voicenotes_status_help(user.status)
    status_help_html = (
        f'<span class="status-help">{html.escape(status_help)}</span>'
        if status_help
        else ""
    )
    enable_disable_form = (
        f"""
        <form method="post" action="/admin/voicenotes-users/{username_path}/disable">
          <input type="hidden" name="csrf_token" value="{csrf}">
          <button type="submit" class="button warning-button">Disable account</button>
        </form>
        """
        if user.enabled
        else f"""
        <form method="post" action="/admin/voicenotes-users/{username_path}/enable">
          <input type="hidden" name="csrf_token" value="{csrf}">
          <button type="submit" class="button">Enable account</button>
        </form>
        """
    )
    return f"""
    <tr>
      <td>
        <div class="account-cell">
          <strong>{html.escape(user.email)}</strong>
          <span>{html.escape(user.display_name or "No display name")}</span>
          <code>{html.escape(user.username)}</code>
        </div>
      </td>
      <td><div class="status-cell">{_status_badge(_voicenotes_status_label(user.status))}{status_help_html}</div></td>
      <td>{_status_badge("enabled" if user.enabled else "disabled")}</td>
      <td><code class="timestamp">{html.escape(user.created_at)}</code></td>
      <td><code class="timestamp">{html.escape(user.updated_at)}</code></td>
      <td>
        <div class="action-panel">
          {enable_disable_form}
          <form method="post" action="/admin/voicenotes-users/{username_path}/reset-password">
            <input type="hidden" name="csrf_token" value="{csrf}">
            <button type="submit" class="button secondary">{reset_label}</button>
          </form>
          <form method="post" action="/admin/voicenotes-users/{username_path}/sign-out">
            <input type="hidden" name="csrf_token" value="{csrf}">
            <button type="submit" class="button secondary">Sign out sessions</button>
          </form>
          <div class="danger-zone">
            <form method="post" action="/admin/voicenotes-users/{username_path}/delete">
              <input type="hidden" name="csrf_token" value="{csrf}">
              <button type="submit" class="button danger-button">Delete account</button>
            </form>
          </div>
        </div>
      </td>
    </tr>
    """


def _render_usage_lookup(
    *,
    csrf_token: str,
    email: str | None = None,
    user: AdminUserRecord | None = None,
    summary: AdminUsageSummary | None = None,
    tokens: list[AdminTokenRecord] | None = None,
) -> str:
    email_value = html.escape(email or "")
    body = f"""
    <header class="page-header compact-header">
      <div>
        <p class="eyebrow">User audit</p>
        <h1>Usage and Tokens</h1>
        <p class="lede">Usage is metadata only: sessions, duration, bytes, token IDs, and timestamps. Audio and transcript text are not stored here.</p>
      </div>
      <a class="button secondary" href="/admin">Admin console</a>
    </header>
    <section class="surface">
      <form method="get" action="/admin/usage" class="lookup-form">
        <label>User email <input required type="email" name="email" value="{email_value}" autocomplete="off"></label>
        <button type="submit" class="button">View usage and tokens</button>
      </form>
    </section>
    """
    if not email:
        return body
    if summary is None:
        summary = AdminUsageSummary(email=email)
    tokens = tokens or []
    user_status = user.status if user else "not registered"
    token_rows = "\n".join(_token_row(token, csrf_token) for token in tokens) or (
        '<tr><td colspan="7" class="empty-state">No tokens have been issued for this user.</td></tr>'
    )
    body += f"""
    <section class="surface">
      <div class="section-header">
        <div>
          <h2>{html.escape(email)}</h2>
          <p class="section-copy">User status {_status_badge(user_status)}</p>
        </div>
      </div>
      <div class="metric-grid usage-metrics">
        <article class="metric-card">
          <span class="metric-label">Sessions</span>
          <strong>{summary.session_count}</strong>
        </article>
        <article class="metric-card">
          <span class="metric-label">Audio minutes</span>
          <strong>{summary.total_audio_minutes:.2f}</strong>
        </article>
        <article class="metric-card">
          <span class="metric-label">Audio bytes</span>
          <strong>{summary.total_audio_bytes}</strong>
        </article>
        <article class="metric-card">
          <span class="metric-label">Last session</span>
          <strong class="small-metric">{html.escape(summary.last_session_at or "never")}</strong>
        </article>
      </div>
      <div class="token-summary">
        <span>{summary.active_token_count} active</span>
        <span>{summary.revoked_token_count} revoked</span>
        <span>{summary.expired_token_count} expired</span>
      </div>
    </section>
    <section class="surface">
      <div class="section-header">
        <div>
          <h2>Tokens</h2>
          <p class="section-copy">Token values are never stored. Only IDs and lifecycle metadata are shown here.</p>
        </div>
      </div>
      <div class="table-wrap">
        <table class="data-table">
          <thead><tr><th>Token ID</th><th>Status</th><th>Scope</th><th>Issued</th><th>Expires</th><th>Revoked</th><th>Action</th></tr></thead>
          <tbody>{token_rows}</tbody>
        </table>
      </div>
    </section>
    """
    return body


def _render_history(*, events: list[AdminHistoryEvent], limit: int) -> str:
    rows = "\n".join(_history_row(event) for event in events) or (
        '<tr><td colspan="7" class="empty-state">No admin history events have been recorded yet.</td></tr>'
    )
    return f"""
    <header class="page-header compact-header">
      <div>
        <p class="eyebrow">Admin audit</p>
        <h1>User Management History</h1>
        <p class="lede">Review user and token lifecycle actions across the transcription service. This history remains available even after a user is deleted.</p>
      </div>
      <a class="button secondary" href="/admin">Admin console</a>
    </header>
    <section class="surface">
      <form method="get" action="/admin/history" class="lookup-form">
        <label>Events to show <input type="number" name="limit" min="25" max="500" value="{limit}"></label>
        <button type="submit" class="button">Refresh history</button>
      </form>
    </section>
    <section class="surface">
      <div class="section-header">
        <div>
          <h2>Lifecycle Events</h2>
          <p class="section-copy">Added, activated, updated, token issued, token revoked, disabled, and deleted events are listed newest first.</p>
        </div>
      </div>
      <div class="table-wrap">
        <table class="data-table">
          <thead><tr><th>Time</th><th>Event</th><th>User</th><th>Actor</th><th>Token ID</th><th>Reason</th><th>Detail</th></tr></thead>
          <tbody>{rows}</tbody>
        </table>
      </div>
    </section>
    """


def _history_row(event: AdminHistoryEvent) -> str:
    return f"""
    <tr>
      <td><code class="timestamp">{html.escape(event.occurred_at)}</code></td>
      <td>{_event_badge(event.event_type)}</td>
      <td>{html.escape(event.email)}</td>
      <td>{html.escape(event.actor_email or "admin")}</td>
      <td><code>{html.escape(event.token_id or "")}</code></td>
      <td>{html.escape(event.reason or "")}</td>
      <td>{html.escape(event.detail or "")}</td>
    </tr>
    """


def _token_row(token: AdminTokenRecord, csrf_token: str) -> str:
    email_path = quote(token.user_email, safe="")
    token_path = quote(token.token_id, safe="")
    revoke_form = ""
    if token.status.lower() == "active":
        revoke_form = f"""
        <form method="post" action="/admin/users/{email_path}/tokens/{token_path}/revoke" class="inline-form">
          <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
          <label class="compact-field"><span>Reason</span><input name="reason" placeholder="Optional"></label>
          <button type="submit" class="button secondary">Revoke</button>
        </form>
        """
    return f"""
    <tr>
      <td><code>{html.escape(token.token_id)}</code></td>
      <td>{_status_badge(token.status)}</td>
      <td>{html.escape(token.scope)}</td>
      <td><code class="timestamp">{html.escape(token.issued_at or "")}</code></td>
      <td><code class="timestamp">{html.escape(token.expires_at or "")}</code></td>
      <td><code class="timestamp">{html.escape(token.revoked_at or "")}</code></td>
      <td>{revoke_form}</td>
    </tr>
    """


def _render_issued_token(
    *,
    email: str,
    token_id: str,
    expires_at: str,
    token: str,
    csrf_token: str,
) -> str:
    email_path = quote(email, safe="")
    token_path = quote(token_id, safe="")
    return """
    <header class="page-header compact-header">
      <div>
        <p class="eyebrow">Token issued</p>
        <h1>Issued Transcription Token</h1>
        <p class="lede">Copy this token now. It is shown once, and the service stores only metadata.</p>
      </div>
      <a class="button secondary" href="/admin">Admin console</a>
    </header>
    <section class="surface">
      <div class="notice warning-notice">Do not take screenshots of this page or paste the token into chat or docs.</div>
      <h2>Next steps</h2>
      <ol>
        <li>Select the full token in the box below and copy it.</li>
        <li>Open the user's Cubicle app and go to Settings -> Live Transcription.</li>
        <li>Paste it into Service token and click Save. Cubicle stores it in Keychain.</li>
        <li>Use endpoint <code>wss://dcabsri6ekziv.cloudfront.net/v1/transcription</code>.</li>
      </ol>
    </section>
    <section class="surface">
      <dl class="detail-list">
        <dt>User</dt><dd>{email}</dd>
        <dt>Token ID</dt><dd><code>{token_id}</code></dd>
        <dt>Expires</dt><dd><code class="timestamp">{expires_at}</code></dd>
      </dl>
      <label>Bearer token <textarea readonly rows="7" aria-label="Issued transcription bearer token">{token}</textarea></label>
    </section>
    <section class="surface danger-surface">
      <h2>Token exposed?</h2>
      <p class="section-copy">If this token was copied to the wrong place, revoke it here and issue a fresh token.</p>
      <form method="post" action="/admin/users/{email_path}/tokens/{token_path}/revoke">
        <input type="hidden" name="csrf_token" value="{csrf_token}">
        <input type="hidden" name="reason" value="revoked from issued-token page">
        <button type="submit" class="button danger-button">Revoke this token</button>
      </form>
    </section>
    """.format(
        email=html.escape(email),
        token_id=html.escape(token_id),
        expires_at=html.escape(expires_at),
        token=html.escape(token),
        email_path=html.escape(email_path),
        token_path=html.escape(token_path),
        csrf_token=html.escape(csrf_token),
    )


def _render_error(message: str) -> str:
    detail = ""
    if "Token ID is required" in message or message == "token_id is required":
        detail = (
            '<p class="section-copy">The token ID is the short UUID shown next to '
            'the issued token. It is not the long bearer token value.</p>'
        )
    return _layout(
        "Admin Error",
        f"""
        <section class="surface error-surface">
          <p class="eyebrow">Request failed</p>
          <h1>Admin request failed</h1>
          <div class="notice warning-notice">{html.escape(message)}</div>
          {detail}
          <p><a class="button secondary" href="/admin">Return to admin console</a></p>
        </section>
        """,
    )


def _login_form(error: str | None = None, *, email: str = "") -> str:
    error_html = f'<div class="notice warning-notice">{html.escape(error)}</div>' if error else ""
    return f"""
    <section class="surface login-surface">
      <p class="eyebrow">Admin console</p>
      <h1>Cubicle Transcription Admin</h1>
      <p class="lede">Sign in with the configured Cognito administrator account. Passwords are verified by Cognito and are not stored by this service.</p>
      {error_html}
      <form method="post" action="/admin/login" class="stacked-form">
        <label>Email <input required type="email" name="email" autocomplete="username" value="{html.escape(email)}"></label>
        <label>Password <input required type="password" name="password" autocomplete="current-password"></label>
        <button type="submit" class="button">Sign in</button>
      </form>
    </section>
    """


def _external_auth_message() -> str:
    return """
    <section class="surface login-surface">
      <p class="eyebrow">Admin console</p>
      <h1>Cubicle Transcription Admin</h1>
      <p class="lede">This console uses Cognito authentication in front of the admin service.</p>
      <div class="notice warning-notice">No extra admin credential is required. Return to <a href="/admin">/admin</a> so Cognito can authenticate the request.</div>
    </section>
    """


def _render_external_auth_failure(settings: AdminSettings) -> str:
    required_group = settings.required_admin_group or "the configured admin group"
    body = f"""
    <section class="surface login-surface">
      <p class="eyebrow">Access denied</p>
      <h1>Cubicle Transcription Admin</h1>
      <div class="notice warning-notice">Cognito sign-in succeeded, but this request did not include an authorized admin identity.</div>
      <p class="section-copy">Confirm the signed-in user is in <code>{html.escape(required_group)}</code>, then open <a href="/admin">/admin</a> again.</p>
      <form method="post" action="/admin/logout"><button type="submit" class="button secondary">Clear admin session</button></form>
    </section>
    """
    return _layout("Cubicle Admin Access Denied", body)


def _record_history(
    store: AdminStore,
    *,
    session: AdminSession,
    event_type: str,
    email: str,
    token_id: str | None = None,
    reason: str | None = None,
    detail: str | None = None,
) -> None:
    store.record_history_event(
        AdminHistoryEvent(
            event_id=str(uuid.uuid4()),
            event_type=event_type,
            email=email,
            occurred_at=_format_dt(datetime.now(timezone.utc)),
            actor_email=session.admin_email,
            token_id=token_id,
            reason=reason,
            detail=detail,
        )
    )


def _status_badge(status: str) -> str:
    normalized = status.strip().lower() or "unknown"
    if normalized in {"active", "enabled", "confirmed"}:
        badge_class = "status-active"
    elif normalized in {"disabled", "revoked", "expired"}:
        badge_class = "status-muted"
    else:
        badge_class = "status-neutral"
    return f'<span class="status-badge {badge_class}">{html.escape(status or "unknown")}</span>'


def _event_badge(event_type: str) -> str:
    normalized = event_type.strip().lower()
    if normalized in {
        "user_added",
        "user_activated",
        "token_issued",
        "audio_tuning_updated",
        "voicenotes_user_created",
        "voicenotes_user_enabled",
    }:
        badge_class = "event-good"
    elif normalized in {
        "token_revoked",
        "tokens_revoked",
        "user_disabled",
        "voicenotes_user_disabled",
        "voicenotes_password_reset",
        "voicenotes_user_signed_out",
    }:
        badge_class = "event-warning"
    elif normalized in {"user_deleted", "voicenotes_user_deleted"}:
        badge_class = "event-danger"
    else:
        badge_class = "event-neutral"
    return f'<span class="event-badge {badge_class}">{html.escape(_event_label(normalized))}</span>'


def _event_label(event_type: str) -> str:
    labels = {
        "user_added": "User added",
        "user_activated": "User activated",
        "user_updated": "User updated",
        "token_issued": "Token issued",
        "token_revoked": "Token revoked",
        "tokens_revoked": "Tokens revoked",
        "user_disabled": "User disabled",
        "user_deleted": "User deleted",
        "audio_tuning_updated": "Audio tuning",
        "voicenotes_user_created": "VoiceNotes user created",
        "voicenotes_user_enabled": "VoiceNotes user enabled",
        "voicenotes_user_disabled": "VoiceNotes user disabled",
        "voicenotes_password_reset": "VoiceNotes password reset",
        "voicenotes_user_signed_out": "VoiceNotes user signed out",
        "voicenotes_user_deleted": "VoiceNotes user deleted",
    }
    return labels.get(event_type, event_type.replace("_", " ").title())


def _layout(title: str, body: str) -> str:
    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{html.escape(title)}</title>
<style>
:root {{
  color-scheme: light;
  --bg: #f5f7fb;
  --surface: #ffffff;
  --surface-subtle: #f8fafc;
  --border: #d7dee8;
  --border-strong: #b7c1cf;
  --text: #18202b;
  --muted: #5f6b7a;
  --blue: #145fd7;
  --blue-dark: #0f4db0;
  --green-bg: #e7f7ee;
  --green-text: #126c3f;
  --gray-bg: #eef2f6;
  --gray-text: #4d5968;
  --amber-bg: #fff4df;
  --amber-text: #8a4b00;
  --red: #c82e2e;
  --red-bg: #fff0f0;
}}
* {{ box-sizing: border-box; }}
body {{
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Inter, sans-serif;
  line-height: 1.45;
}}
main {{
  width: min(1480px, calc(100vw - 48px));
  margin: 0 auto;
  padding: 42px 0 56px;
}}
h1, h2, h3, p {{ margin-top: 0; }}
h1 {{ font-size: 34px; line-height: 1.12; margin-bottom: 10px; }}
h2 {{ font-size: 20px; line-height: 1.2; margin-bottom: 8px; }}
a {{ color: var(--blue); }}
code, .timestamp {{
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
}}
.page-header {{
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 22px;
}}
.compact-header {{ margin-bottom: 18px; }}
.eyebrow {{
  color: var(--blue);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
  margin-bottom: 8px;
}}
.lede {{
  color: var(--muted);
  font-size: 16px;
  max-width: 860px;
  margin-bottom: 0;
}}
.section-copy, .hint {{
  color: var(--muted);
  font-size: 14px;
  margin-bottom: 14px;
}}
.status-cell {{
  display: grid;
  gap: 7px;
  min-width: 170px;
}}
.status-help {{
  color: var(--muted);
  display: block;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.35;
  max-width: 260px;
}}
.header-actions {{
  display: flex;
  gap: 10px;
  align-items: center;
  flex: 0 0 auto;
}}
.header-action {{ margin: 0; }}
.surface, .split-panel, .metric-card {{
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04);
}}
.surface {{ padding: 22px; margin: 18px 0; }}
.split-panel {{
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0;
  overflow: hidden;
  margin: 18px 0;
}}
.voicenotes-split {{
  grid-template-columns: minmax(320px, 1fr) minmax(320px, 1.2fr);
}}
.panel-block {{ padding: 22px; }}
.panel-block + .panel-block {{ border-left: 1px solid var(--border); }}
.section-header {{
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}}
.metric-grid {{
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin: 18px 0;
}}
.usage-metrics {{ grid-template-columns: repeat(4, minmax(0, 1fr)); }}
.metric-card {{ padding: 18px; }}
.metric-label {{
  display: block;
  color: var(--muted);
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 8px;
}}
.metric-card strong {{
  display: block;
  font-size: 30px;
  line-height: 1.1;
}}
.metric-card .small-metric {{
  font-size: 15px;
  overflow-wrap: anywhere;
}}
label {{
  display: block;
  color: var(--text);
  font-size: 14px;
  font-weight: 700;
}}
input, textarea, select {{
  width: 100%;
  min-height: 40px;
  margin-top: 6px;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  background: #ffffff;
  color: var(--text);
  font: inherit;
  padding: 8px 10px;
}}
textarea {{
  min-height: 150px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  resize: vertical;
}}
input:focus, textarea:focus, select:focus {{
  outline: 3px solid rgba(20, 95, 215, 0.16);
  border-color: var(--blue);
}}
form {{ margin: 0; }}
.stacked-form {{
  display: grid;
  gap: 12px;
}}
.stacked-form.compact {{ gap: 8px; }}
.lookup-form {{
  display: grid;
  grid-template-columns: minmax(260px, 1fr) auto;
  gap: 12px;
  align-items: end;
}}
.inline-form {{
  display: flex;
  align-items: end;
  gap: 10px;
  flex-wrap: wrap;
}}
.compact-field {{ flex: 1 1 190px; }}
.compact-field span {{
  display: block;
  color: var(--muted);
  font-size: 12px;
  font-weight: 700;
}}
.button, button {{
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 38px;
  padding: 8px 13px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: var(--blue);
  color: #ffffff;
  font: inherit;
  font-weight: 800;
  text-decoration: none;
  cursor: pointer;
  white-space: nowrap;
}}
.button:hover, button:hover {{ background: var(--blue-dark); }}
.button.secondary, button.secondary {{
  background: #ffffff;
  color: var(--text);
  border-color: var(--border-strong);
}}
.button.secondary:hover, button.secondary:hover {{
  background: var(--surface-subtle);
}}
.warning-button {{
  background: #8a4b00;
}}
.warning-button:hover {{
  background: #6f3c00;
}}
.danger-button {{
  background: var(--red);
}}
.danger-button:hover {{
  background: #a62323;
}}
.table-wrap {{
  width: 100%;
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
}}
.data-table {{
  width: 100%;
  min-width: 980px;
  border-collapse: separate;
  border-spacing: 0;
  background: #ffffff;
}}
.data-table th {{
  background: var(--surface-subtle);
  color: var(--muted);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0;
}}
.data-table th, .data-table td {{
  padding: 14px;
  border-bottom: 1px solid var(--border);
  text-align: left;
  vertical-align: top;
}}
.data-table tr:last-child td {{ border-bottom: 0; }}
.account-cell {{
  display: grid;
  gap: 4px;
  min-width: 230px;
}}
.account-cell span {{
  color: var(--muted);
  font-size: 13px;
}}
.text-link {{ font-weight: 800; }}
.status-badge {{
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 4px 9px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
  text-transform: capitalize;
}}
.status-active {{
  background: var(--green-bg);
  color: var(--green-text);
}}
.status-muted {{
  background: var(--gray-bg);
  color: var(--gray-text);
}}
.status-neutral {{
  background: var(--amber-bg);
  color: var(--amber-text);
}}
.event-badge {{
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 4px 9px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}}
.event-good {{
  background: var(--green-bg);
  color: var(--green-text);
}}
.event-warning {{
  background: var(--amber-bg);
  color: var(--amber-text);
}}
.event-danger {{
  background: var(--red-bg);
  color: var(--red);
}}
.event-neutral {{
  background: var(--gray-bg);
  color: var(--gray-text);
}}
.action-panel {{
  display: grid;
  gap: 10px;
  min-width: 310px;
}}
.action-title {{
  color: var(--muted);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0;
}}
.danger-zone {{
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  border-top: 1px solid var(--border);
  padding-top: 12px;
  margin-top: 2px;
}}
.detail-list {{
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  gap: 10px 18px;
  margin: 0 0 18px;
}}
.detail-list dt {{
  color: var(--muted);
  font-weight: 800;
}}
.detail-list dd {{
  margin: 0;
  overflow-wrap: anywhere;
}}
.compact-detail-list {{
  grid-template-columns: 100px minmax(0, 1fr);
  margin-bottom: 0;
}}
.token-summary {{
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 16px;
}}
.token-summary span {{
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 6px 10px;
  color: var(--muted);
  font-weight: 700;
  background: var(--surface-subtle);
}}
.notice {{
  border-radius: 8px;
  padding: 12px 14px;
  margin-bottom: 16px;
  font-weight: 700;
}}
.warning-notice {{
  background: var(--amber-bg);
  color: var(--amber-text);
  border: 1px solid #f2d29b;
}}
.success-notice {{
  background: var(--green-bg);
  color: var(--green-text);
  border: 1px solid #a9dfbf;
}}
.danger-surface {{
  border-color: #f0b5b5;
  background: var(--red-bg);
}}
.tuning-summary {{
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px;
}}
.tuning-summary strong {{
  display: block;
  font-size: 18px;
  overflow-wrap: anywhere;
}}
.tuning-form {{
  display: grid;
  grid-template-columns: repeat(4, minmax(180px, 1fr));
  gap: 16px;
}}
.tuning-field {{
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 14px;
  background: var(--surface-subtle);
}}
.tuning-label {{
  display: block;
  margin-bottom: 2px;
}}
.recommended-value {{
  display: block;
  color: var(--blue);
  font-size: 12px;
  font-weight: 800;
  margin-top: 8px;
}}
.field-detail {{
  display: block;
  color: var(--muted);
  font-size: 12px;
  font-weight: 600;
  margin-top: 4px;
}}
.tuning-actions {{
  grid-column: 1 / -1;
  display: flex;
  gap: 10px;
  align-items: center;
  justify-content: flex-start;
  border-top: 1px solid var(--border);
  padding-top: 16px;
}}
.error-surface, .login-surface {{
  max-width: 780px;
  margin-left: auto;
  margin-right: auto;
}}
.empty-state {{
  color: var(--muted);
  text-align: center;
  padding: 34px !important;
}}
@media (max-width: 900px) {{
  main {{
    width: min(100vw - 24px, 1480px);
    padding-top: 24px;
  }}
  .page-header, .lookup-form, .header-actions {{
    display: grid;
  }}
  .split-panel {{
    grid-template-columns: 1fr;
  }}
  .panel-block + .panel-block {{
    border-left: 0;
    border-top: 1px solid var(--border);
  }}
  .metric-grid, .usage-metrics {{
    grid-template-columns: 1fr;
  }}
  .tuning-summary, .tuning-form {{
    grid-template-columns: 1fr;
  }}
  .tuning-actions {{
    display: grid;
  }}
  .button, button {{
    width: 100%;
  }}
}}
</style>
</head>
<body><main>{body}</main></body>
</html>"""


def _user_json(user: AdminUserRecord) -> dict[str, Any]:
    return {
        "email": user.email,
        "display_name": user.display_name,
        "role": user.role,
        "status": user.status,
        "created_at": user.created_at,
        "updated_at": user.updated_at,
    }


def _safe_form_int(value: Any, *, default: int, minimum: int, maximum: int) -> int:
    try:
        parsed = int(str(value))
    except (TypeError, ValueError):
        parsed = default
    return min(max(parsed, minimum), maximum)


def _form_float(value: Any, label: str, *, minimum: float, maximum: float) -> float:
    try:
        parsed = float(str(value).strip())
    except (TypeError, ValueError) as exc:
        raise AdminStoreError(f"{label} must be a number") from exc
    if parsed < minimum or parsed > maximum:
        raise AdminStoreError(f"{label} must be between {_plain_number(minimum)} and {_plain_number(maximum)}")
    return parsed


def _form_percent_fraction(value: Any, label: str, *, minimum: float, maximum: float) -> float:
    return _form_float(value, label, minimum=minimum, maximum=maximum) / 100.0


def _dashboard_token_ttl_seconds(request: Request, settings: AdminSettings) -> int:
    return _safe_form_int(
        request.cookies.get(TOKEN_TTL_COOKIE_NAME),
        default=settings.default_user_token_ttl_seconds,
        minimum=60,
        maximum=30 * 24 * 60 * 60,
    )


def _duration_select_options(selected_seconds: int) -> str:
    options = [
        (60 * 60, "1 hour"),
        (24 * 60 * 60, "1 day"),
        (7 * 24 * 60 * 60, "7 days"),
        (14 * 24 * 60 * 60, "14 days"),
        (30 * 24 * 60 * 60, "30 days"),
    ]
    if all(seconds != selected_seconds for seconds, _ in options):
        options.append((selected_seconds, _format_duration_label(selected_seconds)))
    return "\n".join(
        '<option value="{seconds}"{selected}>{label}</option>'.format(
            seconds=seconds,
            selected=" selected" if seconds == selected_seconds else "",
            label=html.escape(label),
        )
        for seconds, label in options
    )


def _format_duration_label(seconds: int) -> str:
    if seconds % (24 * 60 * 60) == 0:
        days = seconds // (24 * 60 * 60)
        return f"{days} day" if days == 1 else f"{days} days"
    if seconds % (60 * 60) == 0:
        hours = seconds // (60 * 60)
        return f"{hours} hour" if hours == 1 else f"{hours} hours"
    minutes = seconds // 60
    if minutes > 0 and seconds % 60 == 0:
        return f"{minutes} minute" if minutes == 1 else f"{minutes} minutes"
    return f"{seconds} seconds"


def _percent_value(value: float) -> str:
    return _plain_number(value * 100.0)


def _format_percent(value: float) -> str:
    return f"{_percent_value(value)}%"


def _format_gain(value: float) -> str:
    return f"{_plain_number(value)}x"


def _plain_number(value: float) -> str:
    return f"{float(value):.3f}".rstrip("0").rstrip(".")


def _format_dt(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def _read_secret(env_name: str, file_env_name: str) -> str | None:
    path = os.environ.get(file_env_name, "").strip()
    if path:
        with open(path, encoding="utf-8") as handle:
            value = handle.read().strip()
            return value or None
    value = os.environ.get(env_name, "").strip()
    return value or None


def _env_bool(name: str, *, default: bool) -> bool:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    try:
        return int(raw.strip())
    except ValueError:
        return default


def _split_csv(value: str | None) -> frozenset[str]:
    if not value:
        return frozenset()
    return frozenset(part.strip().lower() for part in value.replace("\n", ",").split(",") if part.strip())


def _base64url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _base64url_decode(value: str) -> bytes:
    padding = "=" * (-len(value) % 4)
    return base64.urlsafe_b64decode((value + padding).encode("ascii"))
