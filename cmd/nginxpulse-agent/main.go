package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type agentConfig struct {
	Server        string   `json:"server"`
	AccessKey     string   `json:"accessKey"`
	WebsiteID     string   `json:"websiteID"`
	SourceID      string   `json:"sourceID"`
	Paths         []string `json:"paths"`
	PollInterval  string   `json:"pollInterval"`
	BatchSize     int      `json:"batchSize"`
	FlushInterval string   `json:"flushInterval"`
	// InitialTailBytes：首次读取某个文件时（offset=0），如果文件很大，则只从文件尾部读取最近 N 字节。
	// 目的：避免 agent 第一次启动就把历史日志全量推送，导致 server/pgsql 被打爆。
	// 约定：
	// - 0：使用默认值（8MiB）
	// - >0：使用指定值
	// - <0：禁用 tail（从文件头开始读，兼容“全量回放”诉求）
	InitialTailBytes int64 `json:"initialTailBytes"`
	// InitialMaxLines：首次读取某个文件时（offset=0），最多读取多少行。
	// 设为 0 表示不额外限制（仍会受 maxPendingLines 限制）。
	InitialMaxLines int `json:"initialMaxLines"`
	// RequestTimeout：推送日志时的 HTTP 请求超时（例如 "30s", "2m"）。
	RequestTimeout string `json:"requestTimeout"`
	// MaxPendingLines：内存中待发送缓冲区（pending）的最大积压行数；达到后会暂停继续读取新日志，直到积压被发送消化。
	MaxPendingLines int `json:"maxPendingLines"`
	// MaxLineBytes：单行日志的最大字节数（byte）。
	// 超过该上限的行会被跳过（打印 warning），用于避免异常超长行导致巨大内存分配/容器 OOM。
	// 默认：256KiB。
	MaxLineBytes int `json:"maxLineBytes"`
	// RetryBackoffMin：推送失败后的最小退避时间（例如 "1s"）。
	RetryBackoffMin string `json:"retryBackoffMin"`
	// RetryBackoffMax：连续失败时退避时间的最大上限（例如 "30s"）。
	RetryBackoffMax string `json:"retryBackoffMax"`
	// ExitOnMaxBackoff：当退避已达到 RetryBackoffMax 且再次推送仍失败时，是否直接退出进程（让 k8s 重启容器）。
	// 默认：false。
	ExitOnMaxBackoff bool `json:"exitOnMaxBackoff"`
}

type ingestRequest struct {
	WebsiteID string   `json:"website_id"`
	SourceID  string   `json:"source_id"`
	Lines     []string `json:"lines"`
}

type fileState struct {
	offset   int64
	lastSize int64
	partial  string
}

type readStats struct {
	path      string
	from      int64
	to        int64
	fileSize  int64
	lines     int
	bytes     int64
	hasPartial bool
	skippedLines int
	maxLineBytes int
}

