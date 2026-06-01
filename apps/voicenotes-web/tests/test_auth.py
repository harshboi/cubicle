import base64
import json

import httpx

from voicenotes_app.auth import AuthError, CognitoUserAccessValidator, LocalUserDirectory, OIDCClient, SessionCodec, hash_password
from voicenotes_app.models import User
from voicenotes_app.settings import Settings


def test_session_codec_round_trips_user():
    codec = SessionCodec("test-secret", 3600)
    user = User.from_email("Prabhat7@Cisco.com", "Prabhat", auth_subject="cognito-sub-1")

    token = codec.encode(user)
    decoded = codec.decode(token)

    assert decoded.email == "prabhat7@cisco.com"
    assert decoded.display_name == "Prabhat"
    assert decoded.auth_subject == "cognito-sub-1"


def test_session_codec_rejects_tampering():
    codec = SessionCodec("test-secret", 3600)
    token = codec.encode(User.from_email("a@example.com"))

    try:
        codec.decode(token + "x")
    except AuthError:
        pass
    else:
        raise AssertionError("tampered token accepted")


def test_local_directory_accepts_hashed_password():
    password_hash = hash_password("correct-password", iterations=10)
    directory = LocalUserDirectory(f"u@example.com:{password_hash}:User")

    user = directory.authenticate("u@example.com", "correct-password")

    assert user.email == "u@example.com"
    assert user.display_name == "User"


def test_local_directory_rejects_wrong_password():
    directory = LocalUserDirectory("u@example.com:correct:User")

    try:
        directory.authenticate("u@example.com", "wrong")
    except AuthError:
        pass
    else:
        raise AssertionError("wrong password accepted")


def test_oidc_logout_url_uses_login_redirect():
    settings = Settings(
        oidc_client_id="client-id",
        oidc_redirect_uri="https://voicenotes.example.com/auth/callback",
        oidc_logout_url="https://auth.example.com/logout",
    )

    url = OIDCClient(settings).logout_url()

    assert url == (
        "https://auth.example.com/logout?"
        "client_id=client-id&logout_uri=https%3A%2F%2Fvoicenotes.example.com%2Flogin"
    )


async def test_cognito_user_access_validator_accepts_enabled_user():
    fake_cognito = FakeCognito(
        [
            {
                "Enabled": True,
                "UserStatus": "CONFIRMED",
                "Attributes": [
                    {"Name": "email", "Value": "User@Example.com"},
                    {"Name": "sub", "Value": "subject-1"},
                ],
            }
        ]
    )
    settings = Settings(
        auth_mode="oidc",
        oidc_issuer="https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example",
    )

    await CognitoUserAccessValidator(settings, fake_cognito).validate(
        User.from_email("user@example.com", auth_subject="subject-1")
    )

    assert fake_cognito.calls[0]["UserPoolId"] == "us-west-2_example"
    assert fake_cognito.calls[0]["Filter"] == 'email = "user@example.com"'


async def test_cognito_user_access_validator_rejects_missing_user():
    fake_cognito = FakeCognito([])
    settings = Settings(auth_mode="oidc", oidc_user_pool_id="pool-id")

    try:
        await CognitoUserAccessValidator(settings, fake_cognito).validate(
            User.from_email("deleted@example.com", auth_subject="deleted-sub")
        )
    except AuthError:
        pass
    else:
        raise AssertionError("deleted user accepted")


async def test_cognito_user_access_validator_rejects_disabled_user():
    fake_cognito = FakeCognito(
        [
            {
                "Enabled": False,
                "UserStatus": "CONFIRMED",
                "Attributes": [
                    {"Name": "email", "Value": "disabled@example.com"},
                    {"Name": "sub", "Value": "disabled-sub"},
                ],
            }
        ]
    )
    settings = Settings(auth_mode="oidc", oidc_user_pool_id="pool-id")

    try:
        await CognitoUserAccessValidator(settings, fake_cognito).validate(
            User.from_email("disabled@example.com", auth_subject="disabled-sub")
        )
    except AuthError:
        pass
    else:
        raise AssertionError("disabled user accepted")


