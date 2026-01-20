# 数据库结构（PostgreSQL）

## 命名规则
站点 ID 由 `websites[].name` 生成（md5 前 4 位）。以下以 `{site}` 表示站点 ID。

## 核心表
- `{site}_nginx_logs`: 主日志表（按 `timestamp` 分区，当前默认分区为 `{site}_nginx_logs_default`）。
- `{site}_dim_ip` / `{site}_dim_url` / `{site}_dim_referer` / `{site}_dim_ua` / `{site}_dim_location`: 维表。
- `{site}_agg_hourly` / `{site}_agg_daily`: 聚合统计（按小时 / 日）。
- `{site}_agg_hourly_ip` / `{site}_agg_daily_ip`: IP 维度聚合。
- `{site}_first_seen`: 首次访问时间。
- `{site}_sessions` / `{site}_session_state`: 会话明细与状态。
- `{site}_agg_session_daily` / `{site}_agg_entry_daily`: 会话与入口聚合。

## IP 归属地相关
- `ip_geo_cache`: IP -> 归属地缓存（持久化，带容量限制）。
- `ip_geo_pending`: 待解析队列。

## 主要索引
- `{site}_nginx_logs(timestamp)`
- `{site}_nginx_logs(timestamp, ip_id)` 仅 pageview 记录
- `{site}_nginx_logs(ip_id, ua_id, timestamp)` 仅 pageview 记录

## 说明
- 主表为分区表，但当前默认仅创建默认分区，未来可扩展按时间分区。
- 站点改名会导致新建一套表结构。
