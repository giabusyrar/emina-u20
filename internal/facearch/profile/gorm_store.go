package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// upsertLockClass is the class key of a two-key advisory lock. Two-key locks
// do not share a key space with the ai-worker's single-key schema lock
// 1581646198 (docs/SHARED-DATABASE.md). It serialises version computation and
// scope deactivation per brand+app.
const upsertLockClass int32 = 1581646199

// GormStore keeps profiles in face_architecture_profiles.
type GormStore struct {
	db *gorm.DB
}

// NewGormStore returns a Store over db.
func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) Active(ctx context.Context, brandID, appID string) (*FaceArchitectureProfile, error) {
	for _, sc := range Scopes(brandID, appID) {
		var p FaceArchitectureProfile
		err := s.db.WithContext(ctx).
			Where("brand_id = ? AND application_id = ? AND UPPER(status) = ?", sc[0], sc[1], StatusActive).
			Order("version desc").Order("created_at desc").
			First(&p).Error
		if err == nil {
			return &p, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("face-architecture profile lookup: %w", err)
		}
	}
	return nil, ErrNoActiveProfile
}

func (s *GormStore) Upsert(ctx context.Context, in UpsertInput) (*FaceArchitectureProfile, error) {
	in, err := normaliseInput(in)
	if err != nil {
		return nil, err
	}
	var out FaceArchitectureProfile
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, hashtext(?))", upsertLockClass, in.BrandID+"|"+in.ApplicationID).Error; err != nil {
			return fmt.Errorf("lock scope: %w", err)
		}
		var maxVersion int
		if err := tx.Model(&FaceArchitectureProfile{}).
			Where("brand_id = ? AND application_id = ? AND code = ?", in.BrandID, in.ApplicationID, in.Code).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&FaceArchitectureProfile{}).
			Where("brand_id = ? AND application_id = ? AND UPPER(status) = ?", in.BrandID, in.ApplicationID, StatusActive).
			Updates(map[string]any{"status": StatusInactive, "updated_at": now}).Error; err != nil {
			return err
		}
		out = FaceArchitectureProfile{
			ID: uuid.NewString(), BrandID: in.BrandID, ApplicationID: in.ApplicationID, Code: in.Code,
			Title: in.Title, Status: StatusActive, Version: maxVersion + 1, Document: string(in.Document),
		}
		return tx.Create(&out).Error
	})
	if err != nil {
		return nil, fmt.Errorf("face-architecture profile upsert: %w", err)
	}
	return &out, nil
}
