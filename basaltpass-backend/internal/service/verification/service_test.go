package verification

import (
	"testing"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupVerificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&model.PendingSignup{}, &model.VerificationChallenge{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	common.SetDBForTest(db)
	return db
}

func TestGetSignupStatusUsesPersistedState(t *testing.T) {
	db := setupVerificationTestDB(t)
	signup := model.PendingSignup{
		ID:        "status-test-signup",
		Status:    model.SignupStatusCompleted,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&signup).Error; err != nil {
		t.Fatalf("create signup failed: %v", err)
	}

	status, err := (&Service{}).GetSignupStatus(signup.ID)
	if err != nil {
		t.Fatalf("get signup status failed: %v", err)
	}
	if status.Status != "completed" {
		t.Fatalf("expected completed status, got %q", status.Status)
	}
}

func TestVerificationPepperChangesHash(t *testing.T) {
	first := (&Service{pepper: "first"}).hashCode("123456", "salt")
	second := (&Service{pepper: "second"}).hashCode("123456", "salt")
	if first == second {
		t.Fatal("expected different pepper values to produce different hashes")
	}
}
