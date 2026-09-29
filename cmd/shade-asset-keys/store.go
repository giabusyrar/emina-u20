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
	return s.db.WithContext(ctx).Migrator().HasColumn(table, "color_map_url"), nil
}

func (s gormStore) LegacyRows(ctx context.Context) ([]legacyRow, error) {
	var raw []struct {
		ID           string
		ColorMapURL  *string `gorm:"column:color_map_url"`
		AlphaMapURL  *string `gorm:"column:alpha_map_url"`
		FinishMapURL *string `gorm:"column:finish_map_url"`
	}
	err := s.db.WithContext(ctx).Table(table).
		Select("id, color_map_url, alpha_map_url, finish_map_url").
		Where("color_map_url IS NOT NULL OR alpha_map_url IS NOT NULL").
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
		for _, r := range rows {
			updates := map[string]interface{}{"color_map_key": r.ColorKey, "alpha_map_key": r.AlphaKey}
			if r.FinishKey != "" {
				updates["finish_map_key"] = r.FinishKey
			}
			if err := tx.Table(table).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("backfill %s: %w", r.ID, err)
			}
		}
		return tx.Exec("ALTER TABLE " + table +
			" DROP COLUMN color_map_url, DROP COLUMN alpha_map_url, DROP COLUMN finish_map_url").Error
	})
}
