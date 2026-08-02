package data

import (
	"strings"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/dromara/carbon/v2"
	"gorm.io/gorm"
)

const defaultAdminUsername = "admin"

// EnsureDefaultAdmin seeds only the administrator identity. Authentication
// material is deliberately provisioned directly in admin_users.totp_secret by
// the operator; no password or recoverable bootstrap credential is generated.
func EnsureDefaultAdmin() (created bool, err error) {
	err = dao.Mdb.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&mdb.AdminUser{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Create(&mdb.AdminUser{
			Username: defaultAdminUsername,
			Status:   mdb.AdminUserStatusEnable,
		}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// GetAdminUserByUsername returns the row for a username (case-insensitive).
func GetAdminUserByUsername(username string) (*mdb.AdminUser, error) {
	u := new(mdb.AdminUser)
	err := dao.Mdb.Model(u).
		Where("username = ?", strings.ToLower(strings.TrimSpace(username))).
		Limit(1).Find(u).Error
	return u, err
}

// GetAdminUserByID returns the row for an ID.
func GetAdminUserByID(id uint64) (*mdb.AdminUser, error) {
	u := new(mdb.AdminUser)
	err := dao.Mdb.Model(u).Limit(1).Find(u, id).Error
	return u, err
}

// TouchAdminUserLastLogin stamps last_login_at to now.
func TouchAdminUserLastLogin(id uint64) error {
	return dao.Mdb.Model(&mdb.AdminUser{}).
		Where("id = ?", id).
		Update("last_login_at", carbon.Now().StdTime()).Error
}
