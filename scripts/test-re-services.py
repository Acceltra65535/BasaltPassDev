#!/usr/bin/env python3
"""Live OAuth/RBAC/isolation checks for Relock, Recal, and Renote.

Required environment variables:
  BP_ADMIN_EMAIL, BP_ADMIN_PASSWORD, BP_USER_EMAIL, BP_USER_PASSWORD
"""

import http.cookiejar
import json
import os
import urllib.error
import urllib.parse
import urllib.request


BP_BASE = os.getenv("BP_BASE_URL", "http://localhost:8101")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def no_redirect_open(opener, request):
    try:
        return opener.open(request)
    except urllib.error.HTTPError as exc:
        if exc.code in {301, 302, 303, 307, 308}:
            return exc
        raise


def json_request(url, *, method="GET", data=None, token=None, expected=200, headers=None):
    body = None if data is None else json.dumps(data).encode()
    request_headers = {"Accept": "application/json"}
    if data is not None:
        request_headers["Content-Type"] = "application/json"
    if token:
        request_headers["Authorization"] = f"Bearer {token}"
    if headers:
        request_headers.update(headers)
    request = urllib.request.Request(url, data=body, headers=request_headers, method=method)
    try:
        response = urllib.request.urlopen(request, timeout=20)
        status = response.status
        payload = json.loads(response.read() or b"{}")
    except urllib.error.HTTPError as exc:
        status = exc.code
        payload = json.loads(exc.read() or b"{}")
    if status != expected:
        raise AssertionError(f"{method} {url}: expected {expected}, got {status}: {payload}")
    return payload


def bp_login(email, password):
    payload = json_request(f"{BP_BASE}/api/v1/auth/login", method="POST", data={"identifier": email, "password": password})
    token = (payload.get("data") or {}).get("token") or payload.get("access_token")
    if not token:
        raise AssertionError(f"BasaltPass login returned no token: {payload}")
    return token


def oauth_login(backend, bp_token):
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect())
    start = no_redirect_open(opener, urllib.request.Request(f"{backend}/v1/auth/basalt/login"))
    authorize_url = start.headers["Location"]

    authorize_request = urllib.request.Request(authorize_url, headers={"Cookie": f"access_token_user={bp_token}"})
    authorize = no_redirect_open(opener, authorize_request)
    next_url = authorize.headers["Location"]

    if urllib.parse.urlsplit(next_url).path == "/oauth-consent":
        query = urllib.parse.parse_qs(urllib.parse.urlsplit(next_url).query)
        form = {key: values[0] for key, values in query.items() if values}
        form["action"] = "allow"
        consent_request = urllib.request.Request(
            f"{BP_BASE}/api/v1/oauth/consent",
            data=urllib.parse.urlencode(form).encode(),
            headers={"Content-Type": "application/x-www-form-urlencoded", "Cookie": f"access_token_user={bp_token}"},
            method="POST",
        )
        consent = no_redirect_open(opener, consent_request)
        callback_url = consent.headers["Location"]
    else:
        callback_url = next_url

    callback = no_redirect_open(opener, urllib.request.Request(callback_url))
    final_url = callback.headers["Location"]
    token = urllib.parse.parse_qs(urllib.parse.urlsplit(final_url).query).get("token", [None])[0]
    if not token:
        raise AssertionError(f"Application callback returned no local token: {final_url}")
    return token


def ids(items):
    return {item["id"] for item in items}


