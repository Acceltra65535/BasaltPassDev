package wallet

import (
	"testing"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateWalletForAppOwner(t *testing.T) {
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
	tenant := model.Tenant{Name: "Tenant", Code: "owner-wallet", Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	app := model.App{TenantID: tenant.ID, Name: "Worker", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	currency := model.Currency{Code: "CREDIT", Name: "Credit", DecimalPlaces: 0, IsActive: true}
	if err := db.Create(&currency).Error; err != nil {
		t.Fatal(err)
	}

	walletModel, err := NewAdminWalletService().CreateWalletForOwner(model.WalletOwnerApp, app.ID, "CREDIT", 500)
	if err != nil {
		t.Fatal(err)
	}
	if walletModel.OwnerType != model.WalletOwnerApp || walletModel.OwnerID != app.ID || walletModel.Balance != 500 {
		t.Fatalf("unexpected wallet: %+v", walletModel)
	}
	if _, err := NewAdminWalletService().CreateWalletForOwner(model.WalletOwnerApp, app.ID, "CREDIT", 0); err == nil {
		t.Fatal("expected duplicate wallet error")
	}
}
