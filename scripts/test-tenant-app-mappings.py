#!/usr/bin/env python3
"""Live tenant-to-app grant mapping checks against the three demo services.

The test temporarily removes the normal user's explicit app role, grants the
same role through a tenant membership mapping, exercises the real OAuth/S2S
path, proves revocation, and restores the original assignments in ``finally``.

Required environment variables:
  BP_ADMIN_EMAIL, BP_ADMIN_PASSWORD, BP_USER_EMAIL, BP_USER_PASSWORD

Optional environment variables:
  BP_TENANT_ID (default 1), BP_USER_ID (default 2), BP_BASE_URL
"""

import json
import os
import runpy
from pathlib import Path


HELPERS = runpy.run_path(str(Path(__file__).with_name("test-re-services.py")))
BP_BASE = HELPERS["BP_BASE"]
bp_login = HELPERS["bp_login"]
check_service = HELPERS["check_service"]
json_request = HELPERS["json_request"]
oauth_login = HELPERS["oauth_login"]
required = HELPERS["required"]

TENANT_ID = int(os.getenv("BP_TENANT_ID", "1"))
USER_ID = int(os.getenv("BP_USER_ID", "2"))
SERVICES = (
    {"name": "relock", "app_id": int(os.getenv("BP_RELOCK_APP_ID", "8")), "role": "relock.user", "backend": "http://localhost:8108", "protected": "/v1/clock/alarms"},
    {"name": "recal", "app_id": int(os.getenv("BP_RECAL_APP_ID", "9")), "role": "recal.user", "backend": "http://localhost:8110", "protected": "/v1/calendar/events"},
    {"name": "renote", "app_id": int(os.getenv("BP_RENOTE_APP_ID", "10")), "role": "renote.user", "backend": "http://localhost:8109", "protected": "/v1/notes"},
)


def tenant_console_token(user_token):
    authorized = json_request(
        f"{BP_BASE}/api/v1/auth/console/authorize",
        method="POST",
        data={"target": "tenant", "tenant_id": TENANT_ID},
        token=user_token,
        headers={"X-Auth-Scope": "user"},
    )
    exchanged = json_request(
        f"{BP_BASE}/api/v1/auth/console/exchange",
        method="POST",
        data={"code": authorized["code"]},
    )
    if exchanged.get("scope") != "tenant" or not exchanged.get("access_token"):
        raise AssertionError(f"invalid tenant console exchange: {exchanged}")
    return exchanged["access_token"]


def tenant_request(token, path, **kwargs):
    headers = dict(kwargs.pop("headers", {}) or {})
    headers["X-Auth-Scope"] = "tenant"
    return json_request(f"{BP_BASE}/api/v1/tenant{path}", token=token, headers=headers, **kwargs)


def effective_role(grants, code):
    return next((role for role in grants.get("roles", []) if role.get("code") == code), None)


def prepare_mapping(token, service):
    app_id = service["app_id"]
    role_code = service["role"]
    roles = tenant_request(token, f"/apps/{app_id}/roles").get("roles", [])
    role = next((item for item in roles if item.get("code") == role_code), None)
    if not role:
        raise AssertionError(f"{service['name']}: app role {role_code} is missing")

    mappings = tenant_request(token, f"/apps/{app_id}/rbac/mappings").get("data", {}).get("mappings", [])
    duplicate = next(
        (
            item for item in mappings
            if item.get("source", {}).get("type") == "membership_role"
            and item.get("source", {}).get("code") == "member"
            and item.get("target", {}).get("type") == "app_role"
            and item.get("target", {}).get("id") == role["id"]
        ),
        None,
    )
    if duplicate:
        raise AssertionError(
            f"{service['name']}: refusing to mutate pre-existing mapping {duplicate['id']}"
        )

    direct = tenant_request(token, f"/apps/{app_id}/users/{USER_ID}/roles")
    was_explicit = any(item.get("id") == role["id"] for item in direct.get("roles", []))
    mapping_input = {
        "source_type": "membership_role",
        "source_id": 0,
        "source_code": "member",
        "target_type": "app_role",
        "target_id": role["id"],
        "enabled": True,
    }
    preview = tenant_request(
        token,
        f"/apps/{app_id}/rbac/mappings/preview",
        method="POST",
        data=mapping_input,
    )
    if preview.get("data", {}).get("affected_user_count", 0) < 1:
        raise AssertionError(f"{service['name']}: preview did not include the normal user: {preview}")

    created = tenant_request(
        token,
        f"/apps/{app_id}/rbac/mappings",
        method="POST",
        data=mapping_input,
        expected=201,
    )["data"]
    state = {**service, "role_id": role["id"], "mapping_id": created["id"], "was_explicit": was_explicit, "explicit_removed": False, "mapping_deleted": False}

    if was_explicit:
        tenant_request(token, f"/apps/{app_id}/users/{USER_ID}/roles/{role['id']}", method="DELETE")
        state["explicit_removed"] = True

    direct = tenant_request(token, f"/apps/{app_id}/users/{USER_ID}/roles")
    if any(item.get("id") == role["id"] for item in direct.get("roles", [])):
        raise AssertionError(f"{service['name']}: mapped role was materialized as an explicit assignment")
    mapped = effective_role(direct, role_code) or effective_role(
        tenant_request(token, f"/apps/{app_id}/users/{USER_ID}/effective-grants").get("data", {}),
        role_code,
    )
    if not mapped or not any(
        source.get("type") == "tenant_mapping" and source.get("mapping_id") == created["id"]
        for source in mapped.get("sources", [])
    ):
        raise AssertionError(f"{service['name']}: effective role has no mapping provenance: {mapped}")

    # A referenced target must not be deletable, including while the mapping is active.
    tenant_request(token, f"/apps/{app_id}/roles/{role['id']}", method="DELETE", expected=409)
    return state


