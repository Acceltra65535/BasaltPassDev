package appgrant

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateMapping(tenantID, appID, actorID uint, input MappingInput) (MappingView, error) {
	var view MappingView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		service := NewService(tx)
		if err := service.lockApp(tenantID, appID); err != nil {
			return err
		}
		mapping, err := service.mappingFromInput(tenantID, appID, actorID, input)
		if err != nil {
			return err
		}
		if err := tx.Create(&mapping).Error; err != nil {
			return mappingWriteError(err)
		}
		view, err = service.mappingView(mapping)
		return err
	})
	if err != nil {
		return MappingView{}, err
	}
	return view, nil
}

func (s *Service) UpdateMapping(tenantID, appID, mappingID, actorID uint, input MappingInput) (MappingView, error) {
	var view MappingView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		service := NewService(tx)
		if err := service.lockApp(tenantID, appID); err != nil {
			return err
		}
		var existing model.TenantAppGrantMapping
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND app_id = ?", mappingID, tenantID, appID).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		desired, err := service.mappingFromInput(tenantID, appID, actorID, input)
		if err != nil {
			return err
		}
		desired.ID = existing.ID
		desired.CreatedBy = existing.CreatedBy
		desired.CreatedAt = existing.CreatedAt
		if err := tx.Save(&desired).Error; err != nil {
			return mappingWriteError(err)
		}
		view, err = service.mappingView(desired)
		return err
	})
	if err != nil {
		return MappingView{}, err
	}
	return view, nil
}

func (s *Service) DeleteMapping(tenantID, appID, mappingID uint) error {
	_, err := s.DeleteMappingWithView(tenantID, appID, mappingID)
	return err
}

