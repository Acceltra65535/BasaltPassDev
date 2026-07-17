# Tenant → App grant mappings

Tenant administrators can map current tenant access to an application's roles or permissions. These mappings are tenant-owned policy and cannot be submitted by an application manifest.

## Evaluation model

BasaltPass does not create an `app_user_roles` or `app_user_permissions` record for an inherited grant. Each S2S authorization read computes:

```text
effective app roles = active explicit app roles ∪ active mapped app roles
effective app permissions = active explicit app permissions
                          ∪ permissions of effective app roles
                          ∪ active mapped app permissions
```

Supported tenant sources are:

- membership roles from `tenant_users.role`: `owner`, `admin`, `member`, or `user`;
- active Tenant RBAC role assignments;
- effective Tenant permissions, whether directly granted or inherited through an active Tenant RBAC role.

Supported app targets are App roles and App permissions. Mappings use entity IDs, so changing a display name or code does not break the relationship.

## Runtime eligibility

Effective grants are empty when any of the following is true:

- the tenant is not active;
- the app is not active;
- the global user is banned;
- the user is no longer a tenant member or the membership is banned;
- the user has not authorized the app;
- the app-user status is banned or suspended and its deadline has not expired.

An expired source assignment, disabled mapping, mapping before `valid_from`, or mapping after `valid_until` is ignored immediately. A restricted app user remains eligible because restrictions are enforced separately by the app.

Applications that require immediate revocation must query the S2S authorization endpoints on each protected request (or use a cache with an explicitly accepted short TTL). Copying grants into a long-lived application token would reintroduce stale authorization state.

## Tenant management API

All endpoints require an authenticated tenant owner or tenant administrator and are scoped to the current tenant:

```text
GET    /api/v1/tenant/apps/:app_id/rbac/mappings
GET    /api/v1/tenant/apps/:app_id/rbac/mappings/options
POST   /api/v1/tenant/apps/:app_id/rbac/mappings/preview
POST   /api/v1/tenant/apps/:app_id/rbac/mappings
PUT    /api/v1/tenant/apps/:app_id/rbac/mappings/:mapping_id
DELETE /api/v1/tenant/apps/:app_id/rbac/mappings/:mapping_id
GET    /api/v1/tenant/apps/:app_id/users/:user_id/effective-grants
```

Example mapping:

```json
{
  "source_type": "membership_role",
  "source_id": 0,
  "source_code": "admin",
  "target_type": "app_role",
  "target_id": 42,
  "enabled": true,
  "valid_from": null,
  "valid_until": null
}
```

For `tenant_role` and `tenant_permission`, provide `source_id` and leave `source_code` empty.

## Provenance and revocation

The effective-grants response includes a `sources` array for every role and permission. Explicit assignments contain an `assignment_id`; inherited grants contain the mapping ID and tenant source. An App role may have both sources. Removing one source does not remove the other.

Inherited grants are read-only in the App user screen. Revoke them by changing the user's Tenant access or by disabling/deleting the mapping.

Roles and permissions referenced by a mapping cannot be deleted manually. RBAC manifest approval and rollback also refuse to remove referenced targets. Source Tenant roles and permissions cannot be deleted until their mappings are removed.

Mapping writes and App RBAC publication/deletion serialize on database row locks. Source and target validation is repeated while locked, preventing a concurrent delete from leaving a dangling mapping.

## OAuth behavior

OAuth authorization creates or updates only the `app_users` authorization relationship. It does not copy inherited roles or permissions. This prevents access from depending on whether or when a user last logged in.
