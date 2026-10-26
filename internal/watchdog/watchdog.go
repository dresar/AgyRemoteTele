package watchdog

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var profileRegex = regexp.MustCompile(`^agy(0[1-9]|1[0-5])$`)

type AlertConfig struct {
	CheckInterval       time.Duration
	HighMemThreshold    float64
	CritMemThreshold    float64
	HighDiskThreshold   float64
	TokenWarningMinutes int
}

type AlertEvent struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Metric    string    `json:"metric"`
	Message   string    `json:"message"`
	Resolved  bool      `json:"resolved"`
}

type Watchdog struct {
	mu              sync.RWMutex
	cfg             AlertConfig
	notifyFunc      func(level string, title string, message string)
	recentAlerts    []AlertEvent
	lastNotified    map[string]time.Time
	cooldown        time.Duration
	profilesRoot    string
	meminfoPath     string
	diskPath        string
	stopChan        chan struct{}
	running         bool
	wg              sync.WaitGroup
	AutoRefreshFunc func(name string) error
}

func defaultAutoRefresh(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "/opt/data/home/agy-profiles/fast_login.py", "refresh", name)
	return cmd.Run()
}

func NewWatchdog(cfg AlertConfig, notifyFunc func(level string, title string, message string)) *Watchdog {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.HighMemThreshold <= 0 {
		cfg.HighMemThreshold = 85.0
	}
	if cfg.CritMemThreshold <= 0 {
		cfg.CritMemThreshold = 92.0
	}
	if cfg.HighDiskThreshold <= 0 {
		cfg.HighDiskThreshold = 90.0
	}
	if cfg.TokenWarningMinutes <= 0 {
		cfg.TokenWarningMinutes = 15
	}

	return &Watchdog{
		cfg:             cfg,
		notifyFunc:      notifyFunc,
		recentAlerts:    make([]AlertEvent, 0),
		lastNotified:    make(map[string]time.Time),
		cooldown:        15 * time.Minute,
		profilesRoot:    "/opt/data/home/agy-profiles",
		meminfoPath:     "/proc/meminfo",
		diskPath:        "/",
		AutoRefreshFunc: defaultAutoRefresh,
	}
}

func (w *Watchdog) SetMeminfoPath(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.meminfoPath = p
}

func (w *Watchdog) SetDiskPath(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.diskPath = p
}

func (w *Watchdog) SetProfilesRoot(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.profilesRoot = p
}

func (w *Watchdog) SetCooldown(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cooldown = d
}

func (w *Watchdog) SetAutoRefreshFunc(fn func(name string) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.AutoRefreshFunc = fn
}

func (w *Watchdog) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopChan = make(chan struct{})
	w.mu.Unlock()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopChan:
				return
			case <-ticker.C:
				w.CheckNow()
			}
		}
	}()
}

func (w *Watchdog) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopChan)
	w.mu.Unlock()

	w.wg.Wait()
}

func (w *Watchdog) GetRecentAlerts() []AlertEvent {
	w.mu.RLock()
	defer w.mu.RUnlock()

	res := make([]AlertEvent, len(w.recentAlerts))
	copy(res, w.recentAlerts)
	return res
}

func (w *Watchdog) CheckNow() []AlertEvent {
	var currentAlerts []AlertEvent

	ramAlerts := w.checkMemory()
	currentAlerts = append(currentAlerts, ramAlerts...)

	diskAlerts := w.checkDisk()
	currentAlerts = append(currentAlerts, diskAlerts...)

	tokenAlerts := w.checkTokens()
	currentAlerts = append(currentAlerts, tokenAlerts...)

	for _, alert := range currentAlerts {
		dedupKey := fmt.Sprintf("%s:%s", alert.Metric, alert.Level)
		w.processAlert(alert, dedupKey)
	}

	return currentAlerts
}

