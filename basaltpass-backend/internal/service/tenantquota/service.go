package tenantquota

import (
	"errors"
	"fmt"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
)

const (
	DefaultMaxApps          = 5
	DefaultMaxUsers         = 100
	DefaultMaxTeams         = 20
	DefaultMaxTokensPerHour = 1000
)

type Resource string

const (
	ResourceApps   Resource = "apps"
	ResourceUsers  Resource = "users"
	ResourceTeams  Resource = "teams"
	ResourceTokens Resource = "tokens"
)

type QuotaExceededError struct {
	Resource Resource
	Limit    int
}

func (e *QuotaExceededError) Error() string {
	switch e.Resource {
	case ResourceApps:
		return fmt.Sprintf("租户应用数量已达到上限（%d）", e.Limit)
	case ResourceUsers:
		return fmt.Sprintf("租户用户数量已达到上限（%d）", e.Limit)
	case ResourceTeams:
		return fmt.Sprintf("租户团队数量已达到上限（%d）", e.Limit)
	case ResourceTokens:
		return fmt.Sprintf("租户每小时令牌数量已达到上限（%d）", e.Limit)
	default:
		return fmt.Sprintf("租户配额已达到上限（%d）", e.Limit)
	}
}

func Get(db *gorm.DB, tenantID uint) (*model.TenantQuota, error) {
	if !db.Migrator().HasTable(&model.TenantQuota{}) {
		return defaults(tenantID), nil
	}

	quota := &model.TenantQuota{}
	if err := db.Where("tenant_id = ?", tenantID).First(quota).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaults(tenantID), nil
		}
		return nil, err
	}
	normalize(quota)
	return quota, nil
}

func EnsureAppsWithinLimit(db *gorm.DB, tenantID uint) error {
	quota, err := Get(db, tenantID)
	if err != nil {
		return err
	}
	if quota.MaxApps <= 0 {
		return nil
	}

	var count int64
	if err := db.Model(&model.App{}).
		Where("tenant_id = ? AND status != ?", tenantID, model.AppStatusDeleted).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(quota.MaxApps) {
		return &QuotaExceededError{Resource: ResourceApps, Limit: quota.MaxApps}
	}
	return nil
}

func EnsureUsersWithinLimit(db *gorm.DB, tenantID uint) error {
	quota, err := Get(db, tenantID)
	if err != nil {
		return err
	}
	if quota.MaxUsers <= 0 {
		return nil
	}

	count, err := CountTenantUsers(db, tenantID)
	if err != nil {
		return err
	}
	if count >= int64(quota.MaxUsers) {
		return &QuotaExceededError{Resource: ResourceUsers, Limit: quota.MaxUsers}
	}
	return nil
}

func EnsureUserCanJoin(db *gorm.DB, tenantID uint, userID uint) error {
	if userID == 0 {
		return EnsureUsersWithinLimit(db, tenantID)
	}

	var user model.User
	if err := db.Select("id", "deleted_at").First(&user, userID).Error; err != nil {
		return err
	}

	var count int64
	if err := db.Model(&model.TenantUser{}).
		Where("tenant_id = ? AND user_id = ? AND role != ?", tenantID, userID, model.TenantRoleBanned).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	return EnsureUsersWithinLimit(db, tenantID)
}

func EnsureTeamsWithinLimit(db *gorm.DB, tenantID uint) error {
	quota, err := Get(db, tenantID)
	if err != nil {
		return err
	}
	if quota.MaxTeams <= 0 {
		return nil
	}

	var count int64
	if err := db.Model(&model.Team{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(quota.MaxTeams) {
		return &QuotaExceededError{Resource: ResourceTeams, Limit: quota.MaxTeams}
	}
	return nil
}

func EnsureTokensWithinLimit(db *gorm.DB, tenantID uint, now time.Time) error {
	quota, err := Get(db, tenantID)
	if err != nil {
		return err
	}
	if quota.MaxTokensPerHour <= 0 {
		return nil
	}

	var count int64
	if err := db.Model(&model.OAuthAccessToken{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, now.Add(-time.Hour)).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(quota.MaxTokensPerHour) {
		return &QuotaExceededError{Resource: ResourceTokens, Limit: quota.MaxTokensPerHour}
	}
	return nil
}

func CountTenantUsers(db *gorm.DB, tenantID uint) (int64, error) {
	var membershipIDs []uint
	if err := db.Model(&model.TenantUser{}).
		Where("tenant_id = ? AND role != ?", tenantID, model.TenantRoleBanned).
		Pluck("user_id", &membershipIDs).Error; err != nil {
		return 0, err
	}

	seen := make(map[uint]struct{}, len(membershipIDs))
	for _, id := range membershipIDs {
		seen[id] = struct{}{}
	}
	return int64(len(seen)), nil
}

func defaults(tenantID uint) *model.TenantQuota {
	return &model.TenantQuota{
		TenantID:         tenantID,
		MaxApps:          DefaultMaxApps,
		MaxUsers:         DefaultMaxUsers,
		MaxTeams:         DefaultMaxTeams,
		MaxTokensPerHour: DefaultMaxTokensPerHour,
	}
}

func normalize(quota *model.TenantQuota) {
	if quota.MaxApps == 0 {
		quota.MaxApps = DefaultMaxApps
	}
	if quota.MaxUsers == 0 {
		quota.MaxUsers = DefaultMaxUsers
	}
	if quota.MaxTeams == 0 {
		quota.MaxTeams = DefaultMaxTeams
	}
	if quota.MaxTokensPerHour == 0 {
		quota.MaxTokensPerHour = DefaultMaxTokensPerHour
	}
}
