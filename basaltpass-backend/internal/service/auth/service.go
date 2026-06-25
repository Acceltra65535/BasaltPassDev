package auth

import (
	"basaltpass-backend/internal/handler/user/security"
	"basaltpass-backend/internal/service/aduit"
	securityservice "basaltpass-backend/internal/service/security"
	settingssvc "basaltpass-backend/internal/service/settings"
	tenantservice "basaltpass-backend/internal/service/tenant"
	"basaltpass-backend/internal/utils"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Service encapsulates auth related operations.
type Service struct{}

var (
	ErrMissingCredentials  = errors.New("identifier and password required")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrPlatformAdminOnly   = errors.New("only administrators can login to platform")
	ErrTenantAccountOnly   = errors.New("tenant account must login via tenant portal")
	ErrTenantLoginDisabled = errors.New("tenant login is disabled")
	ErrServiceUnavailable  = errors.New("authentication service temporarily unavailable")
)

const loginQueryTimeout = 8 * time.Second

func normalizeLoginQueryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrInvalidCredentials
	}
	return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
}

// Register creates a new user with hashed password.
func (s Service) Register(req RegisterRequest) (*model.User, error) {
	if !settingssvc.GetBool("auth.enable_register", true) {
		return nil, errors.New("registration is disabled")
	}
	if req.Email == "" && req.Phone == "" {
		return nil, errors.New("email or phone required")
	}
	if req.Password == "" {
		return nil, errors.New("password required")
	}

	// 验证和标准化手机号为E.164格式
	var normalizedPhone string
	if req.Phone != "" {
		phoneValidator := utils.NewPhoneValidator("+86") // 使用中国为默认国家
		normalized, err := phoneValidator.NormalizeToE164(req.Phone)
		if err != nil {
			return nil, errors.New("手机号格式不正确: " + err.Error())
		}
		normalizedPhone = normalized
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 检查是否是第一个用户
	var userCount int64
	if err := common.DB().Model(&model.User{}).Count(&userCount).Error; err != nil {
		return nil, err
	}
	isFirstUser := userCount == 0

	// 账号唯一性按 enforced_tenant_id 分区：
	// global 账号只要求 enforced_tenant_id=0 内唯一；
	// tenant-local 账号只要求同一 tenant 内唯一，允许同 email 在多个 tenant 下隔离注册。
	db := common.DB()
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	if req.TenantID == 0 {
		var conflict int64
		if err := db.Model(&model.User{}).
			Where("enforced_tenant_id = 0").
			Where("(email != '' AND email = ?) OR (phone != '' AND phone = ?)", normalizedEmail, normalizedPhone).
			Count(&conflict).Error; err != nil {
			return nil, err
		}
		if conflict > 0 {
			return nil, errors.New("user already exists")
		}
	} else {
		if normalizedEmail != "" {
			var sameTenant int64
			if err := db.Model(&model.User{}).Where("email = ? AND enforced_tenant_id = ?", normalizedEmail, req.TenantID).Count(&sameTenant).Error; err != nil {
				return nil, err
			}
			if sameTenant > 0 {
				return nil, errors.New("user already exists in this tenant")
			}
		}
		if normalizedPhone != "" {
			var sameTenantPhone int64
			if err := db.Model(&model.User{}).Where("phone = ? AND enforced_tenant_id = ?", normalizedPhone, req.TenantID).Count(&sameTenantPhone).Error; err != nil {
				return nil, err
			}
			if sameTenantPhone > 0 {
				return nil, errors.New("phone already exists in this tenant")
			}
		}
	}

	// 开始事务
	tx := common.DB().Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	user := &model.User{
		Email:            normalizedEmail,
		Phone:            normalizedPhone,
		PasswordHash:     string(hash),
		Nickname:         "New User",
		EnforcedTenantID: req.TenantID, // tenant-local 登录约束，不代表成员事实来源
	}

	// 仅允许“平台首用户”（enforced_tenant_id=0）自动成为系统管理员。
	// tenant-local 注册创建的普通用户（enforced_tenant_id>0）必须保持非系统管理员。
	if isFirstUser && req.TenantID == 0 {
		t := true
		user.IsSystemAdmin = &t
		user.EnforcedTenantID = 0 // 第一个用户是平台级管理员，不强制绑定租户入口
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if req.TenantID > 0 {
		membership := model.TenantUser{
			UserID:   user.ID,
			TenantID: req.TenantID,
			Role:     model.TenantRoleMember,
		}
		if err := tx.Where("user_id = ? AND tenant_id = ?", user.ID, req.TenantID).
			FirstOrCreate(&membership).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// 仅在平台首用户场景下设置全局管理员能力
	if isFirstUser && req.TenantID == 0 {
		if err := s.setupFirstUserAsGlobalAdmin(tx, user); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return user, nil
}

// LoginResult 用于登录结果的多态返回
// 如果需要二次验证，Need2FA=true，TokenPair为空，PreAuthToken非空（发给客户端）
// 如果不需要二次验证，Need2FA=false，TokenPair有效
// Available2FAMethods: 用户可用的所有2FA方式列表
type LoginResult struct {
	Need2FA             bool     `json:"need_2fa"`
	TwoFAType           string   `json:"2fa_type,omitempty"`              // 默认推荐的2FA方式
	Available2FAMethods []string `json:"available_2fa_methods,omitempty"` // 所有可用的2FA方式
	// PreAuthToken 仅在 Need2FA=true 时非空，客户端持有并在 Verify2FA 中回传。
	// 服务端从该 token 中提取 user_id，客户端不再直接提交 user_id。
	PreAuthToken string `json:"pre_auth_token,omitempty"`
	// UserID 仅供服务端内部使用（审计日志），不通过 JSON 暴露给客户端。
	UserID uint `json:"-"`
	TokenPair
}

// Login 校验用户名密码，判断是否需要二次验证
func (s Service) LoginV2(req LoginRequest) (LoginResult, error) {
	if req.EmailOrPhone == "" || req.Password == "" {
		return LoginResult{}, ErrMissingCredentials
	}
	if req.Scope == "" {
		req.Scope = ConsoleScopeUser
	}
	identifier := strings.TrimSpace(req.EmailOrPhone)

	if req.TenantID > 0 {
		allowed, err := tenantservice.IsTenantLoginAllowed(req.TenantID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return LoginResult{}, ErrInvalidCredentials
			}
			return LoginResult{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
		}
		if !allowed {
			return LoginResult{}, ErrTenantLoginDisabled
		}
	}

	var user model.User
	ctx, cancel := context.WithTimeout(context.Background(), loginQueryTimeout)
	defer cancel()

	db := common.DB().WithContext(ctx)

	// 构建查询条件：email/phone + tenant_id
	query := db.Preload("Passkeys").Where("email = ? OR phone = ?", identifier, identifier)

	if req.TenantID == 0 {
		// 全局登录：仅允许未强制绑定租户入口的账户。
		query = query.Where("enforced_tenant_id = 0")
		if err := query.First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var legacyUser model.User
				if legacyErr := db.Where("email = ? OR phone = ?", identifier, identifier).
					Order("id ASC").
					First(&legacyUser).Error; legacyErr == nil {
					if legacyUser.EnforcedTenantID > 0 {
						return LoginResult{}, ErrTenantAccountOnly
					}
				}
			}
			return LoginResult{}, normalizeLoginQueryError(err)
		}
	} else {
		// 租户登录：优先查询指定租户下的 tenant-local 账户
		query = query.Where("enforced_tenant_id = ?", req.TenantID)
		if err := query.First(&user).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return LoginResult{}, normalizeLoginQueryError(err)
			}

			// 允许全局账号（enforced_tenant_id=0）通过 tenant_users 成员关系登录租户入口。
			var globalUser model.User
			if gErr := db.Where("(email = ? OR phone = ?) AND enforced_tenant_id = 0", identifier, identifier).
				First(&globalUser).Error; gErr != nil {
				return LoginResult{}, normalizeLoginQueryError(err)
			}

			var membershipCount int64
			if mErr := db.Model(&model.TenantUser{}).
				Where("user_id = ? AND tenant_id = ?", globalUser.ID, req.TenantID).
				Count(&membershipCount).Error; mErr != nil {
				return LoginResult{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, mErr)
			}
			if membershipCount == 0 {
				return LoginResult{}, ErrInvalidCredentials
			}

			if pErr := db.Preload("Passkeys").First(&user, globalUser.ID).Error; pErr != nil {
				return LoginResult{}, normalizeLoginQueryError(pErr)
			}
		}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	// 读取管理员配置的 2FA 方式开关
	totpMethodEnabled := settingssvc.GetBool("auth.2fa.totp_enabled", true)
	passkeyMethodEnabled := settingssvc.GetBool("auth.2fa.passkey_enabled", true)
	smsMethodEnabled := settingssvc.GetBool("auth.2fa.sms_enabled", false)

	// 收集用户可用的所有2FA方式
	var availableMethods []string
	var defaultMethod string

	// 查询该用户在当前租户下是否启用了 TOTP（同时需要系统开关打开）
	var totpCfg model.UserTenantTOTP
	totpEnabled := false
	if totpMethodEnabled {
		if err := db.Where("user_id = ? AND tenant_id = ? AND enabled = ?", user.ID, req.TenantID, true).
			First(&totpCfg).Error; err == nil {
			totpEnabled = true
		}
	}

	// 是否具备强二次验证方式（TOTP 或 Passkey，受开关限制）
	hasStrong2FA := totpEnabled || (passkeyMethodEnabled && len(user.Passkeys) > 0)

	if hasStrong2FA {
		// 已开启强二次验证时，仅要求这些方式，不再因为未验证邮箱/手机号而拦截登录
		if totpEnabled {
			availableMethods = append(availableMethods, "totp")
			defaultMethod = "totp"
		}
		if passkeyMethodEnabled && len(user.Passkeys) > 0 {
			availableMethods = append(availableMethods, "passkey")
			if defaultMethod == "" {
				defaultMethod = "passkey"
			}
		}
	} else if settingssvc.GetBool("auth.require_email_verification", false) {
		// 仅在显式要求邮箱验证时，才将 email 作为登录前置校验。
		// 默认关闭，避免未实现完整验证码流程时导致登录被卡住。
		if !user.EmailVerified {
			availableMethods = append(availableMethods, "email")
			if defaultMethod == "" {
				defaultMethod = "email"
			}
		}
	}

	if user.Email != "" && len(availableMethods) > 0 && !containsMethod(availableMethods, "email") {
		availableMethods = append(availableMethods, "email")
		if defaultMethod == "" {
			defaultMethod = "email"
		}
	}

	// SMS 验证码作为额外的 2FA 方式（仅在开关打开且用户有手机号时追加）
	if smsMethodEnabled && user.Phone != "" {
		availableMethods = append(availableMethods, "sms")
		if defaultMethod == "" {
			defaultMethod = "sms"
		}
	}

	// 如果有2FA方式，颁发 pre_auth_token（绑定已验证的用户身份）并要求二次验证。
	// pre_auth_token 5 分钟内有效，客户端回传到 /auth/verify-2fa，
	// 服务端从 token 中读取 user_id，不再信任客户端提交的 user_id。
	if len(availableMethods) > 0 {
		preAuthToken, err := GeneratePreAuthToken(user.ID, req.TenantID)
		if err != nil {
			return LoginResult{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
		}
		return LoginResult{
			Need2FA:             true,
			TwoFAType:           defaultMethod,
			Available2FAMethods: availableMethods,
			PreAuthToken:        preAuthToken,
			UserID:              user.ID, // internal only
		}, nil
	}

	// 不需要二次验证，直接登录
	tokenScope := ConsoleScopeUser
	tokenTenantID := req.TenantID
	if req.Scope == ConsoleScopeAdmin {
		tokenScope = ConsoleScopeAdmin
		tokenTenantID = 0
	}
	tokens, err := GenerateTokenPairWithTenantAndScope(user.ID, tokenTenantID, tokenScope)
	if err != nil {
		return LoginResult{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	return LoginResult{Need2FA: false, TokenPair: tokens, UserID: user.ID}, nil
}

func containsMethod(methods []string, method string) bool {
	for _, item := range methods {
		if item == method {
			return true
		}
	}
	return false
}

// Refresh validates a refresh token and returns a new token pair.
func (s Service) Refresh(refreshToken string) (TokenPair, error) {
	token, err := ParseToken(refreshToken)
	if err != nil || !token.Valid {
		return TokenPair{}, errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != TokenTypeRefresh {
		return TokenPair{}, errors.New("invalid token type")
	}
	jti, ok := claims["jti"].(string)
	if !ok || strings.TrimSpace(jti) == "" {
		return TokenPair{}, errors.New("invalid refresh token state")
	}
	familyID, ok := claims["fam"].(string)
	if !ok || strings.TrimSpace(familyID) == "" {
		return TokenPair{}, errors.New("invalid refresh token state")
	}
	if err := consumeRefreshToken(jti, familyID, refreshToken); err != nil {
		return TokenPair{}, err
	}
	userIDFloat, ok := claims["sub"].(float64)
	if !ok {
		return TokenPair{}, errors.New("invalid subject")
	}
	var claimedTenantID uint
	if tidFloat, ok := claims["tid"].(float64); ok && tidFloat > 0 {
		claimedTenantID = uint(tidFloat)
	}
	// Preserve console scope, but do not trust tenant context from token.
	scope := ConsoleScopeUser
	if scp, ok := claims["scp"].(string); ok && scp != "" {
		scope = scp
	}
	tenantID, err := resolveTokenTenantID(uint(userIDFloat), claimedTenantID, scope)
	if err != nil {
		return TokenPair{}, err
	}
	return generateTokenPairWithTenantScopeAuthMethodsAndFamily(uint(userIDFloat), tenantID, scope, []string{"pwd"}, familyID)
}

// Verify2FA 校验二次验证信息，成功返回token。
// 用户身份从 req.PreAuthToken（由 LoginV2 颁发）中提取，不信任客户端提交的 user_id。
func (s Service) Verify2FA(req Verify2FARequest) (TokenPair, error) {
	// 从 pre_auth_token 中提取已验证的用户身份，防止客户端替换 user_id
	userID, tenantID, err := ParsePreAuthToken(req.PreAuthToken)
	if err != nil {
		return TokenPair{}, errors.New("invalid or expired 2FA session token")
	}

	db := common.DB()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		return TokenPair{}, errors.New("user not found")
	}
	switch req.TwoFAType {
	case "totp":
		if !settingssvc.GetBool("auth.2fa.totp_enabled", true) {
			return TokenPair{}, errors.New("TOTP 2FA is disabled by administrator")
		}
		// 从租户级 TOTP 表中加载该用户在指定租户下的 TOTP 配置
		var totpCfg model.UserTenantTOTP
		if err := db.Where("user_id = ? AND tenant_id = ? AND enabled = ?", userID, tenantID, true).
			First(&totpCfg).Error; err != nil {
			return TokenPair{}, errors.New("TOTP 2FA not enabled for this tenant")
		}
		rawSecret, err := utils.DecryptTOTPSecret(totpCfg.Secret)
		if err != nil || rawSecret == "" {
			return TokenPair{}, errors.New("TOTP configuration error")
		}
		if !security.ValidateTOTP(rawSecret, req.Code) {
			return TokenPair{}, errors.New("invalid TOTP code")
		}
	case "passkey":
		if !settingssvc.GetBool("auth.2fa.passkey_enabled", true) {
			return TokenPair{}, errors.New("Passkey 2FA is disabled by administrator")
		}
		// Passkey 2FA 必须通过专用的 WebAuthn 端点完成完整的挑战-响应验证，
		// 不能在此处以"用户有 passkey"作为充分条件。
		// 正确流程：
		//   1. POST /api/v1/passkey/2fa/begin  → 获取 WebAuthn challenge
		//   2. 浏览器/设备完成 WebAuthn 签名
		//   3. POST /api/v1/passkey/2fa/finish → 验证签名并颁发 token
		return TokenPair{}, errors.New("passkey 2FA requires the dedicated WebAuthn flow: " +
			"call POST /api/v1/passkey/2fa/begin to get a challenge, " +
			"then POST /api/v1/passkey/2fa/finish with the signed credential")
	case "sms":
		if !settingssvc.GetBool("auth.2fa.sms_enabled", false) {
			return TokenPair{}, errors.New("SMS 2FA is disabled by administrator")
		}
		// SMS 验证码校验逻辑待实现
		return TokenPair{}, errors.New("SMS 2FA verification is not yet implemented")
	case "email":
		if err := verifyEmail2FACode(db, user, req.Code); err != nil {
			return TokenPair{}, err
		}
	default:
		return TokenPair{}, errors.New("unsupported 2FA type")
	}
	return GenerateTokenPairWithTenantScopeAndAuthMethods(user.ID, tenantID, ConsoleScopeUser, []string{"pwd", "otp"})
}

func (s Service) SendEmail2FACode(preAuthToken, requestedIP string) error {
	userID, _, err := ParsePreAuthToken(preAuthToken)
	if err != nil {
		return errors.New("invalid or expired 2FA session token")
	}

	db := common.DB()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		return errors.New("user not found")
	}
	if strings.TrimSpace(user.Email) == "" {
		return errors.New("account has no email")
	}

	now := time.Now()
	cooldownCutoff := now.Add(-time.Duration(model.EmailVerificationResendCooldown) * time.Second)
	var recent model.EmailVerificationToken
	if err := db.Where("user_id = ? AND email = ? AND used_at IS NULL AND expires_at > ? AND created_at > ?",
		user.ID, user.Email, now, cooldownCutoff).
		Order("created_at DESC").
		First(&recent).Error; err == nil {
		return errors.New("email verification code was sent recently")
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	code, err := generateEmail2FACode()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	if err := db.Model(&model.EmailVerificationToken{}).
		Where("user_id = ? AND email = ? AND used_at IS NULL", user.ID, user.Email).
		Update("expires_at", now).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	token := model.EmailVerificationToken{
		UserID:      user.ID,
		Email:       user.Email,
		CodeHash:    hashEmail2FACode(code),
		ExpiresAt:   now.Add(time.Duration(model.EmailVerificationTTL) * time.Minute),
		RequestedIP: requestedIP,
	}
	if err := db.Create(&token).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	secSvc := securityservice.NewService(db)
	if err := secSvc.SendEmailVerificationEmail(user.Email, code); err != nil {
		_ = db.Delete(&token).Error
		return fmt.Errorf("failed to send email verification code: %w", err)
	}
	return nil
}

func verifyEmail2FACode(db *gorm.DB, user model.User, code string) error {
	code = strings.TrimSpace(code)
	if user.Email == "" {
		return errors.New("account has no email")
	}
	if code == "" {
		return errors.New("email verification code required")
	}

	var token model.EmailVerificationToken
	if err := db.Where("user_id = ? AND email = ? AND used_at IS NULL AND expires_at > ?",
		user.ID, user.Email, time.Now()).
		Order("created_at DESC").
		First(&token).Error; err != nil {
		return errors.New("verification code not found or expired")
	}
	if token.AttemptCount >= model.EmailVerificationMaxAttempts {
		return errors.New("verification code attempts exceeded")
	}

	if err := db.Model(&token).UpdateColumn("attempt_count", token.AttemptCount+1).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	if hashEmail2FACode(code) != token.CodeHash {
		return errors.New("invalid email verification code")
	}

	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&token).Updates(map[string]interface{}{"used_at": &now}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"email_verified": true, "email_verified_at": &now}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
}

func generateEmail2FACode() (string, error) {
	const digits = "0123456789"
	code := make([]byte, 6)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[n.Int64()]
	}
	return string(code), nil
}

func hashEmail2FACode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}

// setupFirstUserAsGlobalAdmin 设置第一个用户为全局管理员。
// 根据当前规则，全局管理员没有默认 tenant，tenant_id 必须保持 0。
func (s Service) setupFirstUserAsGlobalAdmin(tx *gorm.DB, user *model.User) error {
	// 仅更新平台管理员自身资料，不创建默认租户，也不绑定 tenant_users 关系。
	if err := tx.Model(user).Update("nickname", "系统管理员").Error; err != nil {
		return err
	}

	// 将邮箱验证状态设置为已验证
	if err := tx.Model(user).Update("email_verified", true).Error; err != nil {
		return err
	}

	aduit.LogAudit(user.ID, "首位用户注册", "user", fmt.Sprint(user.ID), "", "自动设置为全局管理员")

	return nil
}
