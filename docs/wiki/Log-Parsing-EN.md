# Log Parsing

## Flow
1. Initial scan: parse recent window after startup.
2. Incremental scan: periodic scan by `system.taskInterval`.
3. Backfill: fill older logs in background.
4. IP geo backfill: resolve IP locations asynchronously.

## Incremental scan & state
- State file: `var/nginxpulse_data/nginx_scan_state.json`
- If current size < last size, the file is treated as rotated and re-parsed.
- Site ID is derived from `websites[].name`. Renaming creates a new site.

## Batch size
- `system.parseBatchSize` controls batch size (default 100).
- Can be overridden by `LOG_PARSE_BATCH_SIZE`.

## Progress & ETA
Endpoint: `GET /api/status`
- `log_parsing_progress`
- `log_parsing_estimated_remaining_seconds`
- `ip_geo_progress`
- `ip_geo_estimated_remaining_seconds`

Poll this endpoint to update progress in UI.

## 10G+ log optimization
- Parsing writes core fields first; IP geo is queued.
- IP geo is resolved in batches after parsing.
- For speed: increase `parseBatchSize`, use faster disk, or split logs by day.

## Retention
- `system.logRetentionDays` controls cleanup.
- Cleanup runs at 02:00 (system timezone).

## Notes
- If reparse happens on restart, make sure no stale process is running.
- Globs may match more files than expected.
- Gzip logs are parsed as full files based on metadata.
