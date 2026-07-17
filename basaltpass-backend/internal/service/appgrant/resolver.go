package appgrant

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
)

type explicitRoleRow struct {
	AssignmentID uint
	ID           uint
	Code         string
	Name         string
	Description  string
	AppID        uint
}

type explicitPermissionRow struct {
	AssignmentID uint
	ID           uint
	Code         string
	Name         string
	Description  string
	Category     string
	AppID        uint
}

type rolePermissionRow struct {
	RoleID       uint
	PermissionID uint
	Code         string
	Name         string
	Description  string
	Category     string
	AppID        uint
}

// Resolve computes current app grants without writing derived assignments.
func (s *Service) Resolve(userID, tenantID, appID uint, now time.Time) (EffectiveGrants, error) {
	result := EffectiveGrants{Roles: []EffectiveRole{}, Permissions: []EffectivePermission{}}
	if userID == 0 || tenantID == 0 || appID == 0 {
		return result, &ValidationError{Message: "invalid effective grant context"}
	}
	now = now.UTC()

	var tenant model.Tenant
	if err := s.db.Select("id", "status").First(&tenant, tenantID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, ErrNotFound
		}
		return result, err
	}
	if tenant.Status != model.TenantStatusActive {
		result.DenialReason = "tenant_inactive"
		return result, nil
	}

	var app model.App
	if err := s.db.Where("id = ? AND tenant_id = ?", appID, tenantID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, ErrNotFound
		}
		return result, err
	}
	if app.Status != model.AppStatusActive {
		result.DenialReason = "app_inactive"
		return result, nil
	}

	var membership model.TenantUser
	if err := s.db.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.DenialReason = "tenant_membership_missing"
			return result, nil
		}
		return result, err
	}
	if membership.Role == model.TenantRoleBanned {
		result.DenialReason = "tenant_membership_banned"
		return result, nil
	}
	var user model.User
	if err := s.db.Select("id", "banned").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.DenialReason = "user_inactive"
			return result, nil
		}
		return result, err
	}
	if user.Banned {
		result.DenialReason = "user_banned"
		return result, nil
	}

	var appUser model.AppUser
	if err := s.db.Where("user_id = ? AND app_id = ?", userID, appID).First(&appUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.DenialReason = "app_authorization_missing"
			return result, nil
		}
		return result, err
	}
	if !appUserAllowsGrants(appUser, now) {
		result.DenialReason = "app_user_inactive"
		return result, nil
	}
	result.Eligible = true

	tenantRoleIDs, tenantPermissionIDs, err := s.userTenantGrantSets(userID, tenantID, now)
	if err != nil {
		return EffectiveGrants{}, err
	}

	roleMap := map[uint]*EffectiveRole{}
	permissionMap := map[uint]*EffectivePermission{}

	var explicitRoles []explicitRoleRow
	if err := s.db.Table("app_user_roles").
		Select("app_user_roles.id AS assignment_id, app_roles.id, app_roles.code, app_roles.name, app_roles.description, app_roles.app_id").
		Joins("JOIN app_roles ON app_roles.id = app_user_roles.role_id").
		Where("app_user_roles.user_id = ? AND app_user_roles.app_id = ? AND app_roles.tenant_id = ?", userID, appID, tenantID).
		Where("app_user_roles.expires_at IS NULL OR app_user_roles.expires_at > ?", now).
		Scan(&explicitRoles).Error; err != nil {
		return EffectiveGrants{}, err
	}
	for _, row := range explicitRoles {
		role := ensureRole(roleMap, row.ID, row.Code, row.Name, row.Description, row.AppID)
		addSource(&role.Sources, GrantSource{Type: "explicit", AssignmentID: row.AssignmentID})
	}

	var explicitPermissions []explicitPermissionRow
	if err := s.db.Table("app_user_permissions").
		Select("app_user_permissions.id AS assignment_id, app_permissions.id, app_permissions.code, app_permissions.name, app_permissions.description, app_permissions.category, app_permissions.app_id").
		Joins("JOIN app_permissions ON app_permissions.id = app_user_permissions.permission_id").
		Where("app_user_permissions.user_id = ? AND app_user_permissions.app_id = ? AND app_permissions.tenant_id = ?", userID, appID, tenantID).
		Where("app_user_permissions.expires_at IS NULL OR app_user_permissions.expires_at > ?", now).
		Scan(&explicitPermissions).Error; err != nil {
		return EffectiveGrants{}, err
	}
	for _, row := range explicitPermissions {
		permission := ensurePermission(permissionMap, row.ID, row.Code, row.Name, row.Description, row.Category, row.AppID)
		addSource(&permission.Sources, GrantSource{Type: "explicit", AssignmentID: row.AssignmentID})
	}

	var mappings []model.TenantAppGrantMapping
	if err := s.db.Where("tenant_id = ? AND app_id = ? AND enabled = ?", tenantID, appID, true).
		Where("valid_from IS NULL OR valid_from <= ?", now).
		Where("valid_until IS NULL OR valid_until > ?", now).
		Order("id ASC").Find(&mappings).Error; err != nil {
		return EffectiveGrants{}, err
	}
	for _, mapping := range mappings {
		if !sourceMatchesUser(mapping, membership.Role, tenantRoleIDs, tenantPermissionIDs) {
			continue
		}
		sourceCode, err := s.mappingSourceCode(mapping)
		if err != nil {
			return EffectiveGrants{}, err
		}
		source := GrantSource{Type: "tenant_mapping", MappingID: mapping.ID, SourceType: string(mapping.SourceType), SourceID: mapping.SourceID, SourceCode: sourceCode}
		switch mapping.TargetType {
		case model.TenantAppGrantTargetAppRole:
			var target model.AppRole
			if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ?", mapping.TargetID, tenantID, appID).First(&target).Error; err != nil {
				return EffectiveGrants{}, fmt.Errorf("mapped app role is missing: %w", err)
			}
			role := ensureRole(roleMap, target.ID, target.Code, target.Name, target.Description, target.AppID)
			addSource(&role.Sources, source)
		case model.TenantAppGrantTargetAppPermission:
			var target model.AppPermission
			if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ?", mapping.TargetID, tenantID, appID).First(&target).Error; err != nil {
				return EffectiveGrants{}, fmt.Errorf("mapped app permission is missing: %w", err)
			}
			permission := ensurePermission(permissionMap, target.ID, target.Code, target.Name, target.Description, target.Category, target.AppID)
			addSource(&permission.Sources, source)
		default:
			return EffectiveGrants{}, &ValidationError{Message: "invalid stored target_type"}
		}
	}

	roleIDs := make([]uint, 0, len(roleMap))
	for id := range roleMap {
		roleIDs = append(roleIDs, id)
	}
	if len(roleIDs) > 0 {
		var links []rolePermissionRow
		if err := s.db.Table("app_role_permissions").
			Select("app_role_permissions.app_role_id AS role_id, app_permissions.id AS permission_id, app_permissions.code, app_permissions.name, app_permissions.description, app_permissions.category, app_permissions.app_id").
			Joins("JOIN app_permissions ON app_permissions.id = app_role_permissions.app_permission_id").
			Where("app_role_permissions.app_role_id IN ? AND app_permissions.tenant_id = ? AND app_permissions.app_id = ?", roleIDs, tenantID, appID).
			Scan(&links).Error; err != nil {
			return EffectiveGrants{}, err
		}
		for _, link := range links {
			role := roleMap[link.RoleID]
			appPermission := model.AppPermission{ID: link.PermissionID, Code: link.Code, Name: link.Name, Description: link.Description, Category: link.Category, AppID: link.AppID, TenantID: tenantID}
			role.Permissions = append(role.Permissions, appPermission)
			permission := ensurePermission(permissionMap, link.PermissionID, link.Code, link.Name, link.Description, link.Category, link.AppID)
			for _, roleSource := range role.Sources {
				roleSource.ViaRoleCode = role.Code
				addSource(&permission.Sources, roleSource)
			}
		}
	}

	for _, role := range roleMap {
		sort.Slice(role.Permissions, func(i, j int) bool { return role.Permissions[i].Code < role.Permissions[j].Code })
		sortSources(role.Sources)
		result.Roles = append(result.Roles, *role)
	}
	for _, permission := range permissionMap {
		sortSources(permission.Sources)
		result.Permissions = append(result.Permissions, *permission)
	}
	sort.Slice(result.Roles, func(i, j int) bool { return result.Roles[i].Code < result.Roles[j].Code })
	sort.Slice(result.Permissions, func(i, j int) bool { return result.Permissions[i].Code < result.Permissions[j].Code })
	return result, nil
}

