"""FastAPI app factory and HTTP/WebSocket routes for VoiceNotes."""

from __future__ import annotations

import json
import logging
from datetime import datetime, timezone, tzinfo
from pathlib import Path
import secrets
from typing import Any
from urllib.parse import quote
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from fastapi import Cookie, Depends, FastAPI, HTTPException, Query, Request, Response, WebSocket
from fastapi.responses import HTMLResponse, JSONResponse, PlainTextResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from .auth import AuthError, CognitoUserAccessValidator, LocalUserDirectory, OIDCClient, SessionCodec, user_from_alb_headers
from .models import User
from .proxy import ActiveSessionRegistry, handle_recording_socket
from .settings import Settings
from .store import AwsNoteStore, LocalNoteStore, NoteNotFound, NoteStore, create_store
from .text_intelligence import create_text_intelligence_client


APP_DIR = Path(__file__).resolve().parent
STATIC_DIR = APP_DIR / "static"
logger = logging.getLogger("voicenotes.app")


class LoginRequest(BaseModel):
    """JSON login payload for local password auth."""

    email: str
    password: str


class TitleUpdate(BaseModel):
    """Patch payload for note-title updates."""

    title: str


class BulkDeleteRequest(BaseModel):
    """Batch delete payload carrying note ids owned by the user."""

    note_ids: list[str]


