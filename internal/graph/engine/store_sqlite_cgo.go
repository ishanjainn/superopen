//go:build tsnative && cgo

package engine

import _ "github.com/mattn/go-sqlite3"

const sqliteDriverName = "sqlite3"
