# IP 归属地解析

## 解析顺序（从快到慢）
1. 数据库缓存（`ip_geo_cache`）
2. 本地库 ip2region（v4/v6）
3. 远程接口 `ip-api.com`（批量）

本地库优先返回；若结果为“未知”或解析失败，则调用远端补全。

## 自定义 IP 归属地 API
可通过 `system.ipGeoApiUrl` 或环境变量 `IP_GEO_API_URL` 指向自定义服务。
注意事项：编写 API 服务时，请务必严格按照本章节所述协议进行设计与返回，否则解析结果不可用。

请求协议：
- `POST` JSON，`Content-Type: application/json`
- 请求体为数组，每个元素包含：
  - `query`：IP 字符串
  - `fields`：返回字段列表（可忽略）
  - `lang`：语言（`zh-CN` / `en`，可忽略）

响应协议：
- 返回 JSON 数组（顺序与请求一致，或通过 `query` 回填）
- 每个元素必须包含以下字段（字段含义如下）：
  - `status`：`success` 表示成功，其他值视为失败
  - `message`：失败原因（可为空）
  - `query`：IP 字符串（用于匹配请求）
  - `country`：国家名称（用于全球维度）
  - `countryCode`：国家代码（如 `CN`、`US`）
  - `region`：区域代码（可为空）
  - `regionName`：省/州名称
  - `city`：城市名称
  - `isp`：运营商名称（可为空）

当 `status != success` 或地址字段为空时，会回填为“未知”。

## 解析流程
- 日志解析时，将 IP 写入 `ip_geo_pending` 队列。
- 定时任务批量处理 pending（默认每次 500 个）。
- 写入 `ip_geo_cache` 并回填日志表中的 location 维度。
- 缓存数量超过 `system.ipGeoCacheLimit` 时会清理最早记录。

## 解析状态与进度
接口: `GET /api/status`
- `ip_geo_parsing`: 是否正在解析
- `ip_geo_pending`: 是否存在待解析队列
- `ip_geo_progress`: 进度（0~1）
- `ip_geo_estimated_remaining_seconds`: 预计剩余秒数

## 结果展示
- 内网 IP: 国内显示“内网”，全球显示“本地网络”。
- 无法解析: 显示“未知”。
- 待解析期间: 可能显示“待解析”或“未知”。

## 常见问题
- 远端接口受网络影响，可能延迟或失败。
- IPv6 也优先本地库，只有本地无法解析才走远端。
- 若想减少远端调用，请确保本地库（v4/v6）可覆盖更多 IP 范围。
