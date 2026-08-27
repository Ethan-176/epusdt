package data

import (
	"strings"
	"testing"

	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	"github.com/spf13/viper"
	"github.com/xssnick/tonutils-go/address"
)

func TestAddWalletAddressWithNetworkNormalizesEvmAddressToLowercase(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	input := "0xA1B2c3D4e5F60718293aBcDeF001122334455667"
	row, err := AddWalletAddressWithNetwork(mdb.NetworkEthereum, input)
	if err != nil {
		t.Fatalf("add wallet: %v", err)
	}
	if row.Address != strings.ToLower(input) {
		t.Fatalf("wallet address = %q, want %q", row.Address, strings.ToLower(input))
	}

	loaded, err := GetWalletAddressByNetworkAndAddress(mdb.NetworkEthereum, strings.ToUpper(input))
	if err != nil {
		t.Fatalf("load wallet by mixed-case address: %v", err)
	}
	if loaded.ID == 0 {
		t.Fatal("expected to find wallet by mixed-case query")
	}
	if loaded.Address != strings.ToLower(input) {
		t.Fatalf("stored wallet address = %q, want lowercase", loaded.Address)
	}
}

func TestGetAvailableWalletAddressByNetworkReturnsLowercaseForEvm(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	mixed := "0xA1B2c3D4e5F60718293aBcDeF001122334455667"
	if err := dao.Mdb.Create(&mdb.WalletAddress{
		Network: mdb.NetworkEthereum,
		Address: mixed,
		Status:  mdb.TokenStatusEnable,
	}).Error; err != nil {
		t.Fatalf("seed mixed-case wallet: %v", err)
	}

	rows, err := GetAvailableWalletAddressByNetwork("Ethereum")
	if err != nil {
		t.Fatalf("list wallets: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("wallet count = %d, want 1", len(rows))
	}
	if rows[0].Address != strings.ToLower(mixed) {
		t.Fatalf("listed wallet address = %q, want %q", rows[0].Address, strings.ToLower(mixed))
	}
}

func TestAddWalletAddressWithNetworkKeepsOriginalCaseForNonEvm(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	tronAddress := "TCaseSensitiveTronAddress001"
	tronRow, err := AddWalletAddressWithNetwork(mdb.NetworkTron, tronAddress)
	if err != nil {
		t.Fatalf("add tron wallet: %v", err)
	}
	if tronRow.Address != tronAddress {
		t.Fatalf("tron wallet address = %q, want %q", tronRow.Address, tronAddress)
	}

	solAddress := "SoLAnACaseSensitiveAddress111111111111111111"
	solRow, err := AddWalletAddressWithNetwork(mdb.NetworkSolana, solAddress)
	if err != nil {
		t.Fatalf("add solana wallet: %v", err)
	}
	if solRow.Address != solAddress {
		t.Fatalf("solana wallet address = %q, want %q", solRow.Address, solAddress)
	}
}

func TestAddWalletAddressWithNetworkNormalizesTonAddressVariants(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	bounceable := "EQC6KV4zs8TJtSZapOrRFmqSkxzpq-oSCoxekQRKElf4nC1I"
	addr := address.MustParseAddr(bounceable)
	expected := addr.Bounce(false).String()

	row, err := AddWalletAddressWithNetwork(mdb.NetworkTon, addr.StringRaw())
	if err != nil {
		t.Fatalf("add raw ton wallet: %v", err)
	}
	if row.Address != expected {
		t.Fatalf("stored TON address = %q, want %q", row.Address, expected)
	}
	if _, err = AddWalletAddressWithNetwork(mdb.NetworkTon, bounceable); err != constant.WalletAddressAlreadyExists {
		t.Fatalf("add equivalent ton wallet error = %v, want already exists", err)
	}
}

func TestAddWalletAddressWithNetworkNormalizesMoveAddressVariants(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	want := "0x000000000000000000000000000000000000000000000000000000000000000a"
	row, err := AddWalletAddressWithNetwork(mdb.NetworkAptos, " A ")
	if err != nil {
		t.Fatalf("add aptos wallet: %v", err)
	}
	if row.Address != want {
		t.Fatalf("stored Aptos address = %q, want %q", row.Address, want)
	}
	if _, err = AddWalletAddressWithNetwork(mdb.NetworkAptos, "0x0A"); err != constant.WalletAddressAlreadyExists {
		t.Fatalf("add equivalent aptos wallet error = %v, want already exists", err)
	}
}

func TestAvailableWalletsFilterDeploymentAllowlist(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()
	viper.Reset()
	defer viper.Reset()

	allowed := "0x1111111111111111111111111111111111111111"
	blocked := "0x2222222222222222222222222222222222222222"
	viper.Set("payment_wallet_allowlist", "ethereum:"+allowed)

	for _, addr := range []string{allowed, blocked} {
		if err := dao.Mdb.Create(&mdb.WalletAddress{
			Network: mdb.NetworkEthereum,
			Address: addr,
			Status:  mdb.TokenStatusEnable,
		}).Error; err != nil {
			t.Fatalf("seed wallet %s: %v", addr, err)
		}
	}

	rows, err := GetAvailableWalletAddressByNetwork(mdb.NetworkEthereum)
	if err != nil {
		t.Fatalf("get available wallets: %v", err)
	}
	if len(rows) != 1 || rows[0].Address != allowed {
		t.Fatalf("available wallets = %#v, want only allowlisted address", rows)
	}

	violations, err := GetWalletAllowlistViolations()
	if err != nil {
		t.Fatalf("get allowlist violations: %v", err)
	}
	if len(violations) != 1 || violations[0].Address != blocked {
		t.Fatalf("violations = %#v, want blocked address", violations)
	}
}

func TestAddAndEnableWalletRejectDeploymentAllowlistViolation(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()
	viper.Reset()
	defer viper.Reset()

	allowed := "0x1111111111111111111111111111111111111111"
	blocked := "0x2222222222222222222222222222222222222222"
	viper.Set("payment_wallet_allowlist", "ethereum:"+allowed)

	if _, err := AddWalletAddressWithNetwork(mdb.NetworkEthereum, blocked); err != constant.WalletAddressNotAllowedErr {
		t.Fatalf("add blocked wallet error = %v, want allowlist error", err)
	}
	row := &mdb.WalletAddress{Network: mdb.NetworkEthereum, Address: blocked, Status: mdb.TokenStatusDisable}
	if err := dao.Mdb.Create(row).Error; err != nil {
		t.Fatalf("seed blocked wallet: %v", err)
	}
	if err := ChangeWalletAddressStatus(row.ID, mdb.TokenStatusEnable); err != constant.WalletAddressNotAllowedErr {
		t.Fatalf("enable blocked wallet error = %v, want allowlist error", err)
	}
}

func TestGetSelectableWalletAddressesPinsEnabledAllowlistedWallet(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	first, err := AddWalletAddressWithNetwork(mdb.NetworkTron, "TSelectableWalletAddress001")
	if err != nil {
		t.Fatalf("add first wallet: %v", err)
	}
	second, err := AddWalletAddressWithNetwork(mdb.NetworkTron, "TSelectableWalletAddress002")
	if err != nil {
		t.Fatalf("add second wallet: %v", err)
	}

	rows, err := GetSelectableWalletAddresses(mdb.NetworkTron, second.ID)
	if err != nil {
		t.Fatalf("select wallet: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != second.ID || rows[0].ID == first.ID {
		t.Fatalf("selected wallets = %#v, want only id=%d", rows, second.ID)
	}
}

func TestGetSelectableWalletAddressesRejectsDisabledWrongNetworkAndBlockedWallet(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	disabled := &mdb.WalletAddress{
		Network: mdb.NetworkTron,
		Address: "TDisabledSelectableWallet001",
		Status:  mdb.TokenStatusDisable,
	}
	if err := dao.Mdb.Create(disabled).Error; err != nil {
		t.Fatalf("seed disabled wallet: %v", err)
	}
	if _, err := GetSelectableWalletAddresses(mdb.NetworkTron, disabled.ID); err != constant.WalletSelectionUnavailableErr {
		t.Fatalf("disabled selection error = %v, want %v", err, constant.WalletSelectionUnavailableErr)
	}
	if _, err := GetSelectableWalletAddresses(mdb.NetworkEthereum, disabled.ID); err != constant.WalletSelectionUnavailableErr {
		t.Fatalf("wrong-network selection error = %v, want %v", err, constant.WalletSelectionUnavailableErr)
	}

	blocked := &mdb.WalletAddress{
		Network: mdb.NetworkEthereum,
		Address: "0x2222222222222222222222222222222222222222",
		Status:  mdb.TokenStatusEnable,
	}
	if err := dao.Mdb.Create(blocked).Error; err != nil {
		t.Fatalf("seed blocked wallet: %v", err)
	}
	viper.Set("payment_wallet_allowlist", "ethereum:0x1111111111111111111111111111111111111111")
	if _, err := GetSelectableWalletAddresses(mdb.NetworkEthereum, blocked.ID); err != constant.WalletAddressNotAllowedErr {
		t.Fatalf("blocked selection error = %v, want %v", err, constant.WalletAddressNotAllowedErr)
	}
}
