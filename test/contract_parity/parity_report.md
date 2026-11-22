# Storage SPI Contract Parity Report — P3-04

- **Generated (UTC):** 2026-11-22T02:38:00Z
- **Engine version:** (devel)
- **Adapters:** Memory (Dev/Test only), PostgreSQL (Production standard).

| Engine | Scenario | Adapter | Result | Semantic differences | Duration |
| :--- | :--- | :--- | :---: | :--- | ---: |
| (devel) | AdapterCapabilitiesValidated | memory | PASS | not observed | 0s |
| (devel) | AdapterCapabilitiesValidated | postgres | PASS | not observed | 541ms |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | memory | PASS | not observed | 0s |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | postgres | PASS | not observed | 691ms |
| (devel) | AuditWriter | memory | PASS | not observed | 0s |
| (devel) | AuditWriter | postgres | PASS | not observed | 567ms |
| (devel) | BalanceReaderAsOfAndCurrency | memory | PASS | not observed | 1ms |
| (devel) | BalanceReaderAsOfAndCurrency | postgres | PASS | not observed | 604ms |
| (devel) | BoundedRetryOnUnitOfWork | memory | PASS | not observed | 2ms |
| (devel) | BoundedRetryOnUnitOfWork | postgres | PASS | not observed | 524ms |
| (devel) | CommitPostingAndBalance | memory | PASS | not observed | 1ms |
| (devel) | CommitPostingAndBalance | postgres | PASS | not observed | 1.111s |
| (devel) | CompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeIsolation | postgres | PASS | not observed | 426ms |
| (devel) | CompanyScopeOnEveryMasterDataPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryMasterDataPort | postgres | PASS | not observed | 586ms |
| (devel) | CompanyScopeOnEveryPort | memory | PASS | not observed | 1ms |
| (devel) | CompanyScopeOnEveryPort | postgres | PASS | not observed | 438ms |
| (devel) | ConcurrentReplay | memory | PASS | not observed | 1ms |
| (devel) | ConcurrentReplay | postgres | PASS | not observed | 457ms |
| (devel) | ConcurrentReplayAtScale | memory | PASS | not observed | 4ms |
| (devel) | ConcurrentReplayAtScale | postgres | PASS | not observed | 461ms |
| (devel) | ConcurrentReversalSingleWinner | memory | PASS | not observed | 4ms |
| (devel) | ConcurrentReversalSingleWinner | postgres | PASS | not observed | 484ms |
| (devel) | ConnectionInterruptionBeforeCommit | memory | PASS | not observed | 1ms |
| (devel) | ConnectionInterruptionBeforeCommit | postgres | PASS | not observed | 613ms |
| (devel) | ContextCancellationNoEffect | memory | PASS | not observed | 0s |
| (devel) | ContextCancellationNoEffect | postgres | PASS | not observed | 741ms |
| (devel) | ContractAndAdapterVersion | memory | PASS | not observed | 0s |
| (devel) | ContractAndAdapterVersion | postgres | PASS | not observed | 706ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | memory | PASS | not observed | 1ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | postgres | PASS | not observed | 475ms |
| (devel) | DraftStore | memory | PASS | not observed | 0s |
| (devel) | DraftStore | postgres | PASS | not observed | 658ms |
| (devel) | FaultInjectionAtomicity | memory | PASS | not observed | 1ms |
| (devel) | FaultInjectionAtomicity | postgres | PASS | not observed | 434ms |
| (devel) | FrozenSnapshotRemainsStable | memory | PASS | not observed | 0s |
| (devel) | FrozenSnapshotRemainsStable | postgres | PASS | not observed | 479ms |
| (devel) | G019EffectiveAccountWindow | memory | PASS | not observed | 0s |
| (devel) | G019EffectiveAccountWindow | postgres | PASS | not observed | 404ms |
| (devel) | G019G022SDKPosting | memory | PASS | not observed | 1ms |
| (devel) | G019G022SDKPosting | postgres | PASS | not observed | 448ms |
| (devel) | G019VersionWritesFailClosed | memory | PASS | not observed | 0s |
| (devel) | G019VersionWritesFailClosed | postgres | PASS | not observed | 429ms |
| (devel) | G022DimensionScopes | memory | PASS | not observed | 0s |
| (devel) | G022DimensionScopes | postgres | PASS | not observed | 461ms |
| (devel) | G022SDKFailClosed | memory | PASS | not observed | 1ms |
| (devel) | G022SDKFailClosed | postgres | PASS | not observed | 410ms |
| (devel) | G022SDKForeignDimension | memory | PASS | not observed | 1ms |
| (devel) | G022SDKForeignDimension | postgres | PASS | not observed | 443ms |
| (devel) | IdempotencyLifecycle | memory | PASS | not observed | 1ms |
| (devel) | IdempotencyLifecycle | postgres | PASS | not observed | 710ms |
| (devel) | IdempotentReplayAndPayloadConflict | memory | PASS | not observed | 1ms |
| (devel) | IdempotentReplayAndPayloadConflict | postgres | PASS | not observed | 431ms |
| (devel) | InterleavedCompanyIsolation | memory | PASS | not observed | 1ms |
| (devel) | InterleavedCompanyIsolation | postgres | PASS | not observed | 509ms |
| (devel) | KernelStampedDates | memory | PASS | not observed | 0s |
| (devel) | KernelStampedDates | postgres | PASS | not observed | 410ms |
| (devel) | LockOrderEnforced | memory | PASS | not observed | 0s |
| (devel) | LockOrderEnforced | postgres | PASS | not observed | 442ms |
| (devel) | PolicyVersionCompanyScope | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionCompanyScope | postgres | PASS | not observed | 405ms |
| (devel) | PolicyVersionHistory | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionHistory | postgres | PASS | not observed | 418ms |
| (devel) | RevaluationAuditAction | memory | PASS | not observed | 0s |
| (devel) | RevaluationAuditAction | postgres | PASS | not observed | 425ms |
| (devel) | RevaluationCompanyScopeIsolation | memory | PASS | not observed | 1ms |
| (devel) | RevaluationCompanyScopeIsolation | postgres | PASS | not observed | 449ms |
| (devel) | RevaluationConcurrentSingleWinner | memory | PASS | not observed | 2ms |
| (devel) | RevaluationConcurrentSingleWinner | postgres | PASS | not observed | 424ms |
| (devel) | RevaluationFaultAfterCommitReplay | memory | PASS | not observed | 1ms |
| (devel) | RevaluationFaultAfterCommitReplay | postgres | PASS | not observed | 415ms |
| (devel) | RevaluationPayloadConflict | memory | PASS | not observed | 1ms |
| (devel) | RevaluationPayloadConflict | postgres | PASS | not observed | 418ms |
| (devel) | RevaluationReplaySamePayload | memory | PASS | not observed | 1ms |
| (devel) | RevaluationReplaySamePayload | postgres | PASS | not observed | 403ms |
| (devel) | RevaluationScopeSeparation | memory | PASS | not observed | 2ms |
| (devel) | RevaluationScopeSeparation | postgres | PASS | not observed | 432ms |
| (devel) | ReversalAtomicity | memory | PASS | not observed | 2ms |
| (devel) | ReversalAtomicity | postgres | PASS | not observed | 434ms |
| (devel) | RollbackAtomicity | memory | PASS | not observed | 1ms |
| (devel) | RollbackAtomicity | postgres | PASS | not observed | 974ms |
| (devel) | SameAccountConcurrentPosting | memory | PASS | not observed | 1ms |
| (devel) | SameAccountConcurrentPosting | postgres | PASS | not observed | 439ms |
| (devel) | ScopeViolationRejected | memory | PASS | not observed | 0s |
| (devel) | ScopeViolationRejected | postgres | PASS | not observed | 487ms |
| (devel) | SequenceAllocator | memory | PASS | not observed | 0s |
| (devel) | SequenceAllocator | postgres | PASS | not observed | 628ms |

- **Totals:** 88 passed, 0 failed, 0 skipped.

## Capability differences

- memory: durability=false
- memory: foreign_keys=false
- memory: savepoints=false
- memory: serializable=true
- postgres: durability=true
- postgres: foreign_keys=true
- postgres: savepoints=false
- postgres: serializable=true

## Declared semantic differences

None declared by the executed scenarios.

## Unexpected differences

None recorded.
