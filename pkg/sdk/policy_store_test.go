package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

type policyStoreFake struct {
	mu       sync.Mutex
	versions map[storage.PolicyKind][]storage.PolicyVersion
	lists    int
	failWith error
}

func (f *policyStoreFake) AppendVersion(_ context.Context, version storage.PolicyVersion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	f.versions[version.Kind] = append(f.versions[version.Kind], version)
	return nil
}

func (f *policyStoreFake) ListVersions(_ context.Context, _ types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	if f.failWith != nil {
		return nil, f.failWith
	}
	return append([]storage.PolicyVersion(nil), f.versions[kind]...), nil
}

func (f *policyStoreFake) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists
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

	before := store.listCount()
	if err := app.loadVersionedPolicies(context.Background(), companyID); err != nil {
		t.Fatalf("reload stored policies: %v", err)
	}
	if store.listCount() != before {
		t.Fatalf("stored policies were reloaded: lists = %d, want %d", store.listCount(), before)
	}
}

// versionedPolicyFixtures returns one valid posting and one valid currency
// version so tests can exercise the exact moment a store becomes authoritative.
func versionedPolicyFixtures(t *testing.T, companyID types.CompanyID, start time.Time, maxBackdating int) (storage.PolicyVersion, storage.PolicyVersion) {
	t.Helper()
	return storage.PolicyVersion{
			CompanyID:     companyID,
			PolicyID:      uuid.New(),
			Kind:          storage.PolicyKindPosting,
			Version:       1,
			EffectiveFrom: start,
			Payload:       policyVersionJSON(t, storage.PostingPolicyPayload{MaxBackdatingDays: maxBackdating}),
		}, storage.PolicyVersion{
			CompanyID:     companyID,
			PolicyID:      uuid.New(),
			Kind:          storage.PolicyKindCurrency,
			Version:       1,
			EffectiveFrom: start,
			Payload: policyVersionJSON(t, storage.CurrencyPolicyPayload{
				FunctionalCurrency:   "USD",
				PresentationCurrency: "USD",
				RoundingMethod:       string(policy.RoundingBankers),
				AllowedCurrencies:    []string{"USD"},
			}),
		}
}

func TestLegacyPoliciesAreReplacedWhenStoreBecomesAuthoritative(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	store := &policyStoreFake{versions: map[storage.PolicyKind][]storage.PolicyVersion{}}
	masterData := policyTestMasterData{company: publicmasterdata.Company{
		ID:                   companyID,
		Code:                 "TRANSITION",
		LegalName:            "Transition Co",
		FunctionalCurrency:   "USD",
		PresentationCurrency: "USD",
		Active:               true,
	}}
	app := &applicationService{
		ports:          storage.Ports{MasterData: masterData},
		policyEngine:   policy.NewPolicyEngine(),
		policyStore:    store,
		policiesLoaded: make(map[types.CompanyID]bool),
	}
	ctx := context.Background()

	// A store with zero versions has not been opted into yet: the legacy
	// 2000-01-01 fallback is still available.
	asOf := time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)
	if err := app.ensurePolicies(ctx, companyID, asOf); err != nil {
		t.Fatalf("legacy fallback before opt-in: %v", err)
	}
	if _, err := app.policyEngine.PostingPolicyFor(companyID, asOf); err != nil {
		t.Fatalf("legacy fallback policy missing: %v", err)
	}

	// The host appends the first stored version. Starting exactly at the
	// synthetic date must not collide with the fallback that was just removed.
	start := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	postingVersion, currencyVersion := versionedPolicyFixtures(t, companyID, start, 5)
	postingVersion.EffectiveTo = &end
	currencyVersion.EffectiveTo = &end
	for _, version := range []storage.PolicyVersion{postingVersion, currencyVersion} {
		if err := store.AppendVersion(ctx, version); err != nil {
			t.Fatalf("append stored version: %v", err)
		}
	}
	if err := app.ensurePolicies(ctx, companyID, asOf); err != nil {
		t.Fatalf("load stored policy after opt-in: %v", err)
	}
	loaded, err := app.policyEngine.PostingPolicyFor(companyID, asOf)
	if err != nil {
		t.Fatalf("stored policy after opt-in: %v", err)
	}
	if loaded.MaxBackdatingDays != 5 {
		t.Fatalf("MaxBackdatingDays = %d, want stored 5", loaded.MaxBackdatingDays)
	}

	// A date no stored version covers must fail closed; the removed synthetic
	// window must not answer it.
	uncovered := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)
	if err := app.ensurePolicies(ctx, companyID, uncovered); err == nil {
		t.Fatal("uncovered date accepted after the store became authoritative")
	}
}

