package network

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"time"
)

type NodeTarget struct {
	Name     string
	Endpoint string
	Ports    []int
}

var DefaultBackboneNodes = []NodeTarget{
	{Name: "Cloudflare Anycast", Endpoint: "1.1.1.1", Ports: []int{443, 53}},
	{Name: "Google Backbone", Endpoint: "8.8.8.8", Ports: []int{443, 53}},
	{Name: "Quad9 Swiss Edge", Endpoint: "9.9.9.9", Ports: []int{443, 53}},
	{Name: "Cisco OpenDNS", Endpoint: "208.67.222.222", Ports: []int{443, 53}},
}

type PingResult struct {
	Name        string        `json:"name"`
	Endpoint    string        `json:"endpoint"`
	Port        int           `json:"port"`
	Ping        time.Duration `json:"-"`
	Jitter      time.Duration `json:"-"`
	PingMs      float64       `json:"ping_ms"`
	MinMs       float64       `json:"min_ms"`
	MaxMs       float64       `json:"max_ms"`
	JitterMs    float64       `json:"jitter_ms"`
	LossPercent float64       `json:"loss_percent"`
	Samples     []float64     `json:"samples_ms,omitempty"`
	Success     bool          `json:"success"`
}

func MeasurePingAndJitter(endpoint string) PingResult {
	name := endpoint
	ports := []int{443, 53, 80}
	for _, n := range DefaultBackboneNodes {
		if n.Endpoint == endpoint {
			name = n.Name
			ports = n.Ports
			break
		}
	}
	return MeasureNodeLatency(context.Background(), NodeTarget{
		Name:     name,
		Endpoint: endpoint,
		Ports:    ports,
	}, 7)
}

func MeasureNodeLatency(ctx context.Context, target NodeTarget, probeCount int) PingResult {
	if probeCount < 3 {
		probeCount = 6
	}
	ports := target.Ports
	if len(ports) == 0 {
		ports = []int{443, 53}
	}

	activePort := ports[0]
	dialer := &net.Dialer{Timeout: 1500 * time.Millisecond}

	// Warm-up & port discovery probe (populates ARP/NAT state table)
	portFound := false
	for _, p := range ports {
		addr := fmt.Sprintf("%s:%d", target.Endpoint, p)
		conn, err := dialer.DialContext(ctx, "tcp4", addr)
		if err == nil {
			_ = conn.Close()
			activePort = p
			portFound = true
			break
		}
	}

	if !portFound {
		return PingResult{
			Name:        target.Name,
			Endpoint:    target.Endpoint,
			Port:        activePort,
			LossPercent: 100.0,
			Success:     false,
		}
	}

	address := fmt.Sprintf("%s:%d", target.Endpoint, activePort)
	var rawSamplesMs []float64
	sent := 0

	for i := 0; i < probeCount; i++ {
		if ctx.Err() != nil {
			break
		}
		sent++
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp4", address)
		if err == nil {
			rtt := time.Since(start)
			_ = conn.Close()
			ms := float64(rtt.Microseconds()) / 1000.0
			rawSamplesMs = append(rawSamplesMs, math.Round(ms*100)/100)
		}

		if i < probeCount-1 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(40 * time.Millisecond):
			}
		}
	}

	if len(rawSamplesMs) == 0 || sent == 0 {
		return PingResult{
			Name:        target.Name,
			Endpoint:    target.Endpoint,
			Port:        activePort,
			LossPercent: 100.0,
			Success:     false,
		}
	}

	// Filter single highest NAT/ARP spike outlier when >= 5 samples exist
	evalSamples := rawSamplesMs
	if len(rawSamplesMs) >= 5 {
		maxIdx := 0
		for i, v := range rawSamplesMs {
			if v > rawSamplesMs[maxIdx] {
				maxIdx = i
			}
		}
		filtered := make([]float64, 0, len(rawSamplesMs)-1)
		for i, v := range rawSamplesMs {
			if i != maxIdx {
				filtered = append(filtered, v)
			}
		}
		evalSamples = filtered
	}

	minMs := evalSamples[0]
	maxMs := evalSamples[0]
	var sumMs float64

	for _, ms := range evalSamples {
		sumMs += ms
		if ms < minMs {
			minMs = ms
		}
		if ms > maxMs {
			maxMs = ms
		}
	}

	// Trimmed average latency
	avgPingMs := sumMs / float64(len(evalSamples))

	// Also compute median for stability comparison
	sorted := make([]float64, len(evalSamples))
	copy(sorted, evalSamples)
	sort.Float64s(sorted)
	medianMs := sorted[len(sorted)/2]
	if avgPingMs > medianMs*1.35 {
		avgPingMs = 0.5*avgPingMs + 0.5*medianMs
	}

	var jitterMs float64
	if len(evalSamples) > 1 {
		var diffSum float64
		for i := 1; i < len(evalSamples); i++ {
			diffSum += math.Abs(evalSamples[i] - evalSamples[i-1])
		}
		jitterMs = diffSum / float64(len(evalSamples)-1)
	}

	avgDur := time.Duration(avgPingMs * float64(time.Millisecond))
	jitterDur := time.Duration(jitterMs * float64(time.Millisecond))
	lossPct := float64(sent-len(rawSamplesMs)) / float64(sent) * 100.0

	return PingResult{
		Name:        target.Name,
		Endpoint:    target.Endpoint,
		Port:        activePort,
		Ping:        avgDur,
		Jitter:      jitterDur,
		PingMs:      math.Round(avgPingMs*10) / 10,
		MinMs:       math.Round(minMs*10) / 10,
		MaxMs:       math.Round(maxMs*10) / 10,
		JitterMs:    math.Round(jitterMs*100) / 100,
		LossPercent: math.Round(lossPct*10) / 10,
		Samples:     evalSamples,
		Success:     true,
	}
}

