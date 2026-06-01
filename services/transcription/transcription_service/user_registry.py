"""User-registry authorization checks for signed transcription tokens."""

from __future__ import annotations

from dataclasses import dataclass
import os
import time
from typing import Any, Protocol


class UserRegistryError(PermissionError):
    """User or token is not authorized by the external registry."""

    pass


class UserRegistry(Protocol):
    """Authorization backend for signed per-user transcription tokens."""

    def validate_user_token(
        self,
        *,
        user_id: str,
        email: str | None,
        token_id: str | None,
    ) -> None:
        """Raise when the user/token pair is not allowed to stream."""

        ...

    def runtime_status(self) -> dict[str, Any]:
        """Return operator-safe registry readiness metadata."""

        ...


def _env_bool(name: str, *, default: bool = False) -> bool:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, *, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    try:
        return int(raw.strip())
    except ValueError as exc:
        raise UserRegistryError(f"{name} must be an integer") from exc


def _split_csv(value: str | None, *, default: frozenset[str]) -> frozenset[str]:
    if not value:
        return default
    parsed = frozenset(part.strip().lower() for part in value.replace("\n", ",").split(",") if part.strip())
    return parsed or default


@dataclass(frozen=True)
class UserRegistrySettings:
    """Environment-backed registry configuration and backend factory."""

    backend: str = "env"
    user_table_name: str | None = None
    token_ledger_table_name: str | None = None
    region_name: str | None = None
    cache_ttl_seconds: int = 30
    require_token_ledger: bool = False
    active_statuses: frozenset[str] = frozenset({"active"})
    revoked_token_statuses: frozenset[str] = frozenset({"revoked", "disabled", "inactive"})

    @classmethod
    def from_environment(cls) -> "UserRegistrySettings":
        """Load registry settings from process environment variables."""

        backend = os.environ.get("TRANSCRIPTION_USER_REGISTRY_BACKEND", "env").strip().lower() or "env"
        region_name = (
            os.environ.get("TRANSCRIPTION_USER_REGISTRY_REGION")
            or os.environ.get("AWS_REGION")
            or os.environ.get("AWS_DEFAULT_REGION")
            or ""
        ).strip()
        return cls(
            backend=backend,
            user_table_name=os.environ.get("TRANSCRIPTION_USER_REGISTRY_TABLE", "").strip() or None,
            token_ledger_table_name=os.environ.get("TRANSCRIPTION_TOKEN_LEDGER_TABLE", "").strip()
            or None,
            region_name=region_name or None,
            cache_ttl_seconds=max(
                0,
                _env_int("TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS", default=30),
            ),
            require_token_ledger=_env_bool(
                "TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER",
                default=False,
            ),
            active_statuses=_split_csv(
                os.environ.get("TRANSCRIPTION_USER_ACTIVE_STATUSES"),
                default=frozenset({"active"}),
            ),
            revoked_token_statuses=_split_csv(
                os.environ.get("TRANSCRIPTION_REVOKED_TOKEN_STATUSES"),
                default=frozenset({"revoked", "disabled", "inactive"}),
            ),
        )

    def create_registry(self) -> UserRegistry | None:
        """Build the configured registry or disable registry checks."""

        if self.backend in {"env", "disabled", "none"}:
            return None
        if self.backend != "dynamodb":
            raise UserRegistryError(f"unsupported user registry backend: {self.backend}")
        if not self.user_table_name:
            raise UserRegistryError("TRANSCRIPTION_USER_REGISTRY_TABLE is required for DynamoDB registry")
        try:
            import boto3  # type: ignore[import-not-found]
        except ImportError as exc:
            raise UserRegistryError("boto3 is required for DynamoDB user registry") from exc
        client = boto3.client("dynamodb", region_name=self.region_name)
        return DynamoDBUserRegistry(
            client=client,
            user_table_name=self.user_table_name,
            token_ledger_table_name=self.token_ledger_table_name,
            cache_ttl_seconds=self.cache_ttl_seconds,
            require_token_ledger=self.require_token_ledger,
            active_statuses=self.active_statuses,
            revoked_token_statuses=self.revoked_token_statuses,
        )

    def runtime_status(self) -> dict[str, Any]:
        """Expose config presence without leaking table contents."""

        return {
            "backend": self.backend,
            "enabled": self.backend == "dynamodb",
            "user_table_configured": bool(self.user_table_name),
            "token_ledger_table_configured": bool(self.token_ledger_table_name),
            "require_token_ledger": self.require_token_ledger,
            "cache_ttl_seconds": self.cache_ttl_seconds,
        }


@dataclass(frozen=True)
class _CachedResult:
    """Short-lived authorization cache entry, including denials."""

    expires_at: float
    error_message: str | None


