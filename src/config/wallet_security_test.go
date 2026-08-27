package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestPaymentWalletAllowlistIsFailClosed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	allowed := "0x1111111111111111111111111111111111111111"
	viper.Set(paymentWalletAllowlistKey, "ethereum:"+allowed)

	if !IsPaymentWalletAllowed("Ethereum", allowed) {
		t.Fatal("configured wallet should be allowed")
	}
	if IsPaymentWalletAllowed("ethereum", "0x2222222222222222222222222222222222222222") {
		t.Fatal("unlisted wallet should be denied")
	}
	if IsPaymentWalletAllowed("tron", "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t") {
		t.Fatal("unlisted network should be denied")
	}
}

func TestPaymentWalletAllowlistRejectsMalformedEntries(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(paymentWalletAllowlistKey, "ethereum:not-an-address")

	if _, configured, err := PaymentWalletAllowlist(); !configured || err == nil {
		t.Fatalf("configured=%v err=%v, want configured malformed error", configured, err)
	}
	if IsPaymentWalletAllowed("ethereum", "0x1111111111111111111111111111111111111111") {
		t.Fatal("malformed allowlist must fail closed")
	}
}

func TestPaymentWalletAllowlistAbsentPreservesExistingBehavior(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	if !IsPaymentWalletAllowed("tron", "not-validated-without-allowlist") {
		t.Fatal("absent allowlist should preserve existing installations")
	}
}