// MeasureAllNodes concurrently probes all default anti-censorship backbone nodes
// with slight staggering to avoid local router NAT burst queuing.
func MeasureAllNodes(ctx context.Context) ([]PingResult, PingResult) {
	results := make([]PingResult, len(DefaultBackboneNodes))
	var wg sync.WaitGroup

	for i, node := range DefaultBackboneNodes {
		wg.Add(1)
		go func(idx int, target NodeTarget) {
			defer wg.Done()
			if idx > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(idx*15) * time.Millisecond):
				}
			}
			results[idx] = MeasureNodeLatency(ctx, target, 7)
		}(i, node)
	}

	wg.Wait()

	var best PingResult
	for _, r := range results {
		if !r.Success {
			continue
		}
		if !best.Success || r.PingMs < best.PingMs {
			best = r
		}
	}

	return results, best
}

// GradeConnection evaluates network suitability for real-time tasks.
func GradeConnection(pingMs, jitterMs, lossPct, downMbps, upMbps float64) (string, string) {
	if downMbps <= 0 && upMbps <= 0 {
		return "OFFLINE", "No active throughput measured"
	}
	if lossPct > 5.0 || pingMs > 180 {
		return "DEGRADED", "High latency or packet loss detected"
	}
	if pingMs <= 38 && jitterMs <= 10 && downMbps >= 50 && upMbps >= 15 && lossPct == 0 {
		return "OPTIMAL (A+)", "Low-latency gaming, 4K streaming & VoIP ready"
	}
	if pingMs <= 68 && jitterMs <= 20 && downMbps >= 25 && lossPct <= 1.0 {
		return "GOOD (A)", "HD streaming, video calls & competitive gaming"
	}
	if pingMs <= 115 && downMbps >= 10 {
		return "STABLE (B)", "Standard browsing, streaming & voice calls"
	}
	return "CONGESTED (C)", "Bandwidth limited or elevated jitter"
}
