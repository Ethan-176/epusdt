package addressutil

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
)

const (
	NetworkTron     = "tron"
	NetworkSolana   = "solana"
	NetworkEthereum = "ethereum"
	NetworkBSC      = "binance"
	NetworkPolygon  = "polygon"
	NetworkPlasma   = "plasma"
	NetworkTON      = "ton"
	NetworkAptos    = "aptos"
)

// NormalizeWalletAddress returns the canonical form used for comparisons and
// storage. It also rejects unsupported networks and malformed addresses.
func NormalizeWalletAddress(network, input string) (string, error) {
	network = strings.ToLower(strings.TrimSpace(network))
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("wallet address is empty")
	}

	switch network {
	case NetworkTron:
		if !validTronAddress(input) {
			return "", fmt.Errorf("invalid TRON mainnet address")
		}
		return input, nil
	case NetworkSolana:
		if _, err := solana.PublicKeyFromBase58(input); err != nil {
			return "", fmt.Errorf("invalid Solana address")
		}
		return input, nil
	case NetworkEthereum, NetworkBSC, NetworkPolygon, NetworkPlasma:
		if !common.IsHexAddress(input) {
			return "", fmt.Errorf("invalid EVM address")
		}
		return strings.ToLower(input), nil
	case NetworkTON:
		return NormalizeTonAddress(input)
	case NetworkAptos:
		return NormalizeMoveAddress(input)
	default:
		return "", fmt.Errorf("unsupported wallet network %q", network)
	}
}

func validTronAddress(input string) bool {
	if len(input) < 26 || len(input) > 35 || input[0] != 'T' {
		return false
	}
	decoded := base58.Decode(input)
	if len(decoded) != 25 || decoded[0] != 0x41 {
		return false
	}
	first := sha256.Sum256(decoded[:21])
	second := sha256.Sum256(first[:])
	return string(decoded[21:]) == string(second[:4])
}
