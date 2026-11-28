package standards

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type coverageScheduledJob struct {
	name string
	err  error
	runs int
}

func (j *coverageScheduledJob) Name() string { return j.name }
func (j *coverageScheduledJob) Run(context.Context) error {
	j.runs++
	return j.err
}

func TestCoverageSchedulerRegistration(t *testing.T) {
	s := NewDefaultScheduler()
	if s.IsReady() {
		t.Fatal("new scheduler must not be ready")
	}
	s.SetReady(true)
	if !s.IsReady() {
		t.Fatal("SetReady(true) failed")
	}
	s.SetReady(false)

	if err := s.RegisterJob(nil, JobIdempotencyPolicy{}); !errors.Is(err, ErrNilScheduledJob) {
		t.Fatalf("nil job error = %v", err)
	}
	if err := s.RegisterJob(&coverageScheduledJob{name: ""}, JobIdempotencyPolicy{}); err == nil {
		t.Fatal("blank job name must be rejected")
	}

	job := &coverageScheduledJob{name: "daily"}
	if err := s.RegisterJob(job, JobIdempotencyPolicy{}); err != nil {
		t.Fatalf("RegisterJob: %v", err)
	}
	policy := s.policies["daily"]
	if policy.Key != "daily" || policy.LockTimeout != 5*time.Minute {
		t.Fatalf("default policy = %+v", policy)
	}
	custom := JobIdempotencyPolicy{Key: "custom-key", LockTimeout: time.Minute, Idempotent: true}
	if err := s.RegisterJob(job, custom); err != nil {
		t.Fatalf("re-register job: %v", err)
	}
	if got := s.policies["daily"]; got.Key != custom.Key || got.LockTimeout != custom.LockTimeout || !got.Idempotent {
		t.Fatalf("replacement policy = %+v", got)
	}

	if err := s.RunScheduledJob(context.Background(), nil); !errors.Is(err, ErrNilScheduledJob) {
		t.Fatalf("RunScheduledJob(nil) = %v", err)
	}
	if err := s.RunScheduledJob(context.Background(), &coverageScheduledJob{name: "blank-name"}); err == nil {
		t.Fatal("blank job name should fail through RunScheduledJob")
	}
}

func TestCoverageSchedulerRunJobStates(t *testing.T) {
	ctx := context.Background()
	s := NewDefaultScheduler()
	job := &coverageScheduledJob{name: "job-one"}

	if _, err := s.RunJob(ctx, "job-one"); !errors.Is(err, ErrSchedulerNotReady) {
		t.Fatalf("unready scheduler error = %v", err)
	}
	s.SetReady(true)
	if _, err := s.RunJob(ctx, "job-one"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown job error = %v", err)
	}
	if err := s.RegisterJob(job, JobIdempotencyPolicy{}); err != nil {
		t.Fatal(err)
	}

	record, err := s.RunJob(ctx, "job-one")
	if err != nil || record.Status != JobStatusCompleted || record.JobID != "job-one" ||
		record.ExecutionID == "" || record.StartedAt.IsZero() || record.CompletedAt.IsZero() || record.Error != "" {
		t.Fatalf("completed record = %+v, %v", record, err)
	}
	if job.runs != 1 {
		t.Fatalf("job runs = %d", job.runs)
	}

	// Manual lock simulation covers the concurrency guard deterministically.
	s.activeLocks["job-one"] = time.Now().Add(time.Hour)
	if _, err := s.RunJob(ctx, "job-one"); !errors.Is(err, ErrDuplicateJobExecution) ||
		!strings.Contains(err.Error(), "locked") {
		t.Fatalf("locked job error = %v", err)
	}
	s.activeLocks["job-one"] = time.Now().Add(-time.Hour)
	if _, err := s.RunJob(ctx, "job-one"); err != nil {
		t.Fatalf("expired lock should release job: %v", err)
	}

	failing := &coverageScheduledJob{name: "failing", err: errors.New("boom")}
	if err := s.RegisterJob(failing, JobIdempotencyPolicy{Key: "failing"}); err != nil {
		t.Fatal(err)
	}
	failedRecord, err := s.RunJob(ctx, "failing")
	if !errors.Is(err, failing.err) || failedRecord.Status != JobStatusFailed || failedRecord.Error != "boom" {
		t.Fatalf("failed record = %+v, %v", failedRecord, err)
	}

	idempotent := &coverageScheduledJob{name: "idempotent"}
	if err := s.RegisterJob(idempotent, JobIdempotencyPolicy{Key: "idempotent", Idempotent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunJob(ctx, "idempotent"); err != nil {
		t.Fatalf("first idempotent run: %v", err)
	}
	replay, err := s.RunJob(ctx, "idempotent")
	if !errors.Is(err, ErrDuplicateJobExecution) || replay.Status != JobStatusCompleted ||
		!strings.Contains(err.Error(), "already completed") {
		t.Fatalf("idempotent replay = %+v, %v", replay, err)
	}

	history := s.GetExecutionHistory("job-one")
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	history[0].Status = JobStatusPending
	if s.GetExecutionHistory("job-one")[0].Status == JobStatusPending {
		t.Fatal("execution history must be copied")
	}
	if len(s.GetExecutionHistory("unknown")) != 0 {
		t.Fatal("unknown job should have empty history")
	}
}

func TestCoverageSchedulerRunAllJobs(t *testing.T) {
	ctx := context.Background()
	s := NewDefaultScheduler()
	if results := s.RunAllScheduledJobs(ctx); len(results) != 0 {
		t.Fatalf("unready scheduler results = %v", results)
	}
	s.SetReady(true)

	good := &coverageScheduledJob{name: "good"}
	bad := &coverageScheduledJob{name: "bad", err: errors.New("bad job")}
	if err := s.RegisterJob(good, JobIdempotencyPolicy{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterJob(bad, JobIdempotencyPolicy{}); err != nil {
		t.Fatal(err)
	}
	results := s.RunAllScheduledJobs(ctx)
	if len(results) != 2 || results["good"] != nil || !errors.Is(results["bad"], bad.err) {
		t.Fatalf("run all results = %v", results)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	results = s.RunAllScheduledJobs(cancelled)
	for name, err := range results {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled run of %s = %v", name, err)
		}
	}
}

func TestCoverageSchedulerRunScheduledJobRegisters(t *testing.T) {
	s := NewDefaultScheduler()
	s.SetReady(true)
	job := &coverageScheduledJob{name: "on-demand"}
	if err := s.RunScheduledJob(context.Background(), job); err != nil {
		t.Fatalf("RunScheduledJob: %v", err)
	}
	if job.runs != 1 {
		t.Fatalf("job runs = %d", job.runs)
	}
	if err := s.RunScheduledJob(context.Background(), job); err != nil {
		t.Fatalf("re-run registered job: %v", err)
	}
	if job.runs != 2 {
		t.Fatalf("job runs after re-run = %d", job.runs)
	}
}
