from __future__ import annotations

import os
import unittest
from unittest.mock import patch

from transcription_service.admin import (
    AdminSettings,
    _base64url_encode,
    _decode_admin_session,
    _encode_admin_session,
    create_admin_router,
)
from transcription_service.admin_store import (
    AdminHistoryEvent,
    AdminUsageEvent,
    DynamoDBAdminStore,
    InMemoryAdminStore,
    admin_history_pk,
    usage_pk,
    user_pk,
)

try:
    from fastapi import FastAPI
    from fastapi.testclient import TestClient
    import multipart  # type: ignore[import-not-found]
except ImportError:
    FastAPI = None  # type: ignore[assignment]
    TestClient = None  # type: ignore[assignment]
    multipart = None  # type: ignore[assignment]


class FakeDynamoDBClient:
    def __init__(self) -> None:
        self.tables: dict[str, dict[tuple[str, str | None], dict]] = {}

    def get_item(self, *, TableName, Key, ConsistentRead):
        del ConsistentRead
        table = self.tables.setdefault(TableName, {})
        pk = Key["pk"]["S"]
        sk = Key.get("sk", {}).get("S")
        item = table.get((pk, sk))
        if item is None:
            return {}
        return {"Item": item}

    def put_item(self, *, TableName, Item):
        table = self.tables.setdefault(TableName, {})
        pk = Item["pk"]["S"]
        sk = Item.get("sk", {}).get("S")
        table[(pk, sk)] = Item
        return {}

    def delete_item(self, *, TableName, Key):
        table = self.tables.setdefault(TableName, {})
        pk = Key["pk"]["S"]
        sk = Key.get("sk", {}).get("S")
        table.pop((pk, sk), None)
        return {}

    def batch_write_item(self, *, RequestItems):
        for table_name, writes in RequestItems.items():
            table = self.tables.setdefault(table_name, {})
            for write in writes:
                key = write["DeleteRequest"]["Key"]
                pk = key["pk"]["S"]
                sk = key.get("sk", {}).get("S")
                table.pop((pk, sk), None)
        return {"UnprocessedItems": {}}

    def update_item(
        self,
        *,
        TableName,
        Key,
        UpdateExpression,
        ExpressionAttributeNames,
        ExpressionAttributeValues,
    ):
        del UpdateExpression, ExpressionAttributeNames
        table = self.tables.setdefault(TableName, {})
        pk = Key["pk"]["S"]
        sk = Key.get("sk", {}).get("S")
        item = table.setdefault((pk, sk), {"pk": {"S": pk}})
        if sk:
            item["sk"] = {"S": sk}
        for raw_name, value in ExpressionAttributeValues.items():
            name = raw_name.removeprefix(":")
            if name == "updated_at":
                item["updated_at"] = value
            elif name == "revoked_at":
                item["revoked_at"] = value
            elif name == "revoked_reason":
                item["revoked_reason"] = value
            elif name == "status":
                item["status"] = value
        return {}

    def scan(self, **kwargs):
        table = self.tables.setdefault(kwargs["TableName"], {})
        return {"Items": list(table.values())}

    def query(self, *, TableName, KeyConditionExpression, ExpressionAttributeValues, **kwargs):
        del KeyConditionExpression, kwargs
        table = self.tables.setdefault(TableName, {})
        pk = ExpressionAttributeValues[":pk"]["S"]
        return {"Items": [item for (item_pk, _), item in table.items() if item_pk == pk]}


