package network

import (
	"context"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type BandwidthResult struct {
	DownloadSpeedMbps float64
	UploadSpeedMbps   float64
}

type ProgressCallback func(percent float64)

type TransferTelemetry struct {
	Progress         float64
	CurrentMbps      float64
	PeakMbps         float64
	AverageMbps      float64
	BytesTransferred int64
	Elapsed          time.Duration
	ActiveStreams    int
	Samples          []float64
}

type TelemetryCallback func(telemetry TransferTelemetry)

type TransferResult struct {
	SustainedMbps    float64       `json:"sustained_mbps"`
	PeakMbps         float64       `json:"peak_mbps"`
	BytesTransferred int64         `json:"bytes_transferred"`
	Duration         time.Duration `json:"-"`
	Streams          int           `json:"streams"`
	BypassMethod     string        `json:"bypass_method"`
	Samples          []float64     `json:"samples_mbps,omitempty"`
}

type BandwidthOptions struct {
	Mode     BypassMode
	Streams  int
	Duration time.Duration
}

func DefaultDownloadOptions() BandwidthOptions {
	return BandwidthOptions{
		Mode:     ModeAuto,
		Streams:  4,
		Duration: 7 * time.Second,
	}
}

func DefaultUploadOptions() BandwidthOptions {
	return BandwidthOptions{
		Mode:     ModeAuto,
		Streams:  3,
		Duration: 6 * time.Second,
	}
}

var (
	uploadPayloadOnce sync.Once
	uploadPayloadBuf  []byte
)

func getUncompressiblePayload(size int) []byte {
	uploadPayloadOnce.Do(func() {
		// Deterministic fast PRNG buffer (2MB) so compressions on middleboxes cannot cheat
		uploadPayloadBuf = make([]byte, 2*1024*1024)
		var x uint64 = 0x9E3779B97F4A7C15
		for i := 0; i < len(uploadPayloadBuf); i += 8 {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			uploadPayloadBuf[i] = byte(x)
			uploadPayloadBuf[i+1] = byte(x >> 8)
			uploadPayloadBuf[i+2] = byte(x >> 16)
			uploadPayloadBuf[i+3] = byte(x >> 24)
			uploadPayloadBuf[i+4] = byte(x >> 32)
			uploadPayloadBuf[i+5] = byte(x >> 40)
			uploadPayloadBuf[i+6] = byte(x >> 48)
			uploadPayloadBuf[i+7] = byte(x >> 56)
		}
	})
	if size <= len(uploadPayloadBuf) {
		return uploadPayloadBuf[:size]
	}
	return uploadPayloadBuf
}

var downloadEndpoints = []string{
	"https://speed.cloudflare.com/__down?bytes=25000000",
	"https://speed.cloudflare.com/__down?bytes=10000000",
}

// TestDownload runs the multi-stream anti-censorship download test with backward-compatible callback.
func TestDownload(cb ProgressCallback) float64 {
	res := TestDownloadWithContext(context.Background(), DefaultDownloadOptions(), func(t TransferTelemetry) {
		if cb != nil {
			cb(t.Progress)
		}
	})
	return res.SustainedMbps
}

// TestUpload runs the multi-stream anti-censorship upload test with backward-compatible callback.
func TestUpload(cb ProgressCallback) float64 {
	res := TestUploadWithContext(context.Background(), DefaultUploadOptions(), func(t TransferTelemetry) {
		if cb != nil {
			cb(t.Progress)
		}
	})
	return res.SustainedMbps
}

// TestDownloadWithContext performs a time-boxed, multi-stream download benchmark over TLS/443.
func TestDownloadWithContext(parentCtx context.Context, opts BandwidthOptions, cb TelemetryCallback) TransferResult {
	if opts.Streams <= 0 {
		opts.Streams = 4
	}
	if opts.Duration <= 0 {
		opts.Duration = 7 * time.Second
	}
	if opts.Mode == "" {
		opts.Mode = ModeAuto
	}

	// Pre-warm DNS / DoH resolution cache before starting timer
	_, _ = ResolveHostResilient(parentCtx, "speed.cloudflare.com", opts.Mode)

	client, getInfo := NewResilientClient(opts.Mode, opts.Duration+5*time.Second)

	ctx, cancel := context.WithTimeout(parentCtx, opts.Duration)
	defer cancel()

	var (
		totalBytes    atomic.Int64
		firstByteOnce sync.Once
		startTimeMu   sync.RWMutex
		transferStart = time.Now()
		firstByteSeen atomic.Bool
	)

	markFirstByte := func() {
		firstByteOnce.Do(func() {
			startTimeMu.Lock()
			transferStart = time.Now()
			startTimeMu.Unlock()
			firstByteSeen.Store(true)
		})
	}

	getTransferStart := func() time.Time {
		startTimeMu.RLock()
		defer startTimeMu.RUnlock()
		return transferStart
	}

	var wg sync.WaitGroup
	for s := 0; s < opts.Streams; s++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()
			buf := make([]byte, 32*1024)
			epIdx := 0

			for {
				if ctx.Err() != nil {
					return
				}
				targetURL := downloadEndpoints[epIdx%len(downloadEndpoints)]
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
				if err != nil {
					return
				}
				req.Header.Set("User-Agent", "NetProbe-Diagnostic/2.0")
				req.Header.Set("Cache-Control", "no-store")

				resp, err := client.Do(req)
				if err != nil {
					epIdx++
					select {
					case <-ctx.Done():
						return
					case <-time.After(120 * time.Millisecond):
						continue
					}
				}

				for {
					n, readErr := resp.Body.Read(buf)
					if n > 0 {
						markFirstByte()
						totalBytes.Add(int64(n))
					}
					if readErr != nil || ctx.Err() != nil {
						break
					}
				}
				_ = resp.Body.Close()
			}
		}(s)
	}

	return monitorTransfer(ctx, &wg, opts, &totalBytes, &firstByteSeen, getTransferStart, getInfo, cb)
}

