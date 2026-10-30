package database

import "embed"

// Migrations contiene SQL versionado y ejecutado una sola vez por instalación.
//
//go:embed migrations/*.sql
var Migrations embed.FS