class AdminConsoleTests(unittest.TestCase):
    def test_admin_settings_disabled_by_default(self):
        with patch.dict(os.environ, {}, clear=True):
            settings = AdminSettings.from_environment()

        self.assertFalse(settings.enabled)
        self.assertEqual(settings.store_backend, "memory")

    def test_admin_settings_enabled_requires_separate_secrets(self):
        with patch.dict(os.environ, {"TRANSCRIPTION_ADMIN_ENABLED": "true"}, clear=True):
            with self.assertRaises(RuntimeError):
                AdminSettings.from_environment()

    def test_admin_settings_loads_cognito_logout_url(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_ADMIN_ENABLED": "true",
                "TRANSCRIPTION_ADMIN_SESSION_SECRET": "session-secret",
                "TRANSCRIPTION_TOKEN_SIGNING_SECRET": "user-signing-secret",
                "TRANSCRIPTION_ADMIN_COGNITO_LOGOUT_URL": "https://auth.example.com/logout?client_id=abc",
            },
            clear=True,
        ):
            settings = AdminSettings.from_environment()

        self.assertEqual(
            settings.cognito_logout_url,
            "https://auth.example.com/logout?client_id=abc",
        )
        self.assertTrue(settings.runtime_status()["cognito_logout_configured"])

    def test_admin_session_cookie_round_trip_and_tamper_rejection(self):
        cookie = _encode_admin_session("session-secret", ttl_seconds=300)

        session = _decode_admin_session("session-secret", cookie)
        tampered = cookie.replace(".", "x.", 1)

        self.assertIsNotNone(session)
        assert session is not None
        self.assertIsNotNone(session.csrf_token)
        self.assertIsNone(_decode_admin_session("session-secret", tampered))
        self.assertIsNone(_decode_admin_session("wrong-secret", cookie))

    def test_in_memory_admin_store_user_and_token_lifecycle(self):
        store = InMemoryAdminStore()
        user = store.add_user(email="Alice@Example.com", display_name="Alice")
        token = store.record_issued_token(
            email="alice@example.com",
            token_id="token-1",
            scope="transcription:stream",
            issued_at="2026-05-18T20:00:00Z",
            expires_at="2026-05-19T20:00:00Z",
        )
        revoked = store.revoke_token(email="alice@example.com", token_id="token-1", reason="test")
        disabled = store.disable_user(email="alice@example.com")
        deleted_store = InMemoryAdminStore()
        deleted_store.add_user(email="Bob@Example.com", display_name="Bob")
        deleted_store.record_issued_token(
            email="bob@example.com",
            token_id="token-2",
            scope="transcription:stream",
            issued_at="2026-05-18T20:00:00Z",
            expires_at="2026-05-19T20:00:00Z",
        )
        deleted_store.record_usage_event(
            AdminUsageEvent(
                email="bob@example.com",
                session_id="session-2",
                token_id="token-2",
                stopped_at="2026-05-18T20:01:00Z",
                audio_bytes=32000,
                audio_ms=1000,
            )
        )
        deleted_store.delete_user(email="bob@example.com")

        self.assertEqual(user.email, "alice@example.com")
        self.assertEqual(token.status, "active")
        self.assertEqual(revoked.status, "revoked")
        self.assertEqual(disabled.status, "disabled")
        self.assertEqual(deleted_store.list_users(), [])
        self.assertEqual(deleted_store.list_tokens(email="bob@example.com"), [])
        self.assertEqual(deleted_store.usage_summary(email="bob@example.com").session_count, 0)

    def test_in_memory_admin_store_records_history_after_user_delete(self):
        store = InMemoryAdminStore()
        store.add_user(email="alice@example.com")
        store.record_history_event(
            AdminHistoryEvent(
                event_id="event-1",
                event_type="user_deleted",
                email="alice@example.com",
                occurred_at="2026-05-18T20:00:00Z",
                actor_email="admin@example.com",
                detail="deleted from test",
            )
        )
        store.delete_user(email="alice@example.com")

        history = store.list_history_events()

        self.assertEqual(len(history), 1)
        self.assertEqual(history[0].event_type, "user_deleted")
        self.assertEqual(history[0].email, "alice@example.com")

    def test_dynamodb_admin_store_writes_registry_and_token_metadata_only(self):
        client = FakeDynamoDBClient()
        store = DynamoDBAdminStore(
            client=client,
            user_table_name="users",
            token_ledger_table_name="tokens",
            audit_table_name="audit",
        )

        store.add_user(email="alice@example.com", display_name="Alice")
        store.record_issued_token(
            email="alice@example.com",
            token_id="token-1",
            scope="transcription:stream",
            issued_at="2026-05-18T20:00:00Z",
            expires_at="2026-05-19T20:00:00Z",
        )
        store.record_issued_token(
            email="alice@example.com",
            token_id="token-2",
            scope="transcription:stream",
            issued_at="2026-05-18T20:00:00Z",
            expires_at="2026-05-19T20:00:00Z",
        )
        store.revoke_token(email="alice@example.com", token_id="token-1", reason="left team")
        store.revoke_all_tokens(email="alice@example.com", reason="cleanup")
        store.record_usage_event(
            AdminUsageEvent(
                email="alice@example.com",
                session_id="session-1",
                token_id="token-1",
                started_at="2026-05-18T20:00:00Z",
                stopped_at="2026-05-18T20:01:00Z",
                audio_bytes=32000,
                audio_ms=1000,
            )
        )

        user_item = client.tables["users"][(user_pk("alice@example.com"), None)]
        token_item = client.tables["tokens"][(user_pk("alice@example.com"), "TOKEN#token-1")]
        token_two_item = client.tables["tokens"][(user_pk("alice@example.com"), "TOKEN#token-2")]
        usage_item = client.tables["audit"][(usage_pk("alice@example.com"), "SESSION#2026-05-18T20:01:00Z#session-1")]
        usage = store.usage_summary(email="alice@example.com")

        self.assertEqual(user_item["status"]["S"], "active")
        self.assertEqual(token_item["status"]["S"], "revoked")
        self.assertEqual(token_two_item["status"]["S"], "revoked")
        self.assertEqual(usage_item["event_type"]["S"], "transcription_session_usage")
        self.assertEqual(usage.session_count, 1)
        self.assertEqual(usage.total_audio_bytes, 32000)
        self.assertNotIn("token", token_item)
        self.assertNotIn("bearer", token_item)
        store.delete_user(email="alice@example.com")
        self.assertNotIn((user_pk("alice@example.com"), None), client.tables["users"])
        self.assertEqual(
            [item for (pk, _), item in client.tables["tokens"].items() if pk == user_pk("alice@example.com")],
            [],
        )
        self.assertEqual(
            [item for (pk, _), item in client.tables["audit"].items() if pk == usage_pk("alice@example.com")],
            [],
        )

        store.record_history_event(
            AdminHistoryEvent(
                event_id="event-1",
                event_type="user_deleted",
                email="alice@example.com",
                occurred_at="2026-05-18T20:02:00Z",
                actor_email="admin@example.com",
            )
        )
        self.assertEqual(
            client.tables["audit"][(admin_history_pk(), "EVENT#2026-05-18T20:02:00Z#event-1")][
                "event_type"
            ]["S"],
            "user_deleted",
        )
        self.assertEqual(store.list_history_events()[0].email, "alice@example.com")

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_router_requires_auth_and_can_issue_user_token(self):
        settings = AdminSettings(
            enabled=True,
            admin_token="admin-secret",
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            cookie_secure=False,
        )
        store = InMemoryAdminStore()
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=store))
        client = TestClient(app)

        unauthorized = client.get("/admin/users")
        authorized = client.get("/admin/users", headers={"Authorization": "Bearer admin-secret"})
        add_user = client.post(
            "/admin/users",
            data={"email": "alice@example.com"},
            headers={"Authorization": "Bearer admin-secret"},
            follow_redirects=False,
        )
        issue_token = client.post(
            "/admin/users/alice@example.com/tokens",
            data={"ttl_seconds": "3600"},
            headers={"Authorization": "Bearer admin-secret"},
        )
        set_duration = client.post(
            "/admin/token-duration",
            data={"ttl_seconds": str(7 * 24 * 60 * 60)},
            headers={"Authorization": "Bearer admin-secret"},
            follow_redirects=False,
        )
        duration_page = client.get("/admin", headers={"Authorization": "Bearer admin-secret"})
        history = client.get("/admin/history", headers={"Authorization": "Bearer admin-secret"})

        self.assertEqual(unauthorized.status_code, 401)
        self.assertEqual(authorized.status_code, 200)
        self.assertEqual(add_user.status_code, 303)
        self.assertEqual(issue_token.status_code, 200)
        self.assertEqual(set_duration.status_code, 303)
        self.assertIn("cubicle_admin_token_ttl_seconds=", set_duration.headers.get("set-cookie", ""))
        self.assertIn("Token Duration", duration_page.text)
        self.assertIn('value="604800" selected', duration_page.text)
        self.assertEqual(history.status_code, 200)
        self.assertIn("Issued Transcription Token", issue_token.text)
        self.assertIn("alice@example.com", issue_token.text)
        self.assertIn("Next steps", issue_token.text)
        self.assertIn("Service token", issue_token.text)
        self.assertIn("Revoke this token", issue_token.text)
        self.assertIn("/admin/users/alice%40example.com/tokens/", issue_token.text)
        self.assertIn("User Management History", history.text)
        self.assertIn("User added", history.text)
        self.assertIn("Token issued", history.text)

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_revoke_form_explains_token_id(self):
        settings = AdminSettings(
            enabled=True,
            admin_token="admin-secret",
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            cookie_secure=False,
        )
        store = InMemoryAdminStore()
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=store))
        client = TestClient(app)

        client.post(
            "/admin/users",
            data={"email": "alice@example.com"},
            headers={"Authorization": "Bearer admin-secret"},
        )
        revoke = client.post(
            "/admin/users/alice@example.com/tokens/revoke",
            data={"token_id": ""},
            headers={"Authorization": "Bearer admin-secret"},
        )

        self.assertEqual(revoke.status_code, 400)
        self.assertIn("Token ID is required", revoke.text)
        self.assertIn("short UUID", revoke.text)

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_router_uses_cognito_alb_without_admin_token_prompt(self):
        settings = AdminSettings(
            enabled=True,
            admin_token=None,
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            external_auth_provider="cognito_alb",
            required_admin_group="CubicleTranscriptionAdmins",
            cookie_secure=False,
        )
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=InMemoryAdminStore()))
        client = TestClient(app)
        claims = {
            "sub": "admin-subject",
            "email": "admin@example.com",
            "cognito:groups": ["CubicleTranscriptionAdmins"],
        }
        headers = {
            "x-amzn-oidc-identity": "admin-subject",
            "x-amzn-oidc-data": _fake_jwt(claims),
        }

        unauthenticated_login = client.get("/admin/login", follow_redirects=False)
        no_identity_admin = client.get("/admin", follow_redirects=False)
        redirected = client.get("/admin", headers=headers, follow_redirects=False)
        login = client.get("/admin/login", headers=headers, follow_redirects=False)
        loaded = client.get("/admin", headers=headers)

        self.assertEqual(unauthenticated_login.status_code, 303)
        self.assertEqual(unauthenticated_login.headers.get("location"), "/admin")
        self.assertEqual(no_identity_admin.status_code, 403)
        self.assertIn("authorized admin identity", no_identity_admin.text)
        self.assertEqual(login.status_code, 303)
        self.assertEqual(login.headers.get("location"), "/admin")
        self.assertEqual(redirected.status_code, 303)
        self.assertIn("samesite=lax", redirected.headers.get("set-cookie", "").lower())
        self.assertEqual(loaded.status_code, 200)
        self.assertIn("Usage Lookup", loaded.text)
        self.assertNotIn("Admin token", loaded.text)

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_logout_clears_app_and_alb_auth_cookies(self):
        settings = AdminSettings(
            enabled=True,
            admin_token=None,
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            external_auth_provider="cognito_alb",
            cognito_logout_url="https://auth.example.com/logout?client_id=abc&logout_uri=https%3A%2F%2Fexample.com%2Fadmin%2Flogout",
            cookie_secure=False,
        )
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=InMemoryAdminStore()))
        client = TestClient(app)

        logout = client.post("/admin/logout", follow_redirects=False)
        set_cookie_headers = "\n".join(logout.headers.get_list("set-cookie"))

        self.assertEqual(logout.status_code, 303)
        self.assertEqual(logout.headers.get("location"), settings.cognito_logout_url)
        self.assertIn("cubicle_admin_session=", set_cookie_headers)
        self.assertIn("CubicleAdminAuth=", set_cookie_headers)
        self.assertIn("AWSELBAuthSessionCookie=", set_cookie_headers)
        self.assertIn("AWSELBAuthSessionCookie-0=", set_cookie_headers)
        self.assertIn("Max-Age=0", set_cookie_headers)

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_logout_landing_accepts_cognito_logout_redirect(self):
        settings = AdminSettings(
            enabled=True,
            admin_token=None,
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            external_auth_provider="cognito_alb",
            cognito_logout_url="https://auth.example.com/logout?client_id=abc&logout_uri=https%3A%2F%2Fexample.com%2Fadmin%2Flogout",
            cookie_secure=False,
        )
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=InMemoryAdminStore()))
        client = TestClient(app)

        logout = client.get("/admin/logout", follow_redirects=False)
        set_cookie_headers = "\n".join(logout.headers.get_list("set-cookie"))

        self.assertEqual(logout.status_code, 303)
        self.assertEqual(logout.headers.get("location"), "/admin/login")
        self.assertIn("cubicle_admin_session=", set_cookie_headers)
        self.assertIn("CubicleAdminAuth=", set_cookie_headers)
        self.assertIn("AWSELBAuthSessionCookie=", set_cookie_headers)
        self.assertIn("Max-Age=0", set_cookie_headers)

    @unittest.skipIf(
        FastAPI is None or TestClient is None or multipart is None,
        "FastAPI form runtime dependencies are not installed",
    )
    def test_admin_usage_lookup_shows_metadata_and_token_revoke_actions(self):
        settings = AdminSettings(
            enabled=True,
            admin_token="admin-secret",
            session_secret="session-secret",
            user_token_signing_secret="user-signing-secret",
            cookie_secure=False,
        )
        store = InMemoryAdminStore()
        store.add_user(email="alice@example.com")
        store.record_issued_token(
            email="alice@example.com",
            token_id="token-1",
            scope="transcription:stream",
            issued_at="2026-05-18T20:00:00Z",
            expires_at="2026-05-19T20:00:00Z",
        )
        store.record_usage_event(
            AdminUsageEvent(
                email="alice@example.com",
                session_id="session-1",
                token_id="token-1",
                stopped_at="2026-05-18T20:01:00Z",
                audio_bytes=32000,
                audio_ms=1000,
            )
        )
        app = FastAPI()
        app.include_router(create_admin_router(settings=settings, store=store))
        client = TestClient(app)

        usage = client.get(
            "/admin/usage?email=alice@example.com",
            headers={"Authorization": "Bearer admin-secret"},
        )

        self.assertEqual(usage.status_code, 200)
        self.assertIn("Sessions", usage.text)
        self.assertIn("1", usage.text)
        self.assertIn("32000", usage.text)
        self.assertIn("token-1", usage.text)
        self.assertIn("Revoke", usage.text)


def _fake_jwt(claims: dict) -> str:
    header = _base64url_encode(b'{"alg":"none","typ":"JWT"}')
    import json

    payload = _base64url_encode(json.dumps(claims).encode("utf-8"))
    return f"{header}.{payload}.signature"
