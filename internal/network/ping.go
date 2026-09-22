package network

import (
	"math"
	"net"
	"time"
)

type PingResult struct {
	Endpoint string
	Ping     time.Duration
	Jitter   time.Duration
	Success  bool
}

func MeasurePingAndJitter(endpoint string) PingResult {
	// TCP Ping to port 443 to bypass ICMP blocks
	address := endpoint + ":443"

	var rttList []time.Duration

	// Send 5 ping probes
	for i := 0; i < 5; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			rtt := time.Since(start)
			rttList = append(rttList, rtt)
			conn.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(rttList) == 0 {
		return PingResult{Endpoint: endpoint, Success: false}
	}

	var total time.Duration
	for _, rtt := range rttList {
		total += rtt
	}

	avgPing := total / time.Duration(len(rttList))

	var jitterTotal float64
	if len(rttList) > 1 {
		for i := 1; i < len(rttList); i++ {
			diff := float64(rttList[i].Milliseconds() - rttList[i-1].Milliseconds())
			jitterTotal += math.Abs(diff)
		}
		jitterTotal /= float64(len(rttList) - 1)
	}

	jitter := time.Duration(jitterTotal) * time.Millisecond

	return PingResult{
		Endpoint: endpoint,
		Ping:     avgPing,
		Jitter:   jitter,
		Success:  true,
	}
}
