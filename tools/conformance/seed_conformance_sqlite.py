import json
import sqlite3
import time
import uuid

import bcrypt


DB = "conformance-basaltpass.db"
NOW = time.strftime("%Y-%m-%d %H:%M:%S")


def bcrypt_hash(value: str) -> str:
    return bcrypt.hashpw(value.encode("utf-8"), bcrypt.gensalt(rounds=12)).decode("utf-8")


def table_columns(conn, table):
    return {row[1] for row in conn.execute(f"pragma table_info({table})")}


def insert_or_update(conn, table, where_col, where_val, values):
    cols = table_columns(conn, table)
    values = {k: v for k, v in values.items() if k in cols}
    row = conn.execute(f"select id from {table} where {where_col} = ?", (where_val,)).fetchone()
    if row:
        assignments = ", ".join(f"{k}=?" for k in values)
        conn.execute(f"update {table} set {assignments} where id=?", (*values.values(), row[0]))
        return row[0]
    keys = ", ".join(values)
    placeholders = ", ".join("?" for _ in values)
    cur = conn.execute(f"insert into {table} ({keys}) values ({placeholders})", tuple(values.values()))
    return cur.lastrowid


def main():
    conn = sqlite3.connect(DB)
    conn.execute("pragma foreign_keys=off")

    tenant_id = insert_or_update(conn, "tenants", "code", "conformance", {
        "name": "Conformance Tenant",
        "code": "conformance",
        "description": "OpenID Foundation conformance test tenant",
        "status": "active",
        "plan": "free",
        "metadata": json.dumps({}),
        "created_at": NOW,
        "updated_at": NOW,
    })

    user_id = insert_or_update(conn, "system_auth_users", "email", "conformance@example.com", {
        "created_at": NOW,
        "updated_at": NOW,
        "tenant_id": tenant_id,
        "user_uuid": str(uuid.uuid4()),
        "email": "conformance@example.com",
        "phone": "+12025550123",
        "password_hash": bcrypt_hash("Passw0rd!123"),
        "nickname": "conformance",
        "given_name": "Conformance",
        "family_name": "User",
        "middle_name": "OIDC",
        "locale": "en-US",
        "zoneinfo": "America/Los_Angeles",
        "avatar_url": "https://example.com/conformance.png",
        "email_verified": 1,
        "phone_verified": 1,
        "banned": 0,
        "two_fa_enabled": 0,
        "mfa_enabled": 0,
        "risk_flags": 0,
        "is_system_admin": 0,
        "web_authn_id": b"conformance-webauthn-user-id",
    })

    existing_tu = conn.execute(
        "select id from tenant_users where user_id=? and tenant_id=?", (user_id, tenant_id)
    ).fetchone()
    if not existing_tu:
        conn.execute(
            "insert into tenant_users (user_id, tenant_id, role, created_at, updated_at) values (?, ?, ?, ?, ?)",
            (user_id, tenant_id, "member", NOW, NOW),
        )

    gender_id = insert_or_update(conn, "genders", "code", "male", {
        "created_at": NOW,
        "updated_at": NOW,
        "code": "male",
        "name": "Male",
        "name_cn": "男",
        "sort_order": 1,
        "is_active": 1,
    })

    insert_or_update(conn, "system_user_profiles", "user_id", user_id, {
        "created_at": NOW,
        "updated_at": NOW,
        "user_id": user_id,
        "gender_id": gender_id,
        "timezone": "America/Los_Angeles",
        "birth_date": "1980-01-02 00:00:00",
        "website": "https://example.com/conformance",
        "bio": "OIDC conformance user",
        "location": "Los Angeles",
    })

    app_id = insert_or_update(conn, "apps", "name", "OIDC Conformance App", {
        "tenant_id": tenant_id,
        "name": "OIDC Conformance App",
        "description": "OpenID Foundation conformance client app",
        "is_verified": 1,
        "status": "active",
        "created_at": NOW,
        "updated_at": NOW,
    })

    redirects = ",".join([
        "https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback",
        "https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback?foo=bar",
        "https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback?dummy1=lorem&dummy2=ipsum",
        "https://host.docker.internal:8443/test/a/basaltpass-basic/callback",
        "https://host.docker.internal:8443/test/a/basaltpass-basic/callback?foo=bar",
        "https://host.docker.internal:8443/test/a/basaltpass-basic/callback?dummy1=lorem&dummy2=ipsum",
    ])
    post_logout = ",".join([
        "https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/post_logout_redirect",
        "https://host.docker.internal:8443/test/a/basaltpass-basic/post_logout_redirect",
    ])

    oauth_client_table = "oauth_clients"
    if not conn.execute("select name from sqlite_master where type='table' and name='oauth_clients'").fetchone():
        oauth_client_table = "o_auth_clients"

    def upsert_client(client_id, secret, method):
        insert_or_update(conn, oauth_client_table, "client_id", client_id, {
            "created_at": NOW,
            "updated_at": NOW,
            "app_id": app_id,
            "client_id": client_id,
            "client_secret": bcrypt_hash(secret),
            "token_endpoint_auth_method": method,
            "subject_type": "public",
            "redirect_uris": redirects,
            "post_logout_redirect_uris": post_logout,
            "scopes": "openid,profile,email,offline_access,address,phone",
            "grant_types": "authorization_code,refresh_token",
            "is_active": 1,
            "allowed_origins": "https://localhost.emobix.co.uk:8443,https://host.docker.internal:8443",
            "created_by": user_id,
        })

    upsert_client("basaltpass-basic", "basic-secret", "client_secret_basic")
    upsert_client("basaltpass-basic-2", "basic-secret-2", "client_secret_basic")
    upsert_client("basaltpass-post", "post-secret", "client_secret_post")

    existing_au = conn.execute("select id from app_users where app_id=? and user_id=?", (app_id, user_id)).fetchone()
    if not existing_au:
        conn.execute(
            """
            insert into app_users
              (app_id, user_id, first_authorized_at, last_authorized_at, scopes, status, metadata, created_at, updated_at)
            values (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (app_id, user_id, NOW, NOW, "openid profile email offline_access address phone", "active", json.dumps({}), NOW, NOW),
        )

    conn.commit()
    print("seeded conformance@example.com / Passw0rd!123")
    print("clients: basaltpass-basic/basic-secret, basaltpass-basic-2/basic-secret-2, basaltpass-post/post-secret")


if __name__ == "__main__":
    main()
