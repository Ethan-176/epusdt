package config

import (
	"fmt"
	"strings"

	addressutil "github.com/GMWalletApp/epusdt/util/address"
	"github.com/spf13/viper"
)

const paymentWalletAllowlistKey = "payment_wallet_allowlist"

// PaymentWalletAllowlist parses payment_wallet_allowlist entries formatted as
// network:address and separated by commas or semicolons. When configured, it
// is fail-closed: networks and addresses absent from the list are not allowed.
func PaymentWalletAllowlist() (map[string]map[string]struct{}, bool, error) {
	raw := strings.TrimSpace(viper.GetString(paymentWalletAllowlistKey))
	if raw == "" {
		return nil, false, nil
	}

	allowed := make(map[string]map[string]struct{})
	entries := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		network, address, ok := strings.Cut(entry, ":")
		if !ok || strings.TrimSpace(network) == "" || strings.TrimSpace(address) == "" {
			return nil, true, fmt.Errorf("invalid %s entry %q; expected network:address", paymentWalletAllowlistKey, entry)
		}
		network = strings.ToLower(strings.TrimSpace(network))
		normalized, err := addressutil.NormalizeWalletAddress(network, address)
		if err != nil {
			return nil, true, fmt.Errorf("invalid %s entry %q: %w", paymentWalletAllowlistKey, entry, err)
		}
		if allowed[network] == nil {
			allowed[network] = make(map[string]struct{})
		}
		allowed[network][normalized] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, true, fmt.Errorf("%s is configured but contains no addresses", paymentWalletAllowlistKey)
	}
	return allowed, true, nil
}

// IsPaymentWalletAllowed returns true for every address when the allowlist is
// absent, preserving existing installations. Once configured, malformed
// configuration and missing entries fail closed.
func IsPaymentWalletAllowed(network, address string) bool {
	allowed, configured, err := PaymentWalletAllowlist()
	if !configured {
		return true
	}
	if err != nil {
		return false
	}
	network = strings.ToLower(strings.TrimSpace(network))
	normalized, err := addressutil.NormalizeWalletAddress(network, address)
	if err != nil {
		return false
	}
	_, ok := allowed[network][normalized]
	return ok
}

func TelegramWalletManagementEnabled() bool {
	return viper.GetBool("telegram_wallet_management_enabled")
}
