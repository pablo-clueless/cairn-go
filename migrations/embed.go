// Package migrations embeds the SQL migration files so they ship inside the
// compiled binary and are applied automatically at startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
