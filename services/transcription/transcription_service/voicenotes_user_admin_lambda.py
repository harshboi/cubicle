"""Lambda entry point for VoiceNotes Cognito user and data administration."""

from __future__ import annotations

from datetime import datetime
import json
import os
import secrets
from typing import Any

import boto3
from botocore.exceptions import ClientError


USER_POOL_ID = os.environ["VOICENOTES_COGNITO_USER_POOL_ID"]
REGION = os.environ.get("VOICENOTES_COGNITO_REGION") or os.environ.get("AWS_REGION")
NOTES_TABLE = os.environ.get("VOICENOTES_NOTES_TABLE", "")
AUDIT_TABLE = os.environ.get("VOICENOTES_AUDIT_TABLE", "")
TRANSCRIPT_BUCKET = os.environ.get("VOICENOTES_TRANSCRIPT_BUCKET", "")
CLIENT = boto3.client("cognito-idp", region_name=REGION)
DDB = boto3.client("dynamodb", region_name=REGION)
S3 = boto3.client("s3", region_name=REGION)
TEMP_PASSWORD_LENGTH = 18
TEMP_PASSWORD_LOWER = "abcdefghijkmnopqrstuvwxyz"
TEMP_PASSWORD_UPPER = "ABCDEFGHJKLMNPQRSTUVWXYZ"
TEMP_PASSWORD_DIGITS = "23456789"
TEMP_PASSWORD_SYMBOLS = "!@#%*-_+="

SUPPORTED_USER_ACTIONS = {
    "admin_enable_user": "admin_enable_user",
    "admin_disable_user": "admin_disable_user",
    "admin_user_global_sign_out": "admin_user_global_sign_out",
}


def handler(event: dict[str, Any], context: Any) -> dict[str, Any]:
    try:
        action = str(event.get("action") or "")
        if action == "list_users":
            return {"ok": True, "users": _list_users()}
        if action == "admin_get_user":
            return {"ok": True, "user": _get_user(_required_username(event))}
        if action == "admin_create_user":
            _create_user(
                email=_required_email(event),
                display_name=_optional_string(event.get("display_name")),
            )
            return {"ok": True}
        if action == "admin_reset_user_password":
            _reset_password_or_resend_invite(_required_username(event))
            return {"ok": True}
        if action == "admin_delete_user":
            return {"ok": True, "purge": _delete_user_and_data(_required_username(event))}
        if action in SUPPORTED_USER_ACTIONS:
            getattr(CLIENT, SUPPORTED_USER_ACTIONS[action])(
                UserPoolId=USER_POOL_ID,
                Username=_required_username(event),
            )
            return {"ok": True}
        return {
            "ok": False,
            "error_code": "InvalidAction",
            "message": f"Unsupported VoiceNotes admin action: {action}",
        }
    except ClientError as exc:
        error = exc.response.get("Error", {})
        return {
            "ok": False,
            "error_code": str(error.get("Code") or "Unknown"),
            "message": str(error.get("Message") or exc),
        }
    except ValueError as exc:
        return {
            "ok": False,
            "error_code": "ValidationError",
            "message": str(exc),
        }


def _list_users() -> list[dict[str, Any]]:
    users: list[dict[str, Any]] = []
    paginator = CLIENT.get_paginator("list_users")
    for page in paginator.paginate(UserPoolId=USER_POOL_ID):
        users.extend(_json_safe_user(user) for user in page.get("Users", []))
    return users


def _get_user(username: str) -> dict[str, Any]:
    user = CLIENT.admin_get_user(UserPoolId=USER_POOL_ID, Username=username)
    user.setdefault("Username", username)
    return _json_safe_user(user)


def _create_user(*, email: str, display_name: str | None) -> None:
    attributes = [
        {"Name": "email", "Value": email},
        {"Name": "email_verified", "Value": "true"},
    ]
    if display_name:
        attributes.append({"Name": "name", "Value": display_name})
    CLIENT.admin_create_user(
        UserPoolId=USER_POOL_ID,
        Username=email,
        TemporaryPassword=_generate_temporary_password(),
        UserAttributes=attributes,
        DesiredDeliveryMediums=["EMAIL"],
    )


def _reset_password_or_resend_invite(username: str) -> None:
    user = CLIENT.admin_get_user(UserPoolId=USER_POOL_ID, Username=username)
    if str(user.get("UserStatus") or "") == "FORCE_CHANGE_PASSWORD":
        CLIENT.admin_create_user(
            UserPoolId=USER_POOL_ID,
            Username=_user_email(user) or username,
            TemporaryPassword=_generate_temporary_password(),
            UserAttributes=_invite_attributes(user),
            DesiredDeliveryMediums=["EMAIL"],
            MessageAction="RESEND",
        )
        return
    CLIENT.admin_reset_user_password(UserPoolId=USER_POOL_ID, Username=username)


def _delete_user_and_data(username: str) -> dict[str, int]:
    user = CLIENT.admin_get_user(UserPoolId=USER_POOL_ID, Username=username)
    email = _user_email(user) or username
    user_id = _voicenotes_user_id(email)
    purge_result = _purge_user_data(user_id)
    CLIENT.admin_delete_user(UserPoolId=USER_POOL_ID, Username=username)
    return purge_result


