DROP INDEX IF EXISTS uq_posting_idempotency_company_operation_key;

ALTER TABLE posting_idempotency DROP CONSTRAINT IF EXISTS posting_idempotency_pkey;
ALTER TABLE posting_idempotency ADD CONSTRAINT posting_idempotency_pkey PRIMARY KEY (company_id, idempotency_key);

ALTER TABLE posting_idempotency DROP COLUMN IF EXISTS scope;
