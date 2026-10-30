package store

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaV1 string

// migrations[i] lleva la base de la versión i a la i+1.
var migrations = []string{schemaV1}

func migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(migrations) {
		return fmt.Errorf("la base tiene versión %d, más nueva que la soportada (%d)", current, len(migrations))
	}
	for v := current; v < len(migrations); v++ {
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
