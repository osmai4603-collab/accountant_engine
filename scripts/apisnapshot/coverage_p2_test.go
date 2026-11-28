package main

import (
	"os"
	"testing"
)

// The API snapshot command intentionally exposes only main. Exercising it with
// standard-library import paths keeps the snapshot path deterministic and does
// not require repository fixtures, a network, or a database.
func TestCoverageAPISnapshotMain(t *testing.T) {
	original := os.Args
	defer func() { os.Args = original }()
	os.Args = []string{"apisnapshot", "fmt", "errors", "time", "go/token", "net/url"}
	main()
}
