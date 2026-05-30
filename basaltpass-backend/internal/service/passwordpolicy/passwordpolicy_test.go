package passwordpolicy

import (
	"errors"
	"testing"
)

func testOptions() Options {
	return Options{
		MinLength:              8,
		RequireNumber:          true,
		RequireUppercase:       false,
		RequireLowercase:       true,
		RequireSpecial:         false,
		RejectCommonPasswords:  true,
		RejectLeakedPasswords:  true,
		RejectUserIdentifiers:  true,
		RejectPredictableForms: true,
	}
}

func TestValidateRejectsWeakAndLeakedPasswords(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "common password with suffix", password: "Password123!", wantErr: ErrCommonPassword},
		{name: "leaked exact password", password: "password123", wantErr: ErrLeakedPassword},
		{name: "predictable sequence", password: "abcdefghi1", wantErr: ErrPredictable},
		{name: "repeated characters", password: "aaaaaaaa1", wantErr: ErrPredictable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithOptions(tt.password, testOptions())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateRejectsUserIdentifiers(t *testing.T) {
	err := ValidateWithOptions("Alice2026", testOptions(), "alice@example.com", "Alice")
	if !errors.Is(err, ErrContainsIdentity) {
		t.Fatalf("expected ErrContainsIdentity, got %v", err)
	}
}

func TestValidateAcceptsReasonablePassword(t *testing.T) {
	if err := ValidateWithOptions("RidgeMint82", testOptions(), "alice@example.com"); err != nil {
		t.Fatalf("expected password to pass, got %v", err)
	}
}

func TestValidateRespectsDisabledBlocklists(t *testing.T) {
	opts := testOptions()
	opts.RejectCommonPasswords = false
	opts.RejectLeakedPasswords = false
	opts.RejectPredictableForms = false

	if err := ValidateWithOptions("password123", opts); err != nil {
		t.Fatalf("expected disabled blocklists to allow password, got %v", err)
	}
}
