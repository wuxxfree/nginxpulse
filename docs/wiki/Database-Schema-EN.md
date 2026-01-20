# Database Schema (PostgreSQL)

## Naming
Site ID is derived from `websites[].name` (md5 first 4 chars). Use `{site}` below.

## Core tables
- `{site}_nginx_logs`: main log table (range partitioned by `timestamp`).
- `{site}_dim_ip` / `{site}_dim_url` / `{site}_dim_referer` / `{site}_dim_ua` / `{site}_dim_location`
- `{site}_agg_hourly` / `{site}_agg_daily`
- `{site}_agg_hourly_ip` / `{site}_agg_daily_ip`
- `{site}_first_seen`
- `{site}_sessions` / `{site}_session_state`
- `{site}_agg_session_daily` / `{site}_agg_entry_daily`

## IP geo tables
- `ip_geo_cache`: persistent IP -> location cache
- `ip_geo_pending`: pending queue

## Indexes
- `{site}_nginx_logs(timestamp)`
- `{site}_nginx_logs(timestamp, ip_id)` where pageview
- `{site}_nginx_logs(ip_id, ua_id, timestamp)` where pageview

## Notes
- The log table is partitioned but only a default partition is created now.
- Renaming a site creates a new set of tables.
