package mdb

import "time"

const (
	AdminAuthChallengePasskeyRegister = "passkey_register"
	AdminAuthChallengePasskeyLogin    = "passkey_login"
)

// AdminPasskey stores a WebAuthn credential. CredentialJSON is deliberately
// server-only because it contains the public key and authenticator counters.
type AdminPasskey struct {
	AdminUserID      uint64     `gorm:"column:admin_user_id;index:admin_passkeys_user_index;not null" json:"admin_user_id"`
	Name             string     `gorm:"column:name;size:80;not null" json:"name"`
	CredentialID     string     `gorm:"column:credential_id;type:text;not null" json:"-"`
	CredentialIDHash string     `gorm:"column:credential_id_hash;uniqueIndex:admin_passkeys_credential_hash_uindex;size:64" json:"-"`
	CredentialJSON   string     `gorm:"column:credential_json;type:text;not null" json:"-"`
	LastUsedAt       *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	BaseModel
}

func (a *AdminPasskey) TableName() string { return "admin_passkeys" }

// AdminAuthChallenge keeps WebAuthn ceremony state server-side.
// ChallengeIDHash is a hash of the opaque value returned
// to the browser, so a database read alone cannot consume a live challenge.
type AdminAuthChallenge struct {
	AdminUserID     uint64     `gorm:"column:admin_user_id;index:admin_auth_challenges_user_index;not null"`
	Purpose         string     `gorm:"column:purpose;index:admin_auth_challenges_purpose_index;size:32;not null"`
	ChallengeIDHash string     `gorm:"column:challenge_id_hash;uniqueIndex:admin_auth_challenges_id_uindex;size:64;not null"`
	Payload         string     `gorm:"column:payload;type:text;not null"`
	ClientIPHash    string     `gorm:"column:client_ip_hash;size:64"`
	ExpiresAt       time.Time  `gorm:"column:expires_at;index:admin_auth_challenges_expiry_index;not null"`
	ConsumedAt      *time.Time `gorm:"column:consumed_at"`
	BaseModel
}

func (a *AdminAuthChallenge) TableName() string { return "admin_auth_challenges" }

// AdminLoginThrottle persists failed login counters across restarts. KeyHash
// represents either a normalized account name or a client IP address.
type AdminLoginThrottle struct {
	KeyHash         string    `gorm:"column:key_hash;uniqueIndex:admin_login_throttles_key_uindex;size:64;not null"`
	Scope           string    `gorm:"column:scope;size:16;not null"`
	FailureCount    int       `gorm:"column:failure_count;not null;default:0"`
	WindowStartedAt time.Time `gorm:"column:window_started_at;not null"`
	LastFailedAt    time.Time `gorm:"column:last_failed_at;not null"`
	LockedUntil     time.Time `gorm:"column:locked_until;index:admin_login_throttles_lock_index"`
	BaseModel
}

func (a *AdminLoginThrottle) TableName() string { return "admin_login_throttles" }
