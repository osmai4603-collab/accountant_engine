package governance

import (
	"context"
	"fmt"

	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func VerifyAuditIntegrity(ctx context.Context, repo AuditLogRepository, companyID apiTypes.CompanyID) (bool, error) {
	if repo == nil {
		return false, ErrAuditChainBroken
	}
	chain := NewAuditChain(repo)
	ok, err := chain.VerifyAuditIntegrity(ctx, companyID)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrAuditChainBroken, err)
	}
	return ok, nil
}
