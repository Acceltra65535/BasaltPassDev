package wallet

import (
	"testing"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOwnerWalletTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Tenant{}, &model.TenantUser{}, &model.Team{},
		&model.App{}, &model.Currency{}, &model.Wallet{}, &model.WalletTx{},
	); err != nil {
		t.Fatal(err)
	}
	common.SetDBForTest(db)
	return db
}

func TestAdjustOwnerByCodeCreatesAppWallet(t *testing.T) {
	db := setupOwnerWalletTestDB(t)
	tenant := model.Tenant{Name: "Tenant", Code: "tenant", Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	app := model.App{TenantID: tenant.ID, Name: "Service", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	currency := model.Currency{Code: "CREDIT", Name: "Credit", DecimalPlaces: 0, IsActive: true}
	if err := db.Create(&currency).Error; err != nil {
		t.Fatal(err)
	}

	walletModel, err := AdjustOwnerByCode(model.WalletOwnerApp, app.ID, tenant.ID, "CREDIT", 250, "test_credit", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if walletModel.OwnerType != model.WalletOwnerApp || walletModel.OwnerID != app.ID || walletModel.Balance != 250 {
		t.Fatalf("unexpected app wallet: %+v", walletModel)
	}
	if walletModel.UserID != nil || walletModel.TeamID != nil {
		t.Fatalf("app wallet populated legacy ownership columns: %+v", walletModel)
	}

	replayed, err := AdjustOwnerByCode(model.WalletOwnerApp, app.ID, tenant.ID, "CREDIT", 250, "test_credit", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Balance != 250 {
		t.Fatalf("idempotent replay changed balance: %+v", replayed)
	}
	if _, err := AdjustOwnerByCode(model.WalletOwnerApp, app.ID, tenant.ID, "CREDIT", 1, "test_credit", "request-1"); err == nil {
		t.Fatal("expected idempotency key conflict")
	}

	if _, err := AdjustOwnerByCode(model.WalletOwnerApp, app.ID, tenant.ID, "CREDIT", -300, "test_debit", "request-2"); err == nil {
		t.Fatal("expected insufficient funds error")
	}
	var txCount int64
	if err := db.Model(&model.WalletTx{}).Where("wallet_id = ?", walletModel.ID).Count(&txCount).Error; err != nil {
		t.Fatal(err)
	}
	if txCount != 1 {
		t.Fatalf("expected one committed transaction, got %d", txCount)
	}
}

func TestLegacyUserWalletPopulatesUnifiedOwner(t *testing.T) {
	db := setupOwnerWalletTestDB(t)
	user := model.User{Email: "legacy-wallet@example.com", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	currency := model.Currency{Code: "USD", Name: "US Dollar", DecimalPlaces: 2, IsActive: true}
	if err := db.Create(&currency).Error; err != nil {
		t.Fatal(err)
	}
	userID := user.ID
	currencyID := currency.ID
	walletModel := model.Wallet{TenantID: 9, UserID: &userID, CurrencyID: &currencyID}
	if err := db.Create(&walletModel).Error; err != nil {
		t.Fatal(err)
	}
	if walletModel.OwnerType != model.WalletOwnerUser || walletModel.OwnerID != userID {
		t.Fatalf("legacy ownership was not normalized: %+v", walletModel)
	}
}
