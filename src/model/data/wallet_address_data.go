package data

import (
	"strings"

	"github.com/GMWalletApp/epusdt/config"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	addressutil "github.com/GMWalletApp/epusdt/util/address"
	"github.com/GMWalletApp/epusdt/util/constant"
)

// AddWalletAddress 创建钱包 (默认 tron 网络，用于 Telegram 添加)
func AddWalletAddress(address string) (*mdb.WalletAddress, error) {
	return AddWalletAddressWithNetwork(mdb.NetworkTron, address)
}

// isEVMNetwork 判断是否是 EVM 网络
func isEVMNetwork(network string) bool {
	switch network {
	case mdb.NetworkEthereum, mdb.NetworkBsc, mdb.NetworkPolygon, mdb.NetworkPlasma:
		return true
	}
	return false
}

func normalizeWalletNetwork(network string) string {
	return strings.ToLower(strings.TrimSpace(network))
}

func normalizeWalletAddressByNetwork(network, address string) string {
	address = strings.TrimSpace(address)
	if isEVMNetwork(normalizeWalletNetwork(network)) {
		return strings.ToLower(address)
	}
	return address
}

func normalizeWalletAddressByNetworkE(network, address string) (string, error) {
	network = normalizeWalletNetwork(network)
	address = strings.TrimSpace(address)
	if network == mdb.NetworkTon {
		return addressutil.NormalizeTonAddress(address)
	}
	if network == mdb.NetworkAptos {
		return addressutil.NormalizeMoveAddress(address)
	}
	return normalizeWalletAddressByNetwork(network, address), nil
}

// AddWalletAddressWithNetwork 创建指定网络的钱包地址
func AddWalletAddressWithNetwork(network, address string) (*mdb.WalletAddress, error) {
	network = normalizeWalletNetwork(network)
	var err error
	address, err = normalizeWalletAddressByNetworkE(network, address)
	if err != nil {
		return nil, err
	}
	if !config.IsPaymentWalletAllowed(network, address) {
		return nil, constant.WalletAddressNotAllowedErr
	}

	exist, err := GetWalletAddressByNetworkAndAddress(network, address)
	if err != nil {
		return nil, err
	}
	if exist.ID > 0 {
		return nil, constant.WalletAddressAlreadyExists
	}

	// Check for a soft-deleted record with the same (network, address) and restore it.
	deleted := new(mdb.WalletAddress)
	err = dao.Mdb.Unscoped().
		Where("network = ? AND address = ? AND deleted_at IS NOT NULL", network, address).
		Limit(1).Find(deleted).Error
	if err != nil {
		return nil, err
	}
	if deleted.ID > 0 {
		err = dao.Mdb.Unscoped().Model(deleted).Updates(map[string]interface{}{
			"deleted_at": nil,
			"status":     mdb.TokenStatusEnable,
		}).Error
		return deleted, err
	}

	walletAddress := &mdb.WalletAddress{
		Network: network,
		Address: address,
		Status:  mdb.TokenStatusEnable,
	}
	err = dao.Mdb.Create(walletAddress).Error
	return walletAddress, err
}

// GetWalletAddressByNetworkAndAddress 通过网络和地址查询
func GetWalletAddressByNetworkAndAddress(network, address string) (*mdb.WalletAddress, error) {
	network = normalizeWalletNetwork(network)
	var err error
	address, err = normalizeWalletAddressByNetworkE(network, address)
	if err != nil {
		return nil, err
	}
	walletAddress := new(mdb.WalletAddress)
	err = dao.Mdb.Model(walletAddress).
		Where("network = ?", network).
		Where("address = ?", address).
		Limit(1).Find(walletAddress).Error
	return walletAddress, err
}

// GetWalletAddressByToken 通过钱包地址获取address (兼容旧接口)
func GetWalletAddressByToken(address string) (*mdb.WalletAddress, error) {
	walletAddress := new(mdb.WalletAddress)
	err := dao.Mdb.Model(walletAddress).Limit(1).Find(walletAddress, "address = ?", address).Error
	return walletAddress, err
}

// GetWalletAddressById 通过id获取钱包
func GetWalletAddressById(id uint64) (*mdb.WalletAddress, error) {
	walletAddress := new(mdb.WalletAddress)
	err := dao.Mdb.Model(walletAddress).Limit(1).Find(walletAddress, id).Error
	return walletAddress, err
}

// DeleteWalletAddressById 通过id删除钱包
func DeleteWalletAddressById(id uint64) error {
	err := dao.Mdb.Where("id = ?", id).Delete(&mdb.WalletAddress{}).Error
	return err
}

// GetAvailableWalletAddress 获得所有可用的钱包地址
func GetAvailableWalletAddress() ([]mdb.WalletAddress, error) {
	var WalletAddressList []mdb.WalletAddress
	err := dao.Mdb.Model(WalletAddressList).Where("status = ?", mdb.TokenStatusEnable).Find(&WalletAddressList).Error
	if err != nil {
		return nil, err
	}
	return filterPaymentWalletAllowlist(WalletAddressList), nil
}

