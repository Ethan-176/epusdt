package data

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	adminLoginFailureWindow = 30 * time.Minute
	adminChallengeTTL       = 5 * time.Minute
)

var ErrAdminAuthChallengeInvalid = errors.New("admin authentication challenge is invalid or expired")

func hashAdminAuthValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func AdminLoginThrottleKeys(username, clientIP string) []string {
	username = strings.ToLower(strings.TrimSpace(username))
	clientIP = strings.TrimSpace(clientIP)
	keys := []string{"account:" + hashAdminAuthValue(username)}
	if clientIP != "" {
		keys = append(keys, "ip:"+hashAdminAuthValue(clientIP))
	}
	return keys
}

// AdminLoginLockedUntil returns the longest active account/IP lock.
func AdminLoginLockedUntil(keys []string, now time.Time) (time.Time, error) {
	var rows []mdb.AdminLoginThrottle
	if err := dao.Mdb.Where("key_hash IN ? AND locked_until > ?", keys, now).Find(&rows).Error; err != nil {
		return time.Time{}, err
	}
	var until time.Time
	for _, row := range rows {
		if row.LockedUntil.After(until) {
			until = row.LockedUntil
		}
	}
	return until, nil
}

func adminLockDuration(failures int) time.Duration {
	switch {
	case failures >= 15:
		return 24 * time.Hour
	case failures >= 10:
		return 2 * time.Hour
	case failures >= 5:
		return 15 * time.Minute
	default:
		return 0
	}
}

// RecordAdminLoginFailure increments persistent account and IP counters.
func RecordAdminLoginFailure(keys []string, now time.Time) (time.Time, error) {
	var longest time.Time
	for _, key := range keys {
		scope := "account"
		if strings.HasPrefix(key, "ip:") {
			scope = "ip"
		}
		err := dao.Mdb.Transaction(func(tx *gorm.DB) error {
			var row mdb.AdminLoginThrottle
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key_hash = ?", key).Limit(1).Find(&row).Error
			if err == nil && row.ID == 0 {
				row = mdb.AdminLoginThrottle{KeyHash: key, Scope: scope, FailureCount: 1, WindowStartedAt: now, LastFailedAt: now}
				return tx.Create(&row).Error
			}
			if err != nil {
				return err
			}
			if now.Sub(row.WindowStartedAt) > adminLoginFailureWindow {
				row.FailureCount = 0
				row.WindowStartedAt = now
			}
			row.FailureCount++
			row.LastFailedAt = now
			if duration := adminLockDuration(row.FailureCount); duration > 0 {
				candidate := now.Add(duration)
				if candidate.After(row.LockedUntil) {
					row.LockedUntil = candidate
				}
			}
			if row.LockedUntil.After(longest) {
				longest = row.LockedUntil
			}
			return tx.Save(&row).Error
		})
		if err != nil {
			return time.Time{}, err
		}
	}
	return longest, nil
}

func ClearAdminLoginFailures(keys []string) error {
	return dao.Mdb.Unscoped().Where("key_hash IN ?", keys).Delete(&mdb.AdminLoginThrottle{}).Error
}

func randomOpaqueToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func CreateAdminAuthChallenge(userID uint64, purpose, payload, clientIP string) (string, error) {
	token, err := randomOpaqueToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	row := &mdb.AdminAuthChallenge{
		AdminUserID: userID, Purpose: purpose, ChallengeIDHash: hashAdminAuthValue(token),
		Payload: payload, ClientIPHash: hashAdminAuthValue(clientIP), ExpiresAt: now.Add(adminChallengeTTL),
	}
	if err := dao.Mdb.Create(row).Error; err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeAdminAuthChallenge atomically marks a challenge consumed before its
// sensitive payload is used, preventing replay even when validation fails.
func ConsumeAdminAuthChallenge(token, purpose, clientIP string, userID uint64) (*mdb.AdminAuthChallenge, error) {
	var row mdb.AdminAuthChallenge
	now := time.Now().UTC()
	err := dao.Mdb.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("challenge_id_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", hashAdminAuthValue(token), purpose, now)
		if userID != 0 {
			q = q.Where("admin_user_id = ?", userID)
		}
		if err := q.Take(&row).Error; err != nil {
			return err
		}
		if row.ClientIPHash != hashAdminAuthValue(clientIP) {
			return ErrAdminAuthChallengeInvalid
		}
		result := tx.Model(&mdb.AdminAuthChallenge{}).Where("id = ? AND consumed_at IS NULL", row.ID).Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAdminAuthChallengeInvalid
		}
		row.ConsumedAt = &now
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdminAuthChallengeInvalid
		}
		return nil, err
	}
	return &row, nil
}

func ListAdminPasskeys(userID uint64) ([]mdb.AdminPasskey, error) {
	var rows []mdb.AdminPasskey
	err := dao.Mdb.Where("admin_user_id = ?", userID).Order("id ASC").Find(&rows).Error
	return rows, err
}

func AdminWebAuthnCredentials(userID uint64) ([]webauthn.Credential, error) {
	rows, err := ListAdminPasskeys(userID)
	if err != nil {
		return nil, err
	}
	out := make([]webauthn.Credential, 0, len(rows))
	for _, row := range rows {
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(row.CredentialJSON), &credential); err != nil {
			return nil, fmt.Errorf("decode passkey %d: %w", row.ID, err)
		}
		out = append(out, credential)
	}
	return out, nil
}

func SaveAdminPasskey(userID uint64, name string, credential *webauthn.Credential) error {
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	row := mdb.AdminPasskey{
		AdminUserID: userID, Name: strings.TrimSpace(name), CredentialID: credentialID,
		CredentialIDHash: hashAdminAuthValue(credentialID), CredentialJSON: string(encoded),
	}
	return dao.Mdb.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "credential_id_hash"}}, DoUpdates: clause.AssignmentColumns([]string{"credential_id", "credential_json", "last_used_at", "updated_at"})}).Create(&row).Error
}

func TouchAdminPasskey(userID uint64, credential *webauthn.Credential) error {
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	result := dao.Mdb.Model(&mdb.AdminPasskey{}).
		Where("admin_user_id = ? AND credential_id_hash = ?", userID, hashAdminAuthValue(credentialID)).
		Updates(map[string]interface{}{"credential_json": string(encoded), "last_used_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteAdminPasskey(userID, passkeyID uint64) error {
	result := dao.Mdb.Where("id = ? AND admin_user_id = ?", passkeyID, userID).Delete(&mdb.AdminPasskey{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func IncrementAdminAuthVersion(userID uint64) error {
	return dao.Mdb.Model(&mdb.AdminUser{}).Where("id = ?", userID).UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error
}
