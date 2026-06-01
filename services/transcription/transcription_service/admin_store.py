from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Protocol


class AdminStoreError(RuntimeError):
    pass


@dataclass(frozen=True)
class AdminAudioTuningConfig:
    target_rms: float = 0.20
    rms_floor: float = 0.008
    max_gain: float = 24.0
    peak_ceiling: float = 0.92
    updated_at: str | None = None
    updated_by: str | None = None


@dataclass(frozen=True)
class AdminUserRecord:
    email: str
    display_name: str | None = None
    role: str = "transcription_user"
    status: str = "active"
    created_at: str | None = None
    updated_at: str | None = None
    notes: str | None = None


@dataclass(frozen=True)
class AdminTokenRecord:
    user_email: str
    token_id: str
    status: str = "active"
    scope: str = "transcription:stream"
    issued_at: str | None = None
    expires_at: str | None = None
    revoked_at: str | None = None
    revoked_reason: str | None = None


@dataclass(frozen=True)
class AdminUsageSummary:
    email: str
    session_count: int = 0
    total_audio_bytes: int = 0
    total_audio_ms: int = 0
    last_session_at: str | None = None
    active_token_count: int = 0
    revoked_token_count: int = 0
    expired_token_count: int = 0

    @property
    def total_audio_minutes(self) -> float:
        return self.total_audio_ms / 60_000


@dataclass(frozen=True)
class AdminUsageEvent:
    email: str
    session_id: str
    token_id: str | None = None
    auth_mode: str | None = None
    language_mode: str | None = None
    diarization_enabled: bool | None = None
    started_at: str | None = None
    stopped_at: str | None = None
    audio_bytes: int = 0
    audio_ms: int = 0
    stop_reason: str | None = None


@dataclass(frozen=True)
class AdminHistoryEvent:
    event_id: str
    event_type: str
    email: str
    occurred_at: str
    actor_email: str | None = None
    token_id: str | None = None
    reason: str | None = None
    detail: str | None = None


class AdminStore(Protocol):
    def list_users(self) -> list[AdminUserRecord]:
        ...

    def add_user(
        self,
        *,
        email: str,
        display_name: str | None = None,
        role: str = "transcription_user",
        notes: str | None = None,
    ) -> AdminUserRecord:
        ...

    def disable_user(self, *, email: str) -> AdminUserRecord:
        ...

    def revoke_all_tokens(
        self,
        *,
        email: str,
        reason: str | None = None,
    ) -> list[AdminTokenRecord]:
        ...

    def delete_user(self, *, email: str, delete_usage: bool = True) -> None:
        ...

    def record_issued_token(
        self,
        *,
        email: str,
        token_id: str,
        scope: str,
        issued_at: str,
        expires_at: str,
    ) -> AdminTokenRecord:
        ...

    def revoke_token(
        self,
        *,
        email: str,
        token_id: str,
        reason: str | None = None,
    ) -> AdminTokenRecord:
        ...

    def list_tokens(self, *, email: str) -> list[AdminTokenRecord]:
        ...

    def usage_summary(self, *, email: str) -> AdminUsageSummary:
        ...

    def record_usage_event(self, event: AdminUsageEvent) -> None:
        ...

    def record_history_event(self, event: AdminHistoryEvent) -> None:
        ...

    def list_history_events(self, *, limit: int = 100) -> list[AdminHistoryEvent]:
        ...

    def get_audio_tuning_config(self) -> AdminAudioTuningConfig | None:
        ...

    def save_audio_tuning_config(self, config: AdminAudioTuningConfig) -> AdminAudioTuningConfig:
        ...