// DeleteMappingWithView returns the locked pre-delete policy so callers can
// write a complete audit record even though the mapping no longer exists.
func (s *Service) DeleteMappingWithView(tenantID, appID, mappingID uint) (MappingView, error) {
	var view MappingView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := NewService(tx).lockApp(tenantID, appID); err != nil {
			return err
		}
		var mapping model.TenantAppGrantMapping
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND app_id = ?", mappingID, tenantID, appID).First(&mapping).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var err error
		view, err = NewService(tx).mappingView(mapping)
		if err != nil {
			return err
		}
		result := tx.Where("id = ? AND tenant_id = ? AND app_id = ?", mappingID, tenantID, appID).Delete(&model.TenantAppGrantMapping{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return MappingView{}, err
	}
	return view, nil
}

func (s *Service) ListMappings(tenantID, appID uint) ([]MappingView, error) {
	if err := s.ensureApp(tenantID, appID); err != nil {
		return nil, err
	}
	var mappings []model.TenantAppGrantMapping
	if err := s.db.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("id ASC").Find(&mappings).Error; err != nil {
		return nil, err
	}
	views := make([]MappingView, 0, len(mappings))
	for _, mapping := range mappings {
		view, err := s.mappingView(mapping)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) PreviewAffectedUsers(tenantID, appID uint, input MappingInput) (int64, error) {
	mapping, err := s.mappingFromInput(tenantID, appID, 1, input)
	if err != nil {
		return 0, err
	}
	return s.affectedUserCount(mapping)
}

func (s *Service) Options(tenantID, appID uint) (Options, error) {
	if err := s.ensureApp(tenantID, appID); err != nil {
		return Options{}, err
	}
	options := Options{
		MembershipRoles: []GrantEndpoint{
			{Type: string(model.TenantAppGrantSourceMembershipRole), Code: string(model.TenantRoleOwner), Name: "Owner"},
			{Type: string(model.TenantAppGrantSourceMembershipRole), Code: string(model.TenantRoleAdmin), Name: "Admin"},
			{Type: string(model.TenantAppGrantSourceMembershipRole), Code: string(model.TenantRoleMember), Name: "Member"},
			{Type: string(model.TenantAppGrantSourceMembershipRole), Code: string(model.TenantRoleUser), Name: "User"},
		},
		TenantRoles: []GrantEndpoint{}, TenantPermissions: []GrantEndpoint{}, AppRoles: []GrantEndpoint{}, AppPermissions: []GrantEndpoint{},
	}
	var tenantRoles []model.TenantRbacRole
	if err := s.db.Where("tenant_id = ?", tenantID).Order("code ASC").Find(&tenantRoles).Error; err != nil {
		return Options{}, err
	}
	for _, item := range tenantRoles {
		options.TenantRoles = append(options.TenantRoles, GrantEndpoint{Type: string(model.TenantAppGrantSourceTenantRole), ID: item.ID, Code: item.Code, Name: item.Name})
	}
	var tenantPermissions []model.TenantRbacPermission
	if err := s.db.Where("tenant_id = ?", tenantID).Order("code ASC").Find(&tenantPermissions).Error; err != nil {
		return Options{}, err
	}
	for _, item := range tenantPermissions {
		options.TenantPermissions = append(options.TenantPermissions, GrantEndpoint{Type: string(model.TenantAppGrantSourceTenantPermission), ID: item.ID, Code: item.Code, Name: item.Name})
	}
	var appRoles []model.AppRole
	if err := s.db.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("code ASC").Find(&appRoles).Error; err != nil {
		return Options{}, err
	}
	for _, item := range appRoles {
		options.AppRoles = append(options.AppRoles, GrantEndpoint{Type: string(model.TenantAppGrantTargetAppRole), ID: item.ID, Code: item.Code, Name: item.Name})
	}
	var appPermissions []model.AppPermission
	if err := s.db.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("code ASC").Find(&appPermissions).Error; err != nil {
		return Options{}, err
	}
	for _, item := range appPermissions {
		options.AppPermissions = append(options.AppPermissions, GrantEndpoint{Type: string(model.TenantAppGrantTargetAppPermission), ID: item.ID, Code: item.Code, Name: item.Name})
	}
	return options, nil
}

func (s *Service) mappingFromInput(tenantID, appID, actorID uint, input MappingInput) (model.TenantAppGrantMapping, error) {
	if tenantID == 0 || appID == 0 || actorID == 0 {
		return model.TenantAppGrantMapping{}, &ValidationError{Message: "invalid tenant, app, or actor context"}
	}
	if err := s.ensureApp(tenantID, appID); err != nil {
		return model.TenantAppGrantMapping{}, err
	}
	input.SourceCode = strings.TrimSpace(strings.ToLower(input.SourceCode))
	if err := s.validateSource(tenantID, input); err != nil {
		return model.TenantAppGrantMapping{}, err
	}
	if err := s.validateTarget(tenantID, appID, input); err != nil {
		return model.TenantAppGrantMapping{}, err
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && !input.ValidUntil.After(*input.ValidFrom) {
		return model.TenantAppGrantMapping{}, &ValidationError{Message: "valid_until must be after valid_from"}
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	return model.TenantAppGrantMapping{
		TenantID: tenantID, AppID: appID, SourceType: input.SourceType, SourceID: input.SourceID, SourceCode: input.SourceCode,
		TargetType: input.TargetType, TargetID: input.TargetID, Enabled: enabled, ValidFrom: utcTime(input.ValidFrom), ValidUntil: utcTime(input.ValidUntil),
		CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Service) validateSource(tenantID uint, input MappingInput) error {
	switch input.SourceType {
	case model.TenantAppGrantSourceMembershipRole:
		if input.SourceID != 0 {
			return &ValidationError{Message: "membership_role source_id must be zero"}
		}
		allowed := map[string]bool{"owner": true, "admin": true, "member": true, "user": true}
		if !allowed[input.SourceCode] {
			return &ValidationError{Message: "invalid membership role"}
		}
	case model.TenantAppGrantSourceTenantRole:
		if input.SourceID == 0 || input.SourceCode != "" {
			return &ValidationError{Message: "tenant_role requires source_id and no source_code"}
		}
		var source model.TenantRbacRole
		if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND tenant_id = ?", input.SourceID, tenantID).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ValidationError{Message: "tenant role does not belong to tenant"}
			}
			return err
		}
	case model.TenantAppGrantSourceTenantPermission:
		if input.SourceID == 0 || input.SourceCode != "" {
			return &ValidationError{Message: "tenant_permission requires source_id and no source_code"}
		}
		var source model.TenantRbacPermission
		if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND tenant_id = ?", input.SourceID, tenantID).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ValidationError{Message: "tenant permission does not belong to tenant"}
			}
			return err
		}
	default:
		return &ValidationError{Message: "invalid source_type"}
	}
	return nil
}

func (s *Service) validateTarget(tenantID, appID uint, input MappingInput) error {
	if input.TargetID == 0 {
		return &ValidationError{Message: "target_id is required"}
	}
	switch input.TargetType {
	case model.TenantAppGrantTargetAppRole:
		var target model.AppRole
		if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND tenant_id = ? AND app_id = ?", input.TargetID, tenantID, appID).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ValidationError{Message: "target does not belong to app and tenant"}
			}
			return err
		}
	case model.TenantAppGrantTargetAppPermission:
		var target model.AppPermission
		if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND tenant_id = ? AND app_id = ?", input.TargetID, tenantID, appID).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ValidationError{Message: "target does not belong to app and tenant"}
			}
			return err
		}
	default:
		return &ValidationError{Message: "invalid target_type"}
	}
	return nil
}