func main() {
	configPath := flag.String("config", "configs/nginxpulse_agent.json", "agent config path")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		logrus.WithError(err).Error("加载 agent 配置失败")
		os.Exit(1)
	}
	applyEnvOverrides(cfg)

	pollInterval := parseDuration(cfg.PollInterval, time.Second)
	flushInterval := parseDuration(cfg.FlushInterval, 2*time.Second)
	requestTimeout := parseDuration(cfg.RequestTimeout, 90*time.Second)
	backoffMin := parseDuration(cfg.RetryBackoffMin, time.Second)
	backoffMax := parseDuration(cfg.RetryBackoffMax, 30*time.Second)
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	maxPending := cfg.MaxPendingLines
	if maxPending <= 0 {
		maxPending = 5000
	}
	maxLineBytes := cfg.MaxLineBytes
	if maxLineBytes <= 0 {
		maxLineBytes = 256 * 1024
	}
	initialTailBytes := cfg.InitialTailBytes
	// 默认：只从尾部读一小段，避免首次启动灌入历史日志。
	// 说明：
	// - initialTailBytes == 0：使用默认值（8MiB）
	// - initialTailBytes  > 0：使用指定值
	// - initialTailBytes  < 0：禁用 tail（从文件头开始读）
	if initialTailBytes == 0 {
		initialTailBytes = 8 * 1024 * 1024
	} else if initialTailBytes < 0 {
		initialTailBytes = 0
	}
	if initialTailBytes != 0 && initialTailBytes < 1*1024*1024 {
		// 给一个下限，避免配置过小导致频繁从中间切入且有效行过少
		initialTailBytes = 1 * 1024 * 1024
	}
	initialMaxLines := cfg.InitialMaxLines
	if initialMaxLines < 0 {
		initialMaxLines = 0
	}
	sourceID := strings.TrimSpace(cfg.SourceID)
	if sourceID == "" {
		sourceID = "agent"
	}

	endpoint := strings.TrimRight(cfg.Server, "/") + "/api/ingest/logs"
	states := make(map[string]*fileState)
	pending := make([]string, 0, batchSize)
	var (
		nextPushAt    time.Time
		failures      int
		reachedMax    bool
		lastErrLogged time.Time
		lastMemLogged time.Time
		lastReadLogged time.Time
		lastPushLogged time.Time
		lastBackpressureLogged time.Time
	)
	// 用于比较的“有效最大退避时间”（computeBackoff 在 max<=0 时会使用默认值）。
	effectiveBackoffMax := backoffMax
	if effectiveBackoffMax <= 0 {
		effectiveBackoffMax = 30 * time.Second
	}

	logrus.WithFields(logrus.Fields{
		"endpoint":              endpoint,
		"poll_interval":         pollInterval.String(),
		"flush_interval":        flushInterval.String(),
		"batch_size":            batchSize,
		"max_pending_lines":     maxPending,
		"max_line_bytes":        maxLineBytes,
		"initial_tail_bytes":    initialTailBytes,
		"initial_max_lines":     initialMaxLines,
		"request_timeout":       requestTimeout.String(),
		"retry_backoff_min":     backoffMin.String(),
		"retry_backoff_max":     effectiveBackoffMax.String(),
		"exit_on_max_backoff":   cfg.ExitOnMaxBackoff,
		"paths":                 cfg.Paths,
		"website_id":            cfg.WebsiteID,
		"source_id":             sourceID,
	}).Info("nginxpulse-agent: config loaded")

	pollTicker := time.NewTicker(pollInterval)
	flushTicker := time.NewTicker(flushInterval)
	defer pollTicker.Stop()
	defer flushTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			// 背压：如果 pending 积压过大，则暂停读取，直到成功推送一部分数据。
			if len(pending) >= maxPending {
				if time.Since(lastBackpressureLogged) > 10*time.Second {
					lastBackpressureLogged = time.Now()
					logrus.WithFields(logrus.Fields{
						"pending_lines":      len(pending),
						"max_pending_lines":  maxPending,
						"failures":           failures,
						"next_push_in":       durationUntil(nextPushAt).Truncate(time.Millisecond).String(),
					}).Warn("pending buffer is full; pausing reads")
				}
				continue
			}
			for _, path := range cfg.Paths {
				if len(pending) >= maxPending {
					break
				}
				if strings.HasSuffix(strings.ToLower(path), ".gz") {
					continue
				}
				state := states[path]
				if state == nil {
					state = &fileState{}
					states[path] = state
				}
				// 读取限流：避免一次性读出海量行导致内存暴涨/推送洪峰。
				remaining := maxPending - len(pending)
				if remaining <= 0 {
					break
				}
				lines, st, err := readNewLines(path, state, maxLineBytes, remaining, initialTailBytes, initialMaxLines)
				if err != nil {
					logrus.WithError(err).Warnf("读取日志失败: %s", path)
					continue
				}
				if st.lines == 0 {
					continue
				}
				// 大读取/周期性摘要日志：用于辅助定位 OOM 与积压问题。
				if st.lines >= batchSize || st.bytes >= 4*1024*1024 || time.Since(lastReadLogged) > 30*time.Second {
					lastReadLogged = time.Now()
					logrus.WithFields(logrus.Fields{
						"path":          st.path,
						"lines":         st.lines,
						"skipped_lines": st.skippedLines,
						"max_line_bytes": st.maxLineBytes,
						"bytes":         formatBytes(st.bytes),
						"file_size":     formatBytes(st.fileSize),
						"offset_from":   st.from,
						"offset_to":     st.to,
						"offset_delta":  st.to - st.from,
						"has_partial":   st.hasPartial,
						"pending_lines": len(pending),
					}).Info("read new lines")
				}
				pending = append(pending, lines...)
				if len(pending) >= batchSize {
					// 遵守退避窗口：在 backoff 时间内不进行推送尝试。
					if !nextPushAt.IsZero() && time.Now().Before(nextPushAt) {
						continue
					}
					if err := pushLines(requestTimeout, endpoint, cfg.AccessKey, cfg.WebsiteID, sourceID, pending); err != nil {
						failures++
						delay := computeBackoff(failures, backoffMin, backoffMax)
						// 如果此前已达到最大退避，并且等待后依然失败，则按配置可选择直接退出进程。
						if cfg.ExitOnMaxBackoff && reachedMax && delay >= effectiveBackoffMax {
							logrus.WithError(err).Errorf("日志推送连续失败且退避已达上限 %s，终止 agent 进程", effectiveBackoffMax)
							os.Exit(1)
						}
						nextPushAt = time.Now().Add(delay)
						reachedMax = delay >= effectiveBackoffMax
						// 避免刷屏：最多每 5 秒打印一次 warning。
						if time.Since(lastErrLogged) > 5*time.Second {
							lastErrLogged = time.Now()
							logrus.WithError(err).Warnf("日志推送失败，将在 %s 后重试", time.Until(nextPushAt).Truncate(time.Millisecond))
							logrus.WithFields(logrus.Fields{
								"pending_lines":      len(pending),
								"batch_size":         batchSize,
								"failures":           failures,
								"backoff_next":       delay.String(),
								"backoff_max":        effectiveBackoffMax.String(),
								"reached_max_backoff": reachedMax,
							}).Warn("push failed (debug)")
						}
						continue
					}
					// 成功后：周期性打印推送摘要，方便观测吞吐与 pending 容量变化。
					if time.Since(lastPushLogged) > 30*time.Second || failures > 0 {
						lastPushLogged = time.Now()
						logrus.WithFields(logrus.Fields{
							"pushed_lines":     len(pending),
							"pending_lines":    len(pending),
							"pending_cap":      cap(pending),
							"failures_reset":   failures,
						}).Info("push succeeded")
					}
					pending = resetPending(pending, batchSize, maxPending)
					failures = 0
					reachedMax = false
					nextPushAt = time.Time{}
				}
			}
			// 周期性内存统计：用于与 OOMKilled 时间点对齐分析。
			if time.Since(lastMemLogged) > 30*time.Second {
				lastMemLogged = time.Now()
				logMemStats("mem")
				logrus.WithFields(logrus.Fields{
					"pending_lines":     len(pending),
					"batch_size":        batchSize,
					"max_pending_lines": maxPending,
					"failures":          failures,
					"next_push_in":      durationUntil(nextPushAt).Truncate(time.Millisecond).String(),
				}).Info("agent status")
			}
		case <-flushTicker.C:
			if len(pending) == 0 {
				continue
			}
			if !nextPushAt.IsZero() && time.Now().Before(nextPushAt) {
				continue
			}
			if err := pushLines(requestTimeout, endpoint, cfg.AccessKey, cfg.WebsiteID, sourceID, pending); err != nil {
				failures++
				delay := computeBackoff(failures, backoffMin, backoffMax)
				if cfg.ExitOnMaxBackoff && reachedMax && delay >= effectiveBackoffMax {
					logrus.WithError(err).Errorf("日志推送连续失败且退避已达上限 %s，终止 agent 进程", effectiveBackoffMax)
					os.Exit(1)
				}
				nextPushAt = time.Now().Add(delay)
				reachedMax = delay >= effectiveBackoffMax
				if time.Since(lastErrLogged) > 5*time.Second {
					lastErrLogged = time.Now()
					logrus.WithError(err).Warnf("日志推送失败，将在 %s 后重试", time.Until(nextPushAt).Truncate(time.Millisecond))
					logrus.WithFields(logrus.Fields{
						"pending_lines":      len(pending),
						"batch_size":         batchSize,
						"failures":           failures,
						"backoff_next":       delay.String(),
						"backoff_max":        effectiveBackoffMax.String(),
						"reached_max_backoff": reachedMax,
					}).Warn("push failed on flush tick (debug)")
				}
				continue
			}
			if time.Since(lastPushLogged) > 30*time.Second || failures > 0 {
				lastPushLogged = time.Now()
				logrus.WithFields(logrus.Fields{
					"pushed_lines":   len(pending),
					"pending_cap":    cap(pending),
					"trigger":        "flush_interval",
				}).Info("push succeeded")
			}
			pending = resetPending(pending, batchSize, maxPending)
			failures = 0
			reachedMax = false
			nextPushAt = time.Time{}
		}
	}
}

