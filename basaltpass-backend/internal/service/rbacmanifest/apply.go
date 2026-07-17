package rbacmanifest

import (
	"fmt"
	"sort"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
)

func applySnapshot(tx *gorm.DB, tenantID, appID uint, snapshot Snapshot) error {
	if len(snapshot.Permissions) > 500 || len(snapshot.Roles) > 100 {
		return validationError("snapshot exceeds RBAC limits")
	}

	var existingPermissions []model.AppPermission
	if err := tx.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Find(&existingPermissions).Error; err != nil {
		return err
	}
	existingPermissionByCode := make(map[string]model.AppPermission, len(existingPermissions))
	for _, permission := range existingPermissions {
		existingPermissionByCode[permission.Code] = permission
	}

	permissionByCode := make(map[string]model.AppPermission, len(snapshot.Permissions))
	now := time.Now().UTC()
	for _, desired := range snapshot.Permissions {
		permission, exists := existingPermissionByCode[desired.Code]
		if !exists {
			permission = model.AppPermission{
				Code: desired.Code, Name: desired.Name, Description: desired.Description, Category: desired.Category,
				AppID: appID, TenantID: tenantID, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&permission).Error; err != nil {
				return err
			}
		} else {
			updates := map[string]any{"name": desired.Name, "description": desired.Description, "category": desired.Category, "updated_at": now}
			if err := tx.Model(&model.AppPermission{}).
				Where("id = ? AND app_id = ? AND tenant_id = ?", permission.ID, appID, tenantID).
				Updates(updates).Error; err != nil {
				return err
			}
			permission.Name, permission.Description, permission.Category = desired.Name, desired.Description, desired.Category
		}
		permissionByCode[desired.Code] = permission
	}

	var existingRoles []model.AppRole
	if err := tx.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Find(&existingRoles).Error; err != nil {
		return err
	}
	existingRoleByCode := make(map[string]model.AppRole, len(existingRoles))
	for _, role := range existingRoles {
		existingRoleByCode[role.Code] = role
	}

	targetRoleCodes := make(map[string]struct{}, len(snapshot.Roles))
	for _, desired := range snapshot.Roles {
		targetRoleCodes[desired.Code] = struct{}{}
		role, exists := existingRoleByCode[desired.Code]
		if !exists {
			role = model.AppRole{
				Code: desired.Code, Name: desired.Name, Description: desired.Description,
				AppID: appID, TenantID: tenantID, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&role).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&model.AppRole{}).
				Where("id = ? AND app_id = ? AND tenant_id = ?", role.ID, appID, tenantID).
				Updates(map[string]any{"name": desired.Name, "description": desired.Description, "updated_at": now}).Error; err != nil {
				return err
			}
		}

		permissions := make([]model.AppPermission, 0, len(desired.PermissionCodes))
		for _, code := range desired.PermissionCodes {
			permission, ok := permissionByCode[code]
			if !ok {
				return validationError("role %q references unknown permission %q", desired.Code, code)
			}
			permissions = append(permissions, permission)
		}
		if err := tx.Model(&role).Association("Permissions").Replace(permissions); err != nil {
			return err
		}
	}

	for _, role := range existingRoles {
		if _, keep := targetRoleCodes[role.Code]; keep {
			continue
		}
		if err := tx.Exec("DELETE FROM app_role_permissions WHERE app_role_id = ?", role.ID).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND app_id = ? AND tenant_id = ?", role.ID, appID, tenantID).Delete(&model.AppRole{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return conflictError("role %q changed concurrently", role.Code)
		}
	}

	targetPermissionCodes := make(map[string]struct{}, len(snapshot.Permissions))
	for _, permission := range snapshot.Permissions {
		targetPermissionCodes[permission.Code] = struct{}{}
	}
	for _, permission := range existingPermissions {
		if _, keep := targetPermissionCodes[permission.Code]; keep {
			continue
		}
		if err := tx.Exec("DELETE FROM app_role_permissions WHERE app_permission_id = ?", permission.ID).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND app_id = ? AND tenant_id = ?", permission.ID, appID, tenantID).Delete(&model.AppPermission{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return conflictError("permission %q changed concurrently", permission.Code)
		}
	}
	return nil
}

func ensureNoAssignmentRemovalBlocks(diff Diff) error {
	if len(diff.RemovalAssignmentBlocks) == 0 {
		return nil
	}
	blocks := append([]string{}, diff.RemovalAssignmentBlocks...)
	sort.Strings(blocks)
	return conflictError("cannot remove referenced RBAC entities: %v", blocks)
}

func snapshotFromJSON(raw string) (Snapshot, error) {
	var snapshot Snapshot
	if err := decodeStrictJSON([]byte(raw), &snapshot); err != nil {
		return snapshot, fmt.Errorf("decode revision snapshot: %w", err)
	}
	normalizeSnapshot(&snapshot)
	return snapshot, nil
}
