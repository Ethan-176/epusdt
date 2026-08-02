package dao

import (
	"fmt"
	"strings"

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
	table := model.TableName()
	if !migrator.HasTable(table) {
		return nil
	}
	hasToken, err := hasPhysicalColumn(db, table, "token")
	if err != nil {
		return err
	}
	hasAddress, err := hasPhysicalColumn(db, table, "address")
	if err != nil {
		return err
	}
	if !hasToken || hasAddress {
		return nil
	}

	for _, field := range []struct{ model, column string }{
		{"Network", "network"},
		{"Address", "address"},
		{"Remark", "remark"},
		{"Source", "source"},
	} {
		hasColumn, err := hasPhysicalColumn(db, table, field.column)
		if err != nil {
			return err
		}
		if !hasColumn {
			if err := migrator.AddColumn(model, field.model); err != nil {
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
	hasDescription, err := hasPhysicalColumn(db, table, "description")
	if err != nil {
		return err
	}
	if hasDescription {
		if err := db.Exec(
			"UPDATE wallet_address SET remark = COALESCE(description, '') WHERE remark IS NULL OR remark = ''",
		).Error; err != nil {
			return err
		}
	}
	if db.Dialector.Name() == "sqlite" {
		return rebuildLegacySQLiteWalletAddresses(db)
	}
	if hasDescription {
		if err := db.Exec("ALTER TABLE wallet_address DROP COLUMN description").Error; err != nil {
			return err
		}
	}

	// The old token column is NOT NULL. Keeping it would make every wallet
	// created by the v1 admin UI fail because v1 writes address instead.
	return db.Exec("ALTER TABLE wallet_address DROP COLUMN token").Error
}

func rebuildLegacySQLiteWalletAddresses(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE wallet_address RENAME TO wallet_address_legacy_migration").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&mdb.WalletAddress{}); err != nil {
			return err
		}
		const columns = `id, network, address, status, remark, source, created_at, updated_at, deleted_at`
		if err := tx.Exec(
			"INSERT INTO wallet_address (" + columns + ") SELECT " + columns + " FROM wallet_address_legacy_migration",
		).Error; err != nil {
			return err
		}
		return tx.Exec("DROP TABLE wallet_address_legacy_migration").Error
	})
}

func migrateLegacyOrders(db *gorm.DB) error {
	migrator := db.Migrator()
	model := &mdb.Orders{}
	table := model.TableName()
	if !migrator.HasTable(table) {
		return nil
	}
	hasToken, err := hasPhysicalColumn(db, table, "token")
	if err != nil {
		return err
	}
	hasReceiveAddress, err := hasPhysicalColumn(db, table, "receive_address")
	if err != nil {
		return err
	}
	if !hasToken || hasReceiveAddress {
		return nil
	}

	for _, field := range []struct{ model, column string }{
		{"ReceiveAddress", "receive_address"},
		{"Currency", "currency"},
		{"Network", "network"},
		{"Name", "name"},
		{"PaymentType", "payment_type"},
		{"PayProvider", "pay_provider"},
	} {
		hasColumn, err := hasPhysicalColumn(db, table, field.column)
		if err != nil {
			return err
		}
		if !hasColumn {
			if err := migrator.AddColumn(model, field.model); err != nil {
				return err
			}
		}
	}

	// In v0.0.x orders.token stored the TRON receiving address. In v1 it stores
	// the asset symbol, while receive_address holds the wallet address.
	if err := db.Exec(
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
	).Error; err != nil {
		return err
	}

	if db.Dialector.Name() == "sqlite" {
		return rebuildLegacySQLiteOrders(db)
	}
	return nil
}

// GORM's SQLite table-rebuild parser can omit legacy NOT NULL columns when
// AutoMigrate changes their constraints. Rebuild the migrated table explicitly
// so old order identifiers and receiving addresses cannot be lost on upgrade.
func rebuildLegacySQLiteOrders(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE orders RENAME TO orders_legacy_migration").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&mdb.Orders{}); err != nil {
			return err
		}
		const columns = `id, trade_id, order_id, block_transaction_id, actual_amount,
			amount, token, status, notify_url, redirect_url, callback_num,
			callback_confirm, created_at, updated_at, deleted_at, receive_address,
			currency, network, name, payment_type, pay_provider`
		if err := tx.Exec(
			"INSERT INTO orders (" + columns + ") SELECT " + columns + " FROM orders_legacy_migration",
		).Error; err != nil {
			return err
		}
		return tx.Exec("DROP TABLE orders_legacy_migration").Error
	})
}

// SQLite's HasColumn implementation performs a LIKE against the entire CREATE
// TABLE statement. A column such as "address" can therefore falsely match the
// table name "wallet_address". ColumnTypes reports the physical columns and is
// consistent across the supported database drivers.
func hasPhysicalColumn(db *gorm.DB, table, name string) (bool, error) {
	columns, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		return false, err
	}
	for _, column := range columns {
		if strings.EqualFold(column.Name(), name) {
			return true, nil
		}
	}
	return false, nil
}
