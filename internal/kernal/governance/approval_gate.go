package governance

import (
	"context"
	"errors"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrApprovalRequired     = errors.New("active approval is required")
	ErrApprovalHashMismatch = errors.New("approval fingerprint does not match entry content")
)

type ApprovalGate interface {
	IsApproved(context.Context, types.CompanyID, types.JournalEntryID, string) (bool, error)
}

func ValidateContentApproval(approved bool, err error, entry journal.JournalEntry, expectedHash, contentHash string) error {
	if err != nil {
		return err
	}
	if !approved {
		return ErrApprovalRequired
	}
	if expectedHash == "" || contentHash == "" || expectedHash != contentHash {
		return ErrApprovalHashMismatch
	}
	return nil
}
