package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	kernelmasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	postingcompat "github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/reporting"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/subledgers"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func (s *applicationService) ExecuteYearEndClosing(ctx context.Context, command publicapplication.ExecuteYearEndClosingCommand) (publicapplication.YearEndClosingResult, error) {
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionClose, nil, ""); err != nil {
		return publicapplication.YearEndClosingResult{}, err
	}
	if command.CompanyID == (types.CompanyID{}) ||
		command.FiscalYearID == (types.FiscalYearID{}) ||
		command.JournalID == (types.JournalID{}) ||
		command.RetainedEarningsAccountID == (types.AccountID{}) {
		return publicapplication.YearEndClosingResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "company, fiscal year, journal, and retained earnings account are required", nil)
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		return publicapplication.YearEndClosingResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "year-end closing reason is required", nil)
	}
	closingStore, ok := s.ports.MasterData.(storage.YearEndClosingStore)
	if !ok || s.ports.TxRunner == nil {
		return publicapplication.YearEndClosingResult{}, ErrCapabilityNotImplemented
	}

	actorID := strings.TrimSpace(command.Context.Principal.ActorID)
	if actorID == "" {
		actorID = "system"
	}

	var result publicapplication.YearEndClosingResult
	err := s.WithinTransaction(ctx, command.CompanyID, func(txCtx context.Context) error {
		year, err := s.ports.MasterData.GetFiscalYear(txCtx, command.CompanyID, command.FiscalYearID)
		if err != nil {
			return err
		}
		if year.State != kernelmasterdata.FiscalYearOpen {
			return reporting.ErrFiscalYearNotOpen
		}

		currency := strings.TrimSpace(command.Currency)
		if currency == "" {
			company, err := s.ports.MasterData.GetCompany(txCtx, command.CompanyID)
			if err != nil {
				return err
			}
			currency = company.FunctionalCurrency
		}

		repo := &sdkClosingRepository{
			masterData: s.ports.MasterData,
			entries:    s.ports.Entries,
			store:      closingStore,
			companyID:  command.CompanyID,
		}
		engine := reporting.NewClosingEngine(repo)

		checklist, err := engine.EvaluateClosingChecklist(txCtx, year)
		if err != nil {
			return err
		}
		if !checklist.CanClose {
			return reporting.ErrClosingPreCheckFailed
		}

		policy := reporting.ClosingPolicy(command.Policy)
		if policy == "" {
			policy = reporting.ClosingPolicyDirectToRetainedEarnings
		}
		closingEntries, netIncome, err := engine.GenerateClosingMovesContext(
			txCtx,
			year,
			uuid.UUID(command.JournalID),
			uuid.UUID(command.RetainedEarningsAccountID),
			currency,
			reporting.ClosingOptions{
				Policy:                 policy,
				IncomeSummaryAccountID: uuid.UUID(command.IncomeSummaryAccountID),
			},
		)
		if err != nil {
			return err
		}

		if err := s.ensurePolicies(txCtx, command.CompanyID, year.EndDate); err != nil {
			return err
		}

		closingIDs := make([]publictypes.JournalEntryID, 0, len(closingEntries))
		for idx, ce := range closingEntries {
			kernelEntry := compatEntryToKernel(ce, command.CompanyID, command.JournalID)
			if kernelEntry.ID == (types.JournalEntryID{}) {
				kernelEntry.ID = types.NewJournalEntryID()
			}
			opts := posting.PostOptions{
				ActorID:       actorID,
				Reason:        reason,
				CorrelationID: command.Context.CorrelationID,
			}
			key := strings.TrimSpace(command.Context.IdempotencyKey)
			if key == "" {
				key = fmt.Sprintf("yec:%s:%s", command.CompanyID, command.FiscalYearID)
			}
			opts.IdempotencyKey = fmt.Sprintf("%s:close:%d", key, idx)
			posted, err := s.pipeline.PostEntry(txCtx, kernelEntry, opts)
			if err != nil {
				return err
			}
			closingIDs = append(closingIDs, posted.ID)
		}

		periods, err := closingStore.ListFiscalPeriods(txCtx, command.CompanyID, command.FiscalYearID)
		if err != nil {
			return err
		}
		for _, p := range periods {
			if p.Number == 13 && p.State == kernelmasterdata.PeriodOpen {
				if err := closingStore.UpdateFiscalPeriodState(txCtx, command.CompanyID, p.ID, kernelmasterdata.PeriodHardClosed, actorID, reason); err != nil {
					return err
				}
			}
		}
		if err := closingStore.UpdateFiscalYearState(txCtx, command.CompanyID, command.FiscalYearID, kernelmasterdata.FiscalYearHardClosed, actorID, reason); err != nil {
			return err
		}

		var openingID publictypes.JournalEntryID
		if command.NextFiscalYearID != (types.FiscalYearID{}) {
			nextYear, err := s.ports.MasterData.GetFiscalYear(txCtx, command.CompanyID, command.NextFiscalYearID)
			if err != nil {
				return err
			}
			closedYearForOpening := year
			if gap := nextYear.StartDate.Sub(closedYearForOpening.EndDate); gap > 0 && gap <= 24*time.Hour {
				closedYearForOpening.EndDate = nextYear.StartDate.Add(-time.Nanosecond)
			}
			openingMove, err := engine.GenerateYearEndOpeningMoveContext(txCtx, closedYearForOpening, nextYear, uuid.UUID(command.JournalID), currency)
			if err != nil {
				return err
			}
			if len(openingMove.Lines) > 0 {
				if err := s.ensurePolicies(txCtx, command.CompanyID, nextYear.StartDate); err != nil {
					return err
				}
				kernelOpening := compatEntryToKernel(openingMove, command.CompanyID, command.JournalID)
				if kernelOpening.ID == (types.JournalEntryID{}) {
					kernelOpening.ID = types.NewJournalEntryID()
				}
				opts := posting.PostOptions{
					ActorID:       actorID,
					Reason:        reason,
					CorrelationID: command.Context.CorrelationID,
				}
				key := strings.TrimSpace(command.Context.IdempotencyKey)
				if key == "" {
					key = fmt.Sprintf("yec:%s:%s", command.CompanyID, command.FiscalYearID)
				}
				opts.IdempotencyKey = fmt.Sprintf("%s:open", key)
				postedOpening, err := s.pipeline.PostEntry(txCtx, kernelOpening, opts)
				if err != nil {
					return err
				}
				openingID = postedOpening.ID
			}
		}

		now := time.Now().UTC()
		if err := closingStore.SaveYearEndCloseRecord(txCtx, storage.YearEndCloseRecord{
			CompanyID:        command.CompanyID,
			FiscalYearID:     command.FiscalYearID,
			NextFiscalYearID: command.NextFiscalYearID,
			ClosingEntryIDs:  closingIDs,
			OpeningEntryID:   openingID,
			NetIncome:        netIncome.String(),
			Policy:           string(policy),
			ClosedBy:         actorID,
			ClosedAt:         now,
			Reason:           reason,
		}); err != nil {
			return err
		}

		if s.ports.Audit != nil {
			_ = s.ports.Audit.AppendAudit(txCtx, storage.AuditRecord{
				CompanyID:     command.CompanyID,
				EntityType:    "fiscal_year",
				EntityID:      command.FiscalYearID.String(),
				Action:        string(publicapplication.ActionClose),
				ActorID:       actorID,
				Reason:        reason,
				CorrelationID: command.Context.CorrelationID,
				OccurredAt:    now,
			})
		}

		result = publicapplication.YearEndClosingResult{
			ClosingEntryIDs: closingIDs,
			OpeningEntryID:  openingID,
			NetIncome:       netIncome.String(),
		}
		return nil
	})
	if err != nil {
		return publicapplication.YearEndClosingResult{}, classifyError(err)
	}
	return result, nil
}

