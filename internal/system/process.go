package system

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type ProcessInfo struct {
	PID     int
	User    string
	CPU     float64
	Mem     float64
	RSSMB   float64
	State   string
	Command string
}

func ListProcesses(topN int, sortBy string) ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	sysUptime := readSystemUptimeSeconds()
	memTotalKB := readSystemMemTotalKB()
	pageSizeKB := float64(os.Getpagesize()) / 1024.0

	userCache := make(map[string]string)
	var procs []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		pInfo, ok := readProcessInfo(pid, sysUptime, memTotalKB, pageSizeKB, userCache)
		if ok {
			procs = append(procs, pInfo)
		}
	}

	sortProcesses(procs, sortBy)

	if topN > 0 && len(procs) > topN {
		procs = procs[:topN]
	}

	return procs, nil
}

func readProcessInfo(pid int, sysUptime float64, memTotalKB float64, pageSizeKB float64, userCache map[string]string) (ProcessInfo, bool) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return ProcessInfo{}, false
	}

	statStr := string(statData)
	openParen := strings.IndexByte(statStr, '(')
	closeParen := strings.LastIndexByte(statStr, ')')
	if openParen == -1 || closeParen == -1 || closeParen <= openParen {
		return ProcessInfo{}, false
	}

	comm := statStr[openParen+1 : closeParen]
	afterParen := strings.TrimSpace(statStr[closeParen+1:])
	fields := strings.Fields(afterParen)
	if len(fields) < 22 {
		return ProcessInfo{}, false
	}

	state := fields[0]
	utime, _ := strconv.ParseFloat(fields[11], 64)
	stime, _ := strconv.ParseFloat(fields[12], 64)
	starttime, _ := strconv.ParseFloat(fields[19], 64)
	rssPages, _ := strconv.ParseFloat(fields[21], 64)

	rssKB := rssPages * pageSizeKB
	uidStr := ""
	statusState := ""

	statusFile, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err == nil {
		scanner := bufio.NewScanner(statusFile)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "Uid:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					uidStr = parts[1]
				}
			} else if strings.HasPrefix(line, "VmRSS:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if val, parseErr := strconv.ParseFloat(parts[1], 64); parseErr == nil {
						rssKB = val
					}
				}
			} else if strings.HasPrefix(line, "State:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					statusState = parts[1]
				}
			}
		}
		statusFile.Close()
	}

	if statusState != "" {
		state = statusState
	}

	userName := uidStr
	if uidStr != "" {
		if cached, ok := userCache[uidStr]; ok {
			userName = cached
		} else {
			if u, err := user.LookupId(uidStr); err == nil && u.Username != "" {
				userName = u.Username
			}
			userCache[uidStr] = userName
		}
	}

	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	cmdBytes, err := os.ReadFile(cmdlinePath)
	var command string
	if err == nil && len(cmdBytes) > 0 {
		cmdBytes = bytes.ReplaceAll(cmdBytes, []byte{0}, []byte(" "))
		command = strings.TrimSpace(string(cmdBytes))
	}
	if command == "" {
		command = "[" + comm + "]"
	}

	rssMB := math.Round((rssKB/1024.0)*100) / 100

	var memPct float64
	if memTotalKB > 0 {
		memPct = math.Round(((rssKB/memTotalKB)*100.0)*100) / 100
	}

	var cpuPct float64
	totalTimeSec := (utime + stime) / 100.0
	processStartSec := starttime / 100.0
	elapsedSec := sysUptime - processStartSec
	if elapsedSec > 0 && totalTimeSec > 0 {
		cpuPct = math.Round(((totalTimeSec/elapsedSec)*100.0)*100) / 100
	}

	return ProcessInfo{
		PID:     pid,
		User:    userName,
		CPU:     cpuPct,
		Mem:     memPct,
		RSSMB:   rssMB,
		State:   state,
		Command: command,
	}, true
}

func readSystemUptimeSeconds() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) > 0 {
		val, _ := strconv.ParseFloat(fields[0], 64)
		return val
	}
	return 0
}

func readSystemMemTotalKB() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseFloat(fields[1], 64)
				return val
			}
		}
	}
	return 0
}

func sortProcesses(procs []ProcessInfo, sortBy string) {
	lowerSort := strings.ToLower(sortBy)
	switch lowerSort {
	case "mem", "rss":
		sort.Slice(procs, func(i, j int) bool {
			if procs[i].Mem == procs[j].Mem {
				return procs[i].PID < procs[j].PID
			}
			return procs[i].Mem > procs[j].Mem
		})
	case "pid":
		sort.Slice(procs, func(i, j int) bool {
			return procs[i].PID < procs[j].PID
		})
	case "user":
		sort.Slice(procs, func(i, j int) bool {
			if procs[i].User == procs[j].User {
				return procs[i].PID < procs[j].PID
			}
			return procs[i].User < procs[j].User
		})
	default:
		sort.Slice(procs, func(i, j int) bool {
			if procs[i].CPU == procs[j].CPU {
				return procs[i].PID < procs[j].PID
			}
			return procs[i].CPU > procs[j].CPU
		})
	}
}

func KillProcess(pid int, sig syscall.Signal) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to kill system process with PID %d", pid)
	}

	selfPid := os.Getpid()
	if pid == selfPid {
		return fmt.Errorf("refusing to kill current process (PID %d)", pid)
	}

	parentPid := os.Getppid()
	if pid == parentPid {
		return fmt.Errorf("refusing to kill parent process (PID %d)", pid)
	}

	if isKernelTask(pid) {
		return fmt.Errorf("refusing to kill kernel task with PID %d", pid)
	}

	err := syscall.Kill(pid, sig)
	if err != nil {
		return fmt.Errorf("failed to kill PID %d with signal %v: %w", pid, sig, err)
	}
	return nil
}

func isKernelTask(pid int) bool {
	if pid == 2 {
		return true
	}

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return false
	}

	statStr := string(statData)
	closeParen := strings.LastIndexByte(statStr, ')')
	if closeParen != -1 {
		fields := strings.Fields(statStr[closeParen+1:])
		if len(fields) >= 2 {
			ppid, errP := strconv.Atoi(fields[1])
			if errP == nil && ppid == 2 {
				return true
			}
		}
	}

	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	_, errExe := os.Readlink(exePath)
	if errors.Is(errExe, os.ErrNotExist) || os.IsNotExist(errExe) {
		cmdBytes, errCmd := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if errCmd == nil && len(cmdBytes) == 0 {
			return true
		}
	}

	_ = filepath.Base("")
	return false
}
