module github.com/osmai4603-collab/accounting_engine/test/external_consumer

go 1.26.0

require (
	github.com/osmai4603-collab/accounting_engine v0.0.0
	github.com/shopspring/decimal v1.6.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/osmai4603-collab/accounting_engine => ../..
