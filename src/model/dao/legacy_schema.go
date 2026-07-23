package dao

import (
	"fmt"

	"github.com/GMWalletApp/epusdt/model/mdb"
	"gorm.io/gorm"
)

// migrateLegacySchema upgrades the v0.0.x tables before AutoMigrate creates
// indexes for the v1 schema. In particular, every legacy wallet initially has
// an empty (network,address) pair; creating the v1 unique index before copying
// token -> address would therefore fail as soon as more than one wallet exists.
//
// The migration is deliberately idempotent. It only rewrites a table when the
// legacy column layout is detected, so normal v1 startups are read-only here.
func migrateLegacySchema() error {
	if Mdb == nil {
		return fmt.Errorf("primary database is not initialized")
	}
	if err := migrateLegacyWalletAddresses(Mdb); err != nil {
		return fmt.Errorf("migrate legacy wallet_address: %w", err)
	}
	if err := migrateLegacyOrders(Mdb); err != nil {
		return fmt.Errorf("migrate legacy orders: %w", err)
	}
	return nil
}

func migrateLegacyWalletAddresses(db *gorm.DB) error {
	migrator := db.Migrator()
	model := &mdb.WalletAddress{}
	if !migrator.HasTable(model) ||
		!migrator.HasColumn(model, "token") ||
		migrator.HasColumn(model, "address") {
		return nil
	}

	for _, field := range []string{"Network", "Address", "Remark", "Source"} {
		if !migrator.HasColumn(model, field) {
			if err := migrator.AddColumn(model, field); err != nil {
				return err
			}
		}
	}

	if err := db.Exec(
		"UPDATE wallet_address SET address = token, network = ?, source = ? WHERE address IS NULL OR address = ''",
		mdb.NetworkTron,
		mdb.WalletSourceManual,
	).Error; err != nil {
		return err
	}
	if migrator.HasColumn(model, "description") {
		if err := db.Exec(
			"UPDATE wallet_address SET remark = COALESCE(description, '') WHERE remark IS NULL OR remark = ''",
		).Error; err != nil {
			return err
		}
		if err := migrator.DropColumn(model, "description"); err != nil {
			return err
		}
	}

	// The old token column is NOT NULL. Keeping it would make every wallet
	// created by the v1 admin UI fail because v1 writes address instead.
	return migrator.DropColumn(model, "token")
}

func migrateLegacyOrders(db *gorm.DB) error {
	migrator := db.Migrator()
	model := &mdb.Orders{}
	if !migrator.HasTable(model) ||
		!migrator.HasColumn(model, "token") ||
		migrator.HasColumn(model, "receive_address") {
		return nil
	}

	for _, field := range []string{
		"ReceiveAddress",
		"Currency",
		"Network",
		"Name",
		"PaymentType",
		"PayProvider",
	} {
		if !migrator.HasColumn(model, field) {
			if err := migrator.AddColumn(model, field); err != nil {
				return err
			}
		}
	}

	// In v0.0.x orders.token stored the TRON receiving address. In v1 it stores
	// the asset symbol, while receive_address holds the wallet address.
	return db.Exec(
		`UPDATE orders
		 SET receive_address = token,
		     token = 'USDT',
		     network = ?,
		     currency = 'CNY',
		     payment_type = ?,
		     pay_provider = ?
		 WHERE receive_address IS NULL OR receive_address = ''`,
		mdb.NetworkTron,
		mdb.PaymentTypeGmpay,
		mdb.PaymentProviderOnChain,
	).Error
}