type uploadStreamReader struct {
	ctx        context.Context
	payload    []byte
	offset     int
	totalLimit int
	onBytes    func(n int)
}

func (u *uploadStreamReader) Read(p []byte) (int, error) {
	if u.ctx.Err() != nil {
		return 0, u.ctx.Err()
	}
	if u.offset >= u.totalLimit {
		return 0, io.EOF
	}
	rem := u.totalLimit - u.offset
	maxChunk := 16 * 1024 // 16KB pacing chunks for accurate progress
	if len(p) < maxChunk {
		maxChunk = len(p)
	}
	if rem < maxChunk {
		maxChunk = rem
	}

	srcOffset := u.offset % len(u.payload)
	if srcOffset+maxChunk <= len(u.payload) {
		copy(p[:maxChunk], u.payload[srcOffset:srcOffset+maxChunk])
	} else {
		first := len(u.payload) - srcOffset
		copy(p[:first], u.payload[srcOffset:])
		copy(p[first:maxChunk], u.payload[:maxChunk-first])
	}

	u.offset += maxChunk
	if u.onBytes != nil {
		u.onBytes(maxChunk)
	}
	return maxChunk, nil
}

// TestUploadWithContext performs a time-boxed, multi-stream upload benchmark over TLS/443.
func TestUploadWithContext(parentCtx context.Context, opts BandwidthOptions, cb TelemetryCallback) TransferResult {
	if opts.Streams <= 0 {
		opts.Streams = 3
	}
	if opts.Duration <= 0 {
		opts.Duration = 6 * time.Second
	}
	if opts.Mode == "" {
		opts.Mode = ModeAuto
	}

	_, _ = ResolveHostResilient(parentCtx, "speed.cloudflare.com", opts.Mode)
	payload := getUncompressiblePayload(2 * 1024 * 1024)

	client, getInfo := NewResilientClient(opts.Mode, opts.Duration+5*time.Second)

	ctx, cancel := context.WithTimeout(parentCtx, opts.Duration)
	defer cancel()

	var (
		totalBytes    atomic.Int64
		firstByteOnce sync.Once
		startTimeMu   sync.RWMutex
		transferStart = time.Now()
		firstByteSeen atomic.Bool
	)

	markFirstByte := func() {
		firstByteOnce.Do(func() {
			startTimeMu.Lock()
			transferStart = time.Now()
			startTimeMu.Unlock()
			firstByteSeen.Store(true)
		})
	}

	getTransferStart := func() time.Time {
		startTimeMu.RLock()
		defer startTimeMu.RUnlock()
		return transferStart
	}

	var wg sync.WaitGroup
	for s := 0; s < opts.Streams; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			const batchSize = 1536 * 1024 // 1.5 MB per request batch

			for {
				if ctx.Err() != nil {
					return
				}
				reader := &uploadStreamReader{
					ctx:        ctx,
					payload:    payload,
					totalLimit: batchSize,
					onBytes: func(n int) {
						markFirstByte()
						totalBytes.Add(int64(n))
					},
				}

				req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://speed.cloudflare.com/__up", reader)
				if err != nil {
					return
				}
				req.ContentLength = int64(batchSize)
				req.Header.Set("Content-Type", "application/octet-stream")
				req.Header.Set("User-Agent", "NetProbe-Diagnostic/2.0")

				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(100 * time.Millisecond):
						continue
					}
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}

	return monitorTransfer(ctx, &wg, opts, &totalBytes, &firstByteSeen, getTransferStart, getInfo, cb)
}

