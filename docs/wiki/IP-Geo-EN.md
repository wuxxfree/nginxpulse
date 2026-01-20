# IP Geo

## Resolution order (fast to slow)
1. DB cache (`ip_geo_cache`)
2. Local ip2region (v4/v6)
3. Remote `ip-api.com` (batch)

If local lookup fails or returns "unknown", remote lookup is used.

## Custom IP Geo API
You can configure a custom endpoint via `system.ipGeoApiUrl` or `IP_GEO_API_URL`.
Note: when implementing a custom API service, follow the contract in this section strictly; otherwise the parser will not work correctly.

Request:
- `POST` JSON with `Content-Type: application/json`
- Body is an array; each item contains:
  - `query`: IP string
  - `fields`: requested fields (can be ignored)
  - `lang`: language (`zh-CN` / `en`, can be ignored)

Response:
- JSON array (same order as request, or match by `query`)
- Each item must include the fields below:
  - `status`: `success` for success, other values treated as failure
  - `message`: error details (optional)
  - `query`: IP string (used to match results)
  - `country`: country name (global dimension)
  - `countryCode`: country code (e.g. `CN`, `US`)
  - `region`: region code (optional)
  - `regionName`: state/province name
  - `city`: city name
  - `isp`: ISP name (optional)

If `status != success` or location fields are empty, the result is stored as "unknown".

## Flow
- Parsing writes IPs into `ip_geo_pending`.
- A background task processes pending in batches (default 500).
- Results are stored in `ip_geo_cache` and backfilled into log tables.
- Cache is trimmed when exceeding `system.ipGeoCacheLimit`.

## Status & progress
Endpoint: `GET /api/status`
- `ip_geo_parsing`
- `ip_geo_pending`
- `ip_geo_progress`
- `ip_geo_estimated_remaining_seconds`

## Display
- Private IPs: "内网" / "本地网络".
- Unresolved: "未知".
- During parsing: may show "待解析" or "未知".

## Notes
- Remote API depends on network availability.
- IPv6 also uses local first and falls back to remote only when needed.
- Improve hit rate by keeping ip2region (v4/v6) updated.