func (s *Service) lockApp(tenantID, appID uint) error {
	var app model.App
	if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
		Where("id = ? AND tenant_id = ?", appID, tenantID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func mappingWriteError(err error) error {
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate") {
		return ErrConflict
	}
	return err
}

func (s *Service) ensureApp(tenantID, appID uint) error {
	var count int64
	if err := s.db.Model(&model.App{}).Where("id = ? AND tenant_id = ?", appID, tenantID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) mappingView(mapping model.TenantAppGrantMapping) (MappingView, error) {
	source, err := s.describeSource(mapping)
	if err != nil {
		return MappingView{}, err
	}
	target, err := s.describeTarget(mapping)
	if err != nil {
		return MappingView{}, err
	}
	count, err := s.affectedUserCount(mapping)
	if err != nil {
		return MappingView{}, err
	}
	return MappingView{ID: mapping.ID, TenantID: mapping.TenantID, AppID: mapping.AppID, Source: source, Target: target,
		Enabled: mapping.Enabled, ValidFrom: mapping.ValidFrom, ValidUntil: mapping.ValidUntil, AffectedUserCount: count,
		CreatedBy: mapping.CreatedBy, UpdatedBy: mapping.UpdatedBy, CreatedAt: mapping.CreatedAt, UpdatedAt: mapping.UpdatedAt}, nil
}

func (s *Service) describeSource(mapping model.TenantAppGrantMapping) (GrantEndpoint, error) {
	switch mapping.SourceType {
	case model.TenantAppGrantSourceMembershipRole:
		names := map[string]string{"owner": "Owner", "admin": "Admin", "member": "Member", "user": "User"}
		return GrantEndpoint{Type: string(mapping.SourceType), Code: mapping.SourceCode, Name: names[mapping.SourceCode]}, nil
	case model.TenantAppGrantSourceTenantRole:
		var item model.TenantRbacRole
		if err := s.db.Where("id = ? AND tenant_id = ?", mapping.SourceID, mapping.TenantID).First(&item).Error; err != nil {
			return GrantEndpoint{}, fmt.Errorf("resolve tenant role source: %w", err)
		}
		return GrantEndpoint{Type: string(mapping.SourceType), ID: item.ID, Code: item.Code, Name: item.Name}, nil
	case model.TenantAppGrantSourceTenantPermission:
		var item model.TenantRbacPermission
		if err := s.db.Where("id = ? AND tenant_id = ?", mapping.SourceID, mapping.TenantID).First(&item).Error; err != nil {
			return GrantEndpoint{}, fmt.Errorf("resolve tenant permission source: %w", err)
		}
		return GrantEndpoint{Type: string(mapping.SourceType), ID: item.ID, Code: item.Code, Name: item.Name}, nil
	}
	return GrantEndpoint{}, &ValidationError{Message: "invalid stored source_type"}
}

func (s *Service) describeTarget(mapping model.TenantAppGrantMapping) (GrantEndpoint, error) {
	switch mapping.TargetType {
	case model.TenantAppGrantTargetAppRole:
		var item model.AppRole
		if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ?", mapping.TargetID, mapping.TenantID, mapping.AppID).First(&item).Error; err != nil {
			return GrantEndpoint{}, fmt.Errorf("resolve app role target: %w", err)
		}
		return GrantEndpoint{Type: string(mapping.TargetType), ID: item.ID, Code: item.Code, Name: item.Name}, nil
	case model.TenantAppGrantTargetAppPermission:
		var item model.AppPermission
		if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ?", mapping.TargetID, mapping.TenantID, mapping.AppID).First(&item).Error; err != nil {
			return GrantEndpoint{}, fmt.Errorf("resolve app permission target: %w", err)
		}
		return GrantEndpoint{Type: string(mapping.TargetType), ID: item.ID, Code: item.Code, Name: item.Name}, nil
	}
	return GrantEndpoint{}, &ValidationError{Message: "invalid stored target_type"}
}