class InMemoryAdminStore:
    def __init__(self) -> None:
        self._users: dict[str, AdminUserRecord] = {}
        self._tokens: dict[tuple[str, str], AdminTokenRecord] = {}
        self._usage_events: dict[str, list[AdminUsageEvent]] = {}
        self._history_events: list[AdminHistoryEvent] = []
        self._audio_tuning_config: AdminAudioTuningConfig | None = None

    def list_users(self) -> list[AdminUserRecord]:
        return sorted(self._users.values(), key=lambda user: user.email)

    def add_user(
        self,
        *,
        email: str,
        display_name: str | None = None,
        role: str = "transcription_user",
        notes: str | None = None,
    ) -> AdminUserRecord:
        normalized = normalize_email(email)
        now = utc_now()
        previous = self._users.get(normalized)
        user = AdminUserRecord(
            email=normalized,
            display_name=_optional_text(display_name),
            role=_optional_text(role) or "transcription_user",
            status="active",
            created_at=previous.created_at if previous else now,
            updated_at=now,
            notes=_optional_text(notes),
        )
        self._users[normalized] = user
        return user

    def disable_user(self, *, email: str) -> AdminUserRecord:
        normalized = normalize_email(email)
        previous = self._users.get(normalized)
        if previous is None:
            raise AdminStoreError("user does not exist")
        user = AdminUserRecord(
            email=previous.email,
            display_name=previous.display_name,
            role=previous.role,
            status="disabled",
            created_at=previous.created_at,
            updated_at=utc_now(),
            notes=previous.notes,
        )
        self._users[normalized] = user
        return user

    def revoke_all_tokens(
        self,
        *,
        email: str,
        reason: str | None = None,
    ) -> list[AdminTokenRecord]:
        normalized = normalize_email(email)
        if normalized not in self._users:
            raise AdminStoreError("user does not exist")
        revoked: list[AdminTokenRecord] = []
        for token in self.list_tokens(email=normalized):
            if token.status.lower() == "active" and not token.revoked_at:
                revoked.append(self.revoke_token(email=normalized, token_id=token.token_id, reason=reason))
        return revoked

    def delete_user(self, *, email: str, delete_usage: bool = True) -> None:
        normalized = normalize_email(email)
        if normalized not in self._users:
            raise AdminStoreError("user does not exist")
        del self._users[normalized]
        for key in [key for key in self._tokens if key[0] == normalized]:
            del self._tokens[key]
        if delete_usage:
            self._usage_events.pop(normalized, None)

    def record_issued_token(
        self,
        *,
        email: str,
        token_id: str,
        scope: str,
        issued_at: str,
        expires_at: str,
    ) -> AdminTokenRecord:
        normalized = normalize_email(email)
        if normalized not in self._users or self._users[normalized].status != "active":
            raise AdminStoreError("user must be active before issuing a token")
        record = AdminTokenRecord(
            user_email=normalized,
            token_id=_required_text(token_id, "token_id"),
            status="active",
            scope=_required_text(scope, "scope"),
            issued_at=_required_text(issued_at, "issued_at"),
            expires_at=_required_text(expires_at, "expires_at"),
        )
        self._tokens[(normalized, record.token_id)] = record
        return record

    def revoke_token(
        self,
        *,
        email: str,
        token_id: str,
        reason: str | None = None,
    ) -> AdminTokenRecord:
        normalized = normalize_email(email)
        token_key = (normalized, _required_text(token_id, "token_id"))
        previous = self._tokens.get(token_key)
        if previous is None:
            raise AdminStoreError("token does not exist")
        record = AdminTokenRecord(
            user_email=previous.user_email,
            token_id=previous.token_id,
            status="revoked",
            scope=previous.scope,
            issued_at=previous.issued_at,
            expires_at=previous.expires_at,
            revoked_at=utc_now(),
            revoked_reason=_optional_text(reason),
        )
        self._tokens[token_key] = record
        return record

    def list_tokens(self, *, email: str) -> list[AdminTokenRecord]:
        normalized = normalize_email(email)
        return sorted(
            (token for (user_email, _), token in self._tokens.items() if user_email == normalized),
            key=lambda token: token.issued_at or "",
            reverse=True,
        )

    def usage_summary(self, *, email: str) -> AdminUsageSummary:
        normalized = normalize_email(email)
        return _summarize_usage(
            email=normalized,
            tokens=self.list_tokens(email=normalized),
            usage_events=self._usage_events.get(normalized, []),
        )

    def record_usage_event(self, event: AdminUsageEvent) -> None:
        normalized = normalize_email(event.email)
        normalized_event = AdminUsageEvent(
            email=normalized,
            session_id=_required_text(event.session_id, "session_id"),
            token_id=_optional_text(event.token_id),
            auth_mode=_optional_text(event.auth_mode),
            language_mode=_optional_text(event.language_mode),
            diarization_enabled=event.diarization_enabled,
            started_at=_optional_text(event.started_at),
            stopped_at=_optional_text(event.stopped_at),
            audio_bytes=max(0, int(event.audio_bytes)),
            audio_ms=max(0, int(event.audio_ms)),
            stop_reason=_optional_text(event.stop_reason),
        )
        self._usage_events.setdefault(normalized, []).append(normalized_event)

    def record_history_event(self, event: AdminHistoryEvent) -> None:
        self._history_events.append(_normalize_history_event(event))

    def list_history_events(self, *, limit: int = 100) -> list[AdminHistoryEvent]:
        bounded_limit = min(max(1, int(limit)), 500)
        return sorted(
            self._history_events,
            key=lambda event: (event.occurred_at, event.event_id),
            reverse=True,
        )[:bounded_limit]

    def get_audio_tuning_config(self) -> AdminAudioTuningConfig | None:
        return self._audio_tuning_config

    def save_audio_tuning_config(self, config: AdminAudioTuningConfig) -> AdminAudioTuningConfig:
        normalized = normalize_audio_tuning_config(
            target_rms=config.target_rms,
            rms_floor=config.rms_floor,
            max_gain=config.max_gain,
            peak_ceiling=config.peak_ceiling,
            updated_at=config.updated_at or utc_now(),
            updated_by=config.updated_by,
        )
        self._audio_tuning_config = normalized
        return normalized