func (s *applicationService) ReopenYearEndClosing(ctx context.Context, command publicapplication.ReopenYearEndClosingCommand) (publicapplication.ReopenYearEndClosingResult, error) {
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionReopen, nil, command.ApprovalReference); err != nil {
		return publicapplication.ReopenYearEndClosingResult{}, err
	}
	if command.CompanyID == (types.CompanyID{}) || command.FiscalYearID == (types.FiscalYearID{}) {
		return publicapplication.ReopenYearEndClosingResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "company and fiscal year are required", nil)
	}
	reason := strings.TrimSpace(command.Reason)
	approvalRef := strings.TrimSpace(command.ApprovalReference)
	if reason == "" || approvalRef == "" {
		return publicapplication.ReopenYearEndClosingResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "reopen reason and approval reference are required", kernelmasterdata.ErrPeriodReopenRequiresAudit)
	}
	closingStore, ok := s.ports.MasterData.(storage.YearEndClosingStore)
	if !ok || s.ports.TxRunner == nil {
		return publicapplication.ReopenYearEndClosingResult{}, ErrCapabilityNotImplemented
	}

	actorID := strings.TrimSpace(command.Context.Principal.ActorID)
	if actorID == "" {
		actorID = "system"
	}

	var result publicapplication.ReopenYearEndClosingResult
	err := s.WithinTransaction(ctx, command.CompanyID, func(txCtx context.Context) error {
		year, err := s.ports.MasterData.GetFiscalYear(txCtx, command.CompanyID, command.FiscalYearID)
		if err != nil {
			return err
		}
		if year.State == kernelmasterdata.FiscalYearOpen {
			return reporting.ErrFiscalYearNotClosedForCarryForward
		}

		rec, found, err := closingStore.GetYearEndCloseRecord(txCtx, command.CompanyID, command.FiscalYearID)
		if err != nil {
			return err
		}
		if !found {
			return reporting.ErrAtomicYearEndReopenRequired
		}

		// Reopen the fiscal year and period 13 first so reversal entries can post into period 13.
		if err := closingStore.UpdateFiscalYearState(txCtx, command.CompanyID, command.FiscalYearID, kernelmasterdata.FiscalYearOpen, actorID, reason); err != nil {
			return err
		}
		periods, err := closingStore.ListFiscalPeriods(txCtx, command.CompanyID, command.FiscalYearID)
		if err != nil {
			return err
		}
		for _, p := range periods {
			if p.Number == 13 && p.State != kernelmasterdata.PeriodOpen {
				if err := closingStore.UpdateFiscalPeriodState(txCtx, command.CompanyID, p.ID, kernelmasterdata.PeriodOpen, actorID, reason); err != nil {
					return err
				}
			}
		}

		toReverse := make([]publictypes.JournalEntryID, 0, len(rec.ClosingEntryIDs)+1)
		if rec.OpeningEntryID != (publictypes.JournalEntryID{}) {
			toReverse = append(toReverse, rec.OpeningEntryID)
		}
		toReverse = append(toReverse, rec.ClosingEntryIDs...)

		reversalIDs := make([]publictypes.JournalEntryID, 0, len(toReverse))
		for idx, entryID := range toReverse {
			orig, err := s.ports.Entries.GetEntry(txCtx, command.CompanyID, entryID)
			if err != nil {
				return err
			}
			if orig.Status == journal.EntryReversed {
				continue
			}
			revDate := orig.EffectiveDate
			if revDate.IsZero() {
				revDate = time.Now().UTC()
			}
			reversal, err := posting.BuildReversalEntry(orig, revDate, actorID, reason)
			if err != nil {
				return err
			}
			if orig.TargetPeriodNumber == 13 {
				reversal.TargetPeriodNumber = 13
			}
			if err := s.ensurePolicies(txCtx, command.CompanyID, reversal.EffectiveDate); err != nil {
				return err
			}
			opts := posting.PostOptions{
				ActorID:       actorID,
				Reason:        reason,
				CorrelationID: command.Context.CorrelationID,
			}
			key := strings.TrimSpace(command.Context.IdempotencyKey)
			if key == "" {
				key = fmt.Sprintf("yec:%s:%s", command.CompanyID, command.FiscalYearID)
			}
			opts.IdempotencyKey = fmt.Sprintf("%s:reopen:%d", key, idx)
			postedRev, err := s.pipeline.PostEntry(txCtx, reversal, opts)
			if err != nil {
				return err
			}
			reversalIDs = append(reversalIDs, postedRev.ID)
		}

		now := time.Now().UTC()
		rec.ReopenedBy = actorID
		rec.ReopenedAt = &now
		rec.ReopenReason = reason
		rec.ApprovalReference = approvalRef
		if err := closingStore.SaveYearEndCloseRecord(txCtx, rec); err != nil {
			return err
		}

		if s.ports.Audit != nil {
			_ = s.ports.Audit.AppendAudit(txCtx, storage.AuditRecord{
				CompanyID:     command.CompanyID,
				EntityType:    "fiscal_year",
				EntityID:      command.FiscalYearID.String(),
				Action:        string(publicapplication.ActionReopen),
				ActorID:       actorID,
				Reason:        reason,
				CorrelationID: command.Context.CorrelationID,
				OccurredAt:    now,
			})
		}

		result = publicapplication.ReopenYearEndClosingResult{
			ReversalEntryIDs: reversalIDs,
		}
		return nil
	})
	if err != nil {
		return publicapplication.ReopenYearEndClosingResult{}, classifyError(err)
	}
	return result, nil
}

