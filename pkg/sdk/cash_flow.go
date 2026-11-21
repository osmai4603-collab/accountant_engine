package sdk

import (
	"context"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
)

func (s *defaultReportingService) GetCashFlow(
	ctx context.Context,
	request reporting.CashFlowRequest,
) (reporting.CashFlowStatementReport, error) {
	if s.store == nil {
		return reporting.CashFlowStatementReport{}, ErrCapabilityNotImplemented
	}
	// If the reporting store directly provides cash flow capability:
	if provider, ok := s.store.(interface {
		GetCashFlow(context.Context, reporting.CashFlowRequest) (reporting.CashFlowStatementReport, error)
	}); ok {
		return provider.GetCashFlow(ctx, request)
	}
	return reporting.CashFlowStatementReport{}, ErrCapabilityNotImplemented
}

