package oauth

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/config"
	"basaltpass-backend/internal/model"
	"basaltpass-backend/internal/utils"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	randv2 "math/rand/v2"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

var (
	signingKeyMutex sync.Mutex
)

// OIDCDiscoveryResponse OIDC Discovery响应
type OIDCDiscoveryResponse struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	UserinfoEndpoint                           string   `json:"userinfo_endpoint"`
	JwksURI                                    string   `json:"jwks_uri"`
	RegistrationEndpoint                       string   `json:"registration_endpoint,omitempty"`
	RevocationEndpoint                         string   `json:"revocation_endpoint,omitempty"`
	IntrospectionEndpoint                      string   `json:"introspection_endpoint,omitempty"`
	ScopesSupported                            []string `json:"scopes_supported"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	ResponseModesSupported                     []string `json:"response_modes_supported"`
	PromptValuesSupported                      []string `json:"prompt_values_supported,omitempty"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgValuesSupported []string `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`
	SubjectTypesSupported                      []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported           []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                            []string `json:"claims_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	ClaimsParameterSupported                   bool     `json:"claims_parameter_supported"`
	RequestParameterSupported                  bool     `json:"request_parameter_supported"`
	RequestObjectSigningAlgValuesSupported     []string `json:"request_object_signing_alg_values_supported,omitempty"`

	CheckSessionIframe string `json:"check_session_iframe,omitempty"`
	EndSessionEndpoint string `json:"end_session_endpoint,omitempty"`
}

// DiscoveryHandler OIDC Discovery端点
// GET /.well-known/openid-configuration
func DiscoveryHandler(c *fiber.Ctx) error {
	issuer := oidcIssuer()

	discovery := &OIDCDiscoveryResponse{
		Issuer:                issuer,
		AuthorizationEndpoint: issuer + "/oauth/authorize",
		TokenEndpoint:         issuer + "/oauth/token",
		UserinfoEndpoint:      issuer + "/oauth/userinfo",
		JwksURI:               issuer + "/oauth/jwks",
		RevocationEndpoint:    issuer + "/oauth/revoke",
		IntrospectionEndpoint: issuer + "/oauth/introspect",

		ScopesSupported: []string{
			"openid",
			"profile",
			"email",
			"address",
			"phone",
			"offline_access",
		},
		ResponseTypesSupported: []string{
			"code",
		},
		ResponseModesSupported: []string{
			"query",
			"fragment",
		},
		PromptValuesSupported: []string{
			"none",
			"login",
			"consent",
			"select_account",
		},
		GrantTypesSupported: []string{
			"authorization_code",
			"refresh_token",
			"urn:ietf:params:oauth:grant-type:token-exchange",
		},
		TokenEndpointAuthMethodsSupported: []string{
			"client_secret_basic",
			"client_secret_post",
			"none",
			"client_secret_jwt",
			"private_key_jwt",
		},
		TokenEndpointAuthSigningAlgValuesSupported: []string{
			"HS256",
			"RS256",
		},
		SubjectTypesSupported: []string{
			"public",
			"pairwise",
		},
		IDTokenSigningAlgValuesSupported: []string{
			"RS256",
		},
		ClaimsSupported: []string{
			"sub",
			"azp",
			"auth_time",
			"acr",
			"amr",
			"nonce",
			"email",
			"email_verified",
			"phone_number",
			"phone_number_verified",
			"address",
			"given_name",
			"family_name",
			"middle_name",
			"name",
			"nickname",
			"nick_name",
			"profile",
			"website",
			"gender",
			"birthdate",
			"picture",
			"preferred_username",
			"locale",
			"zoneinfo",
			"updated_at",
		},
		CodeChallengeMethodsSupported: []string{
			"S256",
		},
		ClaimsParameterSupported:  false,
		RequestParameterSupported: true,
		RequestObjectSigningAlgValuesSupported: []string{
			"none",
		},

		CheckSessionIframe: issuer + "/check_session_iframe",
		EndSessionEndpoint: issuer + "/end_session",
	}

	return c.JSON(discovery)
}

func oidcIssuer() string {
	if issuer := strings.TrimRight(strings.TrimSpace(config.Get().OIDC.Issuer), "/"); issuer != "" {
		return issuer
	}

	address := strings.TrimSpace(config.Get().Server.Address)
	if address == "" {
		address = "localhost:8101"
	}
	if strings.HasPrefix(address, ":") {
		address = "localhost" + address
	}
	if _, err := strconv.Atoi(address); err == nil {
		address = "localhost:" + address
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}
	return strings.TrimRight(address, "/") + "/api/v1"
}

// JWKSHandler JWKS端点
// GET /oauth/jwks
func JWKSHandler(c *fiber.Ctx) error {
	jwks, err := publicJWKS()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "jwks_unavailable",
			"error_description": "Failed to load OIDC signing keys",
		})
	}

	return c.JSON(jwks)
}

func activeSigningKey() (*rsa.PrivateKey, string, error) {
	record, err := ensureActiveSigningKey()
	if err != nil {
		return nil, "", err
	}
	privateKey, err := parseEncryptedPrivateKey(record.PrivateKeyEncrypted)
	if err != nil {
		return nil, "", err
	}
	return privateKey, record.KID, nil
}

func ensureActiveSigningKey() (*model.OIDCSigningKey, error) {
	signingKeyMutex.Lock()
	defer signingKeyMutex.Unlock()

	db := common.DB()
	if db == nil {
		return nil, fmt.Errorf("database not ready: %v", common.DBErr())
	}

	if err := db.AutoMigrate(&model.OIDCSigningKey{}); err != nil {
		return nil, fmt.Errorf("migrate oidc signing keys: %w", err)
	}

	var existing model.OIDCSigningKey
	err := db.Where("status = ? AND algorithm = ?", model.OIDCSigningKeyStatusActive, "RS256").
		Order("created_at DESC").
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	record, err := newOIDCSigningKeyRecord(model.OIDCSigningKeyStatusActive)
	if err != nil {
		return nil, err
	}
	if err := db.Create(&record).Error; err != nil {
		var raced model.OIDCSigningKey
		if lookupErr := db.Where("status = ? AND algorithm = ?", model.OIDCSigningKeyStatusActive, "RS256").
			Order("created_at DESC").
			First(&raced).Error; lookupErr == nil {
			return &raced, nil
		}
		return nil, err
	}

	return &record, nil
}

func newOIDCSigningKeyRecord(status string) (model.OIDCSigningKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return model.OIDCSigningKey{}, fmt.Errorf("failed to generate RSA key: %w", err)
	}
	kid := fmt.Sprintf("basaltpass-rsa-%d-%08x", time.Now().Unix(), randv2.Uint32())
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	encrypted, err := utils.EncryptOIDCSigningPrivateKey(string(privatePEM))
	if err != nil {
		return model.OIDCSigningKey{}, err
	}
	jwk, err := generateRSAJWK(&key.PublicKey, kid)
	if err != nil {
		return model.OIDCSigningKey{}, err
	}
	publicJWK, err := json.Marshal(jwk)
	if err != nil {
		return model.OIDCSigningKey{}, err
	}
	now := time.Now()
	return model.OIDCSigningKey{
		KID:                 kid,
		Algorithm:           "RS256",
		PrivateKeyEncrypted: encrypted,
		PublicJWK:           string(publicJWK),
		Status:              status,
		NotBefore:           &now,
	}, nil
}

func RotateOIDCSigningKey() (*model.OIDCSigningKey, error) {
	signingKeyMutex.Lock()
	defer signingKeyMutex.Unlock()

	db := common.DB()
	if db == nil {
		return nil, fmt.Errorf("database not ready: %v", common.DBErr())
	}
	if err := db.AutoMigrate(&model.OIDCSigningKey{}); err != nil {
		return nil, fmt.Errorf("migrate oidc signing keys: %w", err)
	}

	now := time.Now()
	retainUntil := now.Add(2 * time.Hour)
	returned := model.OIDCSigningKey{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OIDCSigningKey{}).
			Where("status = ? AND algorithm = ?", model.OIDCSigningKeyStatusActive, "RS256").
			Updates(map[string]interface{}{
				"status":     model.OIDCSigningKeyStatusRetired,
				"rotated_at": now,
				"retired_at": now,
				"not_after":  retainUntil,
			}).Error; err != nil {
			return err
		}

		var standby model.OIDCSigningKey
		if err := tx.Where("status = ? AND algorithm = ?", model.OIDCSigningKeyStatusStandby, "RS256").
			Order("created_at DESC").
			First(&standby).Error; err == nil {
			if err := tx.Model(&standby).Updates(map[string]interface{}{
				"status":     model.OIDCSigningKeyStatusActive,
				"not_before": now,
				"rotated_at": now,
			}).Error; err != nil {
				return err
			}
			standby.Status = model.OIDCSigningKeyStatusActive
			standby.NotBefore = &now
			standby.RotatedAt = &now
			returned = standby
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		record, err := newOIDCSigningKeyRecord(model.OIDCSigningKeyStatusActive)
		if err != nil {
			return err
		}
		record.RotatedAt = &now
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		returned = record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &returned, nil
}

func AdminRotateOIDCSigningKeyHandler(c *fiber.Ctx) error {
	key, err := RotateOIDCSigningKey()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "rotation_failed",
			"error_description": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"message": "OIDC signing key rotated",
		"key": fiber.Map{
			"kid":        key.KID,
			"algorithm":  key.Algorithm,
			"status":     key.Status,
			"not_before": key.NotBefore,
			"rotated_at": key.RotatedAt,
		},
	})
}

func publicJWKS() (map[string]interface{}, error) {
	if _, err := ensureActiveSigningKey(); err != nil {
		return nil, err
	}

	db := common.DB()
	var records []model.OIDCSigningKey
	now := time.Now()
	if err := db.Where(
		"algorithm = ? AND (status IN ? OR (status = ? AND (not_after IS NULL OR not_after > ?)))",
		"RS256",
		[]string{model.OIDCSigningKeyStatusActive, model.OIDCSigningKeyStatusStandby},
		model.OIDCSigningKeyStatusRetired,
		now,
	).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}

	keys := make([]interface{}, 0, len(records))
	for _, record := range records {
		if record.PublicJWK == "" {
			continue
		}
		var jwk map[string]interface{}
		if err := json.Unmarshal([]byte(record.PublicJWK), &jwk); err != nil {
			continue
		}
		keys = append(keys, jwk)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no public signing keys available")
	}

	return map[string]interface{}{"keys": keys}, nil
}

func parseEncryptedPrivateKey(encrypted string) (*rsa.PrivateKey, error) {
	privatePEM, err := utils.DecryptOIDCSigningPrivateKey(encrypted)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, fmt.Errorf("invalid private key PEM")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// generateRSAJWK 生成RSA公钥的JWK表示
func generateRSAJWK(pubKey *rsa.PublicKey, kid string) (map[string]interface{}, error) {
	// 将大整数转换为base64url编码
	nBytes := pubKey.N.Bytes()
	eBytes := big.NewInt(int64(pubKey.E)).Bytes()

	// Base64URL编码（无填充）
	n := base64.RawURLEncoding.EncodeToString(nBytes)
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	jwk := map[string]interface{}{
		"kty": "RSA",   // 密钥类型
		"use": "sig",   // 用途：签名
		"alg": "RS256", // 算法
		"kid": kid,     // 密钥ID
		"n":   n,       // RSA模数
		"e":   e,       // RSA指数
	}

	return jwk, nil
}

// GetPrivateKey 获取私钥（用于JWT签名）
func GetPrivateKey() (*rsa.PrivateKey, error) {
	privateKey, _, err := activeSigningKey()
	return privateKey, err
}

// GetKeyID 获取密钥ID
func GetKeyID() string {
	_, kid, err := activeSigningKey()
	if err != nil {
		return ""
	}
	return kid
}

// CheckSessionIframeHandler 会话检查iframe端点
// GET /check_session_iframe
func CheckSessionIframeHandler(c *fiber.Ctx) error {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>BasaltPass Session Check</title>
</head>
<body>
	<script>
	        window.addEventListener('message', function(e) {
	            e.source.postMessage('unchanged', e.origin);
	        });
	    </script>
</body>
</html>
	`

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

