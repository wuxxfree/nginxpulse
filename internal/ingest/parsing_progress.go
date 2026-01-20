package ingest

import (
	"math"
	"sync"
	"time"
)

type parseProgressState struct {
	TotalBytes     int64
	ProcessedBytes int64
	StartedAt      time.Time
	UpdatedAt      time.Time
}

var (
	parseProgressMu sync.RWMutex
	parseProgress   parseProgressState
)

func resetParsingProgress() {
	now := time.Now()
	parseProgressMu.Lock()
	parseProgress = parseProgressState{
		TotalBytes:     0,
		ProcessedBytes: 0,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	parseProgressMu.Unlock()
}

func setParsingTotalBytes(totalBytes int64) {
	if totalBytes < 0 {
		totalBytes = 0
	}
	parseProgressMu.Lock()
	parseProgress.TotalBytes = totalBytes
	if totalBytes > 0 && parseProgress.ProcessedBytes > totalBytes {
		parseProgress.ProcessedBytes = totalBytes
	}
	parseProgress.UpdatedAt = time.Now()
	parseProgressMu.Unlock()
}

func addParsingProgress(deltaBytes int64) {
	if deltaBytes <= 0 {
		return
	}
	parseProgressMu.Lock()
	parseProgress.ProcessedBytes += deltaBytes
	if parseProgress.TotalBytes > 0 && parseProgress.ProcessedBytes > parseProgress.TotalBytes {
		parseProgress.ProcessedBytes = parseProgress.TotalBytes
	}
	parseProgress.UpdatedAt = time.Now()
	parseProgressMu.Unlock()
}

func finalizeParsingProgress() {
	parseProgressMu.Lock()
	if parseProgress.TotalBytes > 0 {
		parseProgress.ProcessedBytes = parseProgress.TotalBytes
	}
	parseProgress.UpdatedAt = time.Now()
	parseProgressMu.Unlock()
}

func GetIPParsingProgress() int {
	parseProgressMu.RLock()
	total := parseProgress.TotalBytes
	processed := parseProgress.ProcessedBytes
	parseProgressMu.RUnlock()

	if total <= 0 {
		return 0
	}

	progress := float64(processed) / float64(total)
	progress = math.Max(0, math.Min(progress, 1))
	return int(math.Round(progress * 100))
}

func GetIPParsingEstimatedTotalSeconds() int64 {
	parseProgressMu.RLock()
	total := parseProgress.TotalBytes
	processed := parseProgress.ProcessedBytes
	startedAt := parseProgress.StartedAt
	parseProgressMu.RUnlock()

	if total <= 0 || processed <= 0 || startedAt.IsZero() {
		return 0
	}

	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}

	rate := float64(processed) / elapsed
	if rate <= 0 {
		return 0
	}

	estimatedTotal := float64(total) / rate
	if estimatedTotal <= 0 {
		return 0
	}

	return int64(math.Ceil(estimatedTotal))
}

func GetIPParsingEstimatedRemainingSeconds() int64 {
	parseProgressMu.RLock()
	total := parseProgress.TotalBytes
	processed := parseProgress.ProcessedBytes
	startedAt := parseProgress.StartedAt
	parseProgressMu.RUnlock()

	if total <= 0 || processed <= 0 || startedAt.IsZero() {
		return 0
	}

	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}

	rate := float64(processed) / elapsed
	if rate <= 0 {
		return 0
	}

	estimatedTotal := float64(total) / rate
	remaining := estimatedTotal - elapsed
	if remaining <= 0 {
		return 0
	}

	return int64(math.Ceil(remaining))
}
