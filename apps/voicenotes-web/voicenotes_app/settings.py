from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path


def _env_bool(name: str, default: bool) -> bool:
    value = os.environ.get(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int) -> int:
    value = os.environ.get(name)
    if not value:
        return default
    return int(value)


def _env_list(name: str) -> tuple[str, ...]:
    value = os.environ.get(name, "")
    return tuple(item.strip().lower() for item in value.split(",") if item.strip())


@dataclass(frozen=True)
class Settings:
    app_name: str = "VoiceNotes"
    auth_mode: str = "local"
    session_secret: str = "dev-only-voicenotes-session-secret-change-me"
    session_cookie_name: str = "voicenotes_session"
    session_ttl_seconds: int = 12 * 60 * 60
    local_users: str = "prabhat7@cisco.com:voicenotes-dev:Prabhat Singh"
    allowed_email_domains: tuple[str, ...] = ()

    oidc_issuer: str = ""
    oidc_client_id: str = ""
    oidc_client_secret: str = ""
    oidc_redirect_uri: str = ""
    oidc_authorization_endpoint: str = ""
    oidc_token_endpoint: str = ""
    oidc_jwks_uri: str = ""
    oidc_logout_url: str = ""
    oidc_user_pool_id: str = ""
    oidc_cognito_idp_endpoint_url: str = ""
    oidc_session_validation_enabled: bool = True
    oidc_session_validation_ttl_seconds: int = 5
    oidc_session_validation_request_timeout_seconds: int = 3
    oidc_recording_access_check_seconds: int = 5

    storage_backend: str = "local"
    local_data_dir: Path = Path(".voicenotes-data")
    aws_region: str = "us-west-2"
    notes_table_name: str = ""
    audit_table_name: str = ""
    transcript_bucket_name: str = ""
    transcript_kms_key_id: str = ""

    upstream_transcription_url: str = "wss://dcabsri6ekziv.cloudfront.net/v1/transcription"
    upstream_transcription_token: str = ""
    upstream_transcription_signing_secret: str = ""
    upstream_transcription_token_issuer: str = "cubicle-transcription"
    upstream_transcription_token_audience: str = "cubicle-macos"
    upstream_transcription_token_scope: str = "transcription:stream"
    upstream_transcription_token_ttl_seconds: int = 7200
    mock_transcription: bool = True
    monthly_minute_quota: int = 300
    max_recording_seconds: int = 7200
    max_concurrent_sessions_per_user: int = 1

    text_intelligence_enabled: bool = False
    text_intelligence_url: str = ""
    text_intelligence_token: str = ""
    text_intelligence_model: str = "Qwen/Qwen2.5-7B-Instruct"
    text_intelligence_context_lines: int = 2
    text_intelligence_request_timeout_seconds: int = 12
    text_intelligence_flush_timeout_seconds: int = 20
    text_intelligence_summary_enabled: bool = False
    text_intelligence_summary_timeout_seconds: int = 45

    secure_cookies: bool = False

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            auth_mode=os.environ.get("VOICENOTES_AUTH_MODE", cls.auth_mode).strip().lower(),
            session_secret=os.environ.get("VOICENOTES_SESSION_SECRET", cls.session_secret),
            session_cookie_name=os.environ.get("VOICENOTES_SESSION_COOKIE_NAME", cls.session_cookie_name),
            session_ttl_seconds=_env_int("VOICENOTES_SESSION_TTL_SECONDS", cls.session_ttl_seconds),
            local_users=os.environ.get("VOICENOTES_LOCAL_USERS", cls.local_users),
            allowed_email_domains=_env_list("VOICENOTES_ALLOWED_EMAIL_DOMAINS"),
            oidc_issuer=os.environ.get("VOICENOTES_OIDC_ISSUER", ""),
            oidc_client_id=os.environ.get("VOICENOTES_OIDC_CLIENT_ID", ""),
            oidc_client_secret=os.environ.get("VOICENOTES_OIDC_CLIENT_SECRET", ""),
            oidc_redirect_uri=os.environ.get("VOICENOTES_OIDC_REDIRECT_URI", ""),
            oidc_authorization_endpoint=os.environ.get("VOICENOTES_OIDC_AUTHORIZATION_ENDPOINT", ""),
            oidc_token_endpoint=os.environ.get("VOICENOTES_OIDC_TOKEN_ENDPOINT", ""),
            oidc_jwks_uri=os.environ.get("VOICENOTES_OIDC_JWKS_URI", ""),
            oidc_logout_url=os.environ.get("VOICENOTES_OIDC_LOGOUT_URL", ""),
            oidc_user_pool_id=os.environ.get("VOICENOTES_OIDC_USER_POOL_ID", "").strip(),
            oidc_cognito_idp_endpoint_url=os.environ.get("VOICENOTES_OIDC_COGNITO_IDP_ENDPOINT_URL", "").strip(),
            oidc_session_validation_enabled=_env_bool(
                "VOICENOTES_OIDC_SESSION_VALIDATION_ENABLED",
                cls.oidc_session_validation_enabled,
            ),
            oidc_session_validation_ttl_seconds=_env_int(
                "VOICENOTES_OIDC_SESSION_VALIDATION_TTL_SECONDS",
                cls.oidc_session_validation_ttl_seconds,
            ),
            oidc_session_validation_request_timeout_seconds=_env_int(
                "VOICENOTES_OIDC_SESSION_VALIDATION_REQUEST_TIMEOUT_SECONDS",
                cls.oidc_session_validation_request_timeout_seconds,
            ),
            oidc_recording_access_check_seconds=_env_int(
                "VOICENOTES_OIDC_RECORDING_ACCESS_CHECK_SECONDS",
                cls.oidc_recording_access_check_seconds,
            ),
            storage_backend=os.environ.get("VOICENOTES_STORAGE_BACKEND", cls.storage_backend).strip().lower(),
            local_data_dir=Path(os.environ.get("VOICENOTES_LOCAL_DATA_DIR", str(cls.local_data_dir))),
            aws_region=os.environ.get("AWS_REGION", os.environ.get("AWS_DEFAULT_REGION", cls.aws_region)),
            notes_table_name=os.environ.get("VOICENOTES_NOTES_TABLE", ""),
            audit_table_name=os.environ.get("VOICENOTES_AUDIT_TABLE", ""),
            transcript_bucket_name=os.environ.get("VOICENOTES_TRANSCRIPT_BUCKET", ""),
            transcript_kms_key_id=os.environ.get("VOICENOTES_TRANSCRIPT_KMS_KEY_ID", ""),
            upstream_transcription_url=os.environ.get(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_URL", cls.upstream_transcription_url
            ),
            upstream_transcription_token=os.environ.get("VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN", ""),
            upstream_transcription_signing_secret=os.environ.get(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_SIGNING_SECRET", ""
            ).strip(),
            upstream_transcription_token_issuer=os.environ.get(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN_ISSUER", cls.upstream_transcription_token_issuer
            ).strip()
            or cls.upstream_transcription_token_issuer,
            upstream_transcription_token_audience=os.environ.get(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN_AUDIENCE", cls.upstream_transcription_token_audience
            ).strip()
            or cls.upstream_transcription_token_audience,
            upstream_transcription_token_scope=os.environ.get(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN_SCOPE", cls.upstream_transcription_token_scope
            ).strip()
            or cls.upstream_transcription_token_scope,
            upstream_transcription_token_ttl_seconds=_env_int(
                "VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN_TTL_SECONDS",
                cls.upstream_transcription_token_ttl_seconds,
            ),
            mock_transcription=_env_bool("VOICENOTES_MOCK_TRANSCRIPTION", cls.mock_transcription),
            monthly_minute_quota=_env_int("VOICENOTES_MONTHLY_MINUTE_QUOTA", cls.monthly_minute_quota),
            max_recording_seconds=_env_int("VOICENOTES_MAX_RECORDING_SECONDS", cls.max_recording_seconds),
            max_concurrent_sessions_per_user=_env_int(
                "VOICENOTES_MAX_CONCURRENT_SESSIONS_PER_USER", cls.max_concurrent_sessions_per_user
            ),
            text_intelligence_enabled=_env_bool(
                "VOICENOTES_TEXT_INTELLIGENCE_ENABLED",
                cls.text_intelligence_enabled,
            ),
            text_intelligence_url=os.environ.get("VOICENOTES_TEXT_INTELLIGENCE_URL", "").strip(),
            text_intelligence_token=os.environ.get("VOICENOTES_TEXT_INTELLIGENCE_TOKEN", "").strip(),
            text_intelligence_model=os.environ.get(
                "VOICENOTES_TEXT_INTELLIGENCE_MODEL",
                cls.text_intelligence_model,
            ).strip()
            or cls.text_intelligence_model,
            text_intelligence_context_lines=max(
                0,
                _env_int("VOICENOTES_TEXT_INTELLIGENCE_CONTEXT_LINES", cls.text_intelligence_context_lines),
            ),
            text_intelligence_request_timeout_seconds=max(
                1,
                _env_int(
                    "VOICENOTES_TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS",
                    cls.text_intelligence_request_timeout_seconds,
                ),
            ),
            text_intelligence_flush_timeout_seconds=max(
                0,
                _env_int(
                    "VOICENOTES_TEXT_INTELLIGENCE_FLUSH_TIMEOUT_SECONDS",
                    cls.text_intelligence_flush_timeout_seconds,
                ),
            ),
            text_intelligence_summary_enabled=_env_bool(
                "VOICENOTES_TEXT_INTELLIGENCE_SUMMARY_ENABLED",
                cls.text_intelligence_summary_enabled,
            ),
            text_intelligence_summary_timeout_seconds=max(
                1,
                _env_int(
                    "VOICENOTES_TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS",
                    cls.text_intelligence_summary_timeout_seconds,
                ),
            ),
            secure_cookies=_env_bool("VOICENOTES_SECURE_COOKIES", cls.secure_cookies),
        )

    @property
    def production_auth(self) -> bool:
        return self.auth_mode in {"oidc", "alb_cognito"}
