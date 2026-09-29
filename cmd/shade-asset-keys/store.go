package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

const table = "match_shade_assets"

// store is the database side of the migration; tests replace it.
type store interface {
	HasURLColumns(ctx context.Context) (bool, error)
	LegacyRows(ctx context.Context) ([]legacyRow, error)
	// Prepare drops NOT NULL on the legacy URL columns so the new
	// core-engine, which no longer writes them, can insert.
	Prepare(ctx context.Context) error
	// Apply backfills the key columns and drops the URL columns in one
	// transaction.
	Apply(ctx context.Context, rows []keyedRow) error
}

type gormStore struct{ db *gorm.DB }

func (s gormStore) HasURLColumns(ctx context.Context) (bool, error) {
	n, err := countColumns(s.db.WithContext(ctx), "color_map_url")
	return n == 1, err
}

// countColumns counts how many of names exist on the table in the current
// schema. Unlike Migrator().HasColumn it returns the query error.
func countColumns(db *gorm.DB, names ...string) (int, error) {
	var n int64
	err := db.Raw(
		"SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name IN ?",
		table, names).Scan(&n).Error
	return int(n), err
}

// unkeyedRows counts rows that still hold a URL with no key to replace it.
const unkeyedRows = "SELECT count(*) FROM " + table + " WHERE " +
	"(color_map_url IS NOT NULL AND (color_map_key IS NULL OR color_map_key = '')) OR " +
	"(alpha_map_url IS NOT NULL AND (alpha_map_key IS NULL OR alpha_map_key = '')) OR " +
	"(finish_map_url IS NOT NULL AND (finish_map_key IS NULL OR finish_map_key = ''))"

func (s gormStore) LegacyRows(ctx context.Context) ([]legacyRow, error) {
	var raw []struct {
		ID           string
		ColorMapURL  *string `gorm:"column:color_map_url"`
		AlphaMapURL  *string `gorm:"column:alpha_map_url"`
		FinishMapURL *string `gorm:"column:finish_map_url"`
	}
	err := s.db.WithContext(ctx).Table(table).
		Select("id, color_map_url, alpha_map_url, finish_map_url").
		Where("color_map_url IS NOT NULL OR alpha_map_url IS NOT NULL OR finish_map_url IS NOT NULL").
		Order("id").Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	rows := make([]legacyRow, 0, len(raw))
	for _, r := range raw {
		row := legacyRow{ID: r.ID, FinishURL: r.FinishMapURL}
		if r.ColorMapURL != nil {
			row.ColorURL = *r.ColorMapURL
		}
		if r.AlphaMapURL != nil {
			row.AlphaURL = *r.AlphaMapURL
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s gormStore) Prepare(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec(
		"ALTER TABLE " + table + " ALTER COLUMN color_map_url DROP NOT NULL, ALTER COLUMN alpha_map_url DROP NOT NULL").Error
}

func (s gormStore) Apply(ctx context.Context, rows []keyedRow) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keyCols := []string{"color_map_key", "alpha_map_key", "finish_map_key"}
		have, err := countColumns(tx, keyCols...)
		if err != nil {
			return fmt.Errorf("inspect key columns: %w", err)
		}
		if have != len(keyCols) {
			return fmt.Errorf("%s lacks the key columns: deploy the new core-engine first", table)
		}
		for _, r := range rows {
			updates := map[string]interface{}{"color_map_key": r.ColorKey, "alpha_map_key": r.AlphaKey}
			if r.FinishKey != "" {
				updates["finish_map_key"] = r.FinishKey
			}
			if err := tx.Table(table).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("backfill %s: %w", r.ID, err)
			}
		}
		// Rows written with URLs after LegacyRows read them would lose their
		// texture on DROP; roll back instead.
		var unkeyed int64
		if err := tx.Raw(unkeyedRows).Scan(&unkeyed).Error; err != nil {
			return fmt.Errorf("verify backfill: %w", err)
		}
		if unkeyed > 0 {
			return fmt.Errorf("%d row(s) gained URLs during the run; nothing dropped, re-run", unkeyed)
		}
		return tx.Exec("ALTER TABLE " + table +
			" DROP COLUMN color_map_url, DROP COLUMN alpha_map_url, DROP COLUMN finish_map_url").Error
	})
}
