# 快速开始

这是一条最短路径，几分钟内跑起来。

## 方案 A：Docker 单容器（推荐）
```bash
docker run -d --name nginxpulse \
  -p 8088:8088 -p 8089:8089 \
  -e WEBSITES='[{"name":"主站","logPath":"/share/log/nginx/access.log","domains":["example.com"]}]' \
  -v /path/to/nginx/access.log:/share/log/nginx/access.log:ro \
  -v /path/to/nginxpulse_data:/app/var/nginxpulse_data \
  -v /path/to/pgdata:/app/var/pgdata \
  -v /etc/localtime:/etc/localtime:ro \
  nginxpulse:latest
```

打开：
- 前端: `http://localhost:8088`
- API: `http://localhost:8089`

## 方案 B：单体部署（非 Docker）
前提：必须安装并启动 PostgreSQL。

1) 修改 `configs/nginxpulse_config.json`：
```json
"database": {
  "driver": "postgres",
  "dsn": "postgres://nginxpulse:nginxpulse@127.0.0.1:5432/nginxpulse?sslmode=disable"
}
```

2) 启动服务（示例）：
```bash
./nginxpulse
```

## 必读提醒
版本 > 1.5.3 必须部署 PostgreSQL（SQLite 已移除）。

## 时区
本项目使用系统时区解析日志，请确保运行环境时区正确。