func monitorTransfer(
	ctx context.Context,
	wg *sync.WaitGroup,
	opts BandwidthOptions,
	totalBytes *atomic.Int64,
	firstByteSeen *atomic.Bool,
	getTransferStart func() time.Time,
	getInfo func() (string, string),
	cb TelemetryCallback,
) TransferResult {
	ticker := time.NewTicker(85 * time.Millisecond)
	defer ticker.Stop()

	phaseStart := time.Now()
	prevBytes := int64(0)
	prevTick := phaseStart
	smoothedMbps := 0.0
	peakMbps := 0.0
	var samples []float64

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

loop:
	for {
		select {
		case <-doneCh:
			break loop
		case <-ctx.Done():
			break loop
		case now := <-ticker.C:
			curBytes := totalBytes.Load()
			elapsedPhase := now.Sub(phaseStart)
			progress := float64(elapsedPhase) / float64(opts.Duration)
			if progress > 0.99 {
				progress = 0.99
			}
			if progress < 0 {
				progress = 0
			}

			var avgMbps float64
			if firstByteSeen.Load() {
				transferElapsed := now.Sub(getTransferStart()).Seconds()
				if transferElapsed > 0.05 {
					avgMbps = (float64(curBytes) * 8.0) / 1_000_000.0 / transferElapsed
				}
			}

			dt := now.Sub(prevTick).Seconds()
			deltaBytes := curBytes - prevBytes
			prevBytes = curBytes
			prevTick = now

			if dt > 0 && deltaBytes >= 0 {
				instMbps := (float64(deltaBytes) * 8.0) / 1_000_000.0 / dt
				if smoothedMbps == 0 {
					smoothedMbps = instMbps
				} else {
					smoothedMbps = 0.35*instMbps + 0.65*smoothedMbps
				}
				if elapsedPhase > 350*time.Millisecond && smoothedMbps > peakMbps {
					peakMbps = smoothedMbps
				}
				if firstByteSeen.Load() {
					samples = append(samples, math.Round(smoothedMbps*100)/100)
				}
			}

			displayMbps := smoothedMbps
			if avgMbps > 0 && elapsedPhase > 600*time.Millisecond {
				displayMbps = 0.5*smoothedMbps + 0.5*avgMbps
			}

			if cb != nil {
				cb(TransferTelemetry{
					Progress:         progress,
					CurrentMbps:      math.Round(displayMbps*100) / 100,
					PeakMbps:         math.Round(peakMbps*100) / 100,
					AverageMbps:      math.Round(avgMbps*100) / 100,
					BytesTransferred: curBytes,
					Elapsed:          elapsedPhase,
					ActiveStreams:    opts.Streams,
					Samples:          samples,
				})
			}
		}
	}

	// Snapshot final metrics immediately when timer expires so worker teardown latency does not dilute throughput
	finalBytes := totalBytes.Load()
	var transferElapsed float64
	if firstByteSeen.Load() {
		transferElapsed = time.Since(getTransferStart()).Seconds()
	}

	// Wait briefly for workers to close cleanly
	select {
	case <-doneCh:
	case <-time.After(350 * time.Millisecond):
	}

	var sustainedMbps float64
	if finalBytes > 0 && transferElapsed > 0.05 {
		sustainedMbps = (float64(finalBytes) * 8.0) / 1_000_000.0 / transferElapsed
	}

	sustainedMbps = math.Round(sustainedMbps*100) / 100
	if sustainedMbps > peakMbps {
		peakMbps = sustainedMbps
	}
	peakMbps = math.Round(peakMbps*100) / 100

	if cb != nil {
		cb(TransferTelemetry{
			Progress:         1.0,
			CurrentMbps:      sustainedMbps,
			PeakMbps:         peakMbps,
			AverageMbps:      sustainedMbps,
			BytesTransferred: finalBytes,
			Elapsed:          opts.Duration,
			ActiveStreams:    opts.Streams,
			Samples:          samples,
		})
	}

	method, _ := getInfo()

	return TransferResult{
		SustainedMbps:    sustainedMbps,
		PeakMbps:         peakMbps,
		BytesTransferred: finalBytes,
		Duration:         opts.Duration,
		Streams:          opts.Streams,
		BypassMethod:     method,
		Samples:          samples,
	}
}
