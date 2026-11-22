package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

type policyStoreFake struct {
	versions map[storage.PolicyKind][]storage.PolicyVersion
	lists    int
	failWith error
}

func (f *policyStoreFake) AppendVersion(_ context.Context, version storage.PolicyVersion) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.versions[version.Kind] = append(f.versions[version.Kind], version)
	return nil
}

func (f *policyStoreFake) ListVersions(_ context.Context, _ types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	f.lists++
	if f.failWith != nil {
		return nil, f.failWith
	}
	return append([]storage.PolicyVersion(nil), f.versions[kind]...), nil
}

type policyTestMasterData struct {
	storage.MasterDataReader
	company publicmasterdata.Company
}

func (m policyTestMasterData) GetCompany(_ context.Context, companyID types.CompanyID) (publicmasterdata.Company, error) {
	if companyID != m.company.ID {
		return publicmasterdata.Company{}, errors.New("company not found")
	}
	return m.company, nil
}

func policyVersionJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal policy payload: %v", err)
	}
	return payload
}

func TestVersionedPoliciesLoadEveryHistoricalVersionOnce(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	boundary := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)

	postingV1 := storage.PolicyVersion{
		CompanyID:     companyID,
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindPosting,
		Version:       1,
		EffectiveFrom: start,
		EffectiveTo:   &boundary,
		Payload:       policyVersionJSON(t, storage.PostingPolicyPayload{MaxBackdatingDays: 0}),
	}
	postingV2 := storage.PolicyVersion{
		CompanyID:     companyID,
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindPosting,
		Version:       2,
		EffectiveFrom: boundary,
		EffectiveTo:   &end,
		Payload:       policyVersionJSON(t, storage.PostingPolicyPayload{MaxBackdatingDays: 10}),
	}
	currency := storage.PolicyVersion{
		CompanyID:     companyID,
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindCurrency,
		Version:       1,
		EffectiveFrom: start,
		EffectiveTo:   &end,
		Payload: policyVersionJSON(t, storage.CurrencyPolicyPayload{
			FunctionalCurrency:   "USD",
			PresentationCurrency: "USD",
			RoundingMethod:       string(policy.RoundingBankers),
			AllowedCurrencies:    []string{"USD"},
		}),
	}

	store := &policyStoreFake{versions: map[storage.PolicyKind][]storage.PolicyVersion{
		storage.PolicyKindPosting:  {postingV1, postingV2},
		storage.PolicyKindCurrency: {currency},
	}}
	app := &applicationService{
		policyEngine:   policy.NewPolicyEngine(),
		policyStore:    store,
		policiesLoaded: make(map[types.CompanyID]bool),
	}

	if err := app.loadVersionedPolicies(context.Background(), companyID); err != nil {
		t.Fatalf("load stored policies: %v", err)
	}
	early, err := app.policyEngine.PostingPolicyFor(companyID, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("early posting policy: %v", err)
	}
	if early.MaxBackdatingDays != 0 {
		t.Fatalf("early MaxBackdatingDays = %d, want 0", early.MaxBackdatingDays)
	}
	late, err := app.policyEngine.PostingPolicyFor(companyID, time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("late posting policy: %v", err)
	}
	if late.MaxBackdatingDays != 10 {
		t.Fatalf("late MaxBackdatingDays = %d, want 10", late.MaxBackdatingDays)
	}
	if err := app.ensureVersionedPoliciesIsEffective(companyID, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("stored policies not effective: %v", err)
	}

	before := store.lists
	if err := app.loadVersionedPolicies(context.Background(), companyID); err != nil {
		t.Fatalf("reload stored policies: %v", err)
	}
	if store.lists != before {
		t.Fatalf("stored policies were reloaded: lists = %d, want %d", store.lists, before)
	}
}

