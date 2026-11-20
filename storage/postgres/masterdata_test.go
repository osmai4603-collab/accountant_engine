package postgres_test

import (
	"context"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/storage/masterdatacontract"
)

func TestPostgresMasterDataContract(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	masterdatacontract.Run(t, driver)
}

