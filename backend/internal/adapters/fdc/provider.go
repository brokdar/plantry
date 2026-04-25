package fdc

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Provider wraps an FDC Client to implement food.FoodProvider.
type Provider struct {
	client *Client
}

// NewProvider creates a new Provider backed by the given FDC client.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// SearchByName searches for food items by name via FDC (Foundation + SR Legacy).
func (p *Provider) SearchByName(ctx context.Context, query string, limit int) ([]food.Candidate, error) {
	results, err := p.client.SearchByName(ctx, query, []string{"Foundation", "SR Legacy"}, limit)
	if err != nil {
		return nil, err
	}
	return toDomainCandidates(results), nil
}

// GetFoodPortions fetches per-unit gram weights for a single FDC food. This
// is what supplies ingredient-specific density data (e.g., 1 tbsp honey = 21g)
// to the portions table.
func (p *Provider) GetFoodPortions(ctx context.Context, fdcID int) ([]food.PortionInfo, error) {
	detail, err := p.client.GetFood(ctx, fdcID)
	if err != nil {
		return nil, err
	}
	out := make([]food.PortionInfo, 0, len(detail.FoodPortions))
	for _, fp := range detail.FoodPortions {
		out = append(out, food.PortionInfo{
			RawUnit:    fp.MeasureUnitName,
			Modifier:   fp.Modifier,
			GramWeight: fp.GramWeight,
		})
	}
	return out, nil
}

func toDomainCandidates(candidates []Candidate) []food.Candidate {
	out := make([]food.Candidate, len(candidates))
	for i, c := range candidates {
		out[i] = food.Candidate{
			Name:             c.Name,
			SourceName:       c.Name,
			Source:           food.SourceFDC,
			FdcID:            c.FdcID,
			Kcal100g:         c.Kcal100g,
			Protein100g:      c.Protein100g,
			Fat100g:          c.Fat100g,
			Carbs100g:        c.Carbs100g,
			Fiber100g:        c.Fiber100g,
			Sodium100g:       c.Sodium100g,
			SaturatedFat100g: c.SaturatedFat100g,
			TransFat100g:     c.TransFat100g,
			Cholesterol100g:  c.Cholesterol100g,
			Sugar100g:        c.Sugar100g,
			Potassium100g:    c.Potassium100g,
			Calcium100g:      c.Calcium100g,
			Iron100g:         c.Iron100g,
			Magnesium100g:    c.Magnesium100g,
			Phosphorus100g:   c.Phosphorus100g,
			Zinc100g:         c.Zinc100g,
			VitaminA100g:     c.VitaminA100g,
			VitaminC100g:     c.VitaminC100g,
			VitaminD100g:     c.VitaminD100g,
			VitaminB12100g:   c.VitaminB12100g,
			VitaminB6100g:    c.VitaminB6100g,
			Folate100g:       c.Folate100g,
		}
	}
	return out
}