def check_service(name, backend, admin_bp_token, user_bp_token):
    admin_token = oauth_login(backend, admin_bp_token)
    user_token = oauth_login(backend, user_bp_token)
    me = json_request(f"{backend}/v1/auth/me", token=user_token)
    json_request(
        f"{backend}/v1/auth/login",
        method="POST",
        data={"email": "blocked@example.com", "password": "Blocked@123"},
        expected=403,
    )
    json_request(f"{backend}/v1/admin/overview", token=user_token, expected=403)
    overview = json_request(f"{backend}/v1/admin/overview", token=admin_token)
    manifest = json_request(f"{backend}/v1/integrations/basalt/status")
    if manifest.get("state") != "approved":
        raise AssertionError(f"{name} manifest is not approved: {manifest}")

    if name == "relock":
        user_item = json_request(f"{backend}/v1/clock/alarms", method="POST", token=user_token, data={"label": "RBAC user alarm", "hour": 9, "minute": 15, "timezone": "Asia/Shanghai", "repeat_days": [], "enabled": True})
        admin_item = json_request(f"{backend}/v1/clock/alarms", method="POST", token=admin_token, data={"label": "RBAC admin alarm", "hour": 10, "minute": 30, "timezone": "Asia/Shanghai", "repeat_days": [], "enabled": True})
        user_items = json_request(f"{backend}/v1/clock/alarms", token=user_token)
        admin_items = json_request(f"{backend}/v1/clock/alarms", token=admin_token)
        json_request(f"{backend}/v1/clock/alarms/{user_item['id']}", method="PUT", token=admin_token, data={"label": "forbidden", "hour": 0, "minute": 0, "timezone": "UTC", "repeat_days": [], "enabled": False}, expected=404)
    elif name == "recal":
        user_item = json_request(f"{backend}/v1/calendar/events", method="POST", token=user_token, data={"title": "RBAC user event", "description": "private", "event_date": "2026-07-15"})
        admin_item = json_request(f"{backend}/v1/calendar/events", method="POST", token=admin_token, data={"title": "RBAC admin event", "description": "private", "event_date": "2026-07-15"})
        user_items = json_request(f"{backend}/v1/calendar/events", token=user_token)
        admin_items = json_request(f"{backend}/v1/calendar/events", token=admin_token)
        json_request(f"{backend}/v1/calendar/events/{user_item['id']}", method="DELETE", token=admin_token, expected=404)
    else:
        user_item = json_request(f"{backend}/v1/notes", method="POST", token=user_token, data={"title": "RBAC user note", "content_markdown": "private user text", "tags": ["rbac"], "is_favorite": False})
        admin_item = json_request(f"{backend}/v1/notes", method="POST", token=admin_token, data={"title": "RBAC admin note", "content_markdown": "private admin text", "tags": ["rbac"], "is_favorite": False})
        user_items = json_request(f"{backend}/v1/notes", token=user_token)
        admin_items = json_request(f"{backend}/v1/notes", token=admin_token)
        json_request(f"{backend}/v1/notes/{user_item['id']}/render", token=admin_token, expected=404)

    if user_item["id"] not in ids(user_items) or admin_item["id"] in ids(user_items):
        raise AssertionError(f"{name}: normal user list is not isolated")
    if admin_item["id"] not in ids(admin_items) or user_item["id"] in ids(admin_items):
        raise AssertionError(f"{name}: admin workspace list is not isolated")

    # Leave the running demo databases free of repeatable E2E resource fixtures.
    if name == "relock":
        for item in user_items:
            if item.get("label") == "RBAC user alarm":
                json_request(f"{backend}/v1/clock/alarms/{item['id']}", method="DELETE", token=user_token)
        for item in admin_items:
            if item.get("label") == "RBAC admin alarm":
                json_request(f"{backend}/v1/clock/alarms/{item['id']}", method="DELETE", token=admin_token)
    elif name == "recal":
        for item in user_items:
            if item.get("title") == "RBAC user event":
                json_request(f"{backend}/v1/calendar/events/{item['id']}", method="DELETE", token=user_token)
        for item in admin_items:
            if item.get("title") == "RBAC admin event":
                json_request(f"{backend}/v1/calendar/events/{item['id']}", method="DELETE", token=admin_token)
    else:
        for item in user_items:
            if item.get("title") == "RBAC user note":
                json_request(f"{backend}/v1/notes/{item['id']}", method="DELETE", token=user_token)
        for item in admin_items:
            if item.get("title") == "RBAC admin note":
                json_request(f"{backend}/v1/notes/{item['id']}", method="DELETE", token=admin_token)

    return {"oauth_user": me["email"], "manifest": manifest["state"], "admin_users": overview["user_count"], "own_data_isolated": True, "normal_admin_denied": True}


def required(name):
    value = os.getenv(name)
    if not value:
        raise SystemExit(f"missing required environment variable: {name}")
    return value


def main():
    admin_token = bp_login(required("BP_ADMIN_EMAIL"), required("BP_ADMIN_PASSWORD"))
    user_token = bp_login(required("BP_USER_EMAIL"), required("BP_USER_PASSWORD"))
    results = {
        "relock": check_service("relock", "http://localhost:8108", admin_token, user_token),
        "recal": check_service("recal", "http://localhost:8110", admin_token, user_token),
        "renote": check_service("renote", "http://localhost:8109", admin_token, user_token),
    }
    print(json.dumps(results, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