class DynamoDBUserRegistry:
    """DynamoDB-backed authorization gate for active users and issued tokens."""

    def __init__(
        self,
        *,
        client: Any,
        user_table_name: str,
        token_ledger_table_name: str | None = None,
        cache_ttl_seconds: int = 30,
        require_token_ledger: bool = False,
        active_statuses: frozenset[str] = frozenset({"active"}),
        revoked_token_statuses: frozenset[str] = frozenset({"revoked", "disabled", "inactive"}),
        clock: Any = time.time,
    ) -> None:
        self._client = client
        self._user_table_name = user_table_name
        self._token_ledger_table_name = token_ledger_table_name
        self._cache_ttl_seconds = max(0, cache_ttl_seconds)
        self._require_token_ledger = require_token_ledger
        self._active_statuses = active_statuses
        self._revoked_token_statuses = revoked_token_statuses
        self._clock = clock
        self._cache: dict[tuple[str, str | None, str | None], _CachedResult] = {}

    def validate_user_token(
        self,
        *,
        user_id: str,
        email: str | None,
        token_id: str | None,
    ) -> None:
        """Validate against DynamoDB with cached allow/deny results."""

        cache_key = (user_id.lower(), email.lower() if email else None, token_id.lower() if token_id else None)
        cached = self._cache.get(cache_key)
        now = self._clock()
        if cached and cached.expires_at > now:
            if cached.error_message:
                raise UserRegistryError(cached.error_message)
            return

        try:
            self._validate_uncached(user_id=user_id, email=email, token_id=token_id)
        except UserRegistryError as exc:
            self._store_cache(cache_key, str(exc))
            raise
        except Exception as exc:
            message = "user registry lookup failed"
            self._store_cache(cache_key, message)
            raise UserRegistryError(message) from exc
        self._store_cache(cache_key, None)

    def runtime_status(self) -> dict[str, Any]:
        """Return backend readiness and cache policy metadata."""

        return {
            "backend": "dynamodb",
            "user_table_configured": bool(self._user_table_name),
            "token_ledger_table_configured": bool(self._token_ledger_table_name),
            "require_token_ledger": self._require_token_ledger,
            "cache_ttl_seconds": self._cache_ttl_seconds,
        }

    def _store_cache(self, cache_key: tuple[str, str | None, str | None], error_message: str | None) -> None:
        if self._cache_ttl_seconds <= 0:
            return
        self._cache[cache_key] = _CachedResult(
            expires_at=self._clock() + self._cache_ttl_seconds,
            error_message=error_message,
        )

    def _validate_uncached(self, *, user_id: str, email: str | None, token_id: str | None) -> None:
        user_item, user_key = self._load_user_item(user_id=user_id, email=email)
        if not user_item:
            raise UserRegistryError("user is not registered for transcription")
        status = (_item_string(user_item, "status") or "").lower()
        if status not in self._active_statuses:
            raise UserRegistryError("user is not active for transcription")
        if not self._token_ledger_table_name:
            if self._require_token_ledger:
                raise UserRegistryError("token ledger table is required")
            return
        if not token_id:
            if self._require_token_ledger:
                raise UserRegistryError("signed token id is required")
            return
        token_item = self._load_token_item(user_key=user_key, token_id=token_id)
        if not token_item:
            if self._require_token_ledger:
                raise UserRegistryError("signed token was not issued by the registry")
            return
        token_status = (_item_string(token_item, "status") or "").lower()
        if token_status in self._revoked_token_statuses or _item_string(token_item, "revoked_at"):
            raise UserRegistryError("signed token has been revoked")

    def _load_user_item(self, *, user_id: str, email: str | None) -> tuple[dict[str, Any] | None, str]:
        for principal in _candidate_principals(user_id=user_id, email=email):
            user_key = f"USER#{principal}"
            item = self._get_item(self._user_table_name, {"pk": {"S": user_key}})
            if item:
                return item, user_key
        return None, f"USER#{user_id.lower()}"

    def _load_token_item(self, *, user_key: str, token_id: str) -> dict[str, Any] | None:
        assert self._token_ledger_table_name is not None
        token_key = f"TOKEN#{token_id}"
        item = self._get_item(
            self._token_ledger_table_name,
            {"pk": {"S": user_key}, "sk": {"S": token_key}},
        )
        if item:
            return item
        return self._get_item(
            self._token_ledger_table_name,
            {"pk": {"S": token_key}, "sk": {"S": token_key}},
        )

    def _get_item(self, table_name: str, key: dict[str, dict[str, str]]) -> dict[str, Any] | None:
        response = self._client.get_item(TableName=table_name, Key=key, ConsistentRead=True)
        item = response.get("Item")
        if item is not None and not isinstance(item, dict):
            raise UserRegistryError("user registry returned invalid item")
        return item


def _candidate_principals(*, user_id: str, email: str | None) -> list[str]:
    candidates: list[str] = []
    for value in (email, user_id):
        if value and value.strip():
            normalized = value.strip().lower()
            if normalized not in candidates:
                candidates.append(normalized)
    return candidates


def _item_string(item: dict[str, Any], key: str) -> str | None:
    value = item.get(key)
    if value is None:
        return None
    if isinstance(value, str):
        return value.strip() or None
    if isinstance(value, dict):
        for attr_key in ("S", "N", "BOOL"):
            attr_value = value.get(attr_key)
            if attr_value is not None:
                return str(attr_value).strip() or None
    return None