class DynamoDBAdminStore:
    def __init__(
        self,
        *,
        client: Any,
        user_table_name: str,
        token_ledger_table_name: str,
        audit_table_name: str | None = None,
    ) -> None:
        if not user_table_name:
            raise AdminStoreError("user table name is required")
        if not token_ledger_table_name:
            raise AdminStoreError("token ledger table name is required")
        self._client = client
        self._user_table_name = user_table_name
        self._token_ledger_table_name = token_ledger_table_name
        self._audit_table_name = audit_table_name

    def list_users(self) -> list[AdminUserRecord]:
        users: list[AdminUserRecord] = []
        request: dict[str, Any] = {"TableName": self._user_table_name}
        while True:
            response = self._client.scan(**request)
            for item in response.get("Items", []):
                pk = _item_string(item, "pk") or ""
                if not pk.startswith("USER#"):
                    continue
                users.append(_user_from_item(item))
            next_key = response.get("LastEvaluatedKey")
            if not next_key:
                break
            request["ExclusiveStartKey"] = next_key
        return sorted(users, key=lambda user: user.email)

    def add_user(
        self,
        *,
        email: str,
        display_name: str | None = None,
        role: str = "transcription_user",
        notes: str | None = None,
    ) -> AdminUserRecord:
        normalized = normalize_email(email)
        now = utc_now()
        existing = self._get_user(normalized)
        record = AdminUserRecord(
            email=normalized,
            display_name=_optional_text(display_name),
            role=_optional_text(role) or "transcription_user",
            status="active",
            created_at=_item_string(existing, "created_at") if existing else now,
            updated_at=now,
            notes=_optional_text(notes),
        )
        self._client.put_item(
            TableName=self._user_table_name,
            Item={
                "pk": {"S": user_pk(normalized)},
                "email": {"S": record.email},
                "display_name": _string_attr(record.display_name),
                "role": {"S": record.role},
                "status": {"S": record.status},
                "created_at": {"S": record.created_at or now},
                "updated_at": {"S": record.updated_at or now},
                "notes": _string_attr(record.notes),
            },
        )
        return record

    def disable_user(self, *, email: str) -> AdminUserRecord:
        normalized = normalize_email(email)
        existing = self._get_user(normalized)
        if existing is None:
            raise AdminStoreError("user does not exist")
        now = utc_now()
        self._client.update_item(
            TableName=self._user_table_name,
            Key={"pk": {"S": user_pk(normalized)}},
            UpdateExpression="SET #status = :status, updated_at = :updated_at",
            ExpressionAttributeNames={"#status": "status"},
            ExpressionAttributeValues={
                ":status": {"S": "disabled"},
                ":updated_at": {"S": now},
            },
        )
        return AdminUserRecord(
            email=normalized,
            display_name=_item_string(existing, "display_name"),
            role=_item_string(existing, "role") or "transcription_user",
            status="disabled",
            created_at=_item_string(existing, "created_at"),
            updated_at=now,
            notes=_item_string(existing, "notes"),
        )

    def revoke_all_tokens(
        self,
        *,
        email: str,
        reason: str | None = None,
    ) -> list[AdminTokenRecord]:
        normalized = normalize_email(email)
        if self._get_user(normalized) is None:
            raise AdminStoreError("user does not exist")
        revoked: list[AdminTokenRecord] = []
        for token in self.list_tokens(email=normalized):
            if token.status.lower() == "active" and not token.revoked_at:
                revoked.append(self.revoke_token(email=normalized, token_id=token.token_id, reason=reason))
        return revoked

    def delete_user(self, *, email: str, delete_usage: bool = True) -> None:
        normalized = normalize_email(email)
        if self._get_user(normalized) is None:
            raise AdminStoreError("user does not exist")
        token_items = self._query_partition_items(self._token_ledger_table_name, user_pk(normalized))
        self._delete_items(
            self._token_ledger_table_name,
            [_dynamodb_key_from_item(item) for item in token_items],
        )
        if self._audit_table_name and delete_usage:
            usage_items = self._query_partition_items(self._audit_table_name, usage_pk(normalized))
            self._delete_items(
                self._audit_table_name,
                [_dynamodb_key_from_item(item) for item in usage_items],
            )
        self._client.delete_item(
            TableName=self._user_table_name,
            Key={"pk": {"S": user_pk(normalized)}},
        )

    def record_issued_token(
        self,
        *,
        email: str,
        token_id: str,
        scope: str,
        issued_at: str,
        expires_at: str,
    ) -> AdminTokenRecord:
        normalized = normalize_email(email)
        existing = self._get_user(normalized)
        if existing is None or (_item_string(existing, "status") or "").lower() != "active":
            raise AdminStoreError("user must be active before issuing a token")
        token_id = _required_text(token_id, "token_id")
        record = AdminTokenRecord(
            user_email=normalized,
            token_id=token_id,
            status="active",
            scope=_required_text(scope, "scope"),
            issued_at=_required_text(issued_at, "issued_at"),
            expires_at=_required_text(expires_at, "expires_at"),
        )
        self._client.put_item(
            TableName=self._token_ledger_table_name,
            Item={
                "pk": {"S": user_pk(normalized)},
                "sk": {"S": token_sk(token_id)},
                "email": {"S": normalized},
                "token_id": {"S": token_id},
                "status": {"S": record.status},
                "scope": {"S": record.scope},
                "issued_at": {"S": record.issued_at or ""},
                "expires_at": {"S": record.expires_at or ""},
            },
        )
        return record

    def revoke_token(
        self,
        *,
        email: str,
        token_id: str,
        reason: str | None = None,
    ) -> AdminTokenRecord:
        normalized = normalize_email(email)
        token_id = _required_text(token_id, "token_id")
        existing = self._get_token(normalized, token_id)
        if existing is None:
            raise AdminStoreError("token does not exist")
        now = utc_now()
        self._client.update_item(
            TableName=self._token_ledger_table_name,
            Key={"pk": {"S": user_pk(normalized)}, "sk": {"S": token_sk(token_id)}},
            UpdateExpression=(
                "SET #status = :status, revoked_at = :revoked_at, "
                "revoked_reason = :revoked_reason, updated_at = :updated_at"
            ),
            ExpressionAttributeNames={"#status": "status"},
            ExpressionAttributeValues={
                ":status": {"S": "revoked"},
                ":revoked_at": {"S": now},
                ":revoked_reason": _string_attr(reason),
                ":updated_at": {"S": now},
            },
        )
        return AdminTokenRecord(
            user_email=normalized,
            token_id=token_id,
            status="revoked",
            scope=_item_string(existing, "scope") or "transcription:stream",
            issued_at=_item_string(existing, "issued_at"),
            expires_at=_item_string(existing, "expires_at"),
            revoked_at=now,
            revoked_reason=_optional_text(reason),
        )

    def list_tokens(self, *, email: str) -> list[AdminTokenRecord]:
        normalized = normalize_email(email)
        response = self._client.query(
            TableName=self._token_ledger_table_name,
            KeyConditionExpression="pk = :pk",
            ExpressionAttributeValues={":pk": {"S": user_pk(normalized)}},
        )
        tokens = [_token_from_item(item) for item in response.get("Items", [])]
        return sorted(tokens, key=lambda token: token.issued_at or "", reverse=True)

    def usage_summary(self, *, email: str) -> AdminUsageSummary:
        normalized = normalize_email(email)
        usage_events: list[AdminUsageEvent] = []
        if self._audit_table_name:
            request: dict[str, Any] = {
                "TableName": self._audit_table_name,
                "KeyConditionExpression": "pk = :pk",
                "ExpressionAttributeValues": {":pk": {"S": usage_pk(normalized)}},
            }
            while True:
                response = self._client.query(**request)
                usage_events.extend(_usage_from_item(item) for item in response.get("Items", []))
                next_key = response.get("LastEvaluatedKey")
                if not next_key:
                    break
                request["ExclusiveStartKey"] = next_key
        return _summarize_usage(
            email=normalized,
            tokens=self.list_tokens(email=normalized),
            usage_events=usage_events,
        )

    def record_usage_event(self, event: AdminUsageEvent) -> None:
        if not self._audit_table_name:
            return
        normalized = normalize_email(event.email)
        session_id = _required_text(event.session_id, "session_id")
        stopped_at = _optional_text(event.stopped_at) or utc_now()
        self._client.put_item(
            TableName=self._audit_table_name,
            Item={
                "pk": {"S": usage_pk(normalized)},
                "sk": {"S": usage_sk(stopped_at=stopped_at, session_id=session_id)},
                "email": {"S": normalized},
                "session_id": {"S": session_id},
                "token_id": _string_attr(event.token_id),
                "auth_mode": _string_attr(event.auth_mode),
                "language_mode": _string_attr(event.language_mode),
                "diarization_enabled": {"BOOL": bool(event.diarization_enabled)},
                "started_at": _string_attr(event.started_at),
                "stopped_at": {"S": stopped_at},
                "audio_bytes": {"N": str(max(0, int(event.audio_bytes)))},
                "audio_ms": {"N": str(max(0, int(event.audio_ms)))},
                "stop_reason": _string_attr(event.stop_reason),
                "event_type": {"S": "transcription_session_usage"},
            },
        )

    def record_history_event(self, event: AdminHistoryEvent) -> None:
        if not self._audit_table_name:
            return
        normalized = _normalize_history_event(event)
        self._client.put_item(
            TableName=self._audit_table_name,
            Item={
                "pk": {"S": admin_history_pk()},
                "sk": {"S": admin_history_sk(occurred_at=normalized.occurred_at, event_id=normalized.event_id)},
                "event_id": {"S": normalized.event_id},
                "event_type": {"S": normalized.event_type},
                "email": {"S": normalized.email},
                "occurred_at": {"S": normalized.occurred_at},
                "actor_email": _string_attr(normalized.actor_email),
                "token_id": _string_attr(normalized.token_id),
                "reason": _string_attr(normalized.reason),
                "detail": _string_attr(normalized.detail),
            },
        )

    def list_history_events(self, *, limit: int = 100) -> list[AdminHistoryEvent]:
        if not self._audit_table_name:
            return []
        bounded_limit = min(max(1, int(limit)), 500)
        response = self._client.query(
            TableName=self._audit_table_name,
            KeyConditionExpression="pk = :pk",
            ExpressionAttributeValues={":pk": {"S": admin_history_pk()}},
        )
        events = [_history_from_item(item) for item in response.get("Items", [])]
        return sorted(
            events,
            key=lambda event: (event.occurred_at, event.event_id),
            reverse=True,
        )[:bounded_limit]

    def get_audio_tuning_config(self) -> AdminAudioTuningConfig | None:
        response = self._client.get_item(
            TableName=self._user_table_name,
            Key={"pk": {"S": audio_tuning_config_pk()}},
            ConsistentRead=True,
        )
        item = response.get("Item")
        if item is None:
            return None
        if not isinstance(item, dict):
            raise AdminStoreError("DynamoDB audio tuning item is invalid")
        return _audio_tuning_from_item(item)

    def save_audio_tuning_config(self, config: AdminAudioTuningConfig) -> AdminAudioTuningConfig:
        normalized = normalize_audio_tuning_config(
            target_rms=config.target_rms,
            rms_floor=config.rms_floor,
            max_gain=config.max_gain,
            peak_ceiling=config.peak_ceiling,
            updated_at=config.updated_at or utc_now(),
            updated_by=config.updated_by,
        )
        self._client.put_item(
            TableName=self._user_table_name,
            Item=_audio_tuning_item(normalized),
        )
        return normalized

    def _get_user(self, email: str) -> dict[str, Any] | None:
        response = self._client.get_item(
            TableName=self._user_table_name,
            Key={"pk": {"S": user_pk(email)}},
            ConsistentRead=True,
        )
        item = response.get("Item")
        if item is not None and not isinstance(item, dict):
            raise AdminStoreError("DynamoDB user item is invalid")
        return item

    def _get_token(self, email: str, token_id: str) -> dict[str, Any] | None:
        response = self._client.get_item(
            TableName=self._token_ledger_table_name,
            Key={"pk": {"S": user_pk(email)}, "sk": {"S": token_sk(token_id)}},
            ConsistentRead=True,
        )
        item = response.get("Item")
        if item is not None and not isinstance(item, dict):
            raise AdminStoreError("DynamoDB token item is invalid")
        return item

    def _query_partition_items(self, table_name: str, pk: str) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        request: dict[str, Any] = {
            "TableName": table_name,
            "KeyConditionExpression": "pk = :pk",
            "ExpressionAttributeValues": {":pk": {"S": pk}},
        }
        while True:
            response = self._client.query(**request)
            items.extend(response.get("Items", []))
            next_key = response.get("LastEvaluatedKey")
            if not next_key:
                break
            request["ExclusiveStartKey"] = next_key
        return items

    def _delete_items(self, table_name: str, keys: list[dict[str, Any]]) -> None:
        for start in range(0, len(keys), 25):
            batch = keys[start : start + 25]
            if not batch:
                continue
            request_items = {
                table_name: [{"DeleteRequest": {"Key": key}} for key in batch],
            }
            while request_items.get(table_name):
                response = self._client.batch_write_item(RequestItems=request_items)
                request_items = response.get("UnprocessedItems", {})


