package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CronJob struct {
	ID         string     `json:"id"`
	Spec       string     `json:"spec"`
	Command    string     `json:"command"`
	ChatID     int64      `json:"chat_id"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    time.Time  `json:"next_run"`
	RunCount   int        `json:"run_count"`
	LastResult string     `json:"last_result,omitempty"`
}

type Scheduler struct {
	mu           sync.RWMutex
	jobs         map[string]*CronJob
	storePath    string
	dispatchFunc func(chatID int64, cmd string)
	stopChan     chan struct{}
	running      bool
	wg           sync.WaitGroup
}

func NewScheduler(storePath string, dispatchFunc func(chatID int64, cmd string)) *Scheduler {
	if storePath == "" {
		storePath = "data/cron_jobs.json"
	}
	return &Scheduler{
		jobs:         make(map[string]*CronJob),
		storePath:    storePath,
		dispatchFunc: dispatchFunc,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	_ = s.loadUnlocked()
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			case now := <-ticker.C:
				s.tick(now)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *Scheduler) AddJob(spec string, command string, chatID int64) (*CronJob, error) {
	now := time.Now()
	nextRun, err := calculateNextRun(spec, now)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	job := &CronJob{
		ID:        fmt.Sprintf("job_%d", time.Now().UnixNano()),
		Spec:      spec,
		Command:   command,
		ChatID:    chatID,
		Enabled:   true,
		CreatedAt: now,
		NextRun:   nextRun,
		RunCount:  0,
	}

	s.jobs[job.ID] = job
	if err := s.saveUnlocked(); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Scheduler) RemoveJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[id]; !exists {
		return false
	}
	delete(s.jobs, id)
	_ = s.saveUnlocked()
	return true
}

func (s *Scheduler) ListJobs() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*CronJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobCopy := *job
		list = append(list, &jobCopy)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list
}

func (s *Scheduler) RunJobNow(id string) error {
	s.mu.Lock()
	job, exists := s.jobs[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("job not found: %s", id)
	}

	now := time.Now()
	job.LastRun = &now
	job.RunCount++
	if next, err := calculateNextRun(job.Spec, now); err == nil {
		job.NextRun = next
	}
	job.LastResult = "executed"

	chatID := job.ChatID
	cmd := job.Command
	_ = s.saveUnlocked()
	s.mu.Unlock()

	if s.dispatchFunc != nil {
		s.dispatchFunc(chatID, cmd)
	}
	return nil
}

func (s *Scheduler) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveUnlocked()
}

func (s *Scheduler) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *Scheduler) tick(now time.Time) {
	s.mu.Lock()
	var toRun []*CronJob
	for _, job := range s.jobs {
		if !job.Enabled {
			continue
		}
		if !job.NextRun.IsZero() && !job.NextRun.After(now) {
			toRun = append(toRun, job)
			last := now
			job.LastRun = &last
			job.RunCount++
			job.LastResult = "dispatched"
			if next, err := calculateNextRun(job.Spec, now); err == nil {
				job.NextRun = next
			}
		}
	}
	if len(toRun) > 0 {
		_ = s.saveUnlocked()
	}
	s.mu.Unlock()

	for _, job := range toRun {
		if s.dispatchFunc != nil {
			go s.dispatchFunc(job.ChatID, job.Command)
		}
	}
}

func (s *Scheduler) saveUnlocked() error {
	if s.storePath == "" {
		return nil
	}

	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	list := make([]*CronJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		list = append(list, job)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.storePath, data, 0644)
}

func (s *Scheduler) loadUnlocked() error {
	if s.storePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []*CronJob
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	now := time.Now()
	for _, job := range list {
		if job.NextRun.IsZero() || job.NextRun.Before(now) {
			if next, err := calculateNextRun(job.Spec, now); err == nil {
				job.NextRun = next
			}
		}
		s.jobs[job.ID] = job
	}
	return nil
}

func calculateNextRun(spec string, from time.Time) (time.Time, error) {
	clean := strings.TrimSpace(spec)
	if strings.HasPrefix(clean, "@every ") {
		dStr := strings.TrimSpace(strings.TrimPrefix(clean, "@every "))
		d, err := time.ParseDuration(dStr)
		if err != nil || d <= 0 {
			return time.Time{}, fmt.Errorf("invalid interval duration: %s", dStr)
		}
		return from.Add(d), nil
	}

	fields := strings.Fields(clean)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression: %s", spec)
	}

	minSched, err := parseField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, err
	}
	hourSched, err := parseField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, err
	}
	domSched, err := parseField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, err
	}
	monthSched, err := parseField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, err
	}
	dowSched, err := parseField(fields[4], 0, 7)
	if err != nil {
		return time.Time{}, err
	}
	if dowSched[7] {
		dowSched[0] = true
	}

	t := from.Truncate(time.Minute).Add(time.Minute)
	maxMinutes := 525600
	for i := 0; i < maxMinutes; i++ {
		if !monthSched[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !domSched[t.Day()] || !dowSched[int(t.Weekday())] {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !hourSched[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !minSched[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}

	return time.Time{}, fmt.Errorf("next run not found for: %s", spec)
}

func parseField(expr string, minVal, maxVal int) (map[int]bool, error) {
	result := make(map[int]bool)
	parts := strings.Split(expr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty expression part in %s", expr)
		}
		if part == "*" {
			for i := minVal; i <= maxVal; i++ {
				result[i] = true
			}
			continue
		}
		if strings.HasPrefix(part, "*/") {
			stepStr := strings.TrimPrefix(part, "*/")
			step, err := strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step in %s", expr)
			}
			for i := minVal; i <= maxVal; i += step {
				result[i] = true
			}
			continue
		}
		if strings.Contains(part, "-") {
			step := 1
			rangePart := part
			if strings.Contains(part, "/") {
				rangeSplit := strings.Split(part, "/")
				if len(rangeSplit) != 2 {
					return nil, fmt.Errorf("invalid range step in %s", expr)
				}
				rangePart = rangeSplit[0]
				var err error
				step, err = strconv.Atoi(rangeSplit[1])
				if err != nil || step <= 0 {
					return nil, fmt.Errorf("invalid range step in %s", expr)
				}
			}
			rangeBounds := strings.Split(rangePart, "-")
			if len(rangeBounds) != 2 {
				return nil, fmt.Errorf("invalid range in %s", expr)
			}
			start, err1 := strconv.Atoi(rangeBounds[0])
			end, err2 := strconv.Atoi(rangeBounds[1])
			if err1 != nil || err2 != nil || start > end || start < minVal || end > maxVal {
				return nil, fmt.Errorf("invalid range bounds in %s", expr)
			}
			for i := start; i <= end; i += step {
				result[i] = true
			}
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil || val < minVal || val > maxVal {
			return nil, fmt.Errorf("value %s out of range [%d-%d]", part, minVal, maxVal)
		}
		result[val] = true
	}
	return result, nil
}
