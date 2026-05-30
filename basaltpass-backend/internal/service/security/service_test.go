package security

import (
	"testing"
	"time"

	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupSecurityTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.SecurityOperation{},
		&model.AuthRefreshToken{},
		&model.OAuthRefreshToken{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}

func TestChangePasswordRevokesRefreshTokens(t *testing.T) {
	db := setupSecurityTestDB(t)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("OldPass123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	user := model.User{
		Email:        "security@example.com",
		PasswordHash: string(passwordHash),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	authRefresh := model.AuthRefreshToken{
		JTI:       "jti-1",
		FamilyID:  "family-1",
		TokenHash: "hash-1",
		UserID:    user.ID,
		Scope:     "user",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	oauthRefresh := model.OAuthRefreshToken{
		Token:     "oauth-refresh",
		ClientID:  "client-1",
		UserID:    user.ID,
		Scopes:    "openid offline_access",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&authRefresh).Error; err != nil {
		t.Fatalf("create auth refresh token failed: %v", err)
	}
	if err := db.Create(&oauthRefresh).Error; err != nil {
		t.Fatalf("create oauth refresh token failed: %v", err)
	}

	err = (&Service{db: db}).ChangePassword(user.ID, &PasswordChangeRequest{
		CurrentPassword: "OldPass123",
		NewPassword:     "NewPass123",
	}, "127.0.0.1", "device")
	if err != nil {
		t.Fatalf("change password failed: %v", err)
	}

	var updated model.AuthRefreshToken
	if err := db.First(&updated, authRefresh.ID).Error; err != nil {
		t.Fatalf("load auth refresh token failed: %v", err)
	}
	if updated.RevokedAt == nil {
		t.Fatal("expected auth refresh token to be revoked")
	}

	var remainingOAuth int64
	if err := db.Model(&model.OAuthRefreshToken{}).Where("user_id = ?", user.ID).Count(&remainingOAuth).Error; err != nil {
		t.Fatalf("count oauth refresh tokens failed: %v", err)
	}
	if remainingOAuth != 0 {
		t.Fatalf("expected oauth refresh tokens to be deleted, got %d", remainingOAuth)
	}
}

func TestSecurityEmailMethodsReturnErrorWhenEmailServiceMissing(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "email change verification",
			run: func() error {
				return svc.sendEmailChangeVerificationEmail("new@example.com", "token", "old@example.com")
			},
		},
		{
			name: "email change notification",
			run: func() error {
				return svc.sendEmailChangeNotificationEmail("old@example.com", "new@example.com", "token")
			},
		},
		{
			name: "email change success",
			run:  func() error { return svc.sendEmailChangeSuccessEmail("new@example.com", "old@example.com") },
		},
		{
			name: "password change notification",
			run:  func() error { return svc.sendPasswordChangeNotificationEmail("user@example.com") },
		},
		{
			name: "password reset",
			run:  func() error { return svc.sendPasswordResetEmail("user@example.com", "token") },
		},
		{
			name: "password reset success",
			run:  func() error { return svc.sendPasswordResetSuccessEmail("user@example.com") },
		},
		{
			name: "email verification",
			run:  func() error { return svc.SendEmailVerificationEmail("user@example.com", "123456") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("expected error when email service is missing")
			}
		})
	}
}