// GetAvailableWalletAddressByNetwork 获得指定网络的所有可用钱包地址
func GetAvailableWalletAddressByNetwork(network string) ([]mdb.WalletAddress, error) {
	network = normalizeWalletNetwork(network)
	var list []mdb.WalletAddress
	err := dao.Mdb.Model(list).
		Where("status = ?", mdb.TokenStatusEnable).
		Where("network = ?", network).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	if isEVMNetwork(network) {
		for i := range list {
			list[i].Address = strings.ToLower(strings.TrimSpace(list[i].Address))
		}
	} else if network == mdb.NetworkTon {
		for i := range list {
			if normalized, err := addressutil.NormalizeTonAddress(list[i].Address); err == nil {
				list[i].Address = normalized
			}
		}
	}
	return filterPaymentWalletAllowlist(list), nil
}

// GetSelectableWalletAddresses returns the enabled, allowlisted wallet rows
// eligible for order allocation. When walletID is non-zero, the result is
// pinned to that exact admin-managed row instead of falling back to another
// address. This keeps the merchant selector from becoming a way to submit an
// arbitrary receive address.
func GetSelectableWalletAddresses(network string, walletID uint64) ([]mdb.WalletAddress, error) {
	network = normalizeWalletNetwork(network)
	if walletID == 0 {
		return GetAvailableWalletAddressByNetwork(network)
	}

	wallet, err := GetWalletAddressById(walletID)
	if err != nil {
		return nil, err
	}
	if wallet.ID == 0 {
		return nil, constant.WalletNotFoundErr
	}
	if normalizeWalletNetwork(wallet.Network) != network || wallet.Status != mdb.TokenStatusEnable {
		return nil, constant.WalletSelectionUnavailableErr
	}

	normalizedAddress, err := normalizeWalletAddressByNetworkE(network, wallet.Address)
	if err != nil {
		return nil, constant.WalletAddressInvalidErr
	}
	if !config.IsPaymentWalletAllowed(network, normalizedAddress) {
		return nil, constant.WalletAddressNotAllowedErr
	}
	wallet.Network = network
	wallet.Address = normalizedAddress
	return []mdb.WalletAddress{*wallet}, nil
}

func filterPaymentWalletAllowlist(list []mdb.WalletAddress) []mdb.WalletAddress {
	filtered := make([]mdb.WalletAddress, 0, len(list))
	for _, wallet := range list {
		if config.IsPaymentWalletAllowed(wallet.Network, wallet.Address) {
			filtered = append(filtered, wallet)
		}
	}
	return filtered
}

// GetAllWalletAddress 获得所有钱包地址
func GetAllWalletAddress() ([]mdb.WalletAddress, error) {
	var WalletAddressList []mdb.WalletAddress
	err := dao.Mdb.Model(WalletAddressList).Find(&WalletAddressList).Error
	return WalletAddressList, err
}

// GetAllWalletAddressByNetwork 获得指定网络的所有钱包地址
func GetAllWalletAddressByNetwork(network string) ([]mdb.WalletAddress, error) {
	network = normalizeWalletNetwork(network)
	var list []mdb.WalletAddress
	err := dao.Mdb.Model(list).Where("network = ?", network).Find(&list).Error
	if err != nil {
		return nil, err
	}
	if isEVMNetwork(network) {
		for i := range list {
			list[i].Address = strings.ToLower(strings.TrimSpace(list[i].Address))
		}
	} else if network == mdb.NetworkTon {
		for i := range list {
			if normalized, err := addressutil.NormalizeTonAddress(list[i].Address); err == nil {
				list[i].Address = normalized
			}
		}
	}
	return list, err
}

// ChangeWalletAddressStatus 启用禁用钱包
func ChangeWalletAddressStatus(id uint64, status int) error {
	if status == mdb.TokenStatusEnable {
		wallet, err := GetWalletAddressById(id)
		if err != nil {
			return err
		}
		if wallet.ID == 0 {
			return constant.WalletNotFoundErr
		}
		if !config.IsPaymentWalletAllowed(wallet.Network, wallet.Address) {
			return constant.WalletAddressNotAllowedErr
		}
	}
	err := dao.Mdb.Model(&mdb.WalletAddress{}).Where("id = ?", id).Update("status", status).Error
	return err
}

// GetWalletAllowlistViolations reports database rows that can never receive a
// new order under the deployment allowlist. Rows remain visible in the admin
// UI for investigation, but selection functions filter them out.
func GetWalletAllowlistViolations() ([]mdb.WalletAddress, error) {
	_, configured, err := config.PaymentWalletAllowlist()
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, nil
	}
	rows, err := GetAllWalletAddress()
	if err != nil {
		return nil, err
	}
	violations := make([]mdb.WalletAddress, 0)
	for _, wallet := range rows {
		if !config.IsPaymentWalletAllowed(wallet.Network, wallet.Address) {
			violations = append(violations, wallet)
		}
	}
	return violations, nil
}