func compatEntryToKernel(ce postingcompat.JournalEntry, companyID types.CompanyID, journalID types.JournalID) journal.JournalEntry {
	lines := make([]journal.EntryLine, 0, len(ce.Lines))
	for _, cl := range ce.Lines {
		lineID := types.JournalEntryLineID(cl.ID)
		if lineID == (types.JournalEntryLineID{}) {
			lineID = types.NewJournalEntryLineID()
		}
		rate := cl.ExchangeRate
		if rate.IsZero() {
			rate = decimal.NewFromInt(1)
		}
		cur := cl.TransactionCurrency
		if cur == "" {
			cur = ce.TransactionCurrency
		}
		fDebit := cl.FunctionalDebit
		if fDebit.Currency() == "" {
			fDebit, _ = types.NewMoneyFromString("0", cur)
		}
		fCredit := cl.FunctionalCredit
		if fCredit.Currency() == "" {
			fCredit, _ = types.NewMoneyFromString("0", cur)
		}
		lines = append(lines, journal.EntryLine{
			ID:                  lineID,
			AccountID:           types.AccountID(cl.AccountID),
			PartnerID:           (*types.PartnerID)(cl.PartnerID),
			CostCenterID:        (*types.CostCenterID)(cl.CostCenterID),
			ProjectID:           (*types.ProjectID)(cl.ProjectID),
			SegmentID:           (*types.SegmentID)(cl.SegmentID),
			BusinessUnitID:      (*types.BusinessUnitID)(cl.BusinessUnitID),
			TransactionAmount:   cl.TransactionAmount,
			TransactionCurrency: cur,
			ExchangeRate:        rate,
			FunctionalDebit:     fDebit,
			FunctionalCredit:    fCredit,
		})
	}
	return journal.JournalEntry{
		ID:                  types.JournalEntryID(ce.ID),
		CompanyID:           companyID,
		JournalID:           journalID,
		FiscalPeriodID:      types.FiscalPeriodID(ce.PeriodID),
		EffectiveDate:       ce.EntryDate,
		PostingDate:         ce.PostingDate,
		Description:         ce.DocumentReference,
		SourceModule:        ce.SourceModule,
		ReferenceID:         ce.Reference,
		TransactionCurrency: ce.TransactionCurrency,
		Classification:      journal.EntryClassification(ce.Classification),
		TargetPeriodNumber:  ce.TargetPeriodNumber,
		AdjustingEntryType:  journal.AdjustingEntryType(ce.AdjustingEntryType),
		AdjustmentReason:    ce.AdjustmentReason,
		Status:              journal.EntryDraft,
		Lines:               lines,
	}
}