def prove_revocation(token, user_bp_token, state):
    # Keep the same application session across the policy change. The demo apps
    # ask BasaltPass over S2S on every protected request, so this verifies live
    # revocation rather than merely proving that a newly issued token is denied.
    active_app_token = oauth_login(state["backend"], user_bp_token)
    json_request(f"{state['backend']}{state['protected']}", token=active_app_token)
    tenant_request(
        token,
        f"/apps/{state['app_id']}/rbac/mappings/{state['mapping_id']}",
        method="DELETE",
        expected=204,
    )
    state["mapping_deleted"] = True
    grants = tenant_request(
        token,
        f"/apps/{state['app_id']}/users/{USER_ID}/effective-grants",
    )["data"]
    if effective_role(grants, state["role"]):
        raise AssertionError(f"{state['name']}: deleted mapping still grants {state['role']}")

    json_request(
        f"{state['backend']}{state['protected']}",
        token=active_app_token,
        expected=403,
    )


def restore(token, states):
    errors = []
    for state in reversed(states):
        try:
            if state.get("was_explicit") and state.get("explicit_removed"):
                tenant_request(
                    token,
                    f"/apps/{state['app_id']}/users/{USER_ID}/roles",
                    method="POST",
                    data={"role_ids": [state["role_id"]]},
                )
                state["explicit_removed"] = False
        except Exception as exc:  # cleanup must continue for the remaining apps
            errors.append(f"restore {state['name']} explicit role: {exc}")
        try:
            if state.get("mapping_id") and not state.get("mapping_deleted"):
                tenant_request(
                    token,
                    f"/apps/{state['app_id']}/rbac/mappings/{state['mapping_id']}",
                    method="DELETE",
                    expected=204,
                )
                state["mapping_deleted"] = True
        except Exception as exc:
            errors.append(f"delete {state['name']} mapping: {exc}")
    if errors:
        raise AssertionError("; ".join(errors))


def main():
    admin_bp_token = bp_login(required("BP_ADMIN_EMAIL"), required("BP_ADMIN_PASSWORD"))
    user_bp_token = bp_login(required("BP_USER_EMAIL"), required("BP_USER_PASSWORD"))
    json_request(
        f"{BP_BASE}/api/v1/auth/console/authorize",
        method="POST",
        data={"target": "tenant", "tenant_id": TENANT_ID},
        token=user_bp_token,
        headers={"X-Auth-Scope": "user"},
        expected=403,
    )
    tenant_token = tenant_console_token(admin_bp_token)
    states = []
    results = {}
    test_error = None
    try:
        for service in SERVICES:
            states.append(prepare_mapping(tenant_token, service))

        # These calls enter through each application's OAuth callback and use its
        # own S2S credentials to fetch the dynamically resolved role from BP.
        for service in SERVICES:
            results[service["name"]] = check_service(
                service["name"], service["backend"], admin_bp_token, user_bp_token
            )

        for state in states:
            prove_revocation(tenant_token, user_bp_token, state)
            results[state["name"]]["mapping_revoked_immediately"] = True
            results[state["name"]]["no_inherited_assignment_row"] = True
    except Exception as exc:
        test_error = exc
    finally:
        try:
            restore(tenant_token, states)
        except Exception as cleanup_exc:
            if test_error:
                raise AssertionError(f"test failed: {test_error}; cleanup failed: {cleanup_exc}") from cleanup_exc
            raise
    if test_error:
        raise test_error
    print(json.dumps(results, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
