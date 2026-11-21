package sdk

import (
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
)

// buildModulePorts resolves the public dependency surface handed to standard
// modules. The ports path exposes storage.Ports readers directly to modules.
func (b *EngineBuilder) buildModulePorts(publicMasterData publicmasterdata.Store) standards.ModulePorts {
	var (
		entries standards.EntryReader
		balance standards.BalanceReader
		md      standards.MasterDataReader
	)

	if b.portsConfigured {
		if b.ports.Entries != nil {
			entries = b.ports.Entries
		}
		if b.ports.Balance != nil {
			balance = b.ports.Balance
		}
		if b.ports.MasterData != nil {
			md = b.ports.MasterData
		}
	}

	return standards.NewModulePorts(entries, balance, md, b.useCases)
}

