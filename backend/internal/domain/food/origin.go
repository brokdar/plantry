package food

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Origin kinds of a leaf the matching path added (MATCH-3).
const (
	OriginImport = "import"
	OriginFind   = "find"
)

// Origin names what a leaf was added for: an import (Ref: source URL) or a
// Chef find (Ref: find id).
type Origin struct {
	Kind string
	Ref  string
}

// OriginStore records leaf origins (sqlite.FoodOriginRepo).
type OriginStore interface {
	AddOrigin(ctx context.Context, foodID int64, o Origin, source string) error
	OriginLeaves(ctx context.Context, o Origin) ([]int64, error)
}

// EnsureLeaf returns the catalogue leaf for an FDC or OFF candidate. It
// creates one, with its origin, only after a duplicate check: a leaf with
// the same FDC id or barcode, or the same food by the synonym table in a
// compatible state ("Zwiebel" is "Onion"), is reused (MATCH-3).
func EnsureLeaf(ctx context.Context, repo Repository, store OriginStore, c Candidate, o Origin) (id int64, created bool, err error) {
	if c.ExistingID != nil {
		return *c.ExistingID, false, nil
	}
	if c.Kcal100g == nil {
		return 0, false, fmt.Errorf("%w: candidate %q has no nutrition", domain.ErrInvalidInput, c.Name)
	}
	leaves, err := listLeavesOf(ctx, repo)
	if err != nil {
		return 0, false, err
	}
	if lf := duplicateLeaf(leaves, c); lf != nil {
		return lf.ID, false, nil
	}
	src := c.Source
	f := &Food{
		Name: c.Name, Kind: KindLeaf, Source: &src,
		Kcal100g: c.Kcal100g, Protein100g: c.Protein100g, Fat100g: c.Fat100g, Carbs100g: c.Carbs100g,
		Fiber100g: c.Fiber100g, Sodium100g: c.Sodium100g, SaturatedFat100g: c.SaturatedFat100g, Sugar100g: c.Sugar100g,
	}
	ref := c.Barcode
	if c.FdcID != 0 {
		ref = strconv.Itoa(c.FdcID)
		f.FdcID = &ref
	} else if c.Barcode != "" {
		f.Barcode = &ref
	}
	if err := repo.Create(ctx, f); err != nil {
		return 0, false, fmt.Errorf("create leaf %q: %w", c.Name, err)
	}
	if err := store.AddOrigin(ctx, f.ID, o, string(c.Source)+":"+ref); err != nil {
		return 0, false, fmt.Errorf("leaf %d origin: %w", f.ID, err)
	}
	return f.ID, true, nil
}

func duplicateLeaf(leaves []catalogueLeaf, c Candidate) *Food {
	keys := lineKeys(c.Name, "")
	e := synonyms.lookup(keys)
	state := stateOf(c.Name + " " + c.SourceName)
	for _, lf := range leaves {
		f := lf.food
		switch {
		case c.FdcID != 0 && f.FdcID != nil && *f.FdcID == strconv.Itoa(c.FdcID),
			c.Barcode != "" && f.Barcode != nil && *f.Barcode == c.Barcode:
			return f
		case catalogueIdentityName(f.Name, e, keys) && !statesConflict(state, stateOf(f.Name)):
			return f
		}
	}
	return nil
}

// RemoveOrphanLeaves deletes the leaves created for o that nothing
// references, for a rejected recipe (MATCH-3); referenced leaves stay.
func RemoveOrphanLeaves(ctx context.Context, repo Repository, store OriginStore, o Origin) (removed int, err error) {
	ids, err := store.OriginLeaves(ctx, o)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		switch err := repo.Delete(ctx, id); {
		case err == nil:
			removed++
		case errors.Is(err, domain.ErrInUse), errors.Is(err, domain.ErrNotFound):
		default:
			return removed, fmt.Errorf("remove leaf %d: %w", id, err)
		}
	}
	return removed, nil
}
