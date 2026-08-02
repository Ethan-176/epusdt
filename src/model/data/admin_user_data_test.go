package data

import (
	"testing"

	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
)

func TestEnsureDefaultAdminSeedsIdentityOnlyAndIsIdempotent(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()

	created, err := EnsureDefaultAdmin()
	if err != nil || !created {
		t.Fatalf("first EnsureDefaultAdmin created=%v err=%v", created, err)
	}
	created, err = EnsureDefaultAdmin()
	if err != nil || created {
		t.Fatalf("second EnsureDefaultAdmin created=%v err=%v", created, err)
	}

	var users []mdb.AdminUser
	if err := dao.Mdb.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != defaultAdminUsername || users[0].TOTPSecret != "" {
		t.Fatalf("unexpected seeded administrators: %#v", users)
	}
}
