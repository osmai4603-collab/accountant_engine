package storage

// EntryStore is the focused contract for journal entry persistence.
type EntryStore interface {
	DraftStore
	EntryReader
	PostingWriter
	SequenceAllocator
}

// AccountStore is the focused contract for account and journal master data.
type AccountStore interface {
	MasterDataReader
}

// BalanceStore is the focused contract for account balance reads.
type BalanceStore interface {
	BalanceReader
}

// AuditStore is the focused contract for immutable audit evidence.
type AuditStore interface {
	AuditWriter
}
