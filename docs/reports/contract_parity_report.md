# Storage SPI Contract Parity Report — P3-04

- **Generated (UTC):** 2026-11-28T00:09:11Z
- **Engine version:** (devel)
- **Adapters:** Memory (Dev/Test only), PostgreSQL (Production standard).

| Engine | Scenario | Adapter | Result | Semantic differences | Duration |
| :--- | :--- | :--- | :---: | :--- | ---: |
| (devel) | AdapterCapabilitiesValidated | memory | PASS | not observed | 0s |
| (devel) | AdapterCapabilitiesValidated | postgres | PASS | not observed | 460ms |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | memory | PASS | not observed | 0s |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | postgres | PASS | not observed | 392ms |
| (devel) | AuditWriter | memory | PASS | not observed | 0s |
| (devel) | AuditWriter | postgres | PASS | not observed | 414ms |
| (devel) | BalanceReaderAsOfAndCurrency | memory | PASS | not observed | 0s |
| (devel) | BalanceReaderAsOfAndCurrency | postgres | PASS | not observed | 438ms |
| (devel) | BoundedRetryOnUnitOfWork | memory | PASS | not observed | 1ms |
| (devel) | BoundedRetryOnUnitOfWork | postgres | PASS | not observed | 519ms |
| (devel) | CommitPostingAndBalance | memory | PASS | not observed | 0s |
| (devel) | CommitPostingAndBalance | postgres | PASS | not observed | 443ms |
| (devel) | CompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeIsolation | postgres | PASS | not observed | 417ms |
| (devel) | CompanyScopeOnEveryMasterDataPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryMasterDataPort | postgres | PASS | not observed | 469ms |
| (devel) | CompanyScopeOnEveryPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryPort | postgres | PASS | not observed | 422ms |
| (devel) | ConcurrentReplay | memory | PASS | not observed | 0s |
| (devel) | ConcurrentReplay | postgres | PASS | not observed | 483ms |
| (devel) | ConcurrentReplayAtScale | memory | PASS | not observed | 1ms |
| (devel) | ConcurrentReplayAtScale | postgres | PASS | not observed | 450ms |
| (devel) | ConcurrentReversalSingleWinner | memory | PASS | not observed | 1ms |
| (devel) | ConcurrentReversalSingleWinner | postgres | PASS | not observed | 441ms |
| (devel) | ConnectionInterruptionBeforeCommit | memory | PASS | not observed | 0s |
| (devel) | ConnectionInterruptionBeforeCommit | postgres | PASS | not observed | 505ms |
| (devel) | ContextCancellationNoEffect | memory | PASS | not observed | 0s |
| (devel) | ContextCancellationNoEffect | postgres | PASS | not observed | 414ms |
| (devel) | ContractAndAdapterVersion | memory | PASS | not observed | 0s |
| (devel) | ContractAndAdapterVersion | postgres | PASS | not observed | 430ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | memory | PASS | not observed | 0s |
| (devel) | CrossAccountReverseOrderNoDeadlock | postgres | PASS | not observed | 436ms |
| (devel) | DraftStore | memory | PASS | not observed | 0s |
| (devel) | DraftStore | postgres | PASS | not observed | 423ms |
| (devel) | FaultInjectionAtomicity | memory | PASS | not observed | 0s |
| (devel) | FaultInjectionAtomicity | postgres | PASS | not observed | 494ms |
| (devel) | FrozenSnapshotRemainsStable | memory | PASS | not observed | 0s |
| (devel) | FrozenSnapshotRemainsStable | postgres | PASS | not observed | 444ms |
| (devel) | G019EffectiveAccountWindow | memory | PASS | not observed | 0s |
| (devel) | G019EffectiveAccountWindow | postgres | PASS | not observed | 416ms |
| (devel) | G019G022SDKPosting | memory | PASS | not observed | 1ms |
| (devel) | G019G022SDKPosting | postgres | PASS | not observed | 510ms |
| (devel) | G019VersionWritesFailClosed | memory | PASS | not observed | 0s |
| (devel) | G019VersionWritesFailClosed | postgres | PASS | not observed | 443ms |
| (devel) | G022DimensionScopes | memory | PASS | not observed | 0s |
| (devel) | G022DimensionScopes | postgres | PASS | not observed | 470ms |
| (devel) | G022SDKFailClosed | memory | PASS | not observed | 0s |
| (devel) | G022SDKFailClosed | postgres | PASS | not observed | 431ms |
| (devel) | G022SDKForeignDimension | memory | PASS | not observed | 0s |
| (devel) | G022SDKForeignDimension | postgres | PASS | not observed | 477ms |
| (devel) | GLRebuildDeterminismAndParity | memory | PASS | not observed | 1ms |
| (devel) | GLRebuildDeterminismAndParity | postgres | PASS | not observed | 440ms |
| (devel) | IdempotencyLifecycle | memory | PASS | not observed | 0s |
| (devel) | IdempotencyLifecycle | postgres | PASS | not observed | 440ms |
| (devel) | IdempotentReplayAndPayloadConflict | memory | PASS | not observed | 0s |
| (devel) | IdempotentReplayAndPayloadConflict | postgres | PASS | not observed | 412ms |
| (devel) | InterleavedCompanyIsolation | memory | PASS | not observed | 0s |
| (devel) | InterleavedCompanyIsolation | postgres | PASS | not observed | 445ms |
| (devel) | KernelStampedDates | memory | PASS | not observed | 0s |
| (devel) | KernelStampedDates | postgres | PASS | not observed | 412ms |
| (devel) | LockOrderEnforced | memory | PASS | not observed | 0s |
| (devel) | LockOrderEnforced | postgres | PASS | not observed | 447ms |
| (devel) | PolicyVersionCompanyScope | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionCompanyScope | postgres | PASS | not observed | 427ms |
| (devel) | PolicyVersionHistory | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionHistory | postgres | PASS | not observed | 734ms |
| (devel) | RevaluationAuditAction | memory | PASS | not observed | 0s |
| (devel) | RevaluationAuditAction | postgres | PASS | not observed | 417ms |
| (devel) | RevaluationCompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | RevaluationCompanyScopeIsolation | postgres | PASS | not observed | 505ms |
| (devel) | RevaluationConcurrentSingleWinner | memory | PASS | not observed | 0s |
| (devel) | RevaluationConcurrentSingleWinner | postgres | PASS | not observed | 467ms |
| (devel) | RevaluationFaultAfterCommitReplay | memory | PASS | not observed | 0s |
| (devel) | RevaluationFaultAfterCommitReplay | postgres | PASS | not observed | 403ms |
| (devel) | RevaluationPayloadConflict | memory | PASS | not observed | 0s |
| (devel) | RevaluationPayloadConflict | postgres | PASS | not observed | 416ms |
| (devel) | RevaluationReplaySamePayload | memory | PASS | not observed | 0s |
| (devel) | RevaluationReplaySamePayload | postgres | PASS | not observed | 440ms |
| (devel) | RevaluationScopeSeparation | memory | PASS | not observed | 0s |
| (devel) | RevaluationScopeSeparation | postgres | PASS | not observed | 406ms |
| (devel) | ReversalAtomicity | memory | PASS | not observed | 0s |
| (devel) | ReversalAtomicity | postgres | PASS | not observed | 497ms |
| (devel) | RollbackAtomicity | memory | PASS | not observed | 0s |
| (devel) | RollbackAtomicity | postgres | PASS | not observed | 540ms |
| (devel) | SameAccountConcurrentPosting | memory | PASS | not observed | 0s |
| (devel) | SameAccountConcurrentPosting | postgres | PASS | not observed | 451ms |
| (devel) | ScopeViolationRejected | memory | PASS | not observed | 0s |
| (devel) | ScopeViolationRejected | postgres | PASS | not observed | 466ms |
| (devel) | SequenceAllocator | memory | PASS | not observed | 0s |
| (devel) | SequenceAllocator | postgres | PASS | not observed | 431ms |

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
