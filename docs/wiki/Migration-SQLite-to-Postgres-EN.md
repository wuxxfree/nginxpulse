# SQLite -> PostgreSQL Migration

> SQLite is deprecated. Version > 1.5.3 requires PostgreSQL.

## Strategy
- No SQLite data import. We rebuild by re-parsing logs.
- The legacy SQLite file will be removed after migration.

## Detection rule
Migration notice appears when:
- `var/nginxpulse_data/nginxpulse.db` exists, and
- `var/nginxpulse_data/pg_migration_done` does not exist.

Once migration is done, `pg_migration_done` is created and the notice will not show again.

## Trigger flow
When user confirms:
1. Clear existing stats and logs.
2. Re-parse logs into PostgreSQL.
3. Write migration marker and remove SQLite file.

API:
```http
POST /api/logs/reparse
{
  "id": "",
  "migration": true
}
```

## Manual re-parse
```http
POST /api/logs/reparse
{
  "id": ""
}
```

## Notes
- During migration, stats may be incomplete until parsing finishes.
- Ensure log files are readable and not deleted.
- Changing `websites[].name` creates a new site ID.