// EndSessionHandler 结束会话端点
// GET /end_session
func EndSessionHandler(c *fiber.Ctx) error {
	clearHostedAuthCookies(c)

	idTokenHint := strings.TrimSpace(c.Query("id_token_hint"))
	postLogoutRedirectURI := strings.TrimSpace(c.Query("post_logout_redirect_uri"))
	state := c.Query("state")

	if postLogoutRedirectURI == "" {
		return c.JSON(fiber.Map{"message": "logged_out"})
	}

	client, err := oauthClientFromIDTokenHint(idTokenHint)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Invalid or missing id_token_hint",
		})
	}
	if !client.ValidatePostLogoutRedirectURI(postLogoutRedirectURI) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "post_logout_redirect_uri is not registered for this client",
		})
	}

	redirectURI, err := logoutRedirectWithState(postLogoutRedirectURI, state)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Invalid post_logout_redirect_uri",
		})
	}
	return c.Redirect(redirectURI, fiber.StatusFound)
}

func clearHostedAuthCookies(c *fiber.Ctx) {
	for _, name := range []string{
		"access_token",
		"refresh_token",
		"access_token_user",
		"refresh_token_user",
		"access_token_tenant",
		"refresh_token_tenant",
		"access_token_admin",
		"refresh_token_admin",
	} {
		c.Cookie(&fiber.Cookie{
			Name:     name,
			Value:    "",
			HTTPOnly: true,
			Secure:   c.Secure(),
			SameSite: "Lax",
			Path:     "/",
			MaxAge:   -1,
			Domain:   "",
		})
	}
}

