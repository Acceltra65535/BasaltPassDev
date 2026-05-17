package main

import (
	"basaltpass-backend/internal/model"
	"basaltpass-backend/internal/utils"
	"log"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	db, err := gorm.Open(sqlite.Open("conformance-basaltpass.db"), &gorm.Config{})
	must(err)

	now := time.Now()
	tenant := model.Tenant{Name: "Conformance Tenant", Code: "conformance", Status: model.TenantStatusActive}
	must(db.Where("code = ?", tenant.Code).FirstOrCreate(&tenant).Error)

	passwordHash, err := utils.HashPassword("Passw0rd!123")
	must(err)
	isAdmin := false
	user := model.User{
		TenantID:       tenant.ID,
		Email:          "conformance@example.com",
		PasswordHash:   passwordHash,
		Nickname:       "conformance",
		GivenName:      "Conformance",
		FamilyName:     "User",
		MiddleName:     "OIDC",
		Locale:         "en-US",
		Zoneinfo:       "America/Los_Angeles",
		EmailVerified:  true,
		IsSystemAdmin:  &isAdmin,
		WebAuthnUserID: []byte("conformance-webauthn-user-id"),
	}
	must(db.Where("email = ? AND tenant_id = ?", user.Email, tenant.ID).Assign(user).FirstOrCreate(&user).Error)
	must(db.Where("user_id = ? AND tenant_id = ?", user.ID, tenant.ID).
		FirstOrCreate(&model.TenantUser{UserID: user.ID, TenantID: tenant.ID, Role: model.TenantRoleMember}).Error)

	app := model.App{TenantID: tenant.ID, Name: "OIDC Conformance App", Status: model.AppStatusActive, IsVerified: true}
	must(db.Where("tenant_id = ? AND name = ?", tenant.ID, app.Name).FirstOrCreate(&app).Error)

	redirects := []string{
		"https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback",
		"https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback?foo=bar",
		"https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/callback?dummy1=lorem&dummy2=ipsum",
		"https://host.docker.internal:8443/test/a/basaltpass-basic/callback",
		"https://host.docker.internal:8443/test/a/basaltpass-basic/callback?foo=bar",
	}
	postLogout := []string{
		"https://localhost.emobix.co.uk:8443/test/a/basaltpass-basic/post_logout_redirect",
		"https://host.docker.internal:8443/test/a/basaltpass-basic/post_logout_redirect",
	}

	upsertClient := func(clientID, secret, method string) {
		client := model.OAuthClient{
			AppID:                   app.ID,
			ClientID:                clientID,
			ClientSecret:            secret,
			TokenEndpointAuthMethod: method,
			SubjectType:             model.OAuthSubjectTypePublic,
			RedirectURIs:            strings.Join(redirects, ","),
			PostLogoutRedirectURIs:  strings.Join(postLogout, ","),
			Scopes:                  "openid,profile,email,offline_access,address,phone",
			GrantTypes:              "authorization_code,refresh_token",
			IsActive:                true,
			CreatedBy:               user.ID,
			AllowedOrigins:          "https://localhost.emobix.co.uk:8443,https://host.docker.internal:8443",
		}
		client.HashClientSecret()
		var existing model.OAuthClient
		err := db.Where("client_id = ?", clientID).First(&existing).Error
		if err == nil {
			client.ID = existing.ID
			client.CreatedAt = existing.CreatedAt
			client.UpdatedAt = now
			must(db.Save(&client).Error)
			return
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			must(err)
		}
		must(db.Create(&client).Error)
	}

	upsertClient("basaltpass-basic", "basic-secret", model.OAuthTokenEndpointAuthClientSecretBasic)
	upsertClient("basaltpass-basic-2", "basic-secret-2", model.OAuthTokenEndpointAuthClientSecretBasic)
	upsertClient("basaltpass-post", "post-secret", model.OAuthTokenEndpointAuthClientSecretPost)

	must(db.Where("app_id = ? AND user_id = ?", app.ID, user.ID).FirstOrCreate(&model.AppUser{
		AppID:             app.ID,
		UserID:            user.ID,
		FirstAuthorizedAt: now,
		LastAuthorizedAt:  now,
		Scopes:            "openid profile email offline_access address phone",
		Status:            model.AppUserStatusActive,
	}).Error)

	log.Printf("seeded user=%s password=%s clients=%s,%s,%s", user.Email, "Passw0rd!123", "basaltpass-basic", "basaltpass-basic-2", "basaltpass-post")
}
