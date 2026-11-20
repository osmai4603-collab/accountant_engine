package governance

import (
	"errors"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrSelfApprovalProhibited       = errors.New("maker-checker violation: self-approval is prohibited above threshold")
	ErrBankDutySegregationViolation = errors.New("segregation of duties: bank reconciliation and disbursement must be separated")
	ErrFinancialControllerRequired  = errors.New("financial controller approval required for annual close")
)

type Permission string

const (
	PermissionDraft         Permission = "DRAFT"
	PermissionPost          Permission = "POST"
	PermissionReverse       Permission = "REVERSE"
	PermissionClose         Permission = "CLOSE"
	PermissionBankReconcile Permission = "BANK_RECONCILE"
	PermissionDisburse      Permission = "DISBURSE"
	PermissionController    Permission = "FINANCIAL_CONTROLLER"
)

// ValidatePostingApproval verifies maker-checker separation: an actor cannot approve their own entry
// if the entry amount exceeds the configured policy threshold.
func ValidatePostingApproval(creatorID, approverID string, amount, threshold types.Money) error {
	if creatorID == approverID && amount.GreaterThan(threshold) {
		return ErrSelfApprovalProhibited
	}
	return nil
}

// ValidateBankDutySegregation verifies segregation of duties between reconciliation and disbursement.
func ValidateBankDutySegregation(reconcilerID, disburserID string) error {
	if reconcilerID != "" && reconcilerID == disburserID {
		return ErrBankDutySegregationViolation
	}
	return nil
}

// ValidateAnnualClose verifies that annual closing has controller authorization.
func ValidateAnnualClose(authenticated bool, permissions []Permission) error {
	if !authenticated {
		return ErrFinancialControllerRequired
	}
	for _, p := range permissions {
		if p == PermissionController {
			return nil
		}
	}
	return ErrFinancialControllerRequired
}
