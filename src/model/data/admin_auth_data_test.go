package data

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestAdminAuthChallengeIsBoundAndOneTime(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	token, err := CreateAdminAuthChallenge(7, mdb.AdminAuthChallengePasskeyRegister, "payload", "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConsumeAdminAuthChallenge(token, mdb.AdminAuthChallengePasskeyRegister, "203.0.113.11", 7); !errors.Is(err, ErrAdminAuthChallengeInvalid) {
		t.Fatalf("wrong IP error=%v", err)
	}
	if _, err := ConsumeAdminAuthChallenge(token, mdb.AdminAuthChallengePasskeyRegister, "203.0.113.10", 7); err != nil {
		t.Fatalf("correctly bound challenge failed: %v", err)
	}
	if _, err := ConsumeAdminAuthChallenge(token, mdb.AdminAuthChallengePasskeyRegister, "203.0.113.10", 7); !errors.Is(err, ErrAdminAuthChallengeInvalid) {
		t.Fatalf("replayed challenge error=%v", err)
	}
}

func TestAdminLoginThrottlePersistsAndClears(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	keys := AdminLoginThrottleKeys("Admin", "203.0.113.20")
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if _, err := RecordAdminLoginFailure(keys, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	until, err := AdminLoginLockedUntil(keys, now)
	if err != nil || !until.After(now.Add(14*time.Minute)) {
		t.Fatalf("locked until=%v err=%v", until, err)
	}
	if err := ClearAdminLoginFailures(keys); err != nil {
		t.Fatal(err)
	}
	until, err = AdminLoginLockedUntil(keys, now)
	if err != nil || !until.IsZero() {
		t.Fatalf("lock was not cleared: %v err=%v", until, err)
	}
}

func TestAdminPasskeyUsesFixedLengthCredentialHash(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	credential := &webauthn.Credential{ID: bytes.Repeat([]byte{0xab}, 900), PublicKey: []byte("public-key")}
	if err := SaveAdminPasskey(7, "long credential", credential); err != nil {
		t.Fatal(err)
	}
	var row mdb.AdminPasskey
	if err := dao.Mdb.Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if len(row.CredentialID) <= 1024 {
		t.Fatalf("test credential ID length=%d, want over 1024", len(row.CredentialID))
	}
	if len(row.CredentialIDHash) != 64 || row.CredentialIDHash != hashAdminAuthValue(row.CredentialID) {
		t.Fatalf("invalid credential hash %q", row.CredentialIDHash)
	}
}
