package nettools

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PingResult struct {
	Host        string
	PacketsSent int
	PacketsRecv int
	PacketLoss  float64
	MinRTT      time.Duration
	AvgRTT      time.Duration
	MaxRTT      time.Duration
	RawOutput   string
}

type HTTPProbeResult struct {
	URL           string
	StatusCode    int
	StatusText    string
	Proto         string
	TTFB          time.Duration
	TotalDuration time.Duration
	ContentLength int64
	ContentType   string
	TLSIssuer     string
	TLSExpiry     *time.Time
	Headers       map[string]string
}

type DNSRecord struct {
	Type   string
	Values []string
}

type InterfaceStats struct {
	Name      string
	RXBytes   uint64
	TXBytes   uint64
	RXPackets uint64
	TXPackets uint64
	RXErrors  uint64
	TXErrors  uint64
}

var (
	rxPacketStats = regexp.MustCompile(`(\d+)\s+packets transmitted,\s+(\d+)\s+(?:packets\s+)?received.*?(?:,\s*\+?\d+\s+errors)?,?\s+([0-9.]+)%\s+packet loss`)
	rxRTTStats    = regexp.MustCompile(`(?:rtt|round-trip)\s+min/avg/max/(?:mdev|stddev)\s*=\s*([0-9.]+)/([0-9.]+)/([0-9.]+)`)
)

func Ping(ctx context.Context, host string, count int) (*PingResult, error) {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return nil, fmt.Errorf("host cannot be empty")
	}

	if strings.HasPrefix(trimmedHost, "-") || strings.ContainsAny(trimmedHost, " \t\r\n;\"'`$&|<>") {
		return nil, fmt.Errorf("invalid host format: %s", trimmedHost)
	}

	if count <= 0 {
		count = 3
	} else if count > 10 {
		count = 10
	}

	pingPath := "/bin/ping"
	if _, err := os.Stat(pingPath); err != nil {
		looked, lookErr := exec.LookPath("ping")
		if lookErr != nil {
			return nil, fmt.Errorf("ping binary not found on host: %w", lookErr)
		}
		pingPath = looked
	}

	cmd := exec.CommandContext(ctx, pingPath, "-c", strconv.Itoa(count), "-W", "2", trimmedHost)
	out, cmdErr := cmd.CombinedOutput()
	outStr := string(out)

	res, parseErr := parsePingOutput(trimmedHost, outStr)
	if parseErr == nil {
		return res, nil
	}

	if cmdErr != nil {
		return nil, fmt.Errorf("ping execution failed: %s (%w)", strings.TrimSpace(outStr), cmdErr)
	}

	return nil, parseErr
}

func parsePingOutput(host string, raw string) (*PingResult, error) {
	matchPackets := rxPacketStats.FindStringSubmatch(raw)
	if len(matchPackets) < 4 {
		return nil, fmt.Errorf("unable to parse packet statistics from ping output")
	}

	sent, _ := strconv.Atoi(matchPackets[1])
	recv, _ := strconv.Atoi(matchPackets[2])
	loss, _ := strconv.ParseFloat(matchPackets[3], 64)

	var minRTT, avgRTT, maxRTT time.Duration
	matchRTT := rxRTTStats.FindStringSubmatch(raw)
	if len(matchRTT) >= 4 {
		minF, _ := strconv.ParseFloat(matchRTT[1], 64)
		avgF, _ := strconv.ParseFloat(matchRTT[2], 64)
		maxF, _ := strconv.ParseFloat(matchRTT[3], 64)

		minRTT = time.Duration(minF * float64(time.Millisecond))
		avgRTT = time.Duration(avgF * float64(time.Millisecond))
		maxRTT = time.Duration(maxF * float64(time.Millisecond))
	}

	return &PingResult{
		Host:        host,
		PacketsSent: sent,
		PacketsRecv: recv,
		PacketLoss:  loss,
		MinRTT:      minRTT,
		AvgRTT:      avgRTT,
		MaxRTT:      maxRTT,
		RawOutput:   raw,
	}, nil
}

func ProbeHTTP(ctx context.Context, targetURL string, timeout time.Duration) (*HTTPProbeResult, error) {
	cleanURL := strings.TrimSpace(targetURL)
	if cleanURL == "" {
		return nil, fmt.Errorf("target URL cannot be empty")
	}

	if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
		cleanURL = "https://" + cleanURL
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DisableKeepAlives: true,
		Proxy:             http.ProxyFromEnvironment,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	var (
		mu   sync.Mutex
		ttfb time.Duration
	)

	start := time.Now()

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			mu.Lock()
			if ttfb == 0 {
				ttfb = time.Since(start)
			}
			mu.Unlock()
		},
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, cleanURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "AgyRemoteTele-NetProbe/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	_, _ = io.CopyN(io.Discard, resp.Body, 65536)
	totalDuration := time.Since(start)

	mu.Lock()
	if ttfb == 0 {
		ttfb = totalDuration
	}
	mu.Unlock()

	var (
		tlsIssuer string
		tlsExpiry *time.Time
	)

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		if cert.Issuer.CommonName != "" {
			tlsIssuer = cert.Issuer.CommonName
		} else {
			tlsIssuer = cert.Issuer.String()
		}
		exp := cert.NotAfter
		tlsExpiry = &exp
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}

	return &HTTPProbeResult{
		URL:           cleanURL,
		StatusCode:    resp.StatusCode,
		StatusText:    resp.Status,
		Proto:         resp.Proto,
		TTFB:          ttfb,
		TotalDuration: totalDuration,
		ContentLength: resp.ContentLength,
		ContentType:   resp.Header.Get("Content-Type"),
		TLSIssuer:     tlsIssuer,
		TLSExpiry:     tlsExpiry,
		Headers:       headers,
	}, nil
}

