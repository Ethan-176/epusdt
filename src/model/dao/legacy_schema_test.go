package dao

import (
	"testing"

	"github.com/GMWalletApp/epusdt/model/mdb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateLegacySchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	Mdb = db
	t.Cleanup(func() { Mdb = nil })

	legacyDDL := []string{
		`CREATE TABLE wallet_address (
			id integer PRIMARY KEY AUTOINCREMENT,
			token varchar(50) NOT NULL,
			status integer NOT NULL DEFAULT 1,
			description varchar(100),
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)`,
		`CREATE TABLE orders (
			id integer PRIMARY KEY AUTOINCREMENT,
			trade_id varchar(32) NOT NULL UNIQUE,
			order_id varchar(32) NOT NULL UNIQUE,
			block_transaction_id varchar(128),
			actual_amount decimal(19,4) NOT NULL,
			amount decimal(19,4) NOT NULL,
			token varchar(50) NOT NULL,
			status integer NOT NULL DEFAULT 1,
			notify_url varchar(128) NOT NULL,
			redirect_url varchar(128),
			callback_num integer DEFAULT 0,
			callback_confirm integer DEFAULT 2,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)`,
	}
	for _, statement := range legacyDDL {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := db.Exec(
		"INSERT INTO wallet_address(token,status,description) VALUES (?,?,?),(?,?,?)",
		"TWalletOne", 1, "主钱包", "TWalletTwo", 1, "备用钱包",
	).Error; err != nil {
		t.Fatalf("seed wallets: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO orders(
			trade_id,order_id,actual_amount,amount,token,status,notify_url
		) VALUES (?,?,?,?,?,?,?)`,
		"trade-1", "order-1", 14.28, 100, "TWalletOne", 1, "https://merchant.test/notify",
	).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := migrateLegacySchema(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if err := db.AutoMigrate(&mdb.WalletAddress{}, &mdb.Orders{}); err != nil {
		t.Fatalf("auto migrate v1 schema: %v", err)
	}
	// A second run must be a no-op.
	if err := migrateLegacySchema(); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}

	var wallets []mdb.WalletAddress
	if err := db.Order("id").Find(&wallets).Error; err != nil {
		t.Fatalf("load wallets: %v", err)
	}
	if len(wallets) != 2 {
		t.Fatalf("wallet count = %d, want 2", len(wallets))
	}
	if wallets[0].Address != "TWalletOne" ||
		wallets[0].Network != mdb.NetworkTron ||
		wallets[0].Remark != "主钱包" {
		t.Fatalf("wallet migration = %+v", wallets[0])
	}
	if db.Migrator().HasColumn("wallet_address", "token") {
		t.Fatal("legacy wallet_address.token column was not removed")
	}

	var order mdb.Orders
	if err := db.Where("trade_id = ?", "trade-1").First(&order).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if order.ReceiveAddress != "TWalletOne" ||
		order.Token != "USDT" ||
		order.Network != mdb.NetworkTron ||
		order.Currency != "CNY" {
		t.Fatalf("order migration = %+v", order)
	}
}
