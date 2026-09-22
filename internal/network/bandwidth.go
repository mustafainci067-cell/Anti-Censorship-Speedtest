package network

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"time"
)

type BandwidthResult struct {
	DownloadSpeedMbps float64
	UploadSpeedMbps   float64
}

type ProgressCallback func(percent float64)

type progressReader struct {
	r        io.Reader
	total    int64
	read     int64
	callback ProgressCallback
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	if n > 0 {
		pr.read += int64(n)
		if pr.callback != nil && pr.total > 0 {
			pr.callback(float64(pr.read) / float64(pr.total))
		}
	}
	return
}

func TestDownload(cb ProgressCallback) float64 {
	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()

	resp, err := client.Get("https://speed.cloudflare.com/__down?bytes=50000000") // 50 MB
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	reader := &progressReader{
		r:        resp.Body,
		total:    50000000,
		callback: cb,
	}

	written, err := io.Copy(io.Discard, reader)
	if err != nil || written == 0 {
		return 0
	}

	duration := time.Since(start).Seconds()

	// Speed in Megabits per second = (bytes * 8) / 1,000,000 / duration
	speedMbps := (float64(written) * 8) / 1000000.0 / duration
	return speedMbps
}

func TestUpload(cb ProgressCallback) float64 {
	client := &http.Client{Timeout: 30 * time.Second}

	// 10MB of random dummy data
	total := int64(10000000)
	data := make([]byte, total)
	rand.Read(data)

	reader := &progressReader{
		r:        bytes.NewReader(data),
		total:    total,
		callback: cb,
	}

	start := time.Now()
	resp, err := client.Post("https://speed.cloudflare.com/__up", "application/octet-stream", reader)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start).Seconds()

	speedMbps := (float64(total) * 8) / 1000000.0 / duration
	return speedMbps
}