func oauthClientFromIDTokenHint(idTokenHint string) (*model.OAuthClient, error) {
	if idTokenHint == "" {
		return nil, fmt.Errorf("missing id_token_hint")
	}

	key, err := publicKeyFromIDTokenHint(idTokenHint)
	if err != nil {
		return nil, err
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(idTokenHint, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return key, nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid id_token_hint")
	}

	clientID := strings.TrimSpace(claimString(claims["azp"]))
	audiences := claimStringList(claims["aud"])
	if clientID == "" && len(audiences) == 1 {
		clientID = audiences[0]
	}
	if clientID == "" {
		return nil, fmt.Errorf("missing client audience")
	}

	var client model.OAuthClient
	if err := common.DB().Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func publicKeyFromIDTokenHint(idTokenHint string) (*rsa.PublicKey, error) {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(idTokenHint, jwt.MapClaims{})
	if err != nil || token == nil {
		return nil, fmt.Errorf("invalid id_token_hint")
	}
	kid, _ := token.Header["kid"].(string)
	if strings.TrimSpace(kid) == "" {
		return nil, fmt.Errorf("missing kid")
	}

	var record model.OIDCSigningKey
	if err := common.DB().Where(
		"kid = ? AND algorithm = ? AND status IN ?",
		kid,
		"RS256",
		[]string{model.OIDCSigningKeyStatusActive, model.OIDCSigningKeyStatusStandby, model.OIDCSigningKeyStatusRetired},
	).First(&record).Error; err != nil {
		return nil, err
	}
	return rsaPublicKeyFromJWK(record.PublicJWK)
}

func rsaPublicKeyFromJWK(publicJWK string) (*rsa.PublicKey, error) {
	var jwk map[string]string
	if err := json.Unmarshal([]byte(publicJWK), &jwk); err != nil {
		return nil, err
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk["n"])
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk["e"])
	if err != nil {
		return nil, err
	}
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, fmt.Errorf("invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func claimString(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func claimStringList(value interface{}) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				items = append(items, s)
			}
		}
		return items
	case jwt.ClaimStrings:
		return []string(v)
	default:
		return nil
	}
}

func logoutRedirectWithState(postLogoutRedirectURI string, state string) (string, error) {
	u, err := url.Parse(postLogoutRedirectURI)
	if err != nil {
		return "", err
	}
	if state != "" {
		q := u.Query()
		q.Set("state", state)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}