def create_app(
    settings: Settings | None = None,
    store: NoteStore | None = None,
    user_access_validator: Any | None = None,
) -> FastAPI:
    """Build the VoiceNotes web app with injectable auth and storage seams."""

    settings = settings or Settings.from_environment()
    store = store or create_store(settings)
    session_codec = SessionCodec(settings.session_secret, settings.session_ttl_seconds)
    local_directory = LocalUserDirectory(settings.local_users, settings.allowed_email_domains)
    user_access_validator = user_access_validator or CognitoUserAccessValidator(settings)
    active_sessions = ActiveSessionRegistry()
    text_intelligence_client = create_text_intelligence_client(settings)

    app = FastAPI(title="VoiceNotes", version="0.1.0")
    app.mount("/static", StaticFiles(directory=STATIC_DIR), name="static")

    @app.middleware("http")
    async def security_headers(request: Request, call_next):
        response = await call_next(request)
        response.headers.setdefault("X-Content-Type-Options", "nosniff")
        response.headers.setdefault("Referrer-Policy", "same-origin")
        response.headers.setdefault("X-Frame-Options", "DENY")
        response.headers.setdefault("Permissions-Policy", "microphone=(self), display-capture=(self)")
        if request.url.path == "/" or request.url.path.startswith("/static/"):
            response.headers["Cache-Control"] = "no-store, max-age=0"
        response.headers.setdefault(
            "Content-Security-Policy",
            "default-src 'self'; connect-src 'self' ws: wss:; script-src 'self'; style-src 'self'; img-src 'self' data:",
        )
        return response

    def current_user_from_cookie(token: str | None) -> User:
        if not token:
            raise AuthError("missing session")
        return session_codec.decode(token)

    async def require_active_user(user: User, *, use_cache: bool = True) -> User:
        await user_access_validator.validate(user, use_cache=use_cache)
        return user

    async def require_user(request: Request) -> User:
        if settings.auth_mode == "alb_cognito":
            try:
                return user_from_alb_headers(request.headers)
            except AuthError:
                pass
        token = request.cookies.get(settings.session_cookie_name)
        try:
            return await require_active_user(current_user_from_cookie(token))
        except AuthError as exc:
            logger.warning("request_auth_failed path=%s error=%s", request.url.path, str(exc))
            raise HTTPException(status_code=401, detail="unauthorized") from exc

    def set_session_cookie(response: Response, user: User) -> None:
        response.set_cookie(
            settings.session_cookie_name,
            session_codec.encode(user),
            httponly=True,
            secure=settings.secure_cookies,
            samesite="lax",
            max_age=settings.session_ttl_seconds,
            path="/",
        )

    def clear_session_cookie(response: Response) -> None:
        response.delete_cookie(settings.session_cookie_name, path="/")

    def oidc_logout_url() -> str:
        if settings.auth_mode != "oidc":
            return ""
        return OIDCClient(settings).logout_url()

    def restart_oidc_login_response() -> Response:
        response = RedirectResponse("/login", status_code=302)
        clear_session_cookie(response)
        response.delete_cookie("voicenotes_oidc_state", path="/")
        response.delete_cookie("voicenotes_oidc_nonce", path="/")
        return response

    @app.get("/healthz")
    async def healthz() -> dict[str, Any]:
        return {
            "status": "ok",
            "service": "voicenotes",
            "auth_mode": settings.auth_mode,
            "storage_backend": settings.storage_backend,
            "mock_transcription": settings.mock_transcription,
        }

    @app.get("/login", response_class=HTMLResponse, response_model=None)
    async def login_page():
        if settings.auth_mode == "oidc":
            state = secrets.token_urlsafe(18)
            nonce = secrets.token_urlsafe(18)
            redirect = RedirectResponse(OIDCClient(settings).authorization_url(state, nonce), status_code=302)
            redirect.set_cookie("voicenotes_oidc_state", state, httponly=True, secure=settings.secure_cookies, samesite="lax")
            redirect.set_cookie("voicenotes_oidc_nonce", nonce, httponly=True, secure=settings.secure_cookies, samesite="lax")
            return redirect
        return _read_static("login.html")

    @app.get("/logout", response_model=None)
    async def logout_page(request: Request) -> Response:
        response = RedirectResponse(oidc_logout_url() or "/login", status_code=302)
        try:
            user = await require_user(request)
            store.record_audit(user, "logout", {})
        except HTTPException:
            pass
        clear_session_cookie(response)
        return response

    @app.get("/auth/callback", response_model=None)
    async def auth_callback(
        request: Request,
        code: str = "",
        state: str = "",
        voicenotes_oidc_state: str | None = Cookie(default=None),
        voicenotes_oidc_nonce: str | None = Cookie(default=None),
    ) -> Response:
        if settings.auth_mode != "oidc":
            return RedirectResponse("/", status_code=302)
        if not code or not state or state != voicenotes_oidc_state:
            return restart_oidc_login_response()
        try:
            user = await OIDCClient(settings).exchange_code(code, voicenotes_oidc_nonce or "")
        except AuthError as exc:
            raise HTTPException(status_code=401, detail="OIDC authentication failed") from exc
        response = RedirectResponse("/", status_code=302)
        set_session_cookie(response, user)
        response.delete_cookie("voicenotes_oidc_state", path="/")
        response.delete_cookie("voicenotes_oidc_nonce", path="/")
        store.record_audit(user, "login_success", {"auth_mode": "oidc"})
        return response

    @app.post("/api/login")
    async def login(payload: LoginRequest) -> Response:
        if settings.auth_mode != "local":
            raise HTTPException(status_code=400, detail="local login is disabled")
        try:
            user = local_directory.authenticate(payload.email, payload.password)
        except AuthError as exc:
            raise HTTPException(status_code=401, detail="invalid credentials") from exc
        response = JSONResponse({"user": _user_payload(user)})
        set_session_cookie(response, user)
        store.record_audit(user, "login_success", {"auth_mode": "local"})
        return response

    @app.post("/api/logout")
    async def logout(request: Request) -> Response:
        response = JSONResponse({"ok": True, "logout_url": oidc_logout_url() or "/login"})
        try:
            user = await require_user(request)
            store.record_audit(user, "logout", {})
        except HTTPException:
            pass
        clear_session_cookie(response)
        return response

    @app.get("/api/me")
    async def me(user: User = Depends(require_user)) -> dict[str, Any]:
        return {"user": _user_payload(user)}

    @app.get("/api/notes")
    async def list_notes(user: User = Depends(require_user)) -> dict[str, Any]:
        notes = [note.to_dict() for note in store.list_notes(user)]
        return {"notes": notes}

    @app.get("/api/notes/{note_id}")
    async def get_note(note_id: str, user: User = Depends(require_user)) -> dict[str, Any]:
        try:
            note, transcript = store.get_note(user, note_id)
        except NoteNotFound as exc:
            raise HTTPException(status_code=404, detail="not found") from exc
        store.record_audit(user, "transcript_read", {"note_id": note_id})
        return {"note": note.to_dict(), "transcript": transcript}

    @app.patch("/api/notes/{note_id}")
    async def update_note(note_id: str, payload: TitleUpdate, user: User = Depends(require_user)) -> dict[str, Any]:
        try:
            note = store.update_note_title(user, note_id, payload.title)
        except NoteNotFound as exc:
            raise HTTPException(status_code=404, detail="not found") from exc
        return {"note": note.to_dict()}

    @app.delete("/api/notes/{note_id}")
    async def delete_note(note_id: str, user: User = Depends(require_user)) -> dict[str, Any]:
        try:
            store.delete_note(user, note_id)
        except NoteNotFound as exc:
            raise HTTPException(status_code=404, detail="not found") from exc
        return {"ok": True}

    @app.post("/api/notes/bulk-delete")
    async def bulk_delete_notes(payload: BulkDeleteRequest, user: User = Depends(require_user)) -> dict[str, Any]:
        note_ids: list[str] = []
        seen: set[str] = set()
        for raw_note_id in payload.note_ids:
            note_id = raw_note_id.strip()
            if note_id and note_id not in seen:
                seen.add(note_id)
                note_ids.append(note_id)
        if not note_ids:
            raise HTTPException(status_code=400, detail="no notes selected")
        if len(note_ids) > 500:
            raise HTTPException(status_code=400, detail="too many notes selected")

        deleted: list[str] = []
        not_found: list[str] = []
        for note_id in note_ids:
            try:
                store.delete_note(user, note_id)
                deleted.append(note_id)
            except NoteNotFound:
                not_found.append(note_id)
        return {"ok": True, "deleted": deleted, "not_found": not_found, "deleted_count": len(deleted)}

    @app.get("/api/notes/{note_id}/download.{extension}")
    async def download_note(
        note_id: str,
        extension: str,
        timezone_name: str = Query(default="", alias="timezone"),
        user: User = Depends(require_user),
    ) -> PlainTextResponse:
        if extension not in {"txt", "md"}:
            raise HTTPException(status_code=404, detail="not found")
        if not isinstance(store, (LocalNoteStore, AwsNoteStore)):
            raise HTTPException(status_code=501, detail="downloads unsupported")
        try:
            note, content = store.transcript_text(user, note_id, extension)
        except NoteNotFound as exc:
            raise HTTPException(status_code=404, detail="not found") from exc
        filename = _download_filename(note.created_at, extension, timezone_name)
        store.record_audit(user, "transcript_downloaded", {"note_id": note_id, "extension": extension})
        media_type = "text/markdown; charset=utf-8" if extension == "md" else "text/plain; charset=utf-8"
        return PlainTextResponse(
            content,
            media_type=media_type,
            headers={"Content-Disposition": _content_disposition(filename)},
        )

    @app.websocket("/ws/record")
    async def record_socket(websocket: WebSocket) -> None:
        token = websocket.cookies.get(settings.session_cookie_name)
        try:
            if settings.auth_mode == "alb_cognito":
                user = user_from_alb_headers(websocket.headers)
            else:
                user = await require_active_user(current_user_from_cookie(token), use_cache=False)
        except AuthError:
            await websocket.close(code=4401, reason="unauthorized")
            return
        await handle_recording_socket(
            websocket,
            user=user,
            settings=settings,
            store=store,
            active_sessions=active_sessions,
            access_validator=lambda active_user: require_active_user(active_user, use_cache=False),
            text_intelligence_client=text_intelligence_client,
        )

    @app.get("/{path:path}", response_class=HTMLResponse, response_model=None)
    async def app_shell(path: str, request: Request):
        if path.startswith("api/") or path.startswith("ws/"):
            raise HTTPException(status_code=404, detail="not found")
        token = request.cookies.get(settings.session_cookie_name)
        if settings.auth_mode != "alb_cognito":
            try:
                await require_active_user(current_user_from_cookie(token))
            except AuthError:
                logger.warning("shell_auth_failed path=/%s", path)
                response = RedirectResponse("/login", status_code=302)
                clear_session_cookie(response)
                return response
        return _read_static("app.html")

    return app