class DynamoDBAudioTuningStore:
    def __init__(self, *, client: Any, table_name: str) -> None:
        if not table_name:
            raise AdminStoreError("audio tuning table name is required")
        self._client = client
        self._table_name = table_name

    def get_audio_tuning_config(self) -> AdminAudioTuningConfig | None:
        response = self._client.get_item(
            TableName=self._table_name,
            Key={"pk": {"S": audio_tuning_config_pk()}},
            ConsistentRead=True,
        )
        item = response.get("Item")
        if item is None:
            return None
        if not isinstance(item, dict):
            raise AdminStoreError("DynamoDB audio tuning item is invalid")
        return _audio_tuning_from_item(item)


def normalize_email(email: str) -> str:
    value = _required_text(email, "email").lower()
    if "@" not in value or value.startswith("@") or value.endswith("@"):
        raise AdminStoreError("email must be a valid user email")
    return value


def user_pk(email: str) -> str:
    return f"USER#{normalize_email(email)}"


def token_sk(token_id: str) -> str:
    return f"TOKEN#{_required_text(token_id, 'token_id')}"


def usage_pk(email: str) -> str:
    return f"USAGE#{normalize_email(email)}"


def usage_sk(*, stopped_at: str, session_id: str) -> str:
    return f"SESSION#{_required_text(stopped_at, 'stopped_at')}#{_required_text(session_id, 'session_id')}"


