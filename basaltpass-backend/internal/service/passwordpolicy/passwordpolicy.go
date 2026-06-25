package passwordpolicy

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	settingssvc "basaltpass-backend/internal/service/settings"
)

var (
	ErrTooShort         = errors.New("password is too short")
	ErrMissingNumber    = errors.New("password must contain a number")
	ErrMissingUppercase = errors.New("password must contain an uppercase letter")
	ErrMissingLowercase = errors.New("password must contain a lowercase letter")
	ErrMissingSpecial   = errors.New("password must contain a special character")
	ErrCommonPassword   = errors.New("password is too common")
	ErrLeakedPassword   = errors.New("password appears in a known leaked password list")
	ErrContainsIdentity = errors.New("password must not contain account identifiers")
	ErrPredictable      = errors.New("password is too predictable")
)

type Options struct {
	MinLength              int
	RequireNumber          bool
	RequireUppercase       bool
	RequireLowercase       bool
	RequireSpecial         bool
	RejectCommonPasswords  bool
	RejectLeakedPasswords  bool
	RejectUserIdentifiers  bool
	RejectPredictableForms bool
}

func OptionsFromSettings() Options {
	return Options{
		MinLength:              settingssvc.GetInt("auth.password_policy.min_length", 8),
		RequireNumber:          settingssvc.GetBool("auth.password_policy.require_numbers", true),
		RequireUppercase:       settingssvc.GetBool("auth.password_policy.require_uppercase", false),
		RequireLowercase:       settingssvc.GetBool("auth.password_policy.require_lowercase", true),
		RequireSpecial:         settingssvc.GetBool("auth.password_policy.require_special", false),
		RejectCommonPasswords:  settingssvc.GetBool("auth.password_policy.reject_common_passwords", true),
		RejectLeakedPasswords:  settingssvc.GetBool("auth.password_policy.reject_leaked_passwords", true),
		RejectUserIdentifiers:  settingssvc.GetBool("auth.password_policy.reject_user_identifiers", true),
		RejectPredictableForms: settingssvc.GetBool("auth.password_policy.reject_predictable_forms", true),
	}
}

func Validate(password string, identifiers ...string) error {
	return ValidateWithOptions(password, OptionsFromSettings(), identifiers...)
}

func ValidateWithOptions(password string, opts Options, identifiers ...string) error {
	if opts.MinLength <= 0 {
		opts.MinLength = 8
	}
	if len([]rune(password)) < opts.MinLength {
		return fmt.Errorf("%w: at least %d characters", ErrTooShort, opts.MinLength)
	}

	var hasNumber, hasUppercase, hasLowercase, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsDigit(r):
			hasNumber = true
		case unicode.IsUpper(r):
			hasUppercase = true
		case unicode.IsLower(r):
			hasLowercase = true
		case unicode.IsLetter(r):
		default:
			hasSpecial = true
		}
	}
	if opts.RequireNumber && !hasNumber {
		return ErrMissingNumber
	}
	if opts.RequireUppercase && !hasUppercase {
		return ErrMissingUppercase
	}
	if opts.RequireLowercase && !hasLowercase {
		return ErrMissingLowercase
	}
	if opts.RequireSpecial && !hasSpecial {
		return ErrMissingSpecial
	}

	normalized := normalize(password)
	if opts.RejectLeakedPasswords && isLeakedPassword(password) {
		return ErrLeakedPassword
	}
	if opts.RejectCommonPasswords && isCommonPassword(password, normalized) {
		return ErrCommonPassword
	}
	if opts.RejectPredictableForms && isPredictable(password, normalized) {
		return ErrPredictable
	}
	if opts.RejectUserIdentifiers && containsIdentifier(normalized, identifiers) {
		return ErrContainsIdentity
	}

	return nil
}

