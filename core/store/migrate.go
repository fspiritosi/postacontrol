package store

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaV1 string

//go:embed schema_v2.sql
var schemaV2 string

//go:embed schema_v3.sql
var schemaV3 string

//go:embed schema_v4.sql
var schemaV4 string

//go:embed schema_v5.sql
var schemaV5 string

//go:embed schema_v6.sql
var schemaV6 string

//go:embed schema_v7.sql
var schemaV7 string

//go:embed schema_v8.sql
var schemaV8 string

//go:embed schema_v9.sql
var schemaV9 string

// migrations[i] lleva la base de la versión i a la i+1.
var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9}

func migrate(db *sql.DB) error { return migrateTo(db, len(migrations)) }

// migrateTo migra hasta la versión target (solo los tests usan un tope menor).
func migrateTo(db *sql.DB, target int) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(migrations) {
		return fmt.Errorf("la base tiene versión %d, más nueva que la soportada (%d)", current, len(migrations))
	}
	for v := current; v < target; v++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migración a v%d: %w", v+1, err)
		}
		// PRAGMA no admite parámetros; v es un entero propio.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