def admin_history_pk() -> str:
    return "ADMIN_HISTORY"


def admin_history_sk(*, occurred_at: str, event_id: str) -> str:
    return f"EVENT#{_required_text(occurred_at, 'occurred_at')}#{_required_text(event_id, 'event_id')}"


def audio_tuning_config_pk() -> str:
    return "CONFIG#transcription_audio_tuning"


def normalize_audio_tuning_config(
    *,
    target_rms: float,
    rms_floor: float,
    max_gain: float,
    peak_ceiling: float,
    updated_at: str | None = None,
    updated_by: str | None = None,
) -> AdminAudioTuningConfig:
    target = _bounded_float(target_rms, "target_rms", minimum=0.01, maximum=0.60)
    floor = _bounded_float(rms_floor, "rms_floor", minimum=0.0, maximum=target)
    if floor >= target:
        raise AdminStoreError("rms_floor must be lower than target_rms")
    gain = _bounded_float(max_gain, "max_gain", minimum=1.0, maximum=64.0)
    ceiling = _bounded_float(peak_ceiling, "peak_ceiling", minimum=0.10, maximum=0.99)
    return AdminAudioTuningConfig(
        target_rms=target,
        rms_floor=floor,
        max_gain=gain,
        peak_ceiling=ceiling,
        updated_at=_optional_text(updated_at),
        updated_by=_optional_text(updated_by),
    )


