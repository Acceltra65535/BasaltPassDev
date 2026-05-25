package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 使用bcrypt哈希密码
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword 验证密码
func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// totpEncryptionKey 返回用于 AES-256-GCM 加密 TOTP 密钥的 32 字节密钥。
// 优先使用环境变量 TOTP_ENCRYPTION_KEY（原始 32 字节，或 64 字符十六进制，
// 或任意字符串将被 SHA-256 截断为 32 字节）。
// 否则从 JWT_SECRET 派生；若两者都缺失则返回错误，避免可预测默认值。
func totpEncryptionKey() ([]byte, error) {
	if raw := os.Getenv("TOTP_ENCRYPTION_KEY"); raw != "" {
		return normalizeEncryptionKey(raw), nil
	}
	// 回落：从 JWT_SECRET 派生
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("missing encryption key: set TOTP_ENCRYPTION_KEY or JWT_SECRET")
	}
	h := sha256.Sum256([]byte(jwtSecret + ":totp_key_v1"))
	return h[:], nil
}

func normalizeEncryptionKey(raw string) []byte {
	b := []byte(raw)
	if len(b) == 32 {
		return b
	}
	if len(raw) == 64 {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
			return decoded
		}
	}
	h := sha256.Sum256(b)
	return h[:]
}

func oidcSigningKeyEncryptionKey() ([]byte, error) {
	if raw := os.Getenv("OIDC_KEY_ENCRYPTION_SECRET"); raw != "" {
		return normalizeEncryptionKey(raw), nil
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("missing encryption key: set OIDC_KEY_ENCRYPTION_SECRET or JWT_SECRET")
	}
	h := sha256.Sum256([]byte(jwtSecret + ":oidc_signing_key_v1"))
	return h[:], nil
}

func encryptWithAESGCM(plaintext string, key []byte, prefix string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptWithAESGCM(stored string, key []byte, prefix string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(stored[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}
	return string(plaintext), nil
}

const totpEncPrefix = "enc:v1:"
const oidcSigningKeyEncPrefix = "enc:oidc:v1:"
const oauthClientSecretEncPrefix = "enc:oauth-client-secret:v1:"

// EncryptTOTPSecret 用 AES-256-GCM 加密 TOTP 明文密钥，返回带前缀的 base64url 字符串。
// 空字符串原样返回（表示未配置 TOTP）。
func EncryptTOTPSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key, err := totpEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("totp encrypt: key derivation failed: %w", err)
	}
	encrypted, err := encryptWithAESGCM(plaintext, key, totpEncPrefix)
	if err != nil {
		return "", fmt.Errorf("totp encrypt: %w", err)
	}
	return encrypted, nil
}

// DecryptTOTPSecret 解密由 EncryptTOTPSecret 生成的密文。
// 若输入不带 "enc:v1:" 前缀，视为历史明文直接返回（向后兼容）。
func DecryptTOTPSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, totpEncPrefix) {
		// 历史明文，直接返回
		return stored, nil
	}
	key, err := totpEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("totp decrypt: key derivation failed: %w", err)
	}
	plaintext, err := decryptWithAESGCM(stored, key, totpEncPrefix)
	if err != nil {
		return "", fmt.Errorf("totp decrypt: %w", err)
	}
	return plaintext, nil
}

func EncryptOIDCSigningPrivateKey(plaintext string) (string, error) {
	key, err := oidcSigningKeyEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("oidc signing key encrypt: key derivation failed: %w", err)
	}
	encrypted, err := encryptWithAESGCM(plaintext, key, oidcSigningKeyEncPrefix)
	if err != nil {
		return "", fmt.Errorf("oidc signing key encrypt: %w", err)
	}
	return encrypted, nil
}

func DecryptOIDCSigningPrivateKey(stored string) (string, error) {
	key, err := oidcSigningKeyEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("oidc signing key decrypt: key derivation failed: %w", err)
	}
	plaintext, err := decryptWithAESGCM(stored, key, oidcSigningKeyEncPrefix)
	if err != nil {
		return "", fmt.Errorf("oidc signing key decrypt: %w", err)
	}
	return plaintext, nil
}

func EncryptOAuthClientSecret(plaintext string) (string, error) {
	key, err := oidcSigningKeyEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("oauth client secret encrypt: key derivation failed: %w", err)
	}
	encrypted, err := encryptWithAESGCM(plaintext, key, oauthClientSecretEncPrefix)
	if err != nil {
		return "", fmt.Errorf("oauth client secret encrypt: %w", err)
	}
	return encrypted, nil
}

func DecryptOAuthClientSecret(stored string) (string, error) {
	key, err := oidcSigningKeyEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("oauth client secret decrypt: key derivation failed: %w", err)
	}
	plaintext, err := decryptWithAESGCM(stored, key, oauthClientSecretEncPrefix)
	if err != nil {
		return "", fmt.Errorf("oauth client secret decrypt: %w", err)
	}
	return plaintext, nil
}
