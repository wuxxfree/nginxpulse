# Quick Start (EN)

This is the shortest path to a working setup.

## Option A: Docker single-container (recommended)
```bash
docker run -d --name nginxpulse \
  -p 8088:8088 -p 8089:8089 \
  -e WEBSITES='[{"name":"Main","logPath":"/share/log/nginx/access.log","domains":["example.com"]}]' \
  -v /path/to/nginx/access.log:/share/log/nginx/access.log:ro \
  -v /path/to/nginxpulse_data:/app/var/nginxpulse_data \
  -v /path/to/pgdata:/app/var/pgdata \
  -v /etc/localtime:/etc/localtime:ro \
  nginxpulse:latest
```

Open:
- UI: `http://localhost:8088`
- API: `http://localhost:8089`

## Option B: Single binary (no Docker)
Prerequisite: PostgreSQL must be installed and running.

1) Update `configs/nginxpulse_config.json`:
```json
"database": {
  "driver": "postgres",
  "dsn": "postgres://nginxpulse:nginxpulse@127.0.0.1:5432/nginxpulse?sslmode=disable"
}
```

2) Start the server (example):
```bash
./nginxpulse
```

## Required notice
Version > 1.5.3 requires PostgreSQL (SQLite is removed).

## Timezone
Logs are parsed in system timezone. Ensure host timezone is correct.
