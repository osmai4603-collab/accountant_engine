package ifrs9

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/shopspring/decimal"
)

// MeasurementCategory is the host-approved IFRS 9 classification applied to a
// financial asset. The engine validates the reference but does not make the
// business-model or SPPI assessment itself.
type MeasurementCategory string

const (
	AmortisedCost MeasurementCategory = "amortised_cost"
	FVOCI         MeasurementCategory = "fvoci"
	FVTPL         MeasurementCategory = "fvtpl"
)

var (
	ErrInvalidClassification          = errors.New("invalid IFRS 9 host classification")
	ErrApplicationPostingUnavailable  = errors.New("IFRS 9 application posting service is required")
	ErrApplicationPrincipalRequired   = errors.New("IFRS 9 application posting requires an authorized principal")
	ErrApplicationIdempotencyRequired = errors.New("IFRS 9 application posting requires an idempotency key")
	ErrInvalidApplicationEntry        = errors.New("invalid IFRS 9 application journal entry")
)

// HostClassification records the host's final business-model and SPPI
// conclusions.
type HostClassification struct {
	MeasurementCategory     MeasurementCategory
	BusinessModelReference  string
	SPPIAssessmentReference string
}

func (c HostClassification) Validate() error {
	if !validMeasurementCategory(c.MeasurementCategory) ||
		strings.TrimSpace(c.BusinessModelReference) == "" ||
		strings.TrimSpace(c.SPPIAssessmentReference) == "" {
		return ErrInvalidClassification
	}
	return nil
}

func validMeasurementCategory(category MeasurementCategory) bool {
	return category == AmortisedCost || category == FVOCI || category == FVTPL
}

// PostJournalEntryThroughApplication creates and posts an IFRS 9 draft through
// the public application service. The principal must already be present in ctx
// and authorized for the entry's company. The idempotency key is explicit
// because the public journal.Entry carries accounting content only.
func PostJournalEntryThroughApplication(
	ctx context.Context,
	app publicapplication.Service,
	entry journal.JournalEntry,
	idempotencyKey string,
) (publicapplication.EntryResult, error) {
	if app == nil {
		return publicapplication.EntryResult{}, ErrApplicationPostingUnavailable
	}
	if ctx == nil {
		return publicapplication.EntryResult{}, ErrApplicationPrincipalRequired
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return publicapplication.EntryResult{}, ErrApplicationIdempotencyRequired
	}
	if err := validateApplicationEntry(entry); err != nil {
		return publicapplication.EntryResult{}, err
	}
	principal, ok := publicapplication.PrincipalFromContext(ctx)
	if !ok || principal.ActorID == "" {
		return publicapplication.EntryResult{}, ErrApplicationPrincipalRequired
	}
	authorizedCompany := false
	for _, company := range principal.CompanyIDs {
		if company == entry.CompanyID {
			authorizedCompany = true
			break
		}
	}
	if !authorizedCompany {
		return publicapplication.EntryResult{}, ErrApplicationPrincipalRequired
	}

	metadata, _ := publicapplication.RequestMetadataFromContext(ctx)
	commandContext := publicapplication.Context{
		Principal: principal, CorrelationID: metadata.CorrelationID,
		IdempotencyKey: idempotencyKey, SourceSystem: "ifrs9",
	}
	currency := ""
	exchangeRate := decimal.NewFromInt(1)
	for i, line := range entry.Lines {
		lineCurrency := line.TransactionCurrency
		if lineCurrency == "" {
			lineCurrency = line.TransactionAmount.Currency()
		}
		if lineCurrency == "" {
			return publicapplication.EntryResult{}, fmt.Errorf("%w: line %d has no currency", ErrInvalidApplicationEntry, i)
		}
		if currency == "" {
			currency = lineCurrency
			if line.ExchangeRate.IsPositive() {
				exchangeRate = line.ExchangeRate
			}
		} else if currency != lineCurrency {
			return publicapplication.EntryResult{}, fmt.Errorf("%w: mixed line currencies", ErrInvalidApplicationEntry)
		}
	}
	postingDate := entry.PostingDate
	if postingDate.IsZero() {
		postingDate = entry.EffectiveDate
	}
	created, err := app.CreateDraft(ctx, publicapplication.CreateDraftCommand{
		Context:                     commandContext,
		CompanyID:                   entry.CompanyID,
		JournalID:                   entry.JournalID,
		FiscalPeriodID:              entry.FiscalPeriodID,
		EffectiveDate:               entry.EffectiveDate,
		PostingDate:                 postingDate,
		Description:                 entry.Description,
		SourceModule:                "ifrs9",
		ReferenceID:                 entry.ReferenceID,
		TransactionCurrency:         currency,
		ExchangeRate:                exchangeRate.String(),
		Classification:              entry.Classification,
		TargetPeriodNumber:          entry.TargetPeriodNumber,
		AdjustingEntryType:          entry.AdjustingEntryType,
		AdjustmentReason:            entry.AdjustmentReason,
		AdjustmentApprovalReference: "",
		Lines:                       entry.Lines,
	})
	if err != nil {
		return publicapplication.EntryResult{}, fmt.Errorf("create IFRS 9 draft: %w", err)
	}
	posted, err := app.Post(ctx, publicapplication.PostEntryCommand{
		Context: commandContext, CompanyID: entry.CompanyID, EntryID: created.Entry.ID,
	})
	if err != nil {
		return publicapplication.EntryResult{}, fmt.Errorf("post IFRS 9 draft: %w", err)
	}
	return posted, nil
}

