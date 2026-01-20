package ingest

import (
	"strings"
	"sync"

	"github.com/likaia/nginxpulse/internal/enrich"
	"github.com/likaia/nginxpulse/internal/store"
	"github.com/sirupsen/logrus"
)

const (
	pendingLocationLabel     = "待解析"
	defaultIPGeoResolveBatch = 1000
)

var (
	ipGeoMu      sync.RWMutex
	ipGeoRunning bool
)

func startIPGeoParsing() bool {
	ipGeoMu.Lock()
	defer ipGeoMu.Unlock()
	if ipGeoRunning {
		return false
	}
	ipGeoRunning = true
	return true
}

func finishIPGeoParsing() {
	ipGeoMu.Lock()
	ipGeoRunning = false
	ipGeoMu.Unlock()
}

func IsIPGeoParsing() bool {
	ipGeoMu.RLock()
	defer ipGeoMu.RUnlock()
	return ipGeoRunning
}

// HasPendingIPGeo reports whether pending IP geo entries exist.
func (p *LogParser) HasPendingIPGeo() bool {
	if p == nil || p.repo == nil {
		return false
	}
	pending, err := p.repo.HasIPGeoPending()
	if err != nil {
		logrus.WithError(err).Warn("检测 IP 归属地待解析队列失败")
		return false
	}
	return pending
}

// GetIPGeoPendingCount returns the number of pending IP geo entries.
func (p *LogParser) GetIPGeoPendingCount() int64 {
	if p == nil || p.repo == nil {
		return 0
	}
	total, err := p.repo.CountIPGeoPending()
	if err != nil {
		logrus.WithError(err).Warn("读取 IP 归属地待解析数量失败")
		return 0
	}
	return total
}

// ProcessPendingIPGeo resolves pending IP geo entries and backfills locations.
func (p *LogParser) ProcessPendingIPGeo(limit int) int {
	if p == nil || p.repo == nil || p.demoMode {
		return 0
	}
	if IsIPParsing() {
		return 0
	}
	if !startIPGeoParsing() {
		return 0
	}
	defer finishIPGeoParsing()

	pendingTotal, err := p.repo.CountIPGeoPending()
	if err != nil {
		logrus.WithError(err).Warn("读取 IP 归属地待解析数量失败")
		return 0
	}
	if pendingTotal <= 0 {
		resetIPGeoProgress()
		return 0
	}
	reportIPGeoPendingCount(pendingTotal)
	touchIPGeoProgressStart()

	if limit <= 0 {
		limit = defaultIPGeoResolveBatch
	}

	pending, err := p.repo.FetchIPGeoPending(limit)
	if err != nil {
		logrus.WithError(err).Warn("读取 IP 归属地待解析队列失败")
		return 0
	}
	if len(pending) == 0 {
		return 0
	}

	results := make(map[string]store.IPGeoCacheEntry, len(pending))
	cached, err := p.repo.GetIPGeoCache(pending)
	if err != nil {
		logrus.WithError(err).Warn("读取 IP 归属地缓存失败")
	}

	missing := make([]string, 0, len(pending))
	for _, ip := range pending {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if entry, ok := cached[ip]; ok {
			results[ip] = entry
			continue
		}
		missing = append(missing, ip)
	}

	if len(missing) > 0 {
		fetched, fetchErr := enrich.GetIPLocationBatch(missing)
		if fetchErr != nil {
			logrus.WithError(fetchErr).Warn("IP 归属地远端查询失败，将保留待解析 IP")
		}
		for ip, loc := range fetched {
			results[ip] = store.IPGeoCacheEntry{
				Domestic: loc.Domestic,
				Global:   loc.Global,
				Source:   loc.Source,
			}
		}
		if p.repo != nil && len(fetched) > 0 {
			entries := make(map[string]store.IPGeoCacheEntry, len(fetched))
			for ip, loc := range fetched {
				entries[ip] = store.IPGeoCacheEntry{
					Domestic: loc.Domestic,
					Global:   loc.Global,
					Source:   loc.Source,
				}
			}
			if err := p.repo.UpsertIPGeoCache(entries); err != nil {
				logrus.WithError(err).Warn("写入 IP 归属地缓存失败")
			}
			if p.ipGeoCacheLimit > 0 {
				if err := p.repo.TrimIPGeoCache(p.ipGeoCacheLimit); err != nil {
					logrus.WithError(err).Warn("清理 IP 归属地缓存失败")
				}
			}
		}
		for _, ip := range pending {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if _, ok := results[ip]; ok {
				continue
			}
			results[ip] = store.IPGeoCacheEntry{
				Domestic: "未知",
				Global:   "未知",
				Source:   "unknown",
			}
		}
	} else {
		for _, ip := range pending {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if _, ok := results[ip]; ok {
				continue
			}
			results[ip] = store.IPGeoCacheEntry{
				Domestic: "未知",
				Global:   "未知",
				Source:   "unknown",
			}
		}
	}

	if err := p.repo.UpdateIPGeoLocations(results, pendingLocationLabel); err != nil {
		logrus.WithError(err).Warn("回填 IP 归属地失败")
		return 0
	}

	resolved := make([]string, 0, len(results))
	for ip := range results {
		resolved = append(resolved, ip)
	}
	if len(resolved) > 0 {
		if err := p.repo.DeleteIPGeoPending(resolved); err != nil {
			logrus.WithError(err).Warn("清理 IP 归属地待解析队列失败")
			return 0
		}
	}

	addIPGeoProcessed(int64(len(resolved)))
	if pendingTotal <= int64(len(resolved)) {
		finalizeIPGeoProgress()
	}

	return len(resolved)
}