func loadConfig(path string) (*agentConfig, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			absPath = filepath.Join(cwd, path)
		}
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	cfg := &agentConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Server) == "" {
		return nil, errors.New("server 不能为空")
	}
	if strings.TrimSpace(cfg.WebsiteID) == "" {
		return nil, errors.New("websiteID 不能为空")
	}
	if len(cfg.Paths) == 0 {
		return nil, errors.New("paths 不能为空")
	}
	return cfg, nil
}

func readNewLines(path string, state *fileState, maxLineBytes int, maxLines int, initialTailBytes int64, initialMaxLines int) ([]string, readStats, error) {
	stats := readStats{path: path}
	stats.maxLineBytes = maxLineBytes
	info, err := os.Stat(path)
	if err != nil {
		return nil, stats, err
	}
	size := info.Size()
	stats.fileSize = size
	// 文件被截断/轮转：offset 回到 0，视为“首次读取”场景。
	if size < state.offset {
		state.offset = 0
		state.partial = ""
		state.lastSize = 0
	}
	if size == state.offset {
		return nil, stats, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, stats, err
	}
	defer file.Close()

	// 首次读取：如果文件很大，则从尾部截断读取，避免一次性回放历史日志。
	// 触发条件：offset=0 且 lastSize=0（包含“截断/轮转后 offset 重置”的场景）。
	skipFirstPartial := false
	isFirstRead := state.offset == 0 && state.lastSize == 0 && state.partial == ""
	if isFirstRead && initialTailBytes > 0 && size > initialTailBytes {
		state.offset = size - initialTailBytes
		skipFirstPartial = true
	}

	stats.from = state.offset
	if _, err := file.Seek(state.offset, io.SeekStart); err != nil {
		return nil, stats, err
	}

	// Use ReadSlice-based line reading with a bounded line size to avoid huge allocations
	// when input contains abnormally long lines.
	reader := bufio.NewReaderSize(file, 64*1024)
	lines := []string{}
	seed := state.partial
	state.partial = ""

	// 如果从文件中间开始读取（tail 截断），先丢弃“半行”，从下一行开始，避免产生残缺 JSON。
	// 注意：这一步也要计入 offset 和 stats.bytes。
	if skipFirstPartial {
		discarded, err := discardUntilNextNewline(reader)
		if err != nil {
			return nil, stats, err
		}
		if discarded > 0 {
			state.offset += discarded
			stats.bytes += discarded
		}
		seed = ""
	}

	// 首次读取时的额外行数上限（仍会受 maxLines 限制）。
	if isFirstRead && initialMaxLines > 0 {
		if maxLines <= 0 || initialMaxLines < maxLines {
			maxLines = initialMaxLines
		}
	}

	var lastOverlongLogged time.Time
	logOverlong := func(actualLineBytes int64, from, to int64) {
		// avoid log spam if the input continuously produces overlong lines
		if time.Since(lastOverlongLogged) < 5*time.Second {
			return
		}
		lastOverlongLogged = time.Now()
		logrus.WithFields(logrus.Fields{
			"path":           path,
			"max_line_bytes": maxLineBytes,
			"line_bytes":     actualLineBytes,
			"offset_from":    from,
			"offset_to":      to,
		}).Warn("skipping overlong log line (exceeds maxLineBytes)")
	}

	for {
		if maxLines > 0 && stats.lines >= maxLines {
			break
		}
		line, overlong, bytesRead, hasNewline, eof, err, actualLineBytes := readOneLineLimited(reader, maxLineBytes, seed)
		seed = ""
		if bytesRead > 0 {
			state.offset += bytesRead
			stats.bytes += bytesRead
		}
		if err != nil {
			return lines, stats, err
		}
		if bytesRead == 0 && eof {
			break
		}
		if overlong {
			stats.skippedLines++
			logOverlong(actualLineBytes, state.offset-bytesRead, state.offset)
			// Overlong line discarded; do not carry partial forward.
			if eof && !hasNewline {
				state.partial = ""
				break
			}
			continue
		}
		if eof && !hasNewline {
			// store bounded partial for next read
			if line != "" {
				state.partial = line
				stats.hasPartial = true
			}
			break
		}
		if line != "" {
			lines = append(lines, line)
			stats.lines++
		}
	}
	state.lastSize = size
	stats.to = state.offset
	return lines, stats, nil
}

