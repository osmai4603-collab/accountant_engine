package masterdata

import (
	"context"
	"errors"
	"fmt"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrHeaderAccountPosting      = types.ErrCannotPostToParentAccount
	ErrClosedPeriod              = types.ErrFiscalPeriodClosed
	ErrAccountNotFound           = types.ErrAccountNotFound
	ErrAccountCycle              = errors.New("account hierarchy contains a cycle")
	ErrInvalidParent             = errors.New("invalid parent account")
	ErrCannotPostToParentAccount = types.ErrCannotPostToParentAccount
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
	return nil
}

// ValidateCanPostToAccount verifies that the target account is active, not a header account,
// and has no child accounts.
func ValidateCanPostToAccount(ctx context.Context, reader Reader, companyID types.CompanyID, accountID types.AccountID) error {
	acc, err := reader.GetAccount(ctx, companyID, accountID)
	if err != nil {
		return err
	}
	if !acc.IsActive {
		return fmt.Errorf("account %s is inactive", acc.Code)
	}
	hasChildren, err := reader.HasAccountChildren(ctx, companyID, accountID)
	if err != nil {
		return err
	}
	if hasChildren {
		return ErrHeaderAccountPosting
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
		if acc.ParentID == nil || *acc.ParentID == (types.AccountID{}) {
			roots = append(roots, node)
		} else {
			parent, exists := nodeMap[*acc.ParentID]
			if exists {
				parent.Children = append(parent.Children, node)
			} else {
				roots = append(roots, node)
			}
		}
	}
	return roots, nil
}
