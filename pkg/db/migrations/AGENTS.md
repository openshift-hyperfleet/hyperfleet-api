# Claude Code Guidelines for Migrations

## No Rollbacks — Fix Forward

Never set the `Rollback` field on a `gormigrate.Migration`. If a migration has a problem, ship a new
migration to correct it instead of rolling back the old one. Rollbacks can destroy data (e.g.
`DROP COLUMN`) that was legitimately written after the migration ran, so they are not used here even
though gormigrate supports them.

## Other Rules

See `migration_structs.go` for ID formatting, backwards-compatibility, and type re-declaration rules.
