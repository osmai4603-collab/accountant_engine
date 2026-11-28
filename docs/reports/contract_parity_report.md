# Storage SPI Contract Parity Report — P3-04

- **Generated (UTC):** 2026-11-28T00:48:26Z
- **Engine version:** (devel)
- **Adapters:** Memory (Dev/Test only), PostgreSQL (Production standard).

| Engine | Scenario | Adapter | Result | Semantic differences | Duration |
| :--- | :--- | :--- | :---: | :--- | ---: |
| (devel) | AdapterCapabilitiesValidated | memory | PASS | not observed | 0s |
| (devel) | AdapterCapabilitiesValidated | postgres | PASS | not observed | 422ms |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | memory | PASS | not observed | 0s |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | postgres | PASS | not observed | 421ms |
| (devel) | AuditWriter | memory | PASS | not observed | 0s |
| (devel) | AuditWriter | postgres | PASS | not observed | 509ms |
| (devel) | BalanceReaderAsOfAndCurrency | memory | PASS | not observed | 0s |
| (devel) | BalanceReaderAsOfAndCurrency | postgres | PASS | not observed | 422ms |
| (devel) | BoundedRetryOnUnitOfWork | memory | PASS | not observed | 1ms |
| (devel) | BoundedRetryOnUnitOfWork | postgres | PASS | not observed | 449ms |
| (devel) | CommitPostingAndBalance | memory | PASS | not observed | 0s |
| (devel) | CommitPostingAndBalance | postgres | PASS | not observed | 510ms |
| (devel) | CompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeIsolation | postgres | PASS | not observed | 643ms |
| (devel) | CompanyScopeOnEveryMasterDataPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryMasterDataPort | postgres | PASS | not observed | 499ms |
| (devel) | CompanyScopeOnEveryPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryPort | postgres | PASS | not observed | 666ms |
| (devel) | ConcurrentReplay | memory | PASS | not observed | 0s |
| (devel) | ConcurrentReplay | postgres | PASS | not observed | 684ms |
| (devel) | ConcurrentReplayAtScale | memory | PASS | not observed | 1ms |
| (devel) | ConcurrentReplayAtScale | postgres | PASS | not observed | 670ms |
| (devel) | ConcurrentReversalSingleWinner | memory | PASS | not observed | 3ms |
| (devel) | ConcurrentReversalSingleWinner | postgres | PASS | not observed | 654ms |
| (devel) | ConnectionInterruptionBeforeCommit | memory | PASS | not observed | 0s |
| (devel) | ConnectionInterruptionBeforeCommit | postgres | PASS | not observed | 432ms |
| (devel) | ContextCancellationNoEffect | memory | PASS | not observed | 0s |
| (devel) | ContextCancellationNoEffect | postgres | PASS | not observed | 508ms |
| (devel) | ContractAndAdapterVersion | memory | PASS | not observed | 0s |
| (devel) | ContractAndAdapterVersion | postgres | PASS | not observed | 759ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | memory | PASS | not observed | 1ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | postgres | PASS | not observed | 683ms |
| (devel) | DraftStore | memory | PASS | not observed | 0s |
| (devel) | DraftStore | postgres | PASS | not observed | 581ms |
| (devel) | FaultInjectionAtomicity | memory | PASS | not observed | 0s |
| (devel) | FaultInjectionAtomicity | postgres | PASS | not observed | 586ms |
| (devel) | FrozenSnapshotRemainsStable | memory | PASS | not observed | 0s |
| (devel) | FrozenSnapshotRemainsStable | postgres | PASS | not observed | 718ms |
| (devel) | G019EffectiveAccountWindow | memory | PASS | not observed | 0s |
| (devel) | G019EffectiveAccountWindow | postgres | PASS | not observed | 447ms |
| (devel) | G019G022SDKPosting | memory | PASS | not observed | 0s |
| (devel) | G019G022SDKPosting | postgres | PASS | not observed | 525ms |
| (devel) | G019VersionWritesFailClosed | memory | PASS | not observed | 0s |
| (devel) | G019VersionWritesFailClosed | postgres | PASS | not observed | 768ms |
| (devel) | G022DimensionScopes | memory | PASS | not observed | 0s |
| (devel) | G022DimensionScopes | postgres | PASS | not observed | 596ms |
| (devel) | G022SDKFailClosed | memory | PASS | not observed | 0s |
| (devel) | G022SDKFailClosed | postgres | PASS | not observed | 439ms |
| (devel) | G022SDKForeignDimension | memory | PASS | not observed | 0s |
| (devel) | G022SDKForeignDimension | postgres | PASS | not observed | 454ms |
| (devel) | GLRebuildDeterminismAndParity | memory | PASS | not observed | 0s |
| (devel) | GLRebuildDeterminismAndParity | postgres | PASS | not observed | 421ms |
| (devel) | IdempotencyLifecycle | memory | PASS | not observed | 0s |
| (devel) | IdempotencyLifecycle | postgres | PASS | not observed | 522ms |
| (devel) | IdempotentReplayAndPayloadConflict | memory | PASS | not observed | 0s |
| (devel) | IdempotentReplayAndPayloadConflict | postgres | PASS | not observed | 417ms |
| (devel) | InterleavedCompanyIsolation | memory | PASS | not observed | 0s |
| (devel) | InterleavedCompanyIsolation | postgres | PASS | not observed | 674ms |
| (devel) | KernelStampedDates | memory | PASS | not observed | 0s |
| (devel) | KernelStampedDates | postgres | PASS | not observed | 554ms |
| (devel) | LockOrderEnforced | memory | PASS | not observed | 0s |
| (devel) | LockOrderEnforced | postgres | PASS | not observed | 685ms |
| (devel) | PolicyVersionCompanyScope | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionCompanyScope | postgres | PASS | not observed | 446ms |
| (devel) | PolicyVersionHistory | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionHistory | postgres | PASS | not observed | 432ms |
| (devel) | RevaluationAuditAction | memory | PASS | not observed | 0s |
| (devel) | RevaluationAuditAction | postgres | PASS | not observed | 728ms |
| (devel) | RevaluationCompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | RevaluationCompanyScopeIsolation | postgres | PASS | not observed | 489ms |
| (devel) | RevaluationConcurrentSingleWinner | memory | PASS | not observed | 0s |
| (devel) | RevaluationConcurrentSingleWinner | postgres | PASS | not observed | 484ms |
| (devel) | RevaluationFaultAfterCommitReplay | memory | PASS | not observed | 1ms |
| (devel) | RevaluationFaultAfterCommitReplay | postgres | PASS | not observed | 448ms |
| (devel) | RevaluationPayloadConflict | memory | PASS | not observed | 0s |
| (devel) | RevaluationPayloadConflict | postgres | PASS | not observed | 426ms |
| (devel) | RevaluationReplaySamePayload | memory | PASS | not observed | 0s |
| (devel) | RevaluationReplaySamePayload | postgres | PASS | not observed | 438ms |
| (devel) | RevaluationScopeSeparation | memory | PASS | not observed | 1ms |
| (devel) | RevaluationScopeSeparation | postgres | PASS | not observed | 446ms |
| (devel) | ReversalAtomicity | memory | PASS | not observed | 1ms |
| (devel) | ReversalAtomicity | postgres | PASS | not observed | 649ms |
| (devel) | RollbackAtomicity | memory | PASS | not observed | 0s |
| (devel) | RollbackAtomicity | postgres | PASS | not observed | 805ms |
| (devel) | SameAccountConcurrentPosting | memory | PASS | not observed | 0s |
| (devel) | SameAccountConcurrentPosting | postgres | PASS | not observed | 666ms |
| (devel) | ScopeViolationRejected | memory | PASS | not observed | 0s |
| (devel) | ScopeViolationRejected | postgres | PASS | not observed | 641ms |
| (devel) | SequenceAllocator | memory | PASS | not observed | 0s |
| (devel) | SequenceAllocator | postgres | PASS | not observed | 722ms |

- **Totals:** 90 passed, 0 failed, 0 skipped.

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
