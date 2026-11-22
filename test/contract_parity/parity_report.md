# Storage SPI Contract Parity Report — P3-04

- **Generated (UTC):** 2026-11-22T03:15:14Z
- **Engine version:** (devel)
- **Adapters:** Memory (Dev/Test only), PostgreSQL (Production standard).

| Engine | Scenario | Adapter | Result | Semantic differences | Duration |
| :--- | :--- | :--- | :---: | :--- | ---: |
| (devel) | AdapterCapabilitiesValidated | memory | PASS | not observed | 0s |
| (devel) | AdapterCapabilitiesValidated | postgres | PASS | not observed | 569ms |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | memory | PASS | not observed | 0s |
| (devel) | AuditAtomicAppendSnapshotAndCheckpoint | postgres | PASS | not observed | 546ms |
| (devel) | AuditWriter | memory | PASS | not observed | 0s |
| (devel) | AuditWriter | postgres | PASS | not observed | 683ms |
| (devel) | BalanceReaderAsOfAndCurrency | memory | PASS | not observed | 2ms |
| (devel) | BalanceReaderAsOfAndCurrency | postgres | PASS | not observed | 456ms |
| (devel) | BoundedRetryOnUnitOfWork | memory | PASS | not observed | 2ms |
| (devel) | BoundedRetryOnUnitOfWork | postgres | PASS | not observed | 580ms |
| (devel) | CommitPostingAndBalance | memory | PASS | not observed | 1ms |
| (devel) | CommitPostingAndBalance | postgres | PASS | not observed | 550ms |
| (devel) | CompanyScopeIsolation | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeIsolation | postgres | PASS | not observed | 358ms |
| (devel) | CompanyScopeOnEveryMasterDataPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryMasterDataPort | postgres | PASS | not observed | 409ms |
| (devel) | CompanyScopeOnEveryPort | memory | PASS | not observed | 0s |
| (devel) | CompanyScopeOnEveryPort | postgres | PASS | not observed | 437ms |
| (devel) | ConcurrentReplay | memory | PASS | not observed | 1ms |
| (devel) | ConcurrentReplay | postgres | PASS | not observed | 388ms |
| (devel) | ConcurrentReplayAtScale | memory | PASS | not observed | 6ms |
| (devel) | ConcurrentReplayAtScale | postgres | PASS | not observed | 451ms |
| (devel) | ConcurrentReversalSingleWinner | memory | PASS | not observed | 4ms |
| (devel) | ConcurrentReversalSingleWinner | postgres | PASS | not observed | 483ms |
| (devel) | ConnectionInterruptionBeforeCommit | memory | PASS | not observed | 1ms |
| (devel) | ConnectionInterruptionBeforeCommit | postgres | PASS | not observed | 406ms |
| (devel) | ContextCancellationNoEffect | memory | PASS | not observed | 0s |
| (devel) | ContextCancellationNoEffect | postgres | PASS | not observed | 466ms |
| (devel) | ContractAndAdapterVersion | memory | PASS | not observed | 0s |
| (devel) | ContractAndAdapterVersion | postgres | PASS | not observed | 577ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | memory | PASS | not observed | 1ms |
| (devel) | CrossAccountReverseOrderNoDeadlock | postgres | PASS | not observed | 431ms |
| (devel) | DraftStore | memory | PASS | not observed | 1ms |
| (devel) | DraftStore | postgres | PASS | not observed | 481ms |
| (devel) | FaultInjectionAtomicity | memory | PASS | not observed | 1ms |
| (devel) | FaultInjectionAtomicity | postgres | PASS | not observed | 441ms |
| (devel) | FrozenSnapshotRemainsStable | memory | PASS | not observed | 1ms |
| (devel) | FrozenSnapshotRemainsStable | postgres | PASS | not observed | 418ms |
| (devel) | G019EffectiveAccountWindow | memory | PASS | not observed | 0s |
| (devel) | G019EffectiveAccountWindow | postgres | PASS | not observed | 385ms |
| (devel) | G019G022SDKPosting | memory | PASS | not observed | 1ms |
| (devel) | G019G022SDKPosting | postgres | PASS | not observed | 470ms |
| (devel) | G019VersionWritesFailClosed | memory | PASS | not observed | 0s |
| (devel) | G019VersionWritesFailClosed | postgres | PASS | not observed | 387ms |
| (devel) | G022DimensionScopes | memory | PASS | not observed | 0s |
| (devel) | G022DimensionScopes | postgres | PASS | not observed | 372ms |
| (devel) | G022SDKFailClosed | memory | PASS | not observed | 1ms |
| (devel) | G022SDKFailClosed | postgres | PASS | not observed | 598ms |
| (devel) | G022SDKForeignDimension | memory | PASS | not observed | 1ms |
| (devel) | G022SDKForeignDimension | postgres | PASS | not observed | 546ms |
| (devel) | GLRebuildDeterminismAndParity | memory | PASS | not observed | 2ms |
| (devel) | GLRebuildDeterminismAndParity | postgres | PASS | not observed | 437ms |
| (devel) | IdempotencyLifecycle | memory | PASS | not observed | 1ms |
| (devel) | IdempotencyLifecycle | postgres | PASS | not observed | 732ms |
| (devel) | IdempotentReplayAndPayloadConflict | memory | PASS | not observed | 1ms |
| (devel) | IdempotentReplayAndPayloadConflict | postgres | PASS | not observed | 437ms |
| (devel) | InterleavedCompanyIsolation | memory | PASS | not observed | 1ms |
| (devel) | InterleavedCompanyIsolation | postgres | PASS | not observed | 432ms |
| (devel) | KernelStampedDates | memory | PASS | not observed | 0s |
| (devel) | KernelStampedDates | postgres | PASS | not observed | 406ms |
| (devel) | LockOrderEnforced | memory | PASS | not observed | 0s |
| (devel) | LockOrderEnforced | postgres | PASS | not observed | 374ms |
| (devel) | PolicyVersionCompanyScope | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionCompanyScope | postgres | PASS | not observed | 451ms |
| (devel) | PolicyVersionHistory | memory | PASS | not observed | 0s |
| (devel) | PolicyVersionHistory | postgres | PASS | not observed | 539ms |
| (devel) | RevaluationAuditAction | memory | PASS | not observed | 1ms |
| (devel) | RevaluationAuditAction | postgres | PASS | not observed | 451ms |
| (devel) | RevaluationCompanyScopeIsolation | memory | PASS | not observed | 1ms |
| (devel) | RevaluationCompanyScopeIsolation | postgres | PASS | not observed | 431ms |
| (devel) | RevaluationConcurrentSingleWinner | memory | PASS | not observed | 2ms |
| (devel) | RevaluationConcurrentSingleWinner | postgres | PASS | not observed | 414ms |
| (devel) | RevaluationFaultAfterCommitReplay | memory | PASS | not observed | 1ms |
| (devel) | RevaluationFaultAfterCommitReplay | postgres | PASS | not observed | 394ms |
| (devel) | RevaluationPayloadConflict | memory | PASS | not observed | 1ms |
| (devel) | RevaluationPayloadConflict | postgres | PASS | not observed | 426ms |
| (devel) | RevaluationReplaySamePayload | memory | PASS | not observed | 1ms |
| (devel) | RevaluationReplaySamePayload | postgres | PASS | not observed | 386ms |
| (devel) | RevaluationScopeSeparation | memory | PASS | not observed | 2ms |
| (devel) | RevaluationScopeSeparation | postgres | PASS | not observed | 416ms |
| (devel) | ReversalAtomicity | memory | PASS | not observed | 1ms |
| (devel) | ReversalAtomicity | postgres | PASS | not observed | 432ms |
| (devel) | RollbackAtomicity | memory | PASS | not observed | 1ms |
| (devel) | RollbackAtomicity | postgres | PASS | not observed | 854ms |
| (devel) | SameAccountConcurrentPosting | memory | PASS | not observed | 1ms |
| (devel) | SameAccountConcurrentPosting | postgres | PASS | not observed | 474ms |
| (devel) | ScopeViolationRejected | memory | PASS | not observed | 0s |
| (devel) | ScopeViolationRejected | postgres | PASS | not observed | 388ms |
| (devel) | SequenceAllocator | memory | PASS | not observed | 0s |
| (devel) | SequenceAllocator | postgres | PASS | not observed | 752ms |

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
