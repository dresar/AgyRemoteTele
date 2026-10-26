package system

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type PortInfo struct {
	Protocol    string
	LocalIP     string
	LocalPort   int
	RemoteIP    string
	RemotePort  int
	State       string
	Inode       uint64
	ProcessName string
	PID         int
}

type procSocket struct {
	pid  int
	name string
}

func ListListeningPorts() ([]PortInfo, error) {
	inodeMap := buildSocketInodeMap()

	var ports []PortInfo

	tcpPorts, err := parseProcNetFile("/proc/net/tcp", "tcp", inodeMap)
	if err == nil {
		ports = append(ports, tcpPorts...)
	}

	tcp6Ports, err := parseProcNetFile("/proc/net/tcp6", "tcp6", inodeMap)
	if err == nil {
		ports = append(ports, tcp6Ports...)
	}

	udpPorts, err := parseProcNetFile("/proc/net/udp", "udp", inodeMap)
	if err == nil {
		ports = append(ports, udpPorts...)
	}

	udp6Ports, err := parseProcNetFile("/proc/net/udp6", "udp6", inodeMap)
	if err == nil {
		ports = append(ports, udp6Ports...)
	}

	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol < ports[j].Protocol
		}
		if ports[i].LocalPort != ports[j].LocalPort {
			return ports[i].LocalPort < ports[j].LocalPort
		}
		return ports[i].LocalIP < ports[j].LocalIP
	})

	return ports, nil
}

func parseProcNetFile(filePath string, proto string, inodeMap map[uint64]procSocket) ([]PortInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []PortInfo
	scanner := bufio.NewScanner(f)
	lineIdx := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineIdx++
		if lineIdx == 1 {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		localAddr := fields[1]
		remoteAddr := fields[2]
		stateHex := fields[3]
		inodeStr := fields[9]

		isTCP := strings.HasPrefix(proto, "tcp")
		if isTCP {
			if strings.ToUpper(stateHex) != "0A" {
				continue
			}
		}

		localIP, localPort, errL := decodeHexAddr(localAddr)
		if errL != nil {
			continue
		}

		remoteIP, remotePort, errR := decodeHexAddr(remoteAddr)
		if errR != nil {
			continue
		}

		if !isTCP && remotePort != 0 {
			continue
		}

		inode, _ := strconv.ParseUint(inodeStr, 10, 64)

		stateStr := "LISTEN"

		var procName string
		var procPID int
		if proc, ok := inodeMap[inode]; ok {
			procPID = proc.pid
			procName = proc.name
		}

		results = append(results, PortInfo{
			Protocol:    proto,
			LocalIP:     localIP,
			LocalPort:   localPort,
			RemoteIP:    remoteIP,
			RemotePort:  remotePort,
			State:       stateStr,
			Inode:       inode,
			ProcessName: procName,
			PID:         procPID,
		})
	}

	return results, scanner.Err()
}

func decodeHexAddr(hexAddr string) (string, int, error) {
	parts := strings.Split(hexAddr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", hexAddr)
	}

	ipHex := parts[0]
	portHex := parts[1]

	portVal, err := strconv.ParseInt(portHex, 16, 64)
	if err != nil {
		return "", 0, err
	}

	ipStr, err := decodeHexIP(ipHex)
	if err != nil {
		return "", int(portVal), err
	}

	return ipStr, int(portVal), nil
}

func decodeHexIP(hexIP string) (string, error) {
	hexLen := len(hexIP)
	if hexLen == 8 {
		val, err := strconv.ParseUint(hexIP, 16, 32)
		if err != nil {
			return "", err
		}
		b0 := byte(val & 0xFF)
		b1 := byte((val >> 8) & 0xFF)
		b2 := byte((val >> 16) & 0xFF)
		b3 := byte((val >> 24) & 0xFF)
		return net.IPv4(b0, b1, b2, b3).String(), nil
	}

	if hexLen == 32 {
		var ipBytes [16]byte
		for i := 0; i < 4; i++ {
			chunk := hexIP[i*8 : (i+1)*8]
			val, err := strconv.ParseUint(chunk, 16, 32)
			if err != nil {
				return "", err
			}
			ipBytes[i*4+0] = byte(val & 0xFF)
			ipBytes[i*4+1] = byte((val >> 8) & 0xFF)
			ipBytes[i*4+2] = byte((val >> 16) & 0xFF)
			ipBytes[i*4+3] = byte((val >> 24) & 0xFF)
		}
		return net.IP(ipBytes[:]).String(), nil
	}

	return "", fmt.Errorf("unexpected hex IP length: %d", hexLen)
}

func buildSocketInodeMap() map[uint64]procSocket {
	m := make(map[uint64]procSocket)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return m
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		procName := getProcessShortName(pid)

		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			fdPath := filepath.Join(fdDir, fd.Name())
			link, err := os.Readlink(fdPath)
			if err != nil {
				continue
			}

			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inodeStr := link[8 : len(link)-1]
				inode, errP := strconv.ParseUint(inodeStr, 10, 64)
				if errP == nil {
					m[inode] = procSocket{pid: pid, name: procName}
				}
			}
		}
	}

	return m
}

func getProcessShortName(pid int) string {
	commData, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err == nil {
		name := strings.TrimSpace(string(commData))
		if name != "" {
			return name
		}
	}

	cmdBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err == nil && len(cmdBytes) > 0 {
		cmdBytes = bytes.ReplaceAll(cmdBytes, []byte{0}, []byte(" "))
		fields := strings.Fields(string(cmdBytes))
		if len(fields) > 0 {
			return filepath.Base(fields[0])
		}
	}

	return ""
}
