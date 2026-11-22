package masterdata

import (
	"context"
	"errors"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

// ErrInvalidEffectiveRange rejects an inverted or unbounded account-version
// window. ErrAccountVersionOverlap rejects two versions of the same account
// whose inclusive windows intersect.
var (
	ErrInvalidEffectiveRange = errors.New("invalid effective date range")
	ErrAccountVersionOverlap = errors.New("overlapping account version")
)

// EffectiveAccountReader is the optional read capability that returns the
// full account snapshot that was effective at asOf. Adapters without this
// capability fall back to Store.GetAccount and the kernel fails closed when
// the returned record is not current.
type EffectiveAccountReader interface {
	GetAccountAsOf(
		ctx context.Context,
		companyID types.CompanyID,
		accountID types.AccountID,
		asOf time.Time,
	) (types.Account, error)
}

// AccountVersionWriter seeds a full effective-dated account snapshot. The
// version window is taken from EffectiveFrom/EffectiveTo on the supplied
// account; both bounds are inclusive and a nil EffectiveTo means open-ended.
type AccountVersionWriter interface {
	SaveAccountVersion(ctx context.Context, account types.Account) error
}
