package data

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	"github.com/spf13/viper"
	"github.com/xssnick/tonutils-go/address"
)

func TestEvmTransactionLockAddressIsCaseInsensitive(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	tradeID := "trade-evm-case"
	address := "0xA1B2c3D4e5F60718293aBcDeF001122334455667"

	if err := LockTransaction("Ethereum", address, "usdt", tradeID, 1.23, time.Hour); err != nil {
		t.Fatalf("lock transaction: %v", err)
	}

	gotTradeID, err := GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkEthereum, strings.ToLower(address), "USDT", 1.23)
	if err != nil {
		t.Fatalf("lookup transaction lock: %v", err)
	}
	if gotTradeID != tradeID {
		t.Fatalf("trade id = %q, want %q", gotTradeID, tradeID)
	}

	if err := UnLockTransaction(mdb.NetworkEthereum, strings.ToUpper(address), "USDT", 1.23); err != nil {
		t.Fatalf("unlock transaction: %v", err)
	}

	gotTradeID, err = GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkEthereum, address, "USDT", 1.23)
	if err != nil {
		t.Fatalf("lookup after unlock: %v", err)
	}
	if gotTradeID != "" {
		t.Fatalf("expected lock to be released, got trade id %q", gotTradeID)
	}
}

func TestStatsBucketExprForDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		hourly  bool
		want    string
	}{
		{
			name:    "sqlite daily",
			dialect: "sqlite",
			want:    "substr(created_at, 1, 10)",
		},
		{
			name:    "sqlite hourly",
			dialect: "sqlite",
			hourly:  true,
			want:    "replace(substr(created_at, 1, 13), 'T', ' ') || ':00'",
		},
		{
			name:    "postgres daily",
			dialect: "postgres",
			want:    "TO_CHAR(created_at, 'YYYY-MM-DD')",
		},
		{
			name:    "postgres hourly",
			dialect: "postgres",
			hourly:  true,
			want:    "TO_CHAR(created_at, 'YYYY-MM-DD HH24:00')",
		},
		{
			name:    "mysql daily",
			dialect: "mysql",
			want:    "DATE_FORMAT(created_at, '%Y-%m-%d')",
		},
		{
			name:    "mysql hourly",
			dialect: "mysql",
			hourly:  true,
			want:    "DATE_FORMAT(created_at, '%Y-%m-%d %H:00')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := statsBucketExprForDialect(tt.dialect, "created_at", tt.hourly)
			if err != nil {
				t.Fatalf("statsBucketExprForDialect error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("bucket expr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatsBucketExprRejectsUnsupportedDialect(t *testing.T) {
	if _, err := statsBucketExprForDialect("unsupported", "created_at", false); err == nil {
		t.Fatal("expected unsupported dialect error")
	}
}

func TestTransactionLockPrecisionPreventsEquivalentAmountsOnly(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	if err := SetSetting(mdb.SettingGroupSystem, mdb.SettingKeyAmountPrecision, "2", mdb.SettingTypeInt); err != nil {
		t.Fatalf("set precision 2: %v", err)
	}
	if err := LockTransaction(mdb.NetworkTron, "TPrecisionAddress001", "USDT", "trade-old", 1.23, time.Hour); err != nil {
		t.Fatalf("lock old transaction: %v", err)
	}

	if err := SetSetting(mdb.SettingGroupSystem, mdb.SettingKeyAmountPrecision, "4", mdb.SettingTypeInt); err != nil {
		t.Fatalf("set precision 4: %v", err)
	}
	if err := LockTransaction(mdb.NetworkTron, "TPrecisionAddress001", "USDT", "trade-equivalent", 1.2300, time.Hour); !errors.Is(err, ErrTransactionLocked) {
		t.Fatalf("equivalent lock error = %v, want %v", err, ErrTransactionLocked)
	}
	if err := LockTransaction(mdb.NetworkTron, "TPrecisionAddress001", "USDT", "trade-new", 1.2301, time.Hour); err != nil {
		t.Fatalf("distinct precision lock: %v", err)
	}
}

func TestTransactionLockLookupUsesStoredPrecision(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	if err := SetSetting(mdb.SettingGroupSystem, mdb.SettingKeyAmountPrecision, "4", mdb.SettingTypeInt); err != nil {
		t.Fatalf("set precision 4: %v", err)
	}
	if err := LockTransaction(mdb.NetworkTron, "TPrecisionAddress002", "USDT", "trade-precise", 1.2345, time.Hour); err != nil {
		t.Fatalf("lock precise transaction: %v", err)
	}
	if err := SetSetting(mdb.SettingGroupSystem, mdb.SettingKeyAmountPrecision, "2", mdb.SettingTypeInt); err != nil {
		t.Fatalf("set precision 2: %v", err)
	}

	gotTradeID, err := GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkTron, "TPrecisionAddress002", "USDT", 1.2345)
	if err != nil {
		t.Fatalf("lookup transaction lock: %v", err)
	}
	if gotTradeID != "trade-precise" {
		t.Fatalf("trade id = %q, want trade-precise", gotTradeID)
	}
}

func TestNonEvmTransactionLockAddressRemainsCaseSensitive(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	tradeID := "trade-tron-case"
	address := "TCaseSensitiveAddress001"

	if err := LockTransaction(mdb.NetworkTron, address, "USDT", tradeID, 1.00, time.Hour); err != nil {
		t.Fatalf("lock transaction: %v", err)
	}

	gotTradeID, err := GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkTron, strings.ToLower(address), "USDT", 1.00)
	if err != nil {
		t.Fatalf("lookup transaction lock: %v", err)
	}
	if gotTradeID != "" {
		t.Fatalf("tron address lookup should remain case-sensitive, got trade id %q", gotTradeID)
	}
}

func TestTonTransactionLockAddressUsesRawKey(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	bounceable := "EQC6KV4zs8TJtSZapOrRFmqSkxzpq-oSCoxekQRKElf4nC1I"
	addr := address.MustParseAddr(bounceable)
	nonBounce := addr.Bounce(false).String()

	if err := LockTransaction(mdb.NetworkTon, bounceable, "TON", "trade-ton", 1.23, time.Hour); err != nil {
		t.Fatalf("lock ton transaction: %v", err)
	}
	gotTradeID, err := GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkTon, nonBounce, "TON", 1.23)
	if err != nil {
		t.Fatalf("lookup ton lock: %v", err)
	}
	if gotTradeID != "trade-ton" {
		t.Fatalf("ton lock lookup = %q, want trade-ton", gotTradeID)
	}
	gotTradeID, err = GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkTon, addr.StringRaw(), "TON", 1.23)
	if err != nil {
		t.Fatalf("lookup ton raw lock: %v", err)
	}
	if gotTradeID != "trade-ton" {
		t.Fatalf("ton raw lock lookup = %q, want trade-ton", gotTradeID)
	}
}

