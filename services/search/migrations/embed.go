package migrations

import "embed"

// Files содержит только миграции Search.
//
//go:embed *.sql
var Files embed.FS
