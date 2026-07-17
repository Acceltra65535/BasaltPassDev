package rbacmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{1,99}$`)

var reservedCodePrefixes = []string{"tenant.", "system.", "s2s."}

func DecodeManifest(raw []byte) (*Manifest, error) {
	if len(raw) == 0 {
		return nil, validationError("request body is required")
	}
	if len(raw) > MaxManifestBytes {
		return nil, validationError("manifest exceeds %d bytes", MaxManifestBytes)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return nil, validationError("invalid RBAC-only manifest: %v", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, err
	}
	if err := ValidateAndNormalize(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return validationError("manifest must contain exactly one JSON object")
		}
		return validationError("invalid trailing JSON: %v", err)
	}
	return nil
}

func ValidateAndNormalize(manifest *Manifest) error {
	if manifest == nil {
		return validationError("manifest is required")
	}
	manifest.SchemaVersion = strings.TrimSpace(manifest.SchemaVersion)
	if manifest.SchemaVersion != "0.1.0" && manifest.SchemaVersion != "1.0.0" {
		return validationError("unsupported schema_version %q", manifest.SchemaVersion)
	}
	manifest.Type = strings.TrimSpace(manifest.Type)
	if manifest.Type != "basalt_rbac_bundle" {
		return validationError("type must be basalt_rbac_bundle")
	}
	if manifest.Revision == 0 {
		return validationError("revision must be greater than zero")
	}
	if len(manifest.Permissions) > 500 {
		return validationError("at most 500 permissions are allowed")
	}
	if len(manifest.Roles) > 100 {
		return validationError("at most 100 roles are allowed")
	}
	if len(manifest.RolePermissions) > 5000 {
		return validationError("at most 5000 role_permissions are allowed")
	}

	permissionKeys := make(map[string]struct{}, len(manifest.Permissions))
	for i := range manifest.Permissions {
		permission := &manifest.Permissions[i]
		permission.PermissionKey = strings.ToLower(strings.TrimSpace(permission.PermissionKey))
		permission.DisplayName = strings.TrimSpace(permission.DisplayName)
		permission.Resource = strings.TrimSpace(permission.Resource)
		permission.Action = strings.TrimSpace(permission.Action)
		permission.Scope = strings.TrimSpace(permission.Scope)
		permission.Description = strings.TrimSpace(permission.Description)
		permission.Status = strings.ToLower(strings.TrimSpace(permission.Status))
		if err := validateCode("permission_key", permission.PermissionKey); err != nil {
			return err
		}
		if _, exists := permissionKeys[permission.PermissionKey]; exists {
			return validationError("duplicate permission_key %q", permission.PermissionKey)
		}
		permissionKeys[permission.PermissionKey] = struct{}{}
		if permission.DisplayName == "" || runeLen(permission.DisplayName) > 100 {
			return validationError("permission %q display_name must contain 1-100 characters", permission.PermissionKey)
		}
		if runeLen(permission.Description) > 500 || runeLen(permission.Resource) > 50 || runeLen(permission.Action) > 50 || runeLen(permission.Scope) > 50 {
			return validationError("permission %q contains an oversized field", permission.PermissionKey)
		}
		if permission.Status != "" && permission.Status != "active" {
			return validationError("permission %q status must be active", permission.PermissionKey)
		}
	}

	roleKeys := make(map[string]struct{}, len(manifest.Roles))
	for i := range manifest.Roles {
		role := &manifest.Roles[i]
		role.RoleKey = strings.ToLower(strings.TrimSpace(role.RoleKey))
		role.DisplayName = strings.TrimSpace(role.DisplayName)
		role.Description = strings.TrimSpace(role.Description)
		role.Status = strings.ToLower(strings.TrimSpace(role.Status))
		if err := validateCode("role_key", role.RoleKey); err != nil {
			return err
		}
		if _, exists := roleKeys[role.RoleKey]; exists {
			return validationError("duplicate role_key %q", role.RoleKey)
		}
		roleKeys[role.RoleKey] = struct{}{}
		if role.DisplayName == "" || runeLen(role.DisplayName) > 100 {
			return validationError("role %q display_name must contain 1-100 characters", role.RoleKey)
		}
		if runeLen(role.Description) > 500 {
			return validationError("role %q description exceeds 500 characters", role.RoleKey)
		}
		if role.Priority < 0 || role.Priority > 1000000 {
			return validationError("role %q priority is out of range", role.RoleKey)
		}
		if role.Status != "" && role.Status != "active" {
			return validationError("role %q status must be active", role.RoleKey)
		}
	}

	linkKeys := make(map[string]struct{}, len(manifest.RolePermissions))
	for i := range manifest.RolePermissions {
		link := &manifest.RolePermissions[i]
		link.RoleKey = strings.ToLower(strings.TrimSpace(link.RoleKey))
		link.PermissionKey = strings.ToLower(strings.TrimSpace(link.PermissionKey))
		link.Effect = strings.ToLower(strings.TrimSpace(link.Effect))
		if _, exists := roleKeys[link.RoleKey]; !exists {
			return validationError("role_permissions references unknown role_key %q", link.RoleKey)
		}
		if _, exists := permissionKeys[link.PermissionKey]; !exists {
			return validationError("role_permissions references unknown permission_key %q", link.PermissionKey)
		}
		if link.Effect != "" && link.Effect != "allow" {
			return validationError("role_permissions effect must be allow")
		}
		link.Effect = "allow"
		key := link.RoleKey + "\x00" + link.PermissionKey
		if _, exists := linkKeys[key]; exists {
			return validationError("duplicate role_permissions link %q -> %q", link.RoleKey, link.PermissionKey)
		}
		linkKeys[key] = struct{}{}
	}

	sort.Slice(manifest.Permissions, func(i, j int) bool {
		return manifest.Permissions[i].PermissionKey < manifest.Permissions[j].PermissionKey
	})
	sort.Slice(manifest.Roles, func(i, j int) bool { return manifest.Roles[i].RoleKey < manifest.Roles[j].RoleKey })
	sort.Slice(manifest.RolePermissions, func(i, j int) bool {
		left := manifest.RolePermissions[i].RoleKey + "\x00" + manifest.RolePermissions[i].PermissionKey
		right := manifest.RolePermissions[j].RoleKey + "\x00" + manifest.RolePermissions[j].PermissionKey
		return left < right
	})
	return nil
}

func validateCode(field, code string) error {
	if !codePattern.MatchString(code) {
		return validationError("%s %q must match %s", field, code, codePattern.String())
	}
	for _, prefix := range reservedCodePrefixes {
		if strings.HasPrefix(code, prefix) {
			return validationError("%s %q uses reserved prefix %q", field, code, prefix)
		}
	}
	return nil
}

func SnapshotFromManifest(manifest *Manifest) Snapshot {
	snapshot := Snapshot{Permissions: []SnapshotPermission{}, Roles: []SnapshotRole{}}
	permissionsByRole := make(map[string][]string, len(manifest.Roles))
	for _, link := range manifest.RolePermissions {
		permissionsByRole[link.RoleKey] = append(permissionsByRole[link.RoleKey], link.PermissionKey)
	}
	for _, permission := range manifest.Permissions {
		category := firstNonEmpty(permission.Resource, permission.Scope, "app")
		snapshot.Permissions = append(snapshot.Permissions, SnapshotPermission{
			Code: permission.PermissionKey, Name: permission.DisplayName,
			Description: permission.Description, Category: category,
		})
	}
	for _, role := range manifest.Roles {
		codes := append([]string{}, permissionsByRole[role.RoleKey]...)
		sort.Strings(codes)
		snapshot.Roles = append(snapshot.Roles, SnapshotRole{
			Code: role.RoleKey, Name: role.DisplayName, Description: role.Description,
			PermissionCodes: codes,
		})
	}
	normalizeSnapshot(&snapshot)
	return snapshot
}

func marshalCanonical(value any) ([]byte, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func normalizeSnapshot(snapshot *Snapshot) {
	if snapshot.Permissions == nil {
		snapshot.Permissions = []SnapshotPermission{}
	}
	if snapshot.Roles == nil {
		snapshot.Roles = []SnapshotRole{}
	}
	for i := range snapshot.Roles {
		if snapshot.Roles[i].PermissionCodes == nil {
			snapshot.Roles[i].PermissionCodes = []string{}
		}
		sort.Strings(snapshot.Roles[i].PermissionCodes)
	}
	sort.Slice(snapshot.Permissions, func(i, j int) bool { return snapshot.Permissions[i].Code < snapshot.Permissions[j].Code })
	sort.Slice(snapshot.Roles, func(i, j int) bool { return snapshot.Roles[i].Code < snapshot.Roles[j].Code })
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func runeLen(value string) int { return utf8.RuneCountInString(value) }

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

func conflictError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}
