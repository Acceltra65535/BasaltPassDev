package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPaymentServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Tenant{}, &model.PaymentIntent{}, &model.PaymentSession{}, &model.PaymentWebhookEvent{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	common.SetDBForTest(db)
	return db
}

func stripeTestSignature(payload []byte, secret string, ts time.Time) string {
	timestamp := ts.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyStripeSignatureRejectsStaleTimestamp(t *testing.T) {
	payload := []byte(`{"id":"evt_old","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","object":"payment_intent"}}}`)
	signature := stripeTestSignature(payload, "whsec_test", time.Now().Add(-10*time.Minute))

	if verifyStripeSignature(payload, signature, "whsec_test", time.Now()) {
		t.Fatalf("expected stale Stripe signature to be rejected")
	}
}

func TestProcessStripeWebhookRejectsTenantMismatch(t *testing.T) {
	db := setupPaymentServiceTestDB(t)

	tenantA := model.Tenant{
		Name:   "Tenant A",
		Code:   "tenant-a",
		Status: model.TenantStatusActive,
		Metadata: model.JSONMap{"stripe": map[string]interface{}{
			"enabled":        true,
			"webhook_secret": "whsec_a",
		}},
	}
	tenantB := model.Tenant{
		Name:   "Tenant B",
		Code:   "tenant-b",
		Status: model.TenantStatusActive,
		Metadata: model.JSONMap{"stripe": map[string]interface{}{
			"enabled":        true,
			"webhook_secret": "whsec_b",
		}},
	}
	if err := db.Create(&tenantA).Error; err != nil {
		t.Fatalf("create tenant A failed: %v", err)
	}
	if err := db.Create(&tenantB).Error; err != nil {
		t.Fatalf("create tenant B failed: %v", err)
	}

	user := model.User{Email: "tenant-b-user@example.com", TenantID: tenantB.ID, PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	intent := model.PaymentIntent{
		StripePaymentIntentID: "pi_tenant_b",
		UserID:                user.ID,
		Amount:                1000,
		Currency:              "USD",
		Status:                model.PaymentIntentStatusProcessing,
		Metadata:              fmt.Sprintf(`{"tenant_id":"%d","user_id":"%d"}`, tenantB.ID, user.ID),
	}
	if err := db.Create(&intent).Error; err != nil {
		t.Fatalf("create payment intent failed: %v", err)
	}

	payload := []byte(fmt.Sprintf(`{
		"id":"evt_cross_tenant",
		"type":"payment_intent.succeeded",
		"data":{"object":{
			"id":"pi_tenant_b",
			"object":"payment_intent",
			"metadata":{"tenant_id":"%d"}
		}}
	}`, tenantA.ID))
	signature := stripeTestSignature(payload, "whsec_a", time.Now())

	if _, _, err := ProcessStripeWebhook(payload, signature); err == nil {
		t.Fatalf("expected cross-tenant Stripe webhook to be rejected")
	}
}