func (s *Service) affectedUserCount(mapping model.TenantAppGrantMapping) (int64, error) {
	now := time.Now().UTC()
	if !mapping.Enabled || (mapping.ValidFrom != nil && mapping.ValidFrom.After(now)) || (mapping.ValidUntil != nil && !mapping.ValidUntil.After(now)) {
		return 0, nil
	}
	userIDs, err := s.eligibleAppUserIDs(mapping.TenantID, mapping.AppID, now)
	if err != nil || len(userIDs) == 0 {
		return 0, err
	}
	matching, err := s.usersMatchingSource(mapping.TenantID, userIDs, mapping.SourceType, mapping.SourceID, mapping.SourceCode, now)
	return int64(len(matching)), err
}

func (s *Service) eligibleAppUserIDs(tenantID, appID uint, now time.Time) ([]uint, error) {
	var ids []uint
	err := s.db.Table("app_users").Distinct("app_users.user_id").
		Joins("JOIN tenant_users ON tenant_users.user_id = app_users.user_id AND tenant_users.tenant_id = ?", tenantID).
		Joins("JOIN apps ON apps.id = app_users.app_id AND apps.tenant_id = ?", tenantID).
		Joins("JOIN tenants ON tenants.id = apps.tenant_id").
		Joins("JOIN system_auth_users ON system_auth_users.id = app_users.user_id").
		Where("app_users.app_id = ? AND tenant_users.role != ?", appID, model.TenantRoleBanned).
		Where("apps.status = ? AND tenants.status = ?", model.AppStatusActive, model.TenantStatusActive).
		Where("system_auth_users.banned = ? AND system_auth_users.deleted_at IS NULL", false).
		Where("app_users.status IN ? OR (app_users.status IN ? AND app_users.banned_until IS NOT NULL AND app_users.banned_until <= ?)",
			[]model.AppUserStatus{model.AppUserStatusActive, model.AppUserStatusRestricted},
			[]model.AppUserStatus{model.AppUserStatusBanned, model.AppUserStatusSuspended}, now).
		Pluck("app_users.user_id", &ids).Error
	return ids, err
}

func (s *Service) usersMatchingSource(tenantID uint, candidates []uint, sourceType model.TenantAppGrantSourceType, sourceID uint, sourceCode string, now time.Time) (map[uint]struct{}, error) {
	result := map[uint]struct{}{}
	var ids []uint
	switch sourceType {
	case model.TenantAppGrantSourceMembershipRole:
		if err := s.db.Model(&model.TenantUser{}).Where("tenant_id = ? AND user_id IN ? AND role = ?", tenantID, candidates, sourceCode).Pluck("user_id", &ids).Error; err != nil {
			return nil, err
		}
	case model.TenantAppGrantSourceTenantRole:
		if err := s.db.Model(&model.TenantUserRbacRole{}).
			Where("tenant_id = ? AND user_id IN ? AND role_id = ?", tenantID, candidates, sourceID).
			Where("expires_at IS NULL OR expires_at > ?", now).Distinct("user_id").Pluck("user_id", &ids).Error; err != nil {
			return nil, err
		}
	case model.TenantAppGrantSourceTenantPermission:
		var direct, throughRoles []uint
		if err := s.db.Model(&model.TenantUserRbacPermission{}).
			Where("tenant_id = ? AND user_id IN ? AND permission_id = ?", tenantID, candidates, sourceID).
			Where("expires_at IS NULL OR expires_at > ?", now).Distinct("user_id").Pluck("user_id", &direct).Error; err != nil {
			return nil, err
		}
		if err := s.db.Table("tenant_user_roles").Distinct("tenant_user_roles.user_id").
			Joins("JOIN tenant_role_permissions ON tenant_role_permissions.role_id = tenant_user_roles.role_id").
			Where("tenant_user_roles.tenant_id = ? AND tenant_user_roles.user_id IN ? AND tenant_role_permissions.permission_id = ?", tenantID, candidates, sourceID).
			Where("tenant_user_roles.expires_at IS NULL OR tenant_user_roles.expires_at > ?", now).
			Pluck("tenant_user_roles.user_id", &throughRoles).Error; err != nil {
			return nil, err
		}
		ids = append(direct, throughRoles...)
	default:
		return nil, &ValidationError{Message: "invalid source_type"}
	}
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result, nil
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.UTC()
	return &converted
}

func sortEndpoints(values []GrantEndpoint) {
	sort.Slice(values, func(i, j int) bool { return values[i].Code < values[j].Code })
}
