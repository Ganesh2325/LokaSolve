package migrations

import "embed"

// FS contains ordered SQL migrations. Versions are the file names.
//
//go:embed *.sql
var FS embed.FS
