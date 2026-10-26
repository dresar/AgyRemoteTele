package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAddJobInterval(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	s := NewScheduler(store, nil)

	specs := []string{"@every 1m", "@every 30m", "@every 1h", "@every 500ms"}
	for _, spec := range specs {
		job, err := s.AddJob(spec, "echo test", 12345)
		if err != nil {
			t.Fatalf("expected valid spec %s, got error: %v", spec, err)
		}
		if job.ID == "" {
			t.Errorf("expected non-empty job ID")
		}
		if job.NextRun.Before(time.Now()) {
			t.Errorf("expected next run in future for %s", spec)
		}
	}

	invalidSpecs := []string{"@every", "@every abc", "@every -10m", "@every 0s"}
	for _, spec := range invalidSpecs {
		_, err := s.AddJob(spec, "echo bad", 12345)
		if err == nil {
			t.Errorf("expected error for invalid spec: %s", spec)
		}
	}
}

func TestAddJobCron(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	s := NewScheduler(store, nil)

	validSpecs := []string{
		"* * * * *",
		"0 * * * *",
		"*/15 * * * *",
		"30 4 * * *",
		"0 0 * * 0",
		"15,45 * * * *",
		"0 9-17 * * 1-5",
	}

	for _, spec := range validSpecs {
		job, err := s.AddJob(spec, "ps aux", 999)
		if err != nil {
			t.Fatalf("expected valid spec %s, got error: %v", spec, err)
		}
		if job.NextRun.Before(time.Now()) {
			t.Errorf("expected next run in future for spec %s", spec)
		}
	}

	invalidSpecs := []string{
		"not enough fields",
		"60 * * * *",
		"* 24 * * *",
		"* * 32 * *",
		"* * * 13 *",
		"* * * * 8",
		"*/0 * * * *",
		"10-5 * * * *",
	}

	for _, spec := range invalidSpecs {
		_, err := s.AddJob(spec, "fail", 999)
		if err == nil {
			t.Errorf("expected error for invalid cron spec: %s", spec)
		}
	}
}

func TestRemoveJob(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	s := NewScheduler(store, nil)
	job, err := s.AddJob("@every 10m", "uptime", 100)
	if err != nil {
		t.Fatalf("failed to add job: %v", err)
	}

	if !s.RemoveJob(job.ID) {
		t.Errorf("expected job to be removed successfully")
	}

	if s.RemoveJob(job.ID) {
		t.Errorf("expected second removal to return false")
	}

	if len(s.ListJobs()) != 0 {
		t.Errorf("expected 0 jobs remaining, got %d", len(s.ListJobs()))
	}
}

func TestListJobs(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	s := NewScheduler(store, nil)
	_, _ = s.AddJob("@every 1m", "cmd1", 1)
	_, _ = s.AddJob("@every 2m", "cmd2", 2)
	_, _ = s.AddJob("@every 3m", "cmd3", 3)

	jobs := s.ListJobs()
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	for i := 1; i < len(jobs); i++ {
		if jobs[i].CreatedAt.Before(jobs[i-1].CreatedAt) {
			t.Errorf("expected jobs to be sorted chronologically")
		}
	}
}

func TestRunJobNow(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	var dispatchedChatID int64
	var dispatchedCmd string
	var mu sync.Mutex

	dispatch := func(chatID int64, cmd string) {
		mu.Lock()
		defer mu.Unlock()
		dispatchedChatID = chatID
		dispatchedCmd = cmd
	}

	s := NewScheduler(store, dispatch)
	job, err := s.AddJob("@every 1h", "backup run", 555)
	if err != nil {
		t.Fatalf("failed to add job: %v", err)
	}

	if err := s.RunJobNow(job.ID); err != nil {
		t.Fatalf("run job now failed: %v", err)
	}

	mu.Lock()
	if dispatchedChatID != 555 || dispatchedCmd != "backup run" {
		t.Errorf("unexpected dispatch params: chatID=%d, cmd=%s", dispatchedChatID, dispatchedCmd)
	}
	mu.Unlock()

	jobs := s.ListJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].RunCount != 1 {
		t.Errorf("expected RunCount to be 1, got %d", jobs[0].RunCount)
	}
	if jobs[0].LastRun == nil {
		t.Errorf("expected LastRun to be set")
	}
	if jobs[0].LastResult != "executed" {
		t.Errorf("expected LastResult to be 'executed', got %s", jobs[0].LastResult)
	}

	if err := s.RunJobNow("nonexistent_id"); err == nil {
		t.Errorf("expected error when running nonexistent job")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "sub", "cron_jobs.json")

	s1 := NewScheduler(store, nil)
	j1, _ := s1.AddJob("@every 5m", "echo 1", 11)
	j2, _ := s1.AddJob("0 * * * *", "echo 2", 22)

	if err := s1.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := os.Stat(store); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	s2 := NewScheduler(store, nil)
	if err := s2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	loaded := s2.ListJobs()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 jobs loaded, got %d", len(loaded))
	}

	foundMap := make(map[string]bool)
	for _, j := range loaded {
		foundMap[j.ID] = true
	}
	if !foundMap[j1.ID] || !foundMap[j2.ID] {
		t.Errorf("loaded jobs missing expected IDs")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	var runCount atomic.Int32
	dispatch := func(chatID int64, cmd string) {
		runCount.Add(1)
	}

	s := NewScheduler(store, dispatch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	_, err := s.AddJob("@every 50ms", "fast_cmd", 100)
	if err != nil {
		t.Fatalf("failed to add fast job: %v", err)
	}

	time.Sleep(250 * time.Millisecond)

	s.Stop()

	countAfterStop := runCount.Load()
	if countAfterStop == 0 {
		t.Errorf("expected job to run at least once, got 0")
	}

	time.Sleep(100 * time.Millisecond)
	if runCount.Load() != countAfterStop {
		t.Errorf("expected no runs after Stop, got %d vs %d", runCount.Load(), countAfterStop)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	store := filepath.Join(tmpDir, "cron.json")

	s := NewScheduler(store, func(chatID int64, cmd string) {})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			job, err := s.AddJob("@every 10m", "concur_cmd", int64(id))
			if err == nil {
				_ = s.RunJobNow(job.ID)
				_ = s.ListJobs()
				_ = s.RemoveJob(job.ID)
			}
		}(i)
	}
	wg.Wait()
}