def _purge_user_data(user_id: str) -> dict[str, int]:
    if not NOTES_TABLE or not TRANSCRIPT_BUCKET:
        raise ValueError("VoiceNotes storage purge is not configured.")
    deleted_objects = _delete_s3_prefix(TRANSCRIPT_BUCKET, f"users/{user_id}/")
    deleted_notes = _delete_ddb_partition(NOTES_TABLE, f"USER#{user_id}")
    deleted_audit = _delete_ddb_partition(AUDIT_TABLE, f"USER#{user_id}") if AUDIT_TABLE else 0
    return {
        "s3_objects_deleted": deleted_objects,
        "notes_deleted": deleted_notes,
        "audit_events_deleted": deleted_audit,
    }


def _delete_s3_prefix(bucket: str, prefix: str) -> int:
    deleted = 0
    continuation_token: str | None = None
    while True:
        kwargs: dict[str, Any] = {"Bucket": bucket, "Prefix": prefix}
        if continuation_token:
            kwargs["ContinuationToken"] = continuation_token
        response = S3.list_objects_v2(**kwargs)
        objects = [{"Key": item["Key"]} for item in response.get("Contents", []) if item.get("Key")]
        for index in range(0, len(objects), 1000):
            chunk = objects[index : index + 1000]
            if not chunk:
                continue
            delete_response = S3.delete_objects(Bucket=bucket, Delete={"Objects": chunk, "Quiet": True})
            deleted += len(chunk) - len(delete_response.get("Errors", []))
        if not response.get("IsTruncated"):
            return deleted
        continuation_token = str(response.get("NextContinuationToken") or "")


def _delete_ddb_partition(table_name: str, pk: str) -> int:
    deleted = 0
    exclusive_start_key: dict[str, Any] | None = None
    while True:
        kwargs: dict[str, Any] = {
            "TableName": table_name,
            "KeyConditionExpression": "pk = :pk",
            "ExpressionAttributeValues": {":pk": {"S": pk}},
        }
        if exclusive_start_key:
            kwargs["ExclusiveStartKey"] = exclusive_start_key
        response = DDB.query(**kwargs)
        for item in response.get("Items", []):
            key = {"pk": item["pk"], "sk": item["sk"]}
            DDB.delete_item(TableName=table_name, Key=key)
            deleted += 1
        exclusive_start_key = response.get("LastEvaluatedKey")
        if not exclusive_start_key:
            return deleted


def _voicenotes_user_id(email: str) -> str:
    normalized = email.strip().lower()
    return normalized.replace("@", "_at_").replace(".", "_")


def _generate_temporary_password() -> str:
    characters = TEMP_PASSWORD_LOWER + TEMP_PASSWORD_UPPER + TEMP_PASSWORD_DIGITS + TEMP_PASSWORD_SYMBOLS
    password = [
        secrets.choice(TEMP_PASSWORD_LOWER),
        secrets.choice(TEMP_PASSWORD_UPPER),
        secrets.choice(TEMP_PASSWORD_DIGITS),
        secrets.choice(TEMP_PASSWORD_SYMBOLS),
    ]
    password.extend(secrets.choice(characters) for _ in range(TEMP_PASSWORD_LENGTH - len(password)))
    secrets.SystemRandom().shuffle(password)
    return "".join(password)


def _invite_attributes(user: dict[str, Any]) -> list[dict[str, str]]:
    attributes: list[dict[str, str]] = []
    for attribute in user.get("UserAttributes", []):
        if not isinstance(attribute, dict):
            continue
        name = _optional_string(attribute.get("Name"))
        value = _optional_string(attribute.get("Value"))
        if name == "sub":
            continue
        if name and value:
            attributes.append({"Name": name, "Value": value})
    return attributes


def _user_email(user: dict[str, Any]) -> str | None:
    for attribute in user.get("UserAttributes", []):
        if not isinstance(attribute, dict):
            continue
        if _optional_string(attribute.get("Name")) == "email":
            return _optional_string(attribute.get("Value"))
    return None


def _json_safe_user(user: dict[str, Any]) -> dict[str, Any]:
    return {
        key: _json_safe_value(value)
        for key, value in user.items()
        if key
        in {
            "Username",
            "Attributes",
            "UserAttributes",
            "UserCreateDate",
            "UserLastModifiedDate",
            "Enabled",
            "UserStatus",
        }
    }


def _json_safe_value(value: Any) -> Any:
    if isinstance(value, datetime):
        return value.isoformat()
    if isinstance(value, list):
        return [_json_safe_value(item) for item in value]
    if isinstance(value, dict):
        return {str(key): _json_safe_value(item) for key, item in value.items()}
    return value


def _required_username(event: dict[str, Any]) -> str:
    username = _optional_string(event.get("username"))
    if not username:
        raise ValueError("username is required")
    return username


def _required_email(event: dict[str, Any]) -> str:
    email = _optional_string(event.get("email"))
    if not email:
        raise ValueError("email is required")
    return email.lower()


def _optional_string(value: Any) -> str | None:
    if value is None:
        return None
    text = str(value).strip()
    return text or None
