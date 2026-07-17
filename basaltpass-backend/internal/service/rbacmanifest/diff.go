package rbacmanifest

import (
	"sort"
	"strings"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func captureSnapshot(tx *gorm.DB, tenantID, appID uint, lock bool) (Snapshot, error) {
	snapshot := Snapshot{Permissions: []SnapshotPermission{}, Roles: []SnapshotRole{}}
	permissionQuery := tx.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("code ASC")
	roleQuery := tx.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("code ASC")
	if lock {
		permissionQuery = permissionQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		roleQuery = roleQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var permissions []model.AppPermission
	if err := permissionQuery.Find(&permissions).Error; err != nil {
		return snapshot, err
	}
	for _, permission := range permissions {
		snapshot.Permissions = append(snapshot.Permissions, SnapshotPermission{
			Code: permission.Code, Name: permission.Name, Description: permission.Description, Category: permission.Category,
		})
	}

	var roles []model.AppRole
	if err := roleQuery.Find(&roles).Error; err != nil {
		return snapshot, err
	}
	for _, role := range roles {
		var permissionCodes []string
		if err := tx.Table("app_permissions").
			Select("app_permissions.code").
			Joins("JOIN app_role_permissions ON app_role_permissions.app_permission_id = app_permissions.id").
			Where("app_role_permissions.app_role_id = ? AND app_permissions.app_id = ? AND app_permissions.tenant_id = ?", role.ID, appID, tenantID).
			Order("app_permissions.code ASC").
			Pluck("app_permissions.code", &permissionCodes).Error; err != nil {
			return snapshot, err
		}
		snapshot.Roles = append(snapshot.Roles, SnapshotRole{
			Code: role.Code, Name: role.Name, Description: role.Description, PermissionCodes: permissionCodes,
		})
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}

func calculateDiff(tx *gorm.DB, tenantID, appID uint, current, target Snapshot) (Diff, error) {
	diff := Diff{
		PermissionsAdded: []string{}, PermissionsUpdated: []string{}, PermissionsRemoved: []string{},
		RolesAdded: []string{}, RolesUpdated: []string{}, RolesRemoved: []string{},
		RolePermissionsAdded: []string{}, RolePermissionsRemoved: []string{},
		AssignedRolesAffected: []string{}, RemovalAssignmentBlocks: []string{},
	}

	currentPermissions := make(map[string]SnapshotPermission, len(current.Permissions))
	targetPermissions := make(map[string]SnapshotPermission, len(target.Permissions))
	for _, permission := range current.Permissions {
		currentPermissions[permission.Code] = permission
	}
	for _, permission := range target.Permissions {
		targetPermissions[permission.Code] = permission
		if existing, ok := currentPermissions[permission.Code]; !ok {
			diff.PermissionsAdded = append(diff.PermissionsAdded, permission.Code)
		} else if existing != permission {
			diff.PermissionsUpdated = append(diff.PermissionsUpdated, permission.Code)
		}
	}
	for code := range currentPermissions {
		if _, ok := targetPermissions[code]; !ok {
			diff.PermissionsRemoved = append(diff.PermissionsRemoved, code)
		}
	}

	currentRoles := make(map[string]SnapshotRole, len(current.Roles))
	targetRoles := make(map[string]SnapshotRole, len(target.Roles))
	changedRoleMappings := map[string]struct{}{}
	for _, role := range current.Roles {
		currentRoles[role.Code] = role
	}
	for _, role := range target.Roles {
		targetRoles[role.Code] = role
		existing, ok := currentRoles[role.Code]
		if !ok {
			diff.RolesAdded = append(diff.RolesAdded, role.Code)
			for _, code := range role.PermissionCodes {
				diff.RolePermissionsAdded = append(diff.RolePermissionsAdded, role.Code+" -> "+code)
			}
			continue
		}
		if existing.Name != role.Name || existing.Description != role.Description {
			diff.RolesUpdated = append(diff.RolesUpdated, role.Code)
		}
		added, removed := stringSetDiff(existing.PermissionCodes, role.PermissionCodes)
		for _, code := range added {
			diff.RolePermissionsAdded = append(diff.RolePermissionsAdded, role.Code+" -> "+code)
		}
		for _, code := range removed {
			diff.RolePermissionsRemoved = append(diff.RolePermissionsRemoved, role.Code+" -> "+code)
		}
		if len(added) > 0 || len(removed) > 0 {
			changedRoleMappings[role.Code] = struct{}{}
		}
	}
	for code, role := range currentRoles {
		if _, ok := targetRoles[code]; ok {
			continue
		}
		diff.RolesRemoved = append(diff.RolesRemoved, code)
		changedRoleMappings[code] = struct{}{}
		for _, permissionCode := range role.PermissionCodes {
			diff.RolePermissionsRemoved = append(diff.RolePermissionsRemoved, code+" -> "+permissionCode)
		}
	}

	if len(changedRoleMappings) > 0 {
		codes := mapKeys(changedRoleMappings)
		var assigned []string
		if err := tx.Table("app_roles").Distinct("app_roles.code").
			Joins("JOIN app_user_roles ON app_user_roles.role_id = app_roles.id").
			Where("app_roles.app_id = ? AND app_roles.tenant_id = ? AND app_user_roles.app_id = ? AND app_roles.code IN ?", appID, tenantID, appID, codes).
			Pluck("app_roles.code", &assigned).Error; err != nil {
			return diff, err
		}
		diff.AssignedRolesAffected = assigned
		var mapped []string
		if err := tx.Table("tenant_app_grant_mappings").Distinct("app_roles.code").
			Joins("JOIN app_roles ON app_roles.id = tenant_app_grant_mappings.target_id").
			Where("tenant_app_grant_mappings.tenant_id = ? AND tenant_app_grant_mappings.app_id = ? AND tenant_app_grant_mappings.target_type = ? AND app_roles.code IN ?", tenantID, appID, model.TenantAppGrantTargetAppRole, codes).
			Pluck("app_roles.code", &mapped).Error; err != nil {
			return diff, err
		}
		diff.AssignedRolesAffected = append(diff.AssignedRolesAffected, mapped...)
	}

	for _, code := range diff.RolesRemoved {
		var count int64
		if err := tx.Table("app_user_roles").
			Joins("JOIN app_roles ON app_roles.id = app_user_roles.role_id").
			Where("app_roles.app_id = ? AND app_roles.tenant_id = ? AND app_roles.code = ? AND app_user_roles.app_id = ?", appID, tenantID, code, appID).
			Count(&count).Error; err != nil {
			return diff, err
		}
		if count > 0 {
			diff.RemovalAssignmentBlocks = append(diff.RemovalAssignmentBlocks, "role "+code+" is assigned to users")
		}
		if err := tx.Table("tenant_app_grant_mappings").
			Joins("JOIN app_roles ON app_roles.id = tenant_app_grant_mappings.target_id").
			Where("tenant_app_grant_mappings.tenant_id = ? AND tenant_app_grant_mappings.app_id = ? AND tenant_app_grant_mappings.target_type = ? AND app_roles.code = ?", tenantID, appID, model.TenantAppGrantTargetAppRole, code).
			Count(&count).Error; err != nil {
			return diff, err
		}
		if count > 0 {
			diff.RemovalAssignmentBlocks = append(diff.RemovalAssignmentBlocks, "role "+code+" is referenced by tenant-to-app mappings")
		}
	}
	for _, code := range diff.PermissionsRemoved {
		var count int64
		if err := tx.Table("app_user_permissions").
			Joins("JOIN app_permissions ON app_permissions.id = app_user_permissions.permission_id").
			Where("app_permissions.app_id = ? AND app_permissions.tenant_id = ? AND app_permissions.code = ? AND app_user_permissions.app_id = ?", appID, tenantID, code, appID).
			Count(&count).Error; err != nil {
			return diff, err
		}
		if count > 0 {
			diff.RemovalAssignmentBlocks = append(diff.RemovalAssignmentBlocks, "permission "+code+" is directly granted to users")
		}
		if err := tx.Table("tenant_app_grant_mappings").
			Joins("JOIN app_permissions ON app_permissions.id = tenant_app_grant_mappings.target_id").
			Where("tenant_app_grant_mappings.tenant_id = ? AND tenant_app_grant_mappings.app_id = ? AND tenant_app_grant_mappings.target_type = ? AND app_permissions.code = ?", tenantID, appID, model.TenantAppGrantTargetAppPermission, code).
			Count(&count).Error; err != nil {
			return diff, err
		}
		if count > 0 {
			diff.RemovalAssignmentBlocks = append(diff.RemovalAssignmentBlocks, "permission "+code+" is referenced by tenant-to-app mappings")
		}
	}

	sortDiff(&diff)
	diff.HasChanges = len(diff.PermissionsAdded)+len(diff.PermissionsUpdated)+len(diff.PermissionsRemoved)+
		len(diff.RolesAdded)+len(diff.RolesUpdated)+len(diff.RolesRemoved)+
		len(diff.RolePermissionsAdded)+len(diff.RolePermissionsRemoved) > 0
	return diff, nil
}

func stringSetDiff(current, target []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	targetSet := make(map[string]struct{}, len(target))
	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	for _, value := range target {
		targetSet[value] = struct{}{}
		if _, ok := currentSet[value]; !ok {
			added = append(added, value)
		}
	}
	for _, value := range current {
		if _, ok := targetSet[value]; !ok {
			removed = append(removed, value)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func sortDiff(diff *Diff) {
	groups := [][]string{
		diff.PermissionsAdded, diff.PermissionsUpdated, diff.PermissionsRemoved,
		diff.RolesAdded, diff.RolesUpdated, diff.RolesRemoved,
		diff.RolePermissionsAdded, diff.RolePermissionsRemoved,
		diff.AssignedRolesAffected, diff.RemovalAssignmentBlocks,
	}
	for _, group := range groups {
		sort.Strings(group)
	}
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func equalStrings(left, right []string) bool {
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}