func TestVersionedPoliciesFailClosedForUncoveredDate(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	store := &policyStoreFake{versions: map[storage.PolicyKind][]storage.PolicyVersion{
		storage.PolicyKindPosting: {{
			CompanyID:     companyID,
			PolicyID:      uuid.New(),
			Kind:          storage.PolicyKindPosting,
			Version:       1,
			EffectiveFrom: start,
			EffectiveTo:   &end,
			Payload:       policyVersionJSON(t, storage.PostingPolicyPayload{MaxBackdatingDays: 0}),
		}},
	}}
	app := &applicationService{
		policyEngine:   policy.NewPolicyEngine(),
		policyStore:    store,
		policiesLoaded: make(map[types.CompanyID]bool),
	}
	if err := app.loadVersionedPolicies(context.Background(), companyID); err != nil {
		t.Fatalf("load stored policies: %v", err)
	}
	err := app.ensureVersionedPoliciesIsEffective(companyID, time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("uncovered date accepted when a policy store is configured")
	}
	if !errors.Is(err, policy.ErrPolicyNotFound) && !errors.Is(err, policy.ErrPolicyNotEffective) {
		t.Fatalf("error = %v, want a policy selection failure", err)
	}
}

func TestLegacyPolicyRegistrationDoesNotSwallowErrors(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	oldStart := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	oldEnd := time.Date(2000, time.February, 1, 0, 0, 0, 0, time.UTC)
	engine := policy.NewPolicyEngine()
	if err := engine.SetCurrencyPolicy(policy.CurrencyPolicy{
		CompanyID:            companyID,
		FunctionalCurrency:   "USD",
		PresentationCurrency: "USD",
		RoundingMethod:       policy.RoundingBankers,
		AllowedCurrencies:    []string{"USD"},
		EffectiveDate:        oldStart,
		EffectiveTo:          &oldEnd,
	}); err != nil {
		t.Fatalf("seed overlapping currency policy: %v", err)
	}

	app := &applicationService{
		ports: storage.Ports{MasterData: policyTestMasterData{company: publicmasterdata.Company{
			ID:                   companyID,
			Code:                 "LEGACY",
			LegalName:            "Legacy Policy Co",
			FunctionalCurrency:   "USD",
			PresentationCurrency: "USD",
			Active:               true,
		}}},
		policyEngine: engine,
	}

	err := app.ensurePolicies(context.Background(), companyID, time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("policy registration error was swallowed")
	}
}

func TestPhase11CommitIdempotencyKeepsStampedRevaluationScope(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	opts := posting.PostOptions{
		ActorID:           "actor",
		IdempotencyKey:    "fx-revaluation|key",
		CallerFingerprint: "sha256:stable",
		Scope: posting.IdempotencyScope{
			CompanyID: companyID,
			Operation: posting.IdempotencyRevaluation,
			Key:       "fx-revaluation|key",
		},
	}

	commit := commitIdempotency(opts, storage.OperationPost)
	if commit == nil {
		t.Fatal("commit idempotency was not built")
	}
	if commit.Key != "fx-revaluation|key" || commit.PayloadHash != "sha256:stable" {
		t.Fatalf("idempotency = %+v, want key and stamped fingerprint preserved", commit)
	}
	wantScope := "company=" + companyID.String() + "|operation=revaluation|key=fx-revaluation|key"
	if commit.Scope != wantScope {
		t.Fatalf("scope = %q, want %q", commit.Scope, wantScope)
	}
}

func TestPhase11PolicyAndRevaluationErrorsAreClassified(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "policy version conflict", err: storage.ErrPolicyVersionConflict, want: publicapplication.ErrConflict},
		{name: "policy overlap", err: policy.ErrPolicyOverlap, want: publicapplication.ErrConflict},
		{name: "revaluation already applied", err: posting.ErrRevaluationAlreadyApplied, want: publicapplication.ErrConflict},
		{name: "policy version missing", err: storage.ErrPolicyVersionNotFound, want: publicapplication.ErrNotFound},
		{name: "policy not effective", err: policy.ErrPolicyNotEffective, want: publicapplication.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			classified := classifyError(fmt.Errorf("wrapped: %w", tc.err))
			if !errors.Is(classified, tc.want) {
				t.Fatalf("classified = %v, want %v", classified, tc.want)
			}
		})
	}
}
