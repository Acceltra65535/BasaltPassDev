package main

import (
	"basaltpass-backend/internal/common"
	config "basaltpass-backend/internal/config"
	migration "basaltpass-backend/internal/migration"
	"basaltpass-backend/internal/model"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type serviceDef struct {
	ID          string
	Name        string
	Description string
	BaseURL     string
	Scopes      []string
}

type seededClient struct {
	AppID        uint     `json:"app_id"`
	Name         string   `json:"name"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes"`
}

type report struct {
	TenantID       uint           `json:"tenant_id"`
	TenantCode     string         `json:"tenant_code"`
	AdminUserID    uint           `json:"admin_user_id"`
	Clients        []seededClient `json:"clients"`
	CrossAppTrusts []string       `json:"cross_app_trusts"`
	EnvPath        string         `json:"env_path"`
	SeededAt       time.Time      `json:"seeded_at"`
}

func main() {
	var (
		configPath = flag.String("config", "", "optional BasaltPass config path")
		envPath    = flag.String("env-out", "", "optional env output path")
	)
	flag.Parse()

	if _, err := config.Load(*configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := migration.RunMigrations(); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	outPath := strings.TrimSpace(*envPath)
	if outPath == "" {
		outPath = filepath.Clean(filepath.Join("..", "..", "integration", "secrets", "basaltpass.infopipe.env"))
	}

	rep, err := seed(common.DB(), outPath)
	if err != nil {
		log.Fatalf("seed infopipe apps: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		log.Fatalf("encode report: %v", err)
	}
}

func seed(db *gorm.DB, envPath string) (*report, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not available")
	}
	now := time.Now().UTC()
	tenant := model.Tenant{
		Name:        "Info Pipeline",
		Code:        "infopipe",
		Description: "Local integration tenant for the information pipeline.",
		Status:      model.TenantStatusActive,
		Metadata:    model.JSONMap{"managed_by": "infopipe-seed"},
	}
	if err := db.Where("code = ?", tenant.Code).Assign(tenant).FirstOrCreate(&tenant).Error; err != nil {
		return nil, fmt.Errorf("upsert tenant: %w", err)
	}

	admin := model.User{
		Email:         "infopipe-admin@basalt.local",
		PasswordHash:  mustHash("InfoPipe@12345"),
		Nickname:      "infopipe-admin",
		EmailVerified: true,
	}
	isAdmin := true
	admin.IsSystemAdmin = &isAdmin
	if err := db.Where("email = ? AND enforced_tenant_id = ?", admin.Email, 0).Assign(admin).FirstOrCreate(&admin).Error; err != nil {
		return nil, fmt.Errorf("upsert admin user: %w", err)
	}
	if err := db.Where("tenant_id = ? AND user_id = ?", tenant.ID, admin.ID).
		Assign(model.TenantUser{Role: model.TenantRoleOwner}).
		FirstOrCreate(&model.TenantUser{TenantID: tenant.ID, UserID: admin.ID, Role: model.TenantRoleOwner}).Error; err != nil {
		return nil, fmt.Errorf("upsert tenant owner: %w", err)
	}

	services := []serviceDef{
		{"orion", "Orion", "Mission control and review service.", "http://orion:8120", []string{"s2s.read", "s2s.token_exchange", "apicred.llm", "llm.plan_mission", "llm.generate_product_spec", "llm.review_artifact", "llm.summarize_evidence"}},
		{"vesper", "Vesper", "Analysis and brief generation service.", "http://vesper:8121", []string{"s2s.read", "s2s.token_exchange", "apicred.llm", "llm.extract_entities", "llm.extract_claims", "llm.summarize_event", "llm.generate_brief"}},
		{"cis", "CIS", "Source planning service.", "http://cis:8195", []string{"s2s.read", "s2s.token_exchange", "apicred.llm", "cis.source_plan", "araneae.read", "araneae.write", "hashslip.read", "hashslip.write"}},
		{"docode", "DoCode", "Crawler and code generation service.", "http://docode:8110", []string{"s2s.read", "s2s.token_exchange", "docode.jobs", "docode.write", "llm", "apicred.read"}},
		{"hashslip", "HashSlip", "Fact and artifact store.", "http://hashslip:8106", []string{"s2s.read", "hashslip.read", "hashslip.write"}},
		{"araneae", "Araneae", "Crawler execution service.", "http://araneae-control:8180", []string{"s2s.read", "araneae.read", "araneae.write", "hashslip.write"}},
		{"apicred", "APICred", "LLM capability and provider gateway.", "http://apicred:8103", []string{"s2s.read", "llm", "apicred.read", "llm.gateway", "llm.grants"}},
		{"atlas", "Atlas", "Graph projection service.", "http://atlas:8130", []string{"s2s.read", "hashslip.read", "atlas.read"}},
		{"forge", "Forge", "Replay and evaluation service.", "http://forge:8150", []string{"s2s.read", "hashslip.read", "forge.eval"}},
		{"dispatch", "Dispatch", "Artifact delivery service.", "http://dispatch:8140", []string{"s2s.read", "hashslip.read", "atlas.read", "dispatch.read"}},
	}

	appIDs := map[string]uint{}
	clients := []seededClient{}
	for _, svc := range services {
		app := model.App{
			TenantID:    tenant.ID,
			Name:        svc.Name,
			Description: svc.Description,
			HomepageURL: svc.BaseURL,
			Status:      model.AppStatusActive,
			IsVerified:  true,
		}
		if err := db.Where("tenant_id = ? AND name = ?", tenant.ID, svc.Name).Assign(app).FirstOrCreate(&app).Error; err != nil {
			return nil, fmt.Errorf("upsert app %s: %w", svc.ID, err)
		}
		appIDs[svc.ID] = app.ID
		appUser := model.AppUser{
			AppID:             app.ID,
			UserID:            admin.ID,
			FirstAuthorizedAt: now,
			LastAuthorizedAt:  now,
			Scopes:            strings.Join(svc.Scopes, " "),
			Status:            model.AppUserStatusActive,
		}
		if err := db.Where("app_id = ? AND user_id = ?", app.ID, admin.ID).
			Assign(appUser).
			FirstOrCreate(&model.AppUser{AppID: app.ID, UserID: admin.ID, FirstAuthorizedAt: now, LastAuthorizedAt: now, Scopes: appUser.Scopes, Status: model.AppUserStatusActive}).Error; err != nil {
			return nil, fmt.Errorf("authorize admin for app %s: %w", svc.ID, err)
		}

		secret := "bp-infopipe-" + svc.ID + "-secret"
		client := model.OAuthClient{
			AppID:                   app.ID,
			ClientID:                "infopipe-" + svc.ID,
			ClientSecret:            secret,
			TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthClientSecretPost,
			SubjectType:             model.OAuthSubjectTypePublic,
			RedirectURIs:            "http://localhost/callback",
			Scopes:                  strings.Join(svc.Scopes, ","),
			GrantTypes:              "client_credentials,authorization_code,refresh_token,urn:ietf:params:oauth:grant-type:token-exchange",
			IsActive:                true,
			AllowedOrigins:          "http://localhost",
			CreatedBy:               admin.ID,
		}
		client.HashClientSecret()
		var existing model.OAuthClient
		if err := db.Where("client_id = ?", client.ClientID).First(&existing).Error; err == nil {
			client.ID = existing.ID
			client.Model = existing.Model
			if err := db.Model(&existing).Updates(map[string]any{
				"app_id":                     client.AppID,
				"client_secret":              client.ClientSecret,
				"token_endpoint_auth_method": client.TokenEndpointAuthMethod,
				"subject_type":               client.SubjectType,
				"redirect_uris":              client.RedirectURIs,
				"scopes":                     client.Scopes,
				"grant_types":                client.GrantTypes,
				"is_active":                  true,
				"allowed_origins":            client.AllowedOrigins,
				"created_by":                 admin.ID,
			}).Error; err != nil {
				return nil, fmt.Errorf("update client %s: %w", client.ClientID, err)
			}
		} else if err == gorm.ErrRecordNotFound {
			if err := db.Create(&client).Error; err != nil {
				return nil, fmt.Errorf("create client %s: %w", client.ClientID, err)
			}
		} else {
			return nil, fmt.Errorf("lookup client %s: %w", client.ClientID, err)
		}
		clients = append(clients, seededClient{
			AppID:        app.ID,
			Name:         svc.Name,
			ClientID:     "infopipe-" + svc.ID,
			ClientSecret: secret,
			Scopes:       svc.Scopes,
		})
	}

	trustPairs := [][2]string{
		{"orion", "cis"}, {"orion", "vesper"}, {"orion", "hashslip"}, {"orion", "apicred"}, {"orion", "dispatch"}, {"orion", "atlas"},
		{"vesper", "hashslip"}, {"vesper", "apicred"}, {"vesper", "atlas"},
		{"cis", "araneae"}, {"cis", "hashslip"}, {"cis", "apicred"}, {"cis", "docode"},
		{"docode", "apicred"},
		{"araneae", "hashslip"},
		{"forge", "orion"}, {"forge", "vesper"}, {"forge", "hashslip"}, {"forge", "atlas"},
		{"dispatch", "hashslip"}, {"dispatch", "atlas"},
	}
	trustNames := []string{}
	for _, pair := range trustPairs {
		sourceID, targetID := appIDs[pair[0]], appIDs[pair[1]]
		trust := model.CrossAppTrust{
			TenantID:      tenant.ID,
			SourceAppID:   sourceID,
			TargetAppID:   targetID,
			AllowedScopes: "s2s.read,s2s.token_exchange,apicred.llm,llm,apicred.read,docode.jobs,docode.write,hashslip.read,hashslip.write,araneae.read,araneae.write,llm.gateway,llm.grants,atlas.read,forge.eval,dispatch.read",
			MaxTokenTTL:   3600,
			Description:   "Info pipeline local integration trust: " + pair[0] + " -> " + pair[1],
			IsActive:      true,
			CreatedBy:     admin.ID,
		}
		if err := db.Where("tenant_id = ? AND source_app_id = ? AND target_app_id = ?", tenant.ID, sourceID, targetID).
			Assign(trust).FirstOrCreate(&trust).Error; err != nil {
			return nil, fmt.Errorf("upsert trust %s->%s: %w", pair[0], pair[1], err)
		}
		trustNames = append(trustNames, pair[0]+"->"+pair[1])
	}
	sort.Strings(trustNames)

	rep := &report{
		TenantID:       tenant.ID,
		TenantCode:     tenant.Code,
		AdminUserID:    admin.ID,
		Clients:        clients,
		CrossAppTrusts: trustNames,
		EnvPath:        envPath,
		SeededAt:       now,
	}
	if err := writeEnv(envPath, clients); err != nil {
		return nil, err
	}
	return rep, nil
}

func writeEnv(path string, clients []seededClient) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	values := map[string]string{
		"AUTH_PROVIDER_MODE":            "real",
		"BASALTPASS_PROVIDER_MODE":      "real",
		"BASALTPASS_BASE_URL":           "http://127.0.0.1:8101",
		"BASALTPASS_DOCKER_URL":         "http://host.docker.internal:8101",
		"USE_MOCK_BASALTPASS":           "false",
		"APICRED_REQUIRE_REAL_PROVIDER": "false",
	}
	for _, client := range clients {
		key := strings.ToUpper(strings.ReplaceAll(client.Name, " ", "_"))
		values[key+"_CLIENT_ID"] = client.ClientID
		values[key+"_CLIENT_SECRET"] = client.ClientSecret
	}
	order := make([]string, 0, len(values))
	for key := range values {
		order = append(order, key)
	}
	sort.Strings(order)
	var b strings.Builder
	b.WriteString("# Generated by BasaltPass infopipe-seed. Local integration credentials only.\n")
	for _, key := range order {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(values[key])
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write env: %w", err)
	}
	return nil
}

func mustHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}