def _read_static(name: str) -> str:
    return (STATIC_DIR / name).read_text(encoding="utf-8")


def _user_payload(user: User) -> dict[str, Any]:
    return {"user_id": user.user_id, "email": user.email, "display_name": user.display_name, "role": user.role}


def _content_disposition(filename: str) -> str:
    fallback = _ascii_filename(filename)
    encoded = quote(filename, safe="")
    return f"attachment; filename=\"{fallback}\"; filename*=UTF-8''{encoded}"


def _download_filename(created_at: str | None, extension: str, timezone_name: str) -> str:
    created = _parse_note_timestamp(created_at)
    local_time = created.astimezone(_resolve_timezone(timezone_name))
    abbreviation = _safe_timezone_abbreviation(local_time.tzname())
    return f"VoiceNotes-{local_time:%Y-%m-%d-%H-%M-%S}{abbreviation}.{extension}"


def _parse_note_timestamp(value: str | None) -> datetime:
    if not value:
        return datetime.now(timezone.utc)
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return datetime.now(timezone.utc)
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def _resolve_timezone(timezone_name: str) -> tzinfo:
    candidate = timezone_name.strip()
    if candidate:
        try:
            return ZoneInfo(candidate)
        except ZoneInfoNotFoundError:
            logger.warning("download_timezone_invalid timezone=%s", candidate)
    return timezone.utc


def _safe_timezone_abbreviation(abbreviation: str | None) -> str:
    cleaned = "".join(character for character in abbreviation or "" if character.isascii() and character.isalnum())
    return cleaned or "UTC"


def _ascii_filename(filename: str) -> str:
    cleaned = "".join(
        character if character.isascii() and (character.isalnum() or character in {"-", "_", "."}) else "-"
        for character in filename.strip()
    ).strip(".-")
    return cleaned or "voice-note.txt"


app = create_app()