func kernelEntryToCompat(ke journal.JournalEntry) postingcompat.JournalEntry {
	lines := make([]postingcompat.JournalEntryLine, 0, len(ke.Lines))
	for _, kl := range ke.Lines {
		lines = append(lines, postingcompat.JournalEntryLine{
			CompanyID:           uuid.UUID(ke.CompanyID),
			ID:                  uuid.UUID(kl.ID),
			AccountID:           uuid.UUID(kl.AccountID),
			PartnerID:           (*uuid.UUID)(kl.PartnerID),
			CostCenterID:        (*uuid.UUID)(kl.CostCenterID),
			ProjectID:           (*uuid.UUID)(kl.ProjectID),
			SegmentID:           (*uuid.UUID)(kl.SegmentID),
			BusinessUnitID:      (*uuid.UUID)(kl.BusinessUnitID),
			TransactionAmount:   kl.TransactionAmount,
			TransactionCurrency: kl.TransactionCurrency,
			ExchangeRate:        kl.ExchangeRate,
			FunctionalDebit:     kl.FunctionalDebit,
			FunctionalCredit:    kl.FunctionalCredit,
		})
	}
	return postingcompat.JournalEntry{
		CompanyID:           uuid.UUID(ke.CompanyID),
		ID:                  uuid.UUID(ke.ID),
		SequenceNumber:      ke.SequenceNumber,
		JournalID:           uuid.UUID(ke.JournalID),
		PeriodID:            uuid.UUID(ke.FiscalPeriodID),
		EntryDate:           ke.EffectiveDate,
		PostingDate:         ke.PostingDate,
		SourceModule:        ke.SourceModule,
		DocumentReference:   ke.Description,
		Reference:           ke.ReferenceID,
		TransactionCurrency: ke.TransactionCurrency,
		Classification:      postingcompat.EntryClassification(ke.Classification),
		TargetPeriodNumber:  ke.TargetPeriodNumber,
		AdjustingEntryType:  postingcompat.AdjustingEntryType(ke.AdjustingEntryType),
		AdjustmentReason:    ke.AdjustmentReason,
		Status:              postingcompat.EntryStatus(ke.Status),
		Lines:               lines,
	}
}