def _dynamodb_key_from_item(item: dict[str, Any]) -> dict[str, Any]:
    pk = item.get("pk")
    if not isinstance(pk, dict) or "S" not in pk:
        raise AdminStoreError("DynamoDB item is missing pk")
    key = {"pk": pk}
    sk = item.get("sk")
    if isinstance(sk, dict) and "S" in sk:
        key["sk"] = sk
    return key


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def _user_from_item(item: dict[str, Any]) -> AdminUserRecord:
    email = _item_string(item, "email")
    if not email:
        pk = _item_string(item, "pk") or ""
        email = pk.removeprefix("USER#")
    return AdminUserRecord(
        email=normalize_email(email),
        display_name=_item_string(item, "display_name"),
        role=_item_string(item, "role") or "transcription_user",
        status=_item_string(item, "status") or "active",
        created_at=_item_string(item, "created_at"),
        updated_at=_item_string(item, "updated_at"),
        notes=_item_string(item, "notes"),
    )


def _token_from_item(item: dict[str, Any]) -> AdminTokenRecord:
    email = _item_string(item, "email") or (_item_string(item, "pk") or "").removeprefix("USER#")
    token_id = _item_string(item, "token_id") or (_item_string(item, "sk") or "").removeprefix("TOKEN#")
    return AdminTokenRecord(
        user_email=normalize_email(email),
        token_id=_required_text(token_id, "token_id"),
        status=_item_string(item, "status") or "active",
        scope=_item_string(item, "scope") or "transcription:stream",
        issued_at=_item_string(item, "issued_at"),
        expires_at=_item_string(item, "expires_at"),
        revoked_at=_item_string(item, "revoked_at"),
        revoked_reason=_item_string(item, "revoked_reason"),
    )


