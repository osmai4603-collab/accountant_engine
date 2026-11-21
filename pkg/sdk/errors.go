package sdk

import (
	"errors"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
)

var (
	// ErrCapabilityNotImplemented reports that an optional engine capability was
	// not configured at build time.
	ErrCapabilityNotImplemented = types.ErrCapabilityNotImplemented

	// Storage and builder errors
	ErrStoragePortsRequired = errors.New("storage: WithStoragePorts is required")
	ErrBuilderOptionNil     = errors.New("builder option is nil")

	// Event ingestion errors
	ErrIdempotencyKeyConflict      = errors.New("idempotency key already used with a different payload")
	ErrIdempotencyInProgress       = errors.New("idempotency key processing is in progress")
	ErrIdempotencyTerminalFailure  = errors.New("idempotency key previously failed terminally")
	ErrIdempotencyStateUnavailable = errors.New("durable event idempotency store is unavailable")
	ErrEventMappingNotFound        = errors.New("no account mapping registered for event type")
	ErrExchangeRateUnavailable     = errors.New("exchange rate source is unavailable")
	ErrEventDimensionsUnsupported  = errors.New("event dimensions cannot be resolved to approved master data")
	ErrEventPayloadUnsupported     = errors.New("event payload requires a template that explicitly maps payload fields")
	ErrIdempotencyRecordNotFound   = publicevents.ErrRecordNotFound
	ErrIdempotencyStateConflict    = publicevents.ErrStateConflict
)