// discardUntilNextNewline consumes bytes until the next '\n' (inclusive) or EOF.
// It returns the number of bytes discarded from reader.
func discardUntilNextNewline(reader *bufio.Reader) (int64, error) {
	var discarded int64
	for {
		frag, err := reader.ReadSlice('\n')
		if len(frag) > 0 {
			discarded += int64(len(frag))
			// got newline
			if frag[len(frag)-1] == '\n' {
				return discarded, nil
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			// continue consuming the same long line
			continue
		}
		if errors.Is(err, io.EOF) {
			return discarded, nil
		}
		return discarded, err
	}
}

// readOneLineLimited reads one logical line (terminated by '\n' or EOF) without ever buffering more than maxLineBytes.
// It consumes the full line from reader. If the actual line length exceeds maxLineBytes, overlong=true and line content is discarded.
// bytesRead counts the bytes consumed from reader for this line (including '\n' if present).
// actualLineBytes reports the total bytes of the line content excluding trailing '\n' and '\r' (includes seed bytes).
func readOneLineLimited(reader *bufio.Reader, maxLineBytes int, seed string) (line string, overlong bool, bytesRead int64, hasNewline bool, eof bool, err error, actualLineBytes int64) {
	if maxLineBytes <= 0 {
		maxLineBytes = 256 * 1024
	}

	var buf bytes.Buffer
	if seed != "" {
		// seed comes from previous EOF partial; it is already considered "line content"
		if len(seed) > maxLineBytes {
			overlong = true
			actualLineBytes += int64(len(seed))
		} else {
			buf.WriteString(seed)
			actualLineBytes += int64(len(seed))
		}
	}

	tooLong := overlong
	for {
		frag, e := reader.ReadSlice('\n')
		if len(frag) > 0 {
			bytesRead += int64(len(frag))
			// compute content bytes in this fragment (exclude trailing newline and optional CR before it)
			contentLen := len(frag)
			if frag[len(frag)-1] == '\n' {
				contentLen--
				hasNewline = true
				if contentLen > 0 && frag[contentLen-1] == '\r' {
					contentLen--
				}
			}
			if contentLen < 0 {
				contentLen = 0
			}
			actualLineBytes += int64(contentLen)

			if !tooLong && contentLen > 0 {
				remain := maxLineBytes - buf.Len()
				if remain <= 0 {
					tooLong = true
				} else if contentLen <= remain {
					buf.Write(frag[:contentLen])
				} else {
					buf.Write(frag[:remain])
					tooLong = true
				}
			}
		}

		if e == nil {
			break
		}
		if errors.Is(e, bufio.ErrBufferFull) {
			// continue reading more fragments of the same line
			continue
		}
		if e == io.EOF {
			eof = true
			break
		}
		return "", false, bytesRead, hasNewline, eof, e, actualLineBytes
	}

	if tooLong {
		return "", true, bytesRead, hasNewline, eof, nil, actualLineBytes
	}
	return buf.String(), false, bytesRead, hasNewline, eof, nil, actualLineBytes
}

func pushLines(timeout time.Duration, endpoint, accessKey, websiteID, sourceID string, lines []string) error {
	payload := ingestRequest{
		WebsiteID: websiteID,
		SourceID:  sourceID,
		Lines:     lines,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(accessKey) != "" {
		req.Header.Set("X-NginxPulse-Key", strings.TrimSpace(accessKey))
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func applyEnvOverrides(cfg *agentConfig) {
	if cfg == nil {
		return
	}
	// 说明：配置文件为主，环境变量可覆盖（便于 k8s/daemonset 管理）。
	// 仅覆盖可选项，不覆盖 server/website 等必填项（避免误配置导致 agent 启动失败）。
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_POLL_INTERVAL"); ok && strings.TrimSpace(v) != "" {
		cfg.PollInterval = v
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_FLUSH_INTERVAL"); ok && strings.TrimSpace(v) != "" {
		cfg.FlushInterval = v
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_INITIAL_TAIL_BYTES"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			cfg.InitialTailBytes = n
		}
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_INITIAL_MAX_LINES"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.InitialMaxLines = n
		}
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_BATCH_SIZE"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.BatchSize = n
		}
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_REQUEST_TIMEOUT"); ok && strings.TrimSpace(v) != "" {
		cfg.RequestTimeout = v
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_MAX_PENDING_LINES"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.MaxPendingLines = n
		}
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_MAX_LINE_BYTES"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.MaxLineBytes = n
		}
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_RETRY_BACKOFF_MIN"); ok && strings.TrimSpace(v) != "" {
		cfg.RetryBackoffMin = v
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_RETRY_BACKOFF_MAX"); ok && strings.TrimSpace(v) != "" {
		cfg.RetryBackoffMax = v
	}
	if v, ok := os.LookupEnv("NGINXPULSE_AGENT_EXIT_ON_MAX_BACKOFF"); ok && strings.TrimSpace(v) != "" {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			cfg.ExitOnMaxBackoff = b
		}
	}
}