def _usage_from_item(item: dict[str, Any]) -> AdminUsageEvent:
    email = _item_string(item, "email") or (_item_string(item, "pk") or "").removeprefix("USAGE#")
    return AdminUsageEvent(
        email=normalize_email(email),
        session_id=_required_text(_item_string(item, "session_id") or "", "session_id"),
        token_id=_item_string(item, "token_id"),
        auth_mode=_item_string(item, "auth_mode"),
        language_mode=_item_string(item, "language_mode"),
        diarization_enabled=_item_bool(item, "diarization_enabled"),
        started_at=_item_string(item, "started_at"),
        stopped_at=_item_string(item, "stopped_at"),
        audio_bytes=_item_int(item, "audio_bytes"),
        audio_ms=_item_int(item, "audio_ms"),
        stop_reason=_item_string(item, "stop_reason"),
    )


def _history_from_item(item: dict[str, Any]) -> AdminHistoryEvent:
    email = _item_string(item, "email") or "unknown@example.invalid"
    event_id = _item_string(item, "event_id") or (_item_string(item, "sk") or "").removeprefix("EVENT#")
    return AdminHistoryEvent(
        event_id=_required_text(event_id, "event_id"),
        event_type=_item_string(item, "event_type") or "admin_event",
        email=normalize_email(email),
        occurred_at=_item_string(item, "occurred_at") or "",
        actor_email=_item_string(item, "actor_email"),
        token_id=_item_string(item, "token_id"),
        reason=_item_string(item, "reason"),
        detail=_item_string(item, "detail"),
    )


def _audio_tuning_from_item(item: dict[str, Any]) -> AdminAudioTuningConfig:
    return normalize_audio_tuning_config(
        target_rms=_item_float(item, "target_rms", default=0.20),
        rms_floor=_item_float(item, "rms_floor", default=0.008),
        max_gain=_item_float(item, "max_gain", default=24.0),
        peak_ceiling=_item_float(item, "peak_ceiling", default=0.92),
        updated_at=_item_string(item, "updated_at"),
        updated_by=_item_string(item, "updated_by"),
    )