func LookupDNS(ctx context.Context, host string, recordType string) ([]DNSRecord, error) {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return nil, fmt.Errorf("host cannot be empty")
	}

	cleanType := strings.ToUpper(strings.TrimSpace(recordType))
	var results []DNSRecord

	switch cleanType {
	case "A":
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", trimmedHost)
		if err != nil {
			return nil, err
		}
		var vals []string
		for _, ip := range ips {
			vals = append(vals, ip.String())
		}
		if len(vals) > 0 {
			results = append(results, DNSRecord{Type: "A", Values: vals})
		}

	case "AAAA":
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", trimmedHost)
		if err != nil {
			return nil, err
		}
		var vals []string
		for _, ip := range ips {
			vals = append(vals, ip.String())
		}
		if len(vals) > 0 {
			results = append(results, DNSRecord{Type: "AAAA", Values: vals})
		}

	case "CNAME":
		cname, err := net.DefaultResolver.LookupCNAME(ctx, trimmedHost)
		if err != nil {
			return nil, err
		}
		if cname != "" {
			results = append(results, DNSRecord{Type: "CNAME", Values: []string{cname}})
		}

	case "MX":
		mxs, err := net.DefaultResolver.LookupMX(ctx, trimmedHost)
		if err != nil {
			return nil, err
		}
		var vals []string
		for _, mx := range mxs {
			vals = append(vals, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
		}
		if len(vals) > 0 {
			results = append(results, DNSRecord{Type: "MX", Values: vals})
		}

	case "TXT":
		txts, err := net.DefaultResolver.LookupTXT(ctx, trimmedHost)
		if err != nil {
			return nil, err
		}
		if len(txts) > 0 {
			results = append(results, DNSRecord{Type: "TXT", Values: txts})
		}

	case "", "ALL":
		if ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", trimmedHost); err == nil {
			var vals []string
			for _, ip := range ips {
				vals = append(vals, ip.String())
			}
			if len(vals) > 0 {
				results = append(results, DNSRecord{Type: "A", Values: vals})
			}
		}

		if ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", trimmedHost); err == nil {
			var vals []string
			for _, ip := range ips {
				vals = append(vals, ip.String())
			}
			if len(vals) > 0 {
				results = append(results, DNSRecord{Type: "AAAA", Values: vals})
			}
		}

		if cname, err := net.DefaultResolver.LookupCNAME(ctx, trimmedHost); err == nil && cname != "" && cname != trimmedHost && cname != trimmedHost+"." {
			results = append(results, DNSRecord{Type: "CNAME", Values: []string{cname}})
		}

		if mxs, err := net.DefaultResolver.LookupMX(ctx, trimmedHost); err == nil {
			var vals []string
			for _, mx := range mxs {
				vals = append(vals, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
			}
			if len(vals) > 0 {
				results = append(results, DNSRecord{Type: "MX", Values: vals})
			}
		}

		if txts, err := net.DefaultResolver.LookupTXT(ctx, trimmedHost); err == nil && len(txts) > 0 {
			results = append(results, DNSRecord{Type: "TXT", Values: txts})
		}

		if len(results) == 0 {
			return nil, fmt.Errorf("no DNS records resolved for host: %s", trimmedHost)
		}

	default:
		return nil, fmt.Errorf("unsupported DNS record type: %s", cleanType)
	}

	return results, nil
}

func GetNetworkIO() ([]InterfaceStats, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("reading /proc/net/dev: %w", err)
	}

	return parseNetworkIOBytes(data)
}

func parseNetworkIOBytes(data []byte) ([]InterfaceStats, error) {
	var stats []InterfaceStats
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineCount := 0

	for scanner.Scan() {
		lineCount++
		if lineCount <= 2 {
			continue
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		colonIdx := strings.IndexByte(line, ':')
		if colonIdx == -1 {
			continue
		}

		ifaceName := strings.TrimSpace(line[:colonIdx])
		fields := strings.Fields(line[colonIdx+1:])
		if len(fields) < 16 {
			continue
		}

		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		rxErrors, _ := strconv.ParseUint(fields[2], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		txErrors, _ := strconv.ParseUint(fields[10], 10, 64)

		stats = append(stats, InterfaceStats{
			Name:      ifaceName,
			RXBytes:   rxBytes,
			TXBytes:   txBytes,
			RXPackets: rxPackets,
			TXPackets: txPackets,
			RXErrors:  rxErrors,
			TXErrors:  txErrors,
		})
	}

	return stats, scanner.Err()
}
