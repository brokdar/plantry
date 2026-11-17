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
	conn
}

// NewProfileRepo creates a SQLite-backed profile repository.
func NewProfileRepo(db *sql.DB) *ProfileRepo {
	return &ProfileRepo{conn: conn{db: db}}
}

func (r *ProfileRepo) Get(ctx context.Context) (*profile.Profile, error) {
	row, err := r.queriesFor(ctx).GetProfile(ctx)
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

	goal := p.Goal
	if goal == "" {
		goal = profile.GoalMaintain
	}
	row, err := r.queriesFor(ctx).UpsertProfile(ctx, sqlcgen.UpsertProfileParams{
		KcalTarget:          toNullFloat64(p.KcalTarget),
		ProteinPct:          toNullFloat64(p.ProteinPct),
		FatPct:              toNullFloat64(p.FatPct),
		CarbsPct:            toNullFloat64(p.CarbsPct),
		DietaryRestrictions: string(restrictions),
		Preferences:         string(prefsJSON),
		SystemPrompt:        toNullString(p.SystemPrompt),
		Locale:              p.Locale,
		DietType:            string(p.DietType),
		MeatDaysPerWeekMax:  toNullInt64FromIntPtr(p.MeatDaysPerWeekMax),
		HouseholdPortions:   p.HouseholdPortions,
		Goal:                string(goal),
		WeightKg:            toNullFloat64(p.WeightKg),
		ProteinGPerKg:       toNullFloat64(p.ProteinGPerKg),
		HeightCm:            toNullFloat64(p.HeightCm),
		BirthYear:           toNullInt64FromIntPtr(p.BirthYear),
		Sex:                 sexToNull(p.Sex),
		ActivityLevel:       toNullFloat64(p.ActivityLevel),
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
		DietType:            profile.DietType(row.DietType),
		MeatDaysPerWeekMax:  fromNullInt64ToIntPtr(row.MeatDaysPerWeekMax),
		HouseholdPortions:   row.HouseholdPortions,
		Goal:                profile.Goal(row.Goal),
		WeightKg:            fromNullFloat64(row.WeightKg),
		ProteinGPerKg:       fromNullFloat64(row.ProteinGPerKg),
		HeightCm:            fromNullFloat64(row.HeightCm),
		BirthYear:           fromNullInt64ToIntPtr(row.BirthYear),
		ActivityLevel:       fromNullFloat64(row.ActivityLevel),
	}
	if row.Sex.Valid {
		v := profile.Sex(row.Sex.String)
		p.Sex = &v
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

func fromNullFloat64(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func sexToNull(v *profile.Sex) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*v), Valid: true}
}