func computeBackoff(failures int, min, max time.Duration) time.Duration {
	if failures <= 0 {
		return min
	}
	if min <= 0 {
		min = time.Second
	}
	if max <= 0 {
		max = 30 * time.Second
	}
	// 指数退避并封顶：min * 2^(failures-1)，最大不超过 max。
	delay := min
	for i := 1; i < failures; i++ {
		if delay >= max {
			return max
		}
		delay *= 2
	}
	if delay > max {
		delay = max
	}
	return delay
}

func logMemStats(prefix string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logrus.WithFields(logrus.Fields{
		"heap_alloc":  formatBytes(int64(m.HeapAlloc)),
		"heap_inuse":  formatBytes(int64(m.HeapInuse)),
		"heap_sys":    formatBytes(int64(m.HeapSys)),
		"stack_inuse": formatBytes(int64(m.StackInuse)),
		"sys":         formatBytes(int64(m.Sys)),
		"num_gc":      m.NumGC,
		"gc_pause_ms": time.Duration(m.PauseTotalNs).Milliseconds(),
	}).Info(prefix)
}

func durationUntil(t time.Time) time.Duration {
	if t.IsZero() {
		return 0
	}
	d := time.Until(t)
	if d < 0 {
		return 0
	}
	return d
}

func formatBytes(v int64) string {
	if v < 0 {
		v = 0
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case v >= gb:
		return fmt.Sprintf("%.2fGiB", float64(v)/float64(gb))
	case v >= mb:
		return fmt.Sprintf("%.2fMiB", float64(v)/float64(mb))
	case v >= kb:
		return fmt.Sprintf("%.2fKiB", float64(v)/float64(kb))
	default:
		return fmt.Sprintf("%dB", v)
	}
}

// resetPending 用于释放 pending slice 持有的引用。
// 这里采用“始终换新 slice”的策略：每次推送成功后都丢弃旧的底层数组，
// 让 GC 更容易回收历史积压/异常输入导致的内存占用，从而抑制 heap_sys 长期走高。
func resetPending(pending []string, batchSize, maxPending int) []string {
	_ = pending
	_ = maxPending
	return make([]string, 0, batchSize)
}