type sdkClosingRepository struct {
	masterData storage.MasterDataReader
	entries    storage.EntryReader
	store      storage.YearEndClosingStore
	companyID  types.CompanyID
}

var _ reporting.Repository = (*sdkClosingRepository)(nil)
var _ reporting.FiscalPeriodRepository = (*sdkClosingRepository)(nil)
var _ reporting.FiscalYearHistoryRepository = (*sdkClosingRepository)(nil)

func (r *sdkClosingRepository) ListAccounts(ctx context.Context) ([]kernelmasterdata.Account, error) {
	return r.store.ListAccounts(ctx, r.companyID)
}

func (r *sdkClosingRepository) ListPostedEntries(ctx context.Context, start, end time.Time) ([]postingcompat.JournalEntry, error) {
	return r.ListEntriesForCompanyContext(ctx, uuid.UUID(r.companyID), start, end)
}

func (r *sdkClosingRepository) ListAccountsForCompanyContext(ctx context.Context, companyID uuid.UUID) ([]kernelmasterdata.Account, error) {
	return r.store.ListAccounts(ctx, types.CompanyID(companyID))
}

func (r *sdkClosingRepository) ListAccountsForCompanyAsOfContext(ctx context.Context, companyID uuid.UUID, _ time.Time) ([]kernelmasterdata.Account, error) {
	return r.store.ListAccounts(ctx, types.CompanyID(companyID))
}

