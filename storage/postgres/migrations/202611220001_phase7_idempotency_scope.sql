-- Phase 7: the durable idempotency identity is the kernel-stamped scope,
-- not the bare caller key. Backfill existing rows so post and reversal keys
-- remain distinguishable by operation after the primary key changes.
ALTER TABLE posting_idempotency ADD COLUMN IF NOT EXISTS scope varchar(512);

UPDATE posting_idempotency
SET scope = 'company=' || company_id::text || '|operation=' || operation || '|key=' || idempotency_key
WHERE scope IS NULL OR scope = '';

ALTER TABLE posting_idempotency ALTER COLUMN scope SET NOT NULL;

ALTER TABLE posting_idempotency DROP CONSTRAINT IF EXISTS posting_idempotency_pkey;
ALTER TABLE posting_idempotency ADD CONSTRAINT posting_idempotency_pkey PRIMARY KEY (company_id, scope);

CREATE UNIQUE INDEX IF NOT EXISTS uq_posting_idempotency_company_operation_key
    ON posting_idempotency (company_id, operation, idempotency_key);
