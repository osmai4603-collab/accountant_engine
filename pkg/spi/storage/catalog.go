package storage

import (
	"errors"
	"fmt"
	"sort"
)

// PortDefinition is one entry in the authoritative reference list of storage
// contracts consumed by the engine. Capability ports are listed as well so an
// adapter can be checked against the complete catalog instead of an informal
// subset.
type PortDefinition struct {
	Name        string
	Required    bool
	Description string
}

var portCatalog = []PortDefinition{
	{Name: "Drafts", Required: true, Description: "persists unposted journal entries"},
	{Name: "Entries", Required: true, Description: "reads company-scoped posted entries"},
	{Name: "Posting", Required: true, Description: "atomically commits validated postings and reversals"},
	{Name: "Sequences", Required: true, Description: "allocates unique company/journal sequence numbers"},
	{Name: "MasterData", Required: true, Description: "reads company, account, journal and fiscal calendar master data"},
	{Name: "Balance", Required: false, Description: "SDK/host-required: reads deterministic as-of account balances"},
	{Name: "Audit", Required: false, Description: "SDK/host-required: appends immutable audit evidence atomically with postings"},
	{Name: "Idempotency", Required: false, Description: "SDK/host-required: claims, completes and looks up durable idempotency keys"},
	{Name: "TxRunner", Required: false, Description: "SDK/host-required: runs a callback in one adapter transaction"},
	{Name: "UOWRunner", Required: false, Description: "SDK/host-required: runs a callback in one explicit unit of work"},
	{Name: "Events", Required: false, Description: "optional public event-ingestion stores"},
	{Name: "Private", Required: false, Description: "adapter-specific optional extensions; never required by the kernel"},
}

// PortCatalog returns a defensive copy of the complete storage port reference
// list. It is the single source used by conformance tooling to detect a port
// that was added to Ports without being documented or validated.
func PortCatalog() []PortDefinition {
	result := make([]PortDefinition, len(portCatalog))
	copy(result, portCatalog)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// ValidateContractVersion rejects any contract version other than the exact
// version implemented by this SPI. Version comparison is deliberately strict:
// adapters must opt in to a new contract generation explicitly.
func ValidateContractVersion(version string) error {
	if version != StorageContractVersion {
		return fmt.Errorf("storage: unsupported contract version %q, want %q", version, StorageContractVersion)
	}
	return nil
}

// ValidateAdapterCapabilities checks the declared contract generation and the
// identity fields an adapter must publish before it can be used as evidence of
// conformance.
func ValidateAdapterCapabilities(capabilities AdapterCapabilities) error {
	if err := ValidateContractVersion(capabilities.ContractVersion); err != nil {
		return err
	}
	if capabilities.AdapterName == "" {
		return errors.New("storage: adapter name is required")
	}
	if capabilities.AdapterVersion == "" {
		return errors.New("storage: adapter version is required")
	}
	return nil
}
