package memory

import (
	"context"
	"fmt"
	"sync"
)

type lockKey struct {
	companyID string
	key       string
}

// LockAcquisition locks a specific key within a company.
func (d *Driver) Lock(ctx context.Context, companyID, key string) (unlock func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k := fmt.Sprintf("%s:%s", companyID, key)
	val, _ := d.postingLocks.LoadOrStore(k, &sync.Mutex{})
	mtx := val.(*sync.Mutex)
	mtx.Lock()
	return func() {
		mtx.Unlock()
	}, nil
}