func TestAptosTransactionLockAddressUsesCanonicalKey(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	if err := LockTransaction(mdb.NetworkAptos, "0xA", "USDT", "trade-aptos", 1.23, time.Hour); err != nil {
		t.Fatalf("lock aptos transaction: %v", err)
	}
	gotTradeID, err := GetTradeIdByWalletAddressAndAmountAndToken(mdb.NetworkAptos, "a", "USDT", 1.23)
	if err != nil {
		t.Fatalf("lookup aptos lock: %v", err)
	}
	if gotTradeID != "trade-aptos" {
		t.Fatalf("aptos lock lookup = %q, want trade-aptos", gotTradeID)
	}
}

func TestGetOrderRejectsDatabaseAddressRewriteOutsideDeploymentAllowlist(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()
	viper.Reset()
	defer viper.Reset()

	allowed := "0x1111111111111111111111111111111111111111"
	blocked := "0x2222222222222222222222222222222222222222"
	viper.Set("payment_wallet_allowlist", "ethereum:"+allowed)

	order := &mdb.Orders{
		TradeId:        "allowlist-order-rewrite",
		OrderId:        "allowlist-merchant-order",
		Network:        mdb.NetworkEthereum,
		Token:          "USDT",
		ReceiveAddress: allowed,
		Status:         mdb.StatusWaitPay,
		PayProvider:    mdb.PaymentProviderOnChain,
	}
	if err := dao.Mdb.Create(order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := GetOrderInfoByTradeId(order.TradeId); err != nil {
		t.Fatalf("load allowlisted order: %v", err)
	}

	if err := dao.Mdb.Model(&mdb.Orders{}).Where("id = ?", order.ID).Update("receive_address", blocked).Error; err != nil {
		t.Fatalf("simulate database address rewrite: %v", err)
	}
	if _, err := GetOrderInfoByTradeId(order.TradeId); err != constant.WalletAddressNotAllowedErr {
		t.Fatalf("rewritten order error = %v, want deployment allowlist error", err)
	}
}

func TestOrderAllowlistAllowsWaitSelectAndProviderOrders(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	viper.Set("payment_wallet_allowlist", "ethereum:0x1111111111111111111111111111111111111111")

	waitSelect := &mdb.Orders{BaseModel: mdb.BaseModel{ID: 1}, Status: mdb.StatusWaitSelect, PayProvider: mdb.PaymentProviderOnChain}
	if err := ValidateOrderWalletAllowlist(waitSelect); err != nil {
		t.Fatalf("wait-select order rejected: %v", err)
	}
	provider := &mdb.Orders{BaseModel: mdb.BaseModel{ID: 2}, Status: mdb.StatusWaitPay, PayProvider: mdb.PaymentProviderOkPay, ReceiveAddress: "OKPAY"}
	if err := ValidateOrderWalletAllowlist(provider); err != nil {
		t.Fatalf("provider order rejected: %v", err)
	}
}