func (w *Watchdog) checkMemory() []AlertEvent {
	w.mu.RLock()
	path := w.meminfoPath
	highThresh := w.cfg.HighMemThreshold
	critThresh := w.cfg.CritMemThreshold
	w.mu.RUnlock()

	perc, err := readMemUsage(path)
	if err != nil {
		return nil
	}

	w.mu.Lock()
	if perc < highThresh {
		delete(w.lastNotified, "RAM:WARNING")
		delete(w.lastNotified, "RAM:CRITICAL")
	}
	w.mu.Unlock()

	if perc >= critThresh {
		return []AlertEvent{{
			ID:        fmt.Sprintf("ram_%d", time.Now().UnixNano()),
			Timestamp: time.Now(),
			Level:     "CRITICAL",
			Metric:    "RAM",
			Message:   fmt.Sprintf("RAM usage critical: %.1f%% (threshold %.1f%%)", perc, critThresh),
		}}
	} else if perc >= highThresh {
		return []AlertEvent{{
			ID:        fmt.Sprintf("ram_%d", time.Now().UnixNano()),
			Timestamp: time.Now(),
			Level:     "WARNING",
			Metric:    "RAM",
			Message:   fmt.Sprintf("RAM usage high: %.1f%% (threshold %.1f%%)", perc, highThresh),
		}}
	}

	return nil
}

func (w *Watchdog) checkDisk() []AlertEvent {
	w.mu.RLock()
	path := w.diskPath
	thresh := w.cfg.HighDiskThreshold
	w.mu.RUnlock()

	perc, err := readDiskUsage(path)
	if err != nil {
		return nil
	}

	w.mu.Lock()
	if perc < thresh {
		delete(w.lastNotified, "DISK:WARNING")
	}
	w.mu.Unlock()

	if perc >= thresh {
		return []AlertEvent{{
			ID:        fmt.Sprintf("disk_%d", time.Now().UnixNano()),
			Timestamp: time.Now(),
			Level:     "WARNING",
			Metric:    "DISK",
			Message:   fmt.Sprintf("Disk usage high on %s: %.1f%% (threshold %.1f%%)", path, perc, thresh),
		}}
	}

	return nil
}

func (w *Watchdog) checkTokens() []AlertEvent {
	w.mu.RLock()
	root := w.profilesRoot
	warnMin := w.cfg.TokenWarningMinutes
	refreshFn := w.AutoRefreshFunc
	w.mu.RUnlock()

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var alerts []AlertEvent
	warnDuration := time.Duration(warnMin) * time.Minute
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !profileRegex.MatchString(name) {
			continue
		}
		tokenPath := filepath.Join(root, name, ".gemini/antigravity-cli/antigravity-oauth-token")
		exp, err := readTokenExpiry(tokenPath)
		if err != nil || exp.IsZero() {
			continue
		}

		rem := exp.Sub(now)
		if rem <= warnDuration {
			if refreshFn != nil {
				refreshErr := refreshFn(name)
				if refreshErr != nil {
					alerts = append(alerts, AlertEvent{
						ID:        fmt.Sprintf("token_%s_%d", name, time.Now().UnixNano()),
						Timestamp: now,
						Level:     "CRITICAL",
						Metric:    fmt.Sprintf("TOKEN_%s", name),
						Message:   fmt.Sprintf("Token refresh failed for %s: %v", name, refreshErr),
					})
					continue
				}

				newExp, err := readTokenExpiry(tokenPath)
				if err != nil || newExp.IsZero() {
					alerts = append(alerts, AlertEvent{
						ID:        fmt.Sprintf("token_%s_%d", name, time.Now().UnixNano()),
						Timestamp: now,
						Level:     "CRITICAL",
						Metric:    fmt.Sprintf("TOKEN_%s", name),
						Message:   fmt.Sprintf("Token refresh failed for %s: %v", name, err),
					})
					continue
				}

				newRem := newExp.Sub(time.Now())
				if newRem > 0 {
					log.Printf("Token for %s is valid (%d minutes remaining)", name, int(newRem.Minutes()))
					w.mu.Lock()
					delete(w.lastNotified, fmt.Sprintf("TOKEN_%s:WARNING", name))
					delete(w.lastNotified, fmt.Sprintf("TOKEN_%s:CRITICAL", name))
					w.mu.Unlock()
					continue
				}

				alerts = append(alerts, AlertEvent{
					ID:        fmt.Sprintf("token_%s_%d", name, time.Now().UnixNano()),
					Timestamp: now,
					Level:     "CRITICAL",
					Metric:    fmt.Sprintf("TOKEN_%s", name),
					Message:   fmt.Sprintf("Token refresh failed for %s: token still expired", name),
				})
				continue
			}

			level := "WARNING"
			msg := fmt.Sprintf("Token for %s expires in %d minutes", name, int(rem.Minutes()))
			if rem <= 0 {
				level = "CRITICAL"
				msg = fmt.Sprintf("Token for %s expired at %s", name, exp.UTC().Format(time.RFC3339))
			}
			alerts = append(alerts, AlertEvent{
				ID:        fmt.Sprintf("token_%s_%d", name, time.Now().UnixNano()),
				Timestamp: now,
				Level:     level,
				Metric:    fmt.Sprintf("TOKEN_%s", name),
				Message:   msg,
			})
		} else {
			w.mu.Lock()
			delete(w.lastNotified, fmt.Sprintf("TOKEN_%s:WARNING", name))
			delete(w.lastNotified, fmt.Sprintf("TOKEN_%s:CRITICAL", name))
			w.mu.Unlock()
		}
	}

	return alerts
}

