package system

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Metrics struct {
	AllocatedVCPU float64
	LoadAvg1      float64
	LoadAvg5      float64
	LoadAvg15     float64
	MemTotalMB    uint64
	MemUsedMB     uint64
	MemFreeMB     uint64
	MemPercent    float64
	DiskTotalGB   float64
	DiskUsedGB    float64
	DiskFreeGB    float64
	DiskPercent   float64
	SystemUptime  time.Duration
	Goroutines    int
	BotAllocMB    float64
	BotSysMB      float64
	BotRSSMB      float64
	HeapInUseMB   float64
	StackInUseMB  float64
	NumGC         uint32
}

func CollectMetrics(diskPath string) (*Metrics, error) {
	if diskPath == "" {
		diskPath = "/"
	}

	vcpu := readCgroupCPU()
	load1, load5, load15 := readLoadAvg()
	memTotal, memUsed, memFree, memPct := readMemory()
	diskTotal, diskUsed, diskFree, diskPct := readDisk(diskPath)
	uptime := readUptime()
	rssMB := readProcessRSS()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	botAllocMB := float64(m.Alloc) / (1024 * 1024)
	botSysMB := float64(m.Sys) / (1024 * 1024)
	heapInUseMB := float64(m.HeapInuse) / (1024 * 1024)
	stackInUseMB := float64(m.StackInuse) / (1024 * 1024)

	return &Metrics{
		AllocatedVCPU: vcpu,
		LoadAvg1:      load1,
		LoadAvg5:      load5,
		LoadAvg15:     load15,
		MemTotalMB:    memTotal,
		MemUsedMB:     memUsed,
		MemFreeMB:     memFree,
		MemPercent:    memPct,
		DiskTotalGB:   diskTotal,
		DiskUsedGB:    diskUsed,
		DiskFreeGB:    diskFree,
		DiskPercent:   diskPct,
		SystemUptime:  uptime,
		Goroutines:    runtime.NumGoroutine(),
		BotAllocMB:    botAllocMB,
		BotSysMB:      botSysMB,
		BotRSSMB:      rssMB,
		HeapInUseMB:   heapInUseMB,
		StackInUseMB:  stackInUseMB,
		NumGC:         m.NumGC,
	}, nil
}

func readCgroupCPU() float64 {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err == nil {
		fields := strings.Fields(strings.TrimSpace(string(data)))
		if len(fields) >= 2 && fields[0] != "max" {
			quota, errQ := strconv.ParseFloat(fields[0], 64)
			period, errP := strconv.ParseFloat(fields[1], 64)
			if errQ == nil && errP == nil && period > 0 {
				return quota / period
			}
		}
	}
	return float64(runtime.NumCPU())
}

func readLoadAvg() (float64, float64, float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}

	l1, _ := strconv.ParseFloat(fields[0], 64)
	l5, _ := strconv.ParseFloat(fields[1], 64)
	l15, _ := strconv.ParseFloat(fields[2], 64)
	return l1, l5, l15
}

func readMemory() (uint64, uint64, uint64, float64) {
	if totalMB, usedMB, freeMB, pct, ok := readCgroupMemory(); ok {
		return totalMB, usedMB, freeMB, pct
	}

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0
	}
	defer file.Close()

	var memTotalKB, memAvailKB, memFreeKB uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		valFields := strings.Fields(parts[1])
		if len(valFields) == 0 {
			continue
		}

		val, _ := strconv.ParseUint(valFields[0], 10, 64)
		switch key {
		case "MemTotal":
			memTotalKB = val
		case "MemAvailable":
			memAvailKB = val
		case "MemFree":
			memFreeKB = val
		}
	}

	if memTotalKB == 0 {
		return 0, 0, 0, 0
	}

	if memAvailKB == 0 {
		memAvailKB = memFreeKB
	}

	memUsedKB := memTotalKB - memAvailKB
	memTotalMB := memTotalKB / 1024
	memUsedMB := memUsedKB / 1024
	memFreeMB := memAvailKB / 1024
	memPercent := (float64(memUsedKB) / float64(memTotalKB)) * 100.0

	return memTotalMB, memUsedMB, memFreeMB, memPercent
}

func readCgroupMemory() (uint64, uint64, uint64, float64, bool) {
	maxData, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil {
		maxData, err = os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
		if err != nil {
			return 0, 0, 0, 0, false
		}
	}

	maxStr := strings.TrimSpace(string(maxData))
	if maxStr == "max" || maxStr == "" {
		return 0, 0, 0, 0, false
	}

	limitBytes, err := strconv.ParseUint(maxStr, 10, 64)
	if err != nil || limitBytes == 0 || limitBytes > 10000000000000 {
		return 0, 0, 0, 0, false
	}

	var usedBytes uint64
	if statData, err := os.ReadFile("/sys/fs/cgroup/memory.stat"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(statData)))
		var anon, kernel uint64
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				if fields[0] == "anon" {
					anon = val
				} else if fields[0] == "kernel" {
					kernel = val
				}
			}
		}
		usedBytes = anon + kernel
	}

	if usedBytes == 0 {
		if curData, err := os.ReadFile("/sys/fs/cgroup/memory.current"); err == nil {
			usedBytes, _ = strconv.ParseUint(strings.TrimSpace(string(curData)), 10, 64)
		}
	}

	if usedBytes > limitBytes {
		usedBytes = limitBytes
	}

	totalMB := limitBytes / (1024 * 1024)
	usedMB := usedBytes / (1024 * 1024)
	freeMB := totalMB - usedMB
	pct := (float64(usedBytes) / float64(limitBytes)) * 100.0

	return totalMB, usedMB, freeMB, pct, true
}

func readDisk(path string) (float64, float64, float64, float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, 0
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)

	var pct float64
	if totalBytes > 0 {
		pct = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	return totalGB, usedGB, freeGB, pct
}

func readUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}

	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return time.Duration(secs * float64(time.Second))
}

func readProcessRSS() float64 {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				valKB, err := strconv.ParseFloat(fields[1], 64)
				if err == nil {
					return valKB / 1024.0
				}
			}
		}
	}
	return 0
}

func FormatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