func isCommonPassword(password, normalized string) bool {
	if commonPasswords[normalized] {
		return true
	}
	rawBase := strings.TrimRightFunc(stripSeparators(strings.ToLower(strings.TrimSpace(password))), func(r rune) bool {
		return unicode.IsDigit(r) || !unicode.IsLetter(r)
	})
	if len(rawBase) >= 6 && commonPasswords[commonCandidate(rawBase)] {
		return true
	}
	candidate := commonCandidate(password)
	if commonPasswords[candidate] {
		return true
	}
	candidate = strings.TrimRightFunc(candidate, func(r rune) bool {
		return unicode.IsDigit(r) || !unicode.IsLetter(r)
	})
	return len(candidate) >= 6 && commonPasswords[candidate]
}

func stripSeparators(s string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "")
	return replacer.Replace(s)
}

func commonCandidate(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(
		" ", "",
		"-", "",
		"_", "",
		".", "",
		"@", "a",
		"$", "s",
		"0", "o",
		"3", "e",
		"4", "a",
		"5", "s",
		"7", "t",
	)
	return replacer.Replace(s)
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(
		" ", "",
		"-", "",
		"_", "",
		".", "",
		"@", "a",
		"$", "s",
		"!", "i",
		"1", "i",
		"0", "o",
		"3", "e",
		"4", "a",
		"5", "s",
		"7", "t",
	)
	return replacer.Replace(s)
}

func isLeakedPassword(password string) bool {
	return leakedPasswords[password]
}

func containsIdentifier(normalizedPassword string, identifiers []string) bool {
	for _, identifier := range identifiers {
		for _, part := range identityParts(identifier) {
			if len(part) >= 4 && strings.Contains(normalizedPassword, normalize(part)) {
				return true
			}
		}
	}
	return false
}

func identityParts(identifier string) []string {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" {
		return nil
	}
	parts := []string{identifier}
	if at := strings.IndexByte(identifier, '@'); at > 0 {
		parts = append(parts, identifier[:at])
	}
	fields := strings.FieldsFunc(identifier, func(r rune) bool {
		return r == '@' || r == '.' || r == '-' || r == '_' || unicode.IsSpace(r)
	})
	parts = append(parts, fields...)
	return parts
}

func isPredictable(password, normalized string) bool {
	candidate := strings.TrimRightFunc(stripSeparators(strings.ToLower(strings.TrimSpace(password))), func(r rune) bool {
		return unicode.IsDigit(r) || !unicode.IsLetter(r)
	})
	if len(candidate) >= 8 {
		normalized = candidate
	}
	if len(normalized) < 8 {
		return false
	}
	if allSameRune(normalized) {
		return true
	}
	if strings.Contains("abcdefghijklmnopqrstuvwxyz", normalized) ||
		strings.Contains("zyxwvutsrqponmlkjihgfedcba", normalized) ||
		strings.Contains("01234567890123456789", normalized) ||
		strings.Contains("98765432109876543210", normalized) ||
		strings.Contains("qwertyuiopasdfghjklzxcvbnm", normalized) {
		return true
	}
	return false
}

func allSameRune(s string) bool {
	var first rune
	for i, r := range s {
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			return false
		}
	}
	return true
}

var commonPasswords = map[string]bool{
	"password":     true,
	"admin":        true,
	"qwerty":       true,
	"qwertyuiop":   true,
	"welcome":      true,
	"letmein":      true,
	"iloveyou":     true,
	"monkey":       true,
	"dragon":       true,
	"baseball":     true,
	"football":     true,
	"abcdefg":      true,
	"basaltpass":   true,
	"changeme":     true,
	"test":         true,
	"testpassword": true,
}

// A small built-in blocklist for well-known leaked passwords.
// It is intentionally local so validation never sends user passwords to a third party.
var leakedPasswords = map[string]bool{
	"password":    true,
	"password1":   true,
	"password123": true,
	"qwerty":      true,
	"admin":       true,
	"admin123":    true,
	"hello":       true,
	"letmein":     true,
	"iloveyou":    true,
}
