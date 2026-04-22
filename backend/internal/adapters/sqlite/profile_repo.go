package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// ProfileRepo implements profile.Repository backed by SQLite.
type ProfileRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewProfileRepo creates a SQLite-backed profile repository.
func NewProfileRepo(db *sql.DB) *ProfileRepo {
	return &ProfileRepo{db: db, q: sqlcgen.New(db)}
}

// newProfileRepoTx binds a ProfileRepo to an open transaction.
func newProfileRepoTx(tx *sql.Tx) *ProfileRepo {
	return &ProfileRepo{db: nil, q: sqlcgen.New(tx)}
}

func (r *ProfileRepo) Get(ctx context.Context) (*profile.Profile, error) {
	row, err := r.q.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	return mapProfileToDomain(row), nil
}

func (r *ProfileRepo) Update(ctx context.Context, p *profile.Profile) (*profile.Profile, error) {
	restrictions, err := json.Marshal(p.DietaryRestrictions)
	if err != nil {
		return nil, err
	}
	prefs := p.Preferences
	if prefs == nil {
		prefs = map[string]any{}
	}
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}

	row, err := r.q.UpsertProfile(ctx, sqlcgen.UpsertProfileParams{
		KcalTarget:          toNullFloat64(p.KcalTarget),
		ProteinPct:          toNullFloat64(p.ProteinPct),
		FatPct:              toNullFloat64(p.FatPct),
		CarbsPct:            toNullFloat64(p.CarbsPct),
		DietaryRestrictions: string(restrictions),
		Preferences:         string(prefsJSON),
		SystemPrompt:        toNullString(p.SystemPrompt),
		Locale:              p.Locale,
	})
	if err != nil {
		return nil, err
	}
	return mapProfileToDomain(row), nil
}

func mapProfileToDomain(row sqlcgen.UserProfile) *profile.Profile {
	p := &profile.Profile{
		Locale:              row.Locale,
		DietaryRestrictions: []string{},
		Preferences:         map[string]any{},
	}

	if row.KcalTarget.Valid {
		v := row.KcalTarget.Float64
		p.KcalTarget = &v
	}
	if row.ProteinPct.Valid {
		v := row.ProteinPct.Float64
		p.ProteinPct = &v
	}
	if row.FatPct.Valid {
		v := row.FatPct.Float64
		p.FatPct = &v
	}
	if row.CarbsPct.Valid {
		v := row.CarbsPct.Float64
		p.CarbsPct = &v
	}
	if row.SystemPrompt.Valid {
		v := row.SystemPrompt.String
		p.SystemPrompt = &v
	}

	_ = json.Unmarshal([]byte(row.DietaryRestrictions), &p.DietaryRestrictions)
	_ = json.Unmarshal([]byte(row.Preferences), &p.Preferences)

	p.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt)
	return p
}

func toNullFloat64(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