async def test_cognito_user_access_validator_rejects_pending_first_login_user():
    fake_cognito = FakeCognito(
        [
            {
                "Enabled": True,
                "UserStatus": "FORCE_CHANGE_PASSWORD",
                "Attributes": [
                    {"Name": "email", "Value": "pending@example.com"},
                    {"Name": "sub", "Value": "pending-sub"},
                ],
            }
        ]
    )
    settings = Settings(auth_mode="oidc", oidc_user_pool_id="pool-id")

    try:
        await CognitoUserAccessValidator(settings, fake_cognito).validate(
            User.from_email("pending@example.com", auth_subject="pending-sub")
        )
    except AuthError:
        pass
    else:
        raise AssertionError("pending first-login user accepted")


async def test_cognito_user_access_validator_rejects_recreated_user_with_same_email():
    fake_cognito = FakeCognito(
        [
            {
                "Enabled": True,
                "UserStatus": "CONFIRMED",
                "Attributes": [
                    {"Name": "email", "Value": "user@example.com"},
                    {"Name": "sub", "Value": "new-subject"},
                ],
            }
        ]
    )
    settings = Settings(auth_mode="oidc", oidc_user_pool_id="pool-id")

    try:
        await CognitoUserAccessValidator(settings, fake_cognito).validate(
            User.from_email("user@example.com", auth_subject="old-deleted-subject")
        )
    except AuthError:
        pass
    else:
        raise AssertionError("recreated user accepted with stale subject")


def test_cognito_user_access_validator_uses_configured_endpoint(monkeypatch):
    import boto3

    captured: dict = {}

    def fake_client(service_name: str, **kwargs):
        captured["service_name"] = service_name
        captured.update(kwargs)
        return FakeCognito([])

    monkeypatch.setattr(boto3, "client", fake_client)
    settings = Settings(
        auth_mode="oidc",
        oidc_user_pool_id="pool-id",
        oidc_cognito_idp_endpoint_url="https://cognito-idp-fips.us-west-2.amazonaws.com",
    )

    CognitoUserAccessValidator(settings)._cognito_client()

    assert captured["service_name"] == "cognito-idp"
    assert captured["endpoint_url"] == "https://cognito-idp-fips.us-west-2.amazonaws.com"


async def test_oidc_exchange_falls_back_to_userinfo_when_jwks_times_out(monkeypatch):
    id_token = _fake_jwt({"nonce": "nonce-1", "sub": "user-sub"})
    requests: list[tuple[str, str, dict]] = []

    class FakeResponse:
        def __init__(self, status_code: int, payload: dict):
            self.status_code = status_code
            self._payload = payload

        def json(self):
            return self._payload

        def raise_for_status(self) -> None:
            if self.status_code >= 400:
                raise httpx.HTTPStatusError("failed", request=None, response=None)

    class FakeAsyncClient:
        def __init__(self, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

        async def post(self, url: str, **kwargs):
            requests.append(("POST", url, kwargs))
            return FakeResponse(200, {"id_token": id_token, "access_token": "access-token"})

        async def get(self, url: str, **kwargs):
            requests.append(("GET", url, kwargs))
            if url.endswith("/.well-known/jwks.json"):
                raise httpx.ConnectTimeout("jwks timeout")
            return FakeResponse(200, {"email": "User@Example.com", "name": "User Example", "sub": "user-sub"})

    monkeypatch.setattr("voicenotes_app.auth.httpx.AsyncClient", FakeAsyncClient)
    settings = Settings(
        oidc_client_id="client-id",
        oidc_redirect_uri="https://voicenotes.example.com/auth/callback",
        oidc_token_endpoint="https://auth.example.com/oauth2/token",
        oidc_jwks_uri="https://cognito-idp.example.com/pool/.well-known/jwks.json",
    )

    user = await OIDCClient(settings).exchange_code("auth-code", "nonce-1")

    assert user.email == "user@example.com"
    assert user.display_name == "User Example"
    assert user.auth_subject == "user-sub"
    assert ("GET", "https://auth.example.com/oauth2/userInfo", {"headers": {"Authorization": "Bearer access-token"}}) in requests


def _fake_jwt(claims: dict) -> str:
    header = _b64url(b'{"alg":"none","typ":"JWT"}')
    payload = _b64url(json.dumps(claims).encode("utf-8"))
    return f"{header}.{payload}.signature"


def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode("ascii").rstrip("=")


class FakeCognito:
    def __init__(self, users: list[dict]):
        self.users = users
        self.calls: list[dict] = []

    def list_users(self, **kwargs):
        self.calls.append(kwargs)
        return {"Users": self.users}