func validateApplicationEntry(entry journal.JournalEntry) error {
	if entry.CompanyID == (types.CompanyID{}) || entry.JournalID == (types.JournalID{}) ||
		entry.EffectiveDate.IsZero() || len(entry.Lines) < 2 || !journalEntryBalanced(entry.Lines) {
		return ErrInvalidApplicationEntry
	}
	for i, line := range entry.Lines {
		if line.AccountID == (types.AccountID{}) {
			return fmt.Errorf("%w: line %d has no account", ErrInvalidApplicationEntry, i)
		}
		transactionCurrency := line.TransactionCurrency
		if transactionCurrency == "" {
			transactionCurrency = line.TransactionAmount.Currency()
		}
		if transactionCurrency == "" ||
			(line.TransactionAmount.Currency() != "" && line.TransactionAmount.Currency() != transactionCurrency) ||
			line.FunctionalDebit.Currency() != transactionCurrency ||
			line.FunctionalCredit.Currency() != transactionCurrency {
			return fmt.Errorf("%w: line %d currency mismatch", ErrInvalidApplicationEntry, i)
		}
	}
	return nil
}

// PostVersionedECLThroughApplication builds and posts a single-category ECL
// adjustment through the public application service.
func PostVersionedECLThroughApplication(
	ctx context.Context,
	app publicapplication.Service,
	result VersionedECLResult,
	accounts ECLAccounts,
	companyID, journalID uuid.UUID,
	createdBy string,
) (publicapplication.EntryResult, error) {
	entry, err := BuildVersionedECLEntry(result, accounts, companyID, journalID, createdBy)
	if err != nil {
		return publicapplication.EntryResult{}, err
	}
	category := AmortisedCost
	if len(result.CategoryChanges) == 1 {
		category = result.CategoryChanges[0].Category
	}
	return PostJournalEntryThroughApplication(
		ctx, app, entry,
		VersionedECLIdempotencyKey(result, companyID, journalID, category),
	)
}

// PostCategorizedVersionedECLThroughApplication posts the AC and FVOCI
// allowance movements together as one journal through the public application
// service.
func PostCategorizedVersionedECLThroughApplication(
	ctx context.Context,
	app publicapplication.Service,
	result VersionedECLResult,
	accounts ECLCategoryAccounts,
	companyID, journalID uuid.UUID,
	createdBy string,
) (publicapplication.EntryResult, error) {
	entry, err := BuildVersionedECLApplicationEntry(result, accounts, companyID, journalID, createdBy)
	if err != nil {
		return publicapplication.EntryResult{}, err
	}
	return PostJournalEntryThroughApplication(
		ctx, app, entry,
		VersionedECLApplicationIdempotencyKey(result, companyID, journalID),
	)
}
