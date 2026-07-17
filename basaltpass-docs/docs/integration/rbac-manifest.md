---
title: Automatic RBAC manifest import
---

# Automatic RBAC manifest import

An authenticated app can submit an RBAC-only configuration draft to BasaltPass. Submission never changes live permissions: a tenant administrator must review the generated diff and approve it in the tenant console.

## Required client capability

Grant the app's OAuth client this dedicated scope:

```text
s2s.rbac.manifest.submit
```

Do not send credentials in the query string. Use the same `client_id` and `client_secret` headers as other S2S endpoints over HTTPS.

## Submit a manifest

```http
POST /api/v1/s2s/rbac/manifests
Content-Type: application/json
client_id: <app client id>
client_secret: <app client secret>
```

```json
{
  "schema_version": "1.0.0",
  "type": "basalt_rbac_bundle",
  "revision": 1,
  "permissions": [
    {
      "permission_key": "demo.read",
      "display_name": "Read demo data",
      "resource": "demo",
      "action": "read",
      "scope": "app",
      "description": "Read access",
      "status": "active"
    }
  ],
  "roles": [
    {
      "role_key": "demo.viewer",
      "display_name": "Viewer",
      "description": "Read-only access",
      "assignable": true,
      "priority": 10,
      "status": "active"
    }
  ],
  "role_permissions": [
    {
      "role_key": "demo.viewer",
      "permission_key": "demo.read",
      "effect": "allow"
    }
  ]
}
```

`revision` must increase for every new submission. Repeating the same revision and payload is idempotent; reusing a revision with different content is rejected.

BasaltPass rejects unknown fields, OAuth client configuration, user assignments, reserved permission prefixes, unsupported effects, invalid references, and oversized manifests. The authenticated client determines the app and tenant; no app or tenant identifier is accepted from the body.

## Review and rollback

Open **Tenant Console → Apps → App details → Automatic RBAC Import** to:

- inspect added, updated, and removed roles and permissions;
- see assigned roles affected by mapping changes;
- approve and atomically publish a pending manifest;
- reject a pending manifest;
- inspect immutable published revisions;
- roll back by creating a new revision from an earlier snapshot.

Publishing or rollback is refused when it would delete a role assigned to a user or a permission directly granted to a user. BasaltPass never deletes or changes user-role and user-permission assignments through the manifest workflow.

Approval verifies the effective RBAC baseline digest inside the publish transaction. If RBAC changed after the displayed diff was generated, approval returns a conflict and the app must submit a higher revision. This prevents an administrator from approving a stale diff.

The app can poll its own submission status with:

```http
GET /api/v1/s2s/rbac/manifests/{manifest_id}
```
