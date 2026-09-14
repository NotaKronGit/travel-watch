package migrations

import "embed"

// Files содержит только миграции Cabinet.
//
//go:embed *.sql
var Files embed.FS