func appUserAllowsGrants(appUser model.AppUser, now time.Time) bool {
	switch appUser.Status {
	case model.AppUserStatusActive, model.AppUserStatusRestricted:
		return true
	case model.AppUserStatusBanned, model.AppUserStatusSuspended:
		return appUser.BannedUntil != nil && !appUser.BannedUntil.After(now)
	default:
		return false
	}
}

func (s *Service) userTenantGrantSets(userID, tenantID uint, now time.Time) (map[uint]struct{}, map[uint]struct{}, error) {
	roleIDs := map[uint]struct{}{}
	permissionIDs := map[uint]struct{}{}
	var roles []uint
	if err := s.db.Model(&model.TenantUserRbacRole{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Where("expires_at IS NULL OR expires_at > ?", now).Distinct("role_id").Pluck("role_id", &roles).Error; err != nil {
		return nil, nil, err
	}
	for _, id := range roles {
		roleIDs[id] = struct{}{}
	}
	var directPermissions []uint
	if err := s.db.Model(&model.TenantUserRbacPermission{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Where("expires_at IS NULL OR expires_at > ?", now).Distinct("permission_id").Pluck("permission_id", &directPermissions).Error; err != nil {
		return nil, nil, err
	}
	for _, id := range directPermissions {
		permissionIDs[id] = struct{}{}
	}
	if len(roles) > 0 {
		var rolePermissions []uint
		if err := s.db.Model(&model.TenantRbacRolePermission{}).Where("role_id IN ?", roles).Distinct("permission_id").Pluck("permission_id", &rolePermissions).Error; err != nil {
			return nil, nil, err
		}
		for _, id := range rolePermissions {
			permissionIDs[id] = struct{}{}
		}
	}
	return roleIDs, permissionIDs, nil
}

func sourceMatchesUser(mapping model.TenantAppGrantMapping, membershipRole model.TenantRole, tenantRoleIDs, tenantPermissionIDs map[uint]struct{}) bool {
	switch mapping.SourceType {
	case model.TenantAppGrantSourceMembershipRole:
		return string(membershipRole) == mapping.SourceCode
	case model.TenantAppGrantSourceTenantRole:
		_, ok := tenantRoleIDs[mapping.SourceID]
		return ok
	case model.TenantAppGrantSourceTenantPermission:
		_, ok := tenantPermissionIDs[mapping.SourceID]
		return ok
	default:
		return false
	}
}

func (s *Service) mappingSourceCode(mapping model.TenantAppGrantMapping) (string, error) {
	if mapping.SourceType == model.TenantAppGrantSourceMembershipRole {
		return mapping.SourceCode, nil
	}
	if mapping.SourceType == model.TenantAppGrantSourceTenantRole {
		var role model.TenantRbacRole
		if err := s.db.Select("code").Where("id = ? AND tenant_id = ?", mapping.SourceID, mapping.TenantID).First(&role).Error; err != nil {
			return "", err
		}
		return role.Code, nil
	}
	if mapping.SourceType == model.TenantAppGrantSourceTenantPermission {
		var permission model.TenantRbacPermission
		if err := s.db.Select("code").Where("id = ? AND tenant_id = ?", mapping.SourceID, mapping.TenantID).First(&permission).Error; err != nil {
			return "", err
		}
		return permission.Code, nil
	}
	return "", &ValidationError{Message: "invalid stored source_type"}
}

func ensureRole(values map[uint]*EffectiveRole, id uint, code, name, description string, appID uint) *EffectiveRole {
	if existing := values[id]; existing != nil {
		return existing
	}
	value := &EffectiveRole{ID: id, Code: code, Name: name, Description: description, AppID: appID, Permissions: []model.AppPermission{}, Sources: []GrantSource{}}
	values[id] = value
	return value
}

func ensurePermission(values map[uint]*EffectivePermission, id uint, code, name, description, category string, appID uint) *EffectivePermission {
	if existing := values[id]; existing != nil {
		return existing
	}
	value := &EffectivePermission{ID: id, Code: code, Name: name, Description: description, Category: category, AppID: appID, Sources: []GrantSource{}}
	values[id] = value
	return value
}

func addSource(values *[]GrantSource, value GrantSource) {
	key := sourceKey(value)
	for _, existing := range *values {
		if sourceKey(existing) == key {
			return
		}
	}
	*values = append(*values, value)
}

func sourceKey(value GrantSource) string {
	return fmt.Sprintf("%s:%d:%d:%s", value.Type, value.AssignmentID, value.MappingID, value.ViaRoleCode)
}

func sortSources(values []GrantSource) {
	sort.Slice(values, func(i, j int) bool { return sourceKey(values[i]) < sourceKey(values[j]) })
}
