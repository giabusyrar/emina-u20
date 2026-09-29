package profile_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func TestScopes(t *testing.T) {
	assert.Equal(t, [][2]string{{"wardah", "app1"}, {"wardah", "*"}, {"*", "*"}}, profile.Scopes("wardah", "app1"))
	assert.Equal(t, [][2]string{{"wardah", "*"}, {"*", "*"}}, profile.Scopes("wardah", "*"))
	assert.Equal(t, [][2]string{{"*", "*"}}, profile.Scopes("", ""))
}

// storeContract is the behaviour every Store must have. It runs against
// MemoryStore always and against Postgres when FACEARCH_TEST_DATABASE_URL
// names a scratch database.
func storeContract(t *testing.T, s profile.Store) {
	ctx := context.Background()

	_, err := s.Active(ctx, "brand-a", "app-1")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)

	global, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "base", Document: []byte(`{"n":1}`)})
	require.NoError(t, err)
	assert.Equal(t, 1, global.Version)
	assert.Equal(t, "base", global.Title, "title defaults to code")

	got, err := s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, global.ID, got.ID, "falls back to *+*")

	brand, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "brand-a", ApplicationID: "*", Code: "brand", Title: "Brand A", Document: []byte(`{"n":2}`)})
	require.NoError(t, err)
	got, err = s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, brand.ID, got.ID, "brand+* beats *+*")

	app, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "brand-a", ApplicationID: "app-1", Code: "app", Document: []byte(`{"n":3}`)})
	require.NoError(t, err)
	got, err = s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, app.ID, got.ID, "brand+app beats brand+*")

	got, err = s.Active(ctx, "brand-b", "app-1")
	require.NoError(t, err)
	assert.Equal(t, global.ID, got.ID, "other brands still get *+*")

	v2, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "base", Document: []byte(`{"n":4}`)})
	require.NoError(t, err)
	assert.Equal(t, 2, v2.Version, "next version of the same code")
	got, err = s.Active(ctx, "brand-b", "")
	require.NoError(t, err)
	assert.Equal(t, v2.ID, got.ID)
	assert.JSONEq(t, `{"n":4}`, got.Document)

	other, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "other", Document: []byte(`{"n":5}`)})
	require.NoError(t, err)
	assert.Equal(t, 1, other.Version)
	got, err = s.Active(ctx, "brand-b", "")
	require.NoError(t, err)
	assert.Equal(t, other.ID, got.ID, "an upsert deactivates every other profile in its scope")

	_, err = s.Upsert(ctx, profile.UpsertInput{BrandID: "", ApplicationID: "*", Code: "x", Document: []byte(`{}`)})
	assert.Error(t, err, "scope is explicit")

	_, err = s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "app-1", Code: "x", Document: []byte(`{}`)})
	require.Error(t, err, "*+app is a scope no lookup reads")
	assert.Contains(t, err.Error(), "brand")
	assert.Contains(t, err.Error(), "application")
}

func TestMemoryStoreContract(t *testing.T) {
	storeContract(t, profile.NewMemoryStore())
}

func TestMemoryStore_IgnoresInactiveRows(t *testing.T) {
	s := profile.NewMemoryStore()
	s.Put(profile.FaceArchitectureProfile{BrandID: "*", ApplicationID: "*", Code: "a", Version: 9, Status: "inactive", Document: `{}`})
	_, err := s.Active(context.Background(), "x", "y")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)
}

// TestGormStoreContract needs a disposable Postgres database: it creates
// face_architecture_profiles there and deletes every row in it. Never point
// it at a shared database.
func TestGormStoreContract(t *testing.T) {
	url := os.Getenv("FACEARCH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FACEARCH_TEST_DATABASE_URL not set (scratch Postgres only)")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&profile.FaceArchitectureProfile{}))
	require.NoError(t, db.Exec("DELETE FROM face_architecture_profiles").Error)
	t.Cleanup(func() { db.Exec("DELETE FROM face_architecture_profiles") })
	storeContract(t, profile.NewGormStore(db))
}
