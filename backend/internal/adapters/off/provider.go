package off

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// Provider wraps an OFF Client to implement ingredient.BarcodeProvider.
type Provider struct {
	client *Client
}

// NewProvider creates a new Provider backed by the given OFF client.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// LookupBarcode looks up a food product by barcode via OFF.
func (p *Provider) LookupBarcode(ctx context.Context, barcode string) ([]ingredient.Candidate, error) {
	results, err := p.client.LookupBarcode(ctx, barcode, "en")
	if err != nil {
		return nil, err
	}
	return toDomainCandidates(results), nil
}

// SearchByName searches for food products by name via OFF.
func (p *Provider) SearchByName(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error) {
	results, err := p.client.SearchByName(ctx, query, "en", limit)
	if err != nil {
		return nil, err
	}
	return toDomainCandidates(results), nil
}

func toDomainCandidates(candidates []Candidate) []ingredient.Candidate {
	out := make([]ingredient.Candidate, len(candidates))
	for i, c := range candidates {
		out[i] = ingredient.Candidate{
			Name:             c.Name,
			SourceName:       c.Name,
			Brand:            c.Brand,
			Source:           ingredient.SourceOFF,
			Barcode:          c.Barcode,
			ImageURL:         c.ImageURL,
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
			ServingQuantityG: c.ServingQuantityG,
		}
	}
	return out
}