func TestVersionedPoliciesLoadOnceUnderConcurrentPosts(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	postingVersion, currencyVersion := versionedPolicyFixtures(t, companyID, start, 0)
	store := &policyStoreFake{versions: map[storage.PolicyKind][]storage.PolicyVersion{
		storage.PolicyKindPosting:  {postingVersion},
		storage.PolicyKindCurrency: {currencyVersion},
	}}
	app := &applicationService{
		policyEngine:   policy.NewPolicyEngine(),
		policyStore:    store,
		policiesLoaded: make(map[types.CompanyID]bool),
	}
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- app.ensurePolicies(context.Background(), companyID, asOf)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensurePolicies: %v", err)
		}
	}
	if got := store.listCount(); got != 2 {
		t.Fatalf("ListVersions called %d times, want exactly 2 (one per kind)", got)
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
		{name: "revaluation already applied", err: valuation.ErrRevaluationAlreadyApplied, want: publicapplication.ErrConflict},
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

func TestFailedVersionedPolicyRegistrationRollsBackAndCanRetry(t *testing.T) {
	companyID := types.CompanyID(uuid.New())
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	postingVersion, currencyVersion := versionedPolicyFixtures(t, companyID, start, 7)
	// Structurally valid JSON that the kernel rejects: the adapter's append
	// contract cannot know every semantic rule of the private policy types.
	currencyVersion.Payload = policyVersionJSON(t, storage.CurrencyPolicyPayload{
		FunctionalCurrency:   "ZZZ",
		PresentationCurrency: "USD",
		RoundingMethod:       string(policy.RoundingBankers),
		AllowedCurrencies:    []string{"USD"},
	})
	store := &policyStoreFake{versions: map[storage.PolicyKind][]storage.PolicyVersion{
		storage.PolicyKindPosting:  {postingVersion},
		storage.PolicyKindCurrency: {currencyVersion},
	}}
	app := &applicationService{
		policyEngine:   policy.NewPolicyEngine(),
		policyStore:    store,
		policiesLoaded: make(map[types.CompanyID]bool),
	}
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	if err := app.ensurePolicies(ctx, companyID, asOf); err == nil {
		t.Fatal("invalid stored currency policy was accepted")
	}
	if _, err := app.policyEngine.PostingPolicyFor(companyID, asOf); !errors.Is(err, policy.ErrPolicyNotFound) {
		t.Fatalf("posting version survived a failed load: %v", err)
	}

	_, repaired := versionedPolicyFixtures(t, companyID, start, 7)
	store.mu.Lock()
	store.versions[storage.PolicyKindCurrency][0].Payload = repaired.Payload
	store.mu.Unlock()

	if err := app.ensurePolicies(ctx, companyID, asOf); err != nil {
		t.Fatalf("retry after repairing the stored version: %v", err)
	}
	loaded, err := app.policyEngine.PostingPolicyFor(companyID, asOf)
	if err != nil {
		t.Fatalf("posting policy after retry: %v", err)
	}
	if loaded.MaxBackdatingDays != 7 {
		t.Fatalf("MaxBackdatingDays = %d, want 7", loaded.MaxBackdatingDays)
	}

	// Out-of-band update picked up after InvalidateCompanyPolicies or ReloadCompanyPolicies.
	nextStart := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	v2Posting := storage.PolicyVersion{
		CompanyID:     companyID,
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindPosting,
		Version:       2,
		EffectiveFrom: nextStart,
		Payload:       policyVersionJSON(t, storage.PostingPolicyPayload{MaxBackdatingDays: 14}),
	}
	store.mu.Lock()
	store.versions[storage.PolicyKindPosting] = append(store.versions[storage.PolicyKindPosting], v2Posting)
	store.mu.Unlock()

	if err := app.ReloadCompanyPolicies(ctx, companyID); err != nil {
		t.Fatalf("ReloadCompanyPolicies: %v", err)
	}
	v2Loaded, err := app.policyEngine.PostingPolicyFor(companyID, nextStart.Add(time.Hour))
	if err != nil || v2Loaded.MaxBackdatingDays != 14 {
		t.Fatalf("reloaded v2 policy = (%+v, %v), want MaxBackdatingDays=14", v2Loaded, err)
	}

	app.InvalidateCompanyPolicies(companyID)
	if _, err := app.policyEngine.PostingPolicyFor(companyID, asOf); !errors.Is(err, policy.ErrPolicyNotFound) {
		t.Fatalf("policy after InvalidateCompanyPolicies = %v, want ErrPolicyNotFound", err)
	}
}