def _audio_tuning_item(config: AdminAudioTuningConfig) -> dict[str, Any]:
    return {
        "pk": {"S": audio_tuning_config_pk()},
        "item_type": {"S": "transcription_audio_tuning"},
        "target_rms": {"N": _format_float(config.target_rms)},
        "rms_floor": {"N": _format_float(config.rms_floor)},
        "max_gain": {"N": _format_float(config.max_gain)},
        "peak_ceiling": {"N": _format_float(config.peak_ceiling)},
        "updated_at": {"S": config.updated_at or utc_now()},
        "updated_by": _string_attr(config.updated_by),
    }


def _normalize_history_event(event: AdminHistoryEvent) -> AdminHistoryEvent:
    return AdminHistoryEvent(
        event_id=_required_text(event.event_id, "event_id"),
        event_type=_required_text(event.event_type, "event_type"),
        email=normalize_email(event.email),
        occurred_at=_required_text(event.occurred_at, "occurred_at"),
        actor_email=_optional_text(event.actor_email),
        token_id=_optional_text(event.token_id),
        reason=_optional_text(event.reason),
        detail=_optional_text(event.detail),
    )


def _summarize_usage(
    *,
    email: str,
    tokens: list[AdminTokenRecord],
    usage_events: list[AdminUsageEvent],
) -> AdminUsageSummary:
    now = datetime.now(timezone.utc)
    active = 0
    revoked = 0
    expired = 0
    for token in tokens:
        status = token.status.lower()
        if status == "revoked":
            revoked += 1
        elif _is_expired(token.expires_at, now=now):
            expired += 1
        elif status == "active":
            active += 1
    last_session_at = max((event.stopped_at or event.started_at or "" for event in usage_events), default="")
    return AdminUsageSummary(
        email=normalize_email(email),
        session_count=len(usage_events),
        total_audio_bytes=sum(max(0, int(event.audio_bytes)) for event in usage_events),
        total_audio_ms=sum(max(0, int(event.audio_ms)) for event in usage_events),
        last_session_at=last_session_at or None,
        active_token_count=active,
        revoked_token_count=revoked,
        expired_token_count=expired,
    )


def _is_expired(expires_at: str | None, *, now: datetime) -> bool:
    if not expires_at:
        return False
    try:
        parsed = datetime.fromisoformat(expires_at.replace("Z", "+00:00"))
    except ValueError:
        return False
    return parsed <= now


def _item_string(item: dict[str, Any] | None, key: str) -> str | None:
    if not item:
        return None
    value = item.get(key)
    if value is None:
        return None
    if isinstance(value, str):
        return _optional_text(value)
    if isinstance(value, dict):
        for attr_key in ("S", "N"):
            attr_value = value.get(attr_key)
            if attr_value is not None:
                return _optional_text(str(attr_value))
    return None


def _item_int(item: dict[str, Any] | None, key: str) -> int:
    value = _item_string(item, key)
    if value is None:
        return 0
    try:
        return int(value)
    except ValueError:
        return 0


def _item_float(item: dict[str, Any] | None, key: str, *, default: float) -> float:
    value = _item_string(item, key)
    if value is None:
        return default
    try:
        return float(value)
    except ValueError:
        return default


def _item_bool(item: dict[str, Any] | None, key: str) -> bool | None:
    if not item:
        return None
    value = item.get(key)
    if isinstance(value, bool):
        return value
    if isinstance(value, dict):
        if "BOOL" in value:
            return bool(value["BOOL"])
        if "S" in value:
            return value["S"].lower() in {"1", "true", "yes", "on"}
    return None


def _string_attr(value: str | None) -> dict[str, str]:
    return {"S": _optional_text(value) or ""}


def _optional_text(value: str | None) -> str | None:
    if value is None:
        return None
    stripped = value.strip()
    return stripped or None


def _required_text(value: str, name: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise AdminStoreError(f"{name} is required")
    return value.strip()


def _bounded_float(value: float, name: str, *, minimum: float, maximum: float) -> float:
    try:
        parsed = float(value)
    except (TypeError, ValueError) as exc:
        raise AdminStoreError(f"{name} must be a number") from exc
    if parsed < minimum or parsed > maximum:
        raise AdminStoreError(f"{name} must be between {_format_float(minimum)} and {_format_float(maximum)}")
    return parsed


def _format_float(value: float) -> str:
    return f"{float(value):.6f}".rstrip("0").rstrip(".")
