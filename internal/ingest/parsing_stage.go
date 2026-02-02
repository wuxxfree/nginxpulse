package ingest

import "sync"

type parseStage string

const (
	parseStageNone     parseStage = ""
	parseStageInitial  parseStage = "initial"
	parseStagePeriodic parseStage = "periodic"
	parseStageReparse  parseStage = "reparse"
)

var (
	parseStageMu   sync.RWMutex
	currentStage  parseStage
)

func setParseStage(stage parseStage) {
	parseStageMu.Lock()
	currentStage = stage
	parseStageMu.Unlock()
}

func resetParseStage() {
	setParseStage(parseStageNone)
}

func GetLogParsingStage() string {
	parseStageMu.RLock()
	stage := currentStage
	parseStageMu.RUnlock()
	return string(stage)
}
