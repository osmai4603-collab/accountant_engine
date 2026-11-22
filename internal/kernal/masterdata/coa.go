package masterdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrHeaderAccountPosting      = types.ErrCannotPostToParentAccount
	ErrClosedPeriod              = types.ErrFiscalPeriodClosed
	ErrAccountNotFound           = types.ErrAccountNotFound
	ErrAccountCycle              = errors.New("account hierarchy contains a cycle")
	ErrDuplicateAccountID        = errors.New("account hierarchy contains a duplicate account id")
	ErrInvalidParent             = errors.New("invalid parent account")
	ErrAccountNotCurrent         = errors.New("account is not valid on the posting date")
	ErrAccountIdentityMismatch   = errors.New("account repository returned a different account or company")
	ErrCannotPostToParentAccount = types.ErrCannotPostToParentAccount
	ErrClockRequired             = errors.New("clock is required for current-date master data validation")
)

// HierarchyNode represents an account node in the COA tree.
type HierarchyNode struct {
	Account  Account
	Children []*HierarchyNode
}

// DetectCyclicHierarchy checks for cycles in the account hierarchy.
func DetectCyclicHierarchy(accounts []Account) error {
	byID := make(map[types.AccountID]Account, len(accounts))
	for _, acc := range accounts {
		if acc.ID == (types.AccountID{}) {
			return ErrInvalidParent
		}
		if _, exists := byID[acc.ID]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateAccountID, acc.ID)
		}
		byID[acc.ID] = acc
	}

	for _, acc := range accounts {
		visited := make(map[types.AccountID]bool)
		curr := acc
		for curr.ParentID != nil {
			if visited[curr.ID] {
				return fmt.Errorf("%w: detected at account %s", ErrAccountCycle, curr.Code)
			}
			visited[curr.ID] = true
			parent, exists := byID[*curr.ParentID]
			if !exists {
				// Parent not in current batch or is root
				break
			}
			curr = parent
		}
	}

	// A complete COA snapshot belongs to one company, and a parent taken from
	// that snapshot may not belong to another tenant (RULE-KERNEL-016).
	scopeCompany := types.CompanyID{}
	for _, acc := range accounts {
		if acc.CompanyID == (types.CompanyID{}) {
			if scopeCompany != (types.CompanyID{}) {
				return ErrAccountIdentityMismatch
			}
			continue
		}
		if scopeCompany == (types.CompanyID{}) {
			scopeCompany = acc.CompanyID
			continue
		}
		if scopeCompany != acc.CompanyID {
			return ErrAccountIdentityMismatch
		}
	}
	for _, acc := range accounts {
		if acc.ParentID == nil {
			continue
		}
		parent, exists := byID[*acc.ParentID]
		if !exists {
			continue
		}
		if acc.CompanyID != (types.CompanyID{}) && parent.CompanyID != (types.CompanyID{}) &&
			acc.CompanyID != parent.CompanyID {
			return ErrAccountIdentityMismatch
		}
	}
	return nil
}

// ValidateCanPostToAccount verifies that the target account is active, not a header account,
// and has no child accounts using an injected clock. Posting paths that already
// know the entry date must call ValidateCanPostToAccountAsOf or
// ValidateCanPostToAccountWithClock instead of relying on wall time.
func ValidateCanPostToAccount(ctx context.Context, reader Reader, companyID types.CompanyID, accountID types.AccountID) error {
	return ValidateCanPostToAccountWithClock(ctx, reader, companyID, accountID, SystemClock{})
}

// ValidateCanPostToAccountWithClock resolves "today" from the supplied clock so
// the current-date path is deterministic and testable (plan rule 11).
func ValidateCanPostToAccountWithClock(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	accountID types.AccountID,
	clock Clock,
) error {
	if clock == nil {
		return ErrClockRequired
	}
	return ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, clock.Now().UTC())
}

func ValidateCanPostToAccountAsOf(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	accountID types.AccountID,
	asOf time.Time,
) error {
	_, err := ResolvePostableAccountAsOf(ctx, reader, companyID, accountID, asOf)
	return err
}

// ResolvePostableAccountAsOf validates an account for a posting date and
// returns the exact snapshot that was validated. Callers must use the returned
// value for downstream eligibility decisions instead of issuing a second,
// differently-timed read: a second read can observe a different master-data
// version and would make validation and execution disagree (TOCTOU).
// ADR P3-01 makes the effective-dated snapshot authoritative when the adapter
// implements EffectiveAccountReader.
func ResolvePostableAccountAsOf(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	accountID types.AccountID,
	asOf time.Time,
) (Account, error) {
	if companyID == (types.CompanyID{}) {
		return Account{}, ErrCompanyScopeRequired
	}
	var acc Account
	var err error
	if versioned, ok := reader.(EffectiveAccountReader); ok {
		acc, err = versioned.GetAccountAsOf(ctx, companyID, accountID, asOf)
	} else {
		acc, err = reader.GetAccount(ctx, companyID, accountID)
	}
	if err != nil {
		return Account{}, err
	}
	if err := ValidateAccountScope(acc, companyID, accountID); err != nil {
		return Account{}, err
	}
	if !acc.IsActive {
		return Account{}, fmt.Errorf("account %s is inactive", acc.Code)
	}
	if asOf.IsZero() || !acc.IsCurrent(asOf) {
		return Account{}, fmt.Errorf("%w: %s", ErrAccountNotCurrent, acc.Code)
	}
	hasChildren, err := reader.HasAccountChildren(ctx, companyID, accountID)
	if err != nil {
		return Account{}, err
	}
	if hasChildren {
		return Account{}, ErrHeaderAccountPosting
	}
	return acc, nil
}

// ValidateAccountScope ensures a repository returned the requested account
// within the requested company, not a row with inconsistent ownership.
func ValidateAccountScope(acc Account, companyID types.CompanyID, accountID types.AccountID) error {
	if companyID == (types.CompanyID{}) {
		return ErrCompanyScopeRequired
	}
	if accountID == (types.AccountID{}) || acc.ID != accountID || acc.CompanyID != companyID {
		return ErrAccountIdentityMismatch
	}
	return nil
}

// BuildHierarchyTree constructs an in-memory n-ary tree of accounts from a flat list.
func BuildHierarchyTree(accounts []Account) ([]*HierarchyNode, error) {
	if err := DetectCyclicHierarchy(accounts); err != nil {
		return nil, err
	}

	nodeMap := make(map[types.AccountID]*HierarchyNode, len(accounts))
	for _, acc := range accounts {
		nodeMap[acc.ID] = &HierarchyNode{Account: acc, Children: make([]*HierarchyNode, 0)}
	}

	var roots []*HierarchyNode
	for _, acc := range accounts {
		node := nodeMap[acc.ID]
		if acc.ParentID == nil {
			roots = append(roots, node)
		} else {
			if *acc.ParentID == (types.AccountID{}) {
				return nil, fmt.Errorf("%w: account %s references a zero parent id", ErrInvalidParent, acc.ID)
			}
			parent, exists := nodeMap[*acc.ParentID]
			if exists {
				parent.Children = append(parent.Children, node)
			} else {
				return nil, fmt.Errorf("%w: account %s references missing parent %s", ErrInvalidParent, acc.ID, *acc.ParentID)
			}
		}
	}
	return roots, nil
}
