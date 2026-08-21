//go:build !tsnative || !cgo

package engine

import _ "modernc.org/sqlite"

const sqliteDriverName = "sqlite"
