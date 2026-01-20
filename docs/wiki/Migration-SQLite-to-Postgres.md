# SQLite -> PostgreSQL 迁移

> SQLite 已弃用，版本 > 1.5.3 必须使用 PostgreSQL。

## 迁移策略说明
- 不做 SQLite 数据导入，直接重新解析日志并重建统计。
- 原 SQLite 数据文件会被标记清理（见下文）。

## 迁移检测规则
启动时满足以下条件即判定“需要迁移”，前端会提示弹窗：
- `var/nginxpulse_data/nginxpulse.db` 存在
- `var/nginxpulse_data/pg_migration_done` 不存在

完成迁移后会写入 `pg_migration_done`，后续版本升级不再弹窗。

## 迁移触发流程
用户点击确认后将执行：
1. 清空当前统计数据与访问明细。
2. 重新解析日志，写入 PostgreSQL。
3. 生成迁移标记文件并清理 SQLite 数据文件。

对应接口：
```http
POST /api/logs/reparse
{
  "id": "",
  "migration": true
}
```

## 手动触发重新解析
如果你需要手动重新解析（不走弹窗），可调用：
```http
POST /api/logs/reparse
{
  "id": ""
}
```

## 常见注意点
- 迁移完成前页面统计可能不完整，请等待解析结束。
- 迁移基于日志文件重新生成数据，请确保日志文件可读且未被清理。
- 修改 `websites[].name` 会导致站点 ID 变化，视为新站点。
