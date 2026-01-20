package ingest

import (
	"math"
	"sync"
	"time"
)

type ipGeoProgressState struct {
	Total     int64
	Processed int64
	StartedAt time.Time
	UpdatedAt time.Time
}

var (
	ipGeoProgressMu sync.RWMutex
	ipGeoProgress   ipGeoProgressState
)

func resetIPGeoProgress() {
	ipGeoProgressMu.Lock()
	ipGeoProgress = ipGeoProgressState{}
	ipGeoProgressMu.Unlock()
}

func reportIPGeoPendingCount(remaining int64) {
	if remaining < 0 {
		remaining = 0
	}

	now := time.Now()
	ipGeoProgressMu.Lock()
	defer ipGeoProgressMu.Unlock()

	if remaining == 0 {
		ipGeoProgress = ipGeoProgressState{}
		return
	}

	if ipGeoProgress.Total == 0 || ipGeoProgress.Processed >= ipGeoProgress.Total || ipGeoProgress.StartedAt.IsZero() {
		ipGeoProgress.Total = remaining
		ipGeoProgress.Processed = 0
		ipGeoProgress.StartedAt = now
		ipGeoProgress.UpdatedAt = now
		return
	}

	if remaining+ipGeoProgress.Processed > ipGeoProgress.Total {
		ipGeoProgress.Total = remaining + ipGeoProgress.Processed
	}
	ipGeoProgress.UpdatedAt = now
}

func touchIPGeoProgressStart() {
	now := time.Now()
	ipGeoProgressMu.Lock()
	if ipGeoProgress.Total > 0 && ipGeoProgress.Processed == 0 {
		ipGeoProgress.StartedAt = now
		ipGeoProgress.UpdatedAt = now
	}
	ipGeoProgressMu.Unlock()
}

func addIPGeoProcessed(delta int64) {
	if delta <= 0 {
		return
	}
	ipGeoProgressMu.Lock()
	ipGeoProgress.Processed += delta
	if ipGeoProgress.Total > 0 && ipGeoProgress.Processed > ipGeoProgress.Total {
		ipGeoProgress.Processed = ipGeoProgress.Total
	}
	ipGeoProgress.UpdatedAt = time.Now()
	ipGeoProgressMu.Unlock()
}

func finalizeIPGeoProgress() {
	ipGeoProgressMu.Lock()
	if ipGeoProgress.Total > 0 {
		ipGeoProgress.Processed = ipGeoProgress.Total
	}
	ipGeoProgress.UpdatedAt = time.Now()
	ipGeoProgressMu.Unlock()
}

func GetIPGeoParsingProgress(remaining int64) int {
	reportIPGeoPendingCount(remaining)

	ipGeoProgressMu.RLock()
	total := ipGeoProgress.Total
	processed := ipGeoProgress.Processed
	ipGeoProgressMu.RUnlock()

	if total <= 0 {
		return 0
	}

	progress := float64(processed) / float64(total)
	progress = math.Max(0, math.Min(progress, 1))
	return int(math.Round(progress * 100))
}

func GetIPGeoEstimatedRemainingSeconds(remaining int64) int64 {
	reportIPGeoPendingCount(remaining)

	ipGeoProgressMu.RLock()
	processed := ipGeoProgress.Processed
	startedAt := ipGeoProgress.StartedAt
	ipGeoProgressMu.RUnlock()

	if remaining <= 0 || processed <= 0 || startedAt.IsZero() {
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

	return int64(math.Ceil(float64(remaining) / rate))
}
