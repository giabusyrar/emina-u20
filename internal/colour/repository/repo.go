package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/colour/domain"
	"github.com/experience-gateway/apps/core-engine/internal/colour/service"
)

// Repo reads catalog shades joined with their colorimetry.
type Repo struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type row struct {
	ShadeID     string
	ProductID   string
	ProductName string
	ShadeName   string
	HexColor    string
	Category    string
	LabL, LabA  float64
	LabB        float64
	MunsellV    float64
	MunsellC    float64
	Source      string
	Alpha       float64
	Specular    float64
	Mode        string
}

func (r *Repo) query(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("match_shade_colorimetry AS c").
		Select(`c.shade_id, s.product_id, p.name AS product_name, s.name AS shade_name, s.hex_color,
		        c.category, c.lab_l, c.lab_a, c.lab_b, c.munsell_v, c.munsell_c, c.source,
		        c.alpha, c.specular, c.mode`).
		Joins("JOIN match_shades AS s ON s.id = c.shade_id").
		Joins("JOIN match_product_catalog AS p ON p.id = s.product_id").
		Where("p.is_active = ?", true)
}

// preferBest keeps one row per shade, from its best colour source: cube,
// then swatch, then estimate (domain.SourceRank).
func preferBest(rows []row) []service.Candidate {
	best := map[string]row{}
	order := []string{}
	for _, rw := range rows {
		cur, ok := best[rw.ShadeID]
		if !ok {
			order = append(order, rw.ShadeID)
		}
		if !ok || domain.SourceRank(rw.Source) < domain.SourceRank(cur.Source) {
			best[rw.ShadeID] = rw
		}
	}
	out := make([]service.Candidate, 0, len(order))
	for _, id := range order {
		rw := best[id]
		out = append(out, service.Candidate{
			ShadeCandidate: domain.ShadeCandidate{
				ShadeID:      rw.ShadeID,
				ProductID:    rw.ProductID,
				ProductName:  rw.ProductName,
				ShadeName:    rw.ShadeName,
				Category:     rw.Category,
				HexColor:     rw.HexColor,
				Lab:          domain.Lab{rw.LabL, rw.LabA, rw.LabB},
				V:            rw.MunsellV,
				C:            rw.MunsellC,
				ColourSource: rw.Source,
			},
			Alpha:    rw.Alpha,
			Specular: rw.Specular,
			Mode:     rw.Mode,
		})
	}
	return out
}

func (r *Repo) ListCandidates(ctx context.Context) ([]service.Candidate, error) {
	if r.db == nil {
		return nil, fmt.Errorf("colour repository: no database connection")
	}
	var rows []row
	if err := r.query(ctx).Order("s.product_id, s.name").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing shade colorimetry: %w", err)
	}
	return preferBest(rows), nil
}

func (r *Repo) GetCandidates(ctx context.Context, shadeIDs []string) ([]service.Candidate, error) {
	if r.db == nil {
		return nil, fmt.Errorf("colour repository: no database connection")
	}
	var rows []row
	if err := r.query(ctx).Where("c.shade_id IN ?", shadeIDs).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading shade colorimetry: %w", err)
	}
	return preferBest(rows), nil
}
