# Storage SPI Contract Parity Report — P3-04

- **Generated (UTC):** 2026-11-21T02:52:38Z
- **Engine version:** (devel)
- **Adapters:** Memory (Dev/Test only), PostgreSQL (Production standard).

| Engine | Scenario | Adapter | Result | Semantic differences | Duration |
| :--- | :--- | :--- | :---: | :--- | ---: |
| (devel) | AuditWriter | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | AuditWriter | postgres | PASS | none; storage contract assertions passed | 1.367s |
| (devel) | CommitPostingAndBalance | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | CommitPostingAndBalance | postgres | PASS | none; storage contract assertions passed | 1.028s |
| (devel) | DraftStore | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | DraftStore | postgres | PASS | none; storage contract assertions passed | 5.846s |
| (devel) | IdempotencyLifecycle | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | IdempotencyLifecycle | postgres | PASS | none; storage contract assertions passed | 1.201s |
| (devel) | RollbackAtomicity | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | RollbackAtomicity | postgres | PASS | none; storage contract assertions passed | 1.032s |
| (devel) | SequenceAllocator | memory | PASS | none; storage contract assertions passed | 0s |
| (devel) | SequenceAllocator | postgres | PASS | none; storage contract assertions passed | 2.802s |

- **Totals:** 12 passed, 0 failed, 0 skipped.

## Capability differences

- memory: durability=false
- memory: foreign_keys=false
- memory: savepoints=false
- memory: serializable=true
- postgres: durability=true
- postgres: foreign_keys=true
- postgres: savepoints=false
- postgres: serializable=true

## Unexpected differences

None recorded.
