package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	p50LimitMS = 15.0
	p95LimitMS = 50.0
	tpsTarget  = 1000.0
)

type sloSample struct {
	Scenario string    `json:"scenario"`
	Adapter  string    `json:"adapter"`
	Samples  int       `json:"samples"`
	Duration string    `json:"duration"`
	TPS      float64   `json:"tps"`
	P50MS    float64   `json:"p50_ms"`
	P95MS    float64   `json:"p95_ms"`
	P99MS    float64   `json:"p99_ms"`
	MinMS    float64   `json:"min_ms"`
	MaxMS    float64   `json:"max_ms"`
	Recorded time.Time `json:"recorded_utc"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

// TestPostingLatencySLO is the percentile-based SLO harness required by P7-04.
// It drives the public SDK against an isolated PostgreSQL schema, records the
// latency of every posting operation, and writes the raw aggregate to
// docs/reports/performance_samples.json.
func TestPostingLatencySLO(t *testing.T) {
	if os.Getenv("POSTGRES_URL") == "" {
		t.Skip("latency SLO certification requires an explicit POSTGRES_URL (run T5 with POSTGRES_URL set)")
	}
	h := newPostgresBench(t)
	defer h.cleanup()

	workers := envInt("POSTING_SLO_WORKERS", 16)
	totalOps := envInt("POSTING_SLO_OPS", 6000)

	var next atomic.Int64
	var completed atomic.Int64
	errCh := make(chan error, workers)
	latencies := make([]float64, 0, totalOps)
	var mu sync.Mutex

	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				index := next.Add(1)
				if index > int64(totalOps) {
					return
				}
				opStart := time.Now()
				entry := h.newEntry(t)
				if _, err := h.engine.Ledger().PostEntry(h.postCtx(), entry); err != nil {
					errCh <- err
					return
				}
				elapsed := float64(time.Since(opStart).Microseconds()) / 1000.0
				mu.Lock()
				latencies = append(latencies, elapsed)
				mu.Unlock()
				completed.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("latency SLO posting failed: %v", err)
	}
	elapsed := time.Since(start)
	if completed.Load() != int64(totalOps) {
		t.Fatalf("completed %d operations, want %d", completed.Load(), totalOps)
	}

	sort.Float64s(latencies)
	tps := float64(completed.Load()) / elapsed.Seconds()
	sample := sloSample{
		Scenario: "posting_pipeline",
		Adapter:  "postgres",
		Samples:  len(latencies),
		Duration: elapsed.Round(time.Millisecond).String(),
		TPS:      tps,
		P50MS:    percentile(latencies, 0.50),
		P95MS:    percentile(latencies, 0.95),
		P99MS:    percentile(latencies, 0.99),
		MinMS:    latencies[0],
		MaxMS:    latencies[len(latencies)-1],
		Recorded: time.Now().UTC(),
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	reportDir := filepath.Join(root, "docs", "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("create report directory: %v", err)
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		t.Fatalf("encode samples: %v", err)
	}
	path := filepath.Join(reportDir, "performance_samples.json")
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write samples: %v", err)
	}
	t.Logf("SLO sample written to %s: %+v", path, sample)

	if sample.P50MS > p50LimitMS {
		t.Errorf("P50 %.2f ms exceeds %.2f ms limit", sample.P50MS, p50LimitMS)
	}
	if sample.P95MS > p95LimitMS {
		t.Errorf("P95 %.2f ms exceeds %.2f ms limit", sample.P95MS, p95LimitMS)
	}
	if sample.TPS < tpsTarget {
		t.Errorf("throughput %.1f TPS below %.1f TPS target", sample.TPS, tpsTarget)
	}
}
