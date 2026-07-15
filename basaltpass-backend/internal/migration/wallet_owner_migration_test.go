package migration

import (
	"testing"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateWalletOwnerFieldsMergesDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	common.SetDBForTest(db)
	statements := []string{
		`CREATE TABLE market_wallets (id integer primary key autoincrement, created_at datetime, updated_at datetime, deleted_at datetime, tenant_id integer not null default 0, user_id integer, team_id integer, currency_id integer not null, balance integer, freeze integer)`,
		`CREATE TABLE market_wallet_txes (id integer primary key autoincrement, created_at datetime, updated_at datetime, deleted_at datetime, wallet_id integer, type text, amount integer, status text, reference text)`,
		`INSERT INTO market_wallets (tenant_id, user_id, currency_id, balance, freeze) VALUES (2, 3, 7, 100, 1)`,
		`INSERT INTO market_wallets (tenant_id, user_id, currency_id, balance, freeze) VALUES (2, 3, 7, 25, 2)`,
		`INSERT INTO market_wallet_txes (wallet_id, type, amount, status) VALUES (2, 'credit', 25, 'success')`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := MigrateWalletOwnerFields(); err != nil {
		t.Fatal(err)
	}
	var wallets []model.Wallet
	if err := db.Unscoped().Find(&wallets).Error; err != nil {
		t.Fatal(err)
	}
	if len(wallets) != 1 || wallets[0].Balance != 125 || wallets[0].Freeze != 3 {
		t.Fatalf("duplicate wallets were not merged: %+v", wallets)
	}
	if wallets[0].OwnerType != model.WalletOwnerUser || wallets[0].OwnerID != 3 {
		t.Fatalf("wallet owner was not backfilled: %+v", wallets[0])
	}
	var tx model.WalletTx
	if err := db.First(&tx).Error; err != nil {
		t.Fatal(err)
	}
	if tx.WalletID != wallets[0].ID {
		t.Fatalf("transaction still points at duplicate wallet: %d", tx.WalletID)
	}
}