func (r *sdkClosingRepository) ListEntriesForCompanyContext(ctx context.Context, companyID uuid.UUID, start, end time.Time) ([]postingcompat.JournalEntry, error) {
	entries, err := r.store.ListEntriesByDateRange(ctx, types.CompanyID(companyID), start, end)
	if err != nil {
		return nil, err
	}
	res := make([]postingcompat.JournalEntry, 0, len(entries))
	for _, e := range entries {
		res = append(res, kernelEntryToCompat(e))
	}
	return res, nil
}

func (r *sdkClosingRepository) ListFixedAssets(context.Context) ([]subledgers.FixedAsset, error) {
	return nil, nil
}

func (r *sdkClosingRepository) ListFixedAssetsForCompanyContext(context.Context, uuid.UUID) ([]subledgers.FixedAsset, error) {
	return nil, nil
}

func (r *sdkClosingRepository) ListOpenLines(context.Context, string) ([]subledgers.OpenLine, error) {
	return nil, nil
}

func (r *sdkClosingRepository) ListOpenLinesForCompanyContext(context.Context, uuid.UUID, string) ([]subledgers.OpenLine, error) {
	return nil, nil
}

func (r *sdkClosingRepository) GetRateForCompany(context.Context, uuid.UUID, string, string, time.Time) (decimal.Decimal, error) {
	return decimal.NewFromInt(1), nil
}

func (r *sdkClosingRepository) GetRateOfTypeForCompany(context.Context, uuid.UUID, valuation.RateType, string, string, time.Time) (decimal.Decimal, error) {
	return decimal.NewFromInt(1), nil
}

func (r *sdkClosingRepository) GetFiscalYearForCompanyContext(ctx context.Context, companyID, fiscalYearID uuid.UUID) (kernelmasterdata.FiscalYear, error) {
	return r.masterData.GetFiscalYear(ctx, types.CompanyID(companyID), types.FiscalYearID(fiscalYearID))
}

func (r *sdkClosingRepository) UpdatePeriodStateForCompanyContext(ctx context.Context, companyID, periodID uuid.UUID, state kernelmasterdata.PeriodState) error {
	return r.store.UpdateFiscalPeriodState(ctx, types.CompanyID(companyID), types.FiscalPeriodID(periodID), state, "system", "year-end closing")
}

func (r *sdkClosingRepository) UpdateFiscalYearStateForCompanyContext(ctx context.Context, companyID, fiscalYearID uuid.UUID, state kernelmasterdata.FiscalYearState) error {
	return r.store.UpdateFiscalYearState(ctx, types.CompanyID(companyID), types.FiscalYearID(fiscalYearID), state, "system", "year-end closing")
}

func (r *sdkClosingRepository) GetEntryByIDForCompanyContext(ctx context.Context, companyID, entryID uuid.UUID) (postingcompat.JournalEntry, error) {
	e, err := r.entries.GetEntry(ctx, types.CompanyID(companyID), types.JournalEntryID(entryID))
	if err != nil {
		return postingcompat.JournalEntry{}, err
	}
	return kernelEntryToCompat(e), nil
}

func (r *sdkClosingRepository) ListFiscalPeriodsForCompanyContext(ctx context.Context, companyID, fiscalYearID uuid.UUID) ([]kernelmasterdata.FiscalPeriod, error) {
	return r.store.ListFiscalPeriods(ctx, types.CompanyID(companyID), types.FiscalYearID(fiscalYearID))
}

func (r *sdkClosingRepository) IsFirstFiscalYearForCompanyContext(ctx context.Context, companyID, fiscalYearID uuid.UUID) (bool, error) {
	years, err := r.store.ListFiscalYears(ctx, types.CompanyID(companyID))
	if err != nil {
		return false, err
	}
	if len(years) == 0 {
		return true, nil
	}
	return years[0].ID == types.FiscalYearID(fiscalYearID), nil
}

var _ = errors.Is
