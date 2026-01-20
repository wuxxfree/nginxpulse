package web

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/likaia/nginxpulse/internal/analytics"
	"github.com/likaia/nginxpulse/internal/config"
)

const exportBatchSize = 1000
const csvContentType = "text/csv; charset=utf-8"

func exportLogsCSV(
	writer io.Writer,
	statsFactory *analytics.StatsFactory,
	query analytics.StatsQuery,
	lang string,
) error {
	manager, ok := statsFactory.GetManager("logs")
	if !ok {
		return fmt.Errorf("\u65e5\u5fd7\u7ba1\u7406\u5668\u672a\u521d\u59cb\u5316")
	}

	if _, err := writer.Write([]byte("\ufeff")); err != nil {
		return err
	}

	normalizedLang := normalizeExportLang(lang)

	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(logsExportHeaders(normalizedLang)); err != nil {
		return err
	}

	for page := 1; ; page++ {
		query.ExtraParam["page"] = page
		query.ExtraParam["pageSize"] = exportBatchSize

		result, err := manager.Query(query)
		if err != nil {
			return err
		}
		logsResult, ok := result.(analytics.LogsStats)
		if !ok {
			return fmt.Errorf("\u65e5\u5fd7\u5bfc\u51fa\u7ed3\u679c\u89e3\u6790\u5931\u8d25")
		}
		if len(logsResult.Logs) == 0 {
			break
		}

		for _, log := range logsResult.Logs {
			row := buildLogExportRow(log, normalizedLang)
			if err := csvWriter.Write(row); err != nil {
				return err
			}
		}

		if logsResult.Pagination.Pages > 0 && page >= logsResult.Pagination.Pages {
			break
		}
	}

	csvWriter.Flush()
	return csvWriter.Error()
}

func buildLogExportRow(log analytics.LogEntry, lang string) []string {
	location := strings.TrimSpace(log.DomesticLocation)
	if location == "" {
		location = strings.TrimSpace(log.GlobalLocation)
	}
	if location == "" {
		location = "-"
	}

	requestText := strings.TrimSpace(fmt.Sprintf("%s %s", log.Method, log.URL))
	if requestText == "" {
		requestText = "-"
	}

	referer := strings.TrimSpace(log.Referer)
	if referer == "" {
		referer = "-"
	}

	browser := strings.TrimSpace(log.UserBrowser)
	if browser == "" {
		browser = "-"
	}

	os := strings.TrimSpace(log.UserOS)
	if os == "" {
		os = "-"
	}

	device := strings.TrimSpace(log.UserDevice)
	if device == "" {
		device = "-"
	}

	pvText := "\u5426"
	if lang == config.EnglishLanguage {
		pvText = "No"
	}
	if log.PageviewFlag {
		pvText = "\u662f"
		if lang == config.EnglishLanguage {
			pvText = "Yes"
		}
	}

	timeText := log.Time
	if timeText == "" && log.Timestamp > 0 {
		timeText = time.Unix(log.Timestamp, 0).Format("2006-01-02 15:04:05")
	}

	return []string{
		timeText,
		log.IP,
		location,
		requestText,
		strconv.Itoa(log.StatusCode),
		strconv.FormatInt(int64(log.BytesSent), 10),
		referer,
		browser,
		os,
		device,
		pvText,
	}
}

func logsExportHeaders(lang string) []string {
	if lang == config.EnglishLanguage {
		return []string{
			"Time",
			"IP",
			"Location",
			"Request",
			"Status",
			"Bytes",
			"Referer",
			"Browser",
			"OS",
			"Device",
			"PV",
		}
	}
	return []string{
		"\u65f6\u95f4",
		"IP",
		"\u4f4d\u7f6e",
		"\u8bf7\u6c42",
		"\u72b6\u6001\u7801",
		"\u6d41\u91cf",
		"\u6765\u6e90",
		"\u6d4f\u89c8\u5668",
		"\u7cfb\u7edf",
		"\u8bbe\u5907",
		"PV",
	}
}

func normalizeExportLang(lang string) string {
	normalized := config.NormalizeLanguage(lang)
	if normalized == "" {
		return config.GetLanguage()
	}
	return normalized
}
