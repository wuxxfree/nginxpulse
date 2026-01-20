# 日志解析机制

## 整体流程
1. 初始扫描：启动时先解析“最近窗口”日志。
2. 增量扫描：定时任务按 `system.taskInterval` 继续扫描新增内容。
3. 历史回填：在后台逐步补齐历史日志（不阻塞实时解析）。
4. IP 归属地回填：解析日志后异步解析 IP 归属地并回填。

## 增量解析与状态文件
- 状态文件: `var/nginxpulse_data/nginx_scan_state.json`
- 若文件大小小于上次记录大小，视为轮转，从头解析。
- 站点 ID 由 `websites[].name` 生成，改名会产生新站点并重新解析。

## 批次与性能
- `system.parseBatchSize` 控制批次大小，默认 100。
- 也可通过环境变量 `LOG_PARSE_BATCH_SIZE` 覆盖。

## 解析进度与预计剩余
接口: `GET /api/status`
- `log_parsing_progress`: 解析进度（0~1）
- `log_parsing_estimated_remaining_seconds`: 预计剩余秒数
- `ip_geo_progress`: IP 归属地解析进度（0~1）
- `ip_geo_estimated_remaining_seconds`: IP 归属地预计剩余秒数

前端可按固定间隔轮询该接口以刷新进度。

## 10G+ 大日志优化思路
- 解析日志时只写入基础字段，IP 归属地放入待解析队列。
- 归属地解析在后台批量回填，不阻塞主解析。
- 如需更快：调大 `parseBatchSize`、提高机器 IO 或将日志按天切分。

## 日志清理
- `system.logRetentionDays` 控制保留天数。
- 清理任务在系统时间凌晨 2 点触发（按系统时区）。

## 常见注意点
- 若重启后重复解析，请确认没有残留进程占用同一端口。
- 日志路径支持通配符，注意匹配到的文件数量。
- gzip 日志会按文件全量解析（基于文件元信息判断是否变更）。