func (w *Watchdog) processAlert(alert AlertEvent, dedupKey string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.recentAlerts = append(w.recentAlerts, alert)
	if len(w.recentAlerts) > 100 {
		w.recentAlerts = w.recentAlerts[len(w.recentAlerts)-100:]
	}

	last, exists := w.lastNotified[dedupKey]
	if exists && time.Since(last) < w.cooldown {
		return
	}

	w.lastNotified[dedupKey] = time.Now()
	if w.notifyFunc != nil {
		go w.notifyFunc(alert.Level, fmt.Sprintf("%s Alert", alert.Metric), alert.Message)
	}
}

func readMemUsage(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var memTotal, memAvailable, memFree, buffers, cached uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		parts := strings.Fields(s.Text())
		if len(parts) < 2 {
			continue
		}
		val, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		switch parts[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		case "MemFree:":
			memFree = val
		case "Buffers:":
			buffers = val
		case "Cached:":
			cached = val
		}
	}

	if memTotal == 0 {
		return 0, fmt.Errorf("invalid meminfo: memTotal is 0")
	}

	var used uint64
	if memAvailable > 0 {
		used = memTotal - memAvailable
	} else {
		used = memTotal - (memFree + buffers + cached)
	}

	return (float64(used) / float64(memTotal)) * 100.0, nil
}

func readDiskUsage(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	if stat.Blocks == 0 {
		return 0, fmt.Errorf("invalid blocks")
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	return (float64(used) / float64(total)) * 100.0, nil
}

func readTokenExpiry(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}

	type tokenDoc struct {
		Token struct {
			Expiry string `json:"expiry"`
		} `json:"token"`
		IDToken string `json:"id_token"`
	}

	var doc tokenDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return time.Time{}, err
	}

	if doc.Token.Expiry != "" {
		t, err := time.Parse(time.RFC3339Nano, doc.Token.Expiry)
		if err == nil {
			return t, nil
		}
		t, err = time.Parse(time.RFC3339, doc.Token.Expiry)
		if err == nil {
			return t, nil
		}
	}

	if doc.IDToken != "" {
		parts := strings.Split(doc.IDToken, ".")
		if len(parts) >= 2 {
			jwtPayload := parts[1]
			if rem := len(jwtPayload) % 4; rem > 0 {
				jwtPayload += strings.Repeat("=", 4-rem)
			}
			raw, err := base64.URLEncoding.DecodeString(jwtPayload)
			if err == nil {
				type payload struct {
					Exp int64 `json:"exp"`
				}
				var p payload
				if err := json.Unmarshal(raw, &p); err == nil && p.Exp > 0 {
					return time.Unix(p.Exp, 0), nil
				}
			}
		}
	}

	return time.Time{}, fmt.Errorf("no expiry found")
}
