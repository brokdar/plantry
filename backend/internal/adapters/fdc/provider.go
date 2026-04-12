package fdc

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// Provider wraps an FDC Client to implement ingredient.FoodProvider.
type Provider struct {
	client *Client
}

// NewProvider creates a new Provider backed by the given FDC client.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// SearchByName searches for food items by name via FDC (Foundation + SR Legacy).
func (p *Provider) SearchByName(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error) {
	results, err := p.client.SearchByName(ctx, query, []string{"Foundation", "SR Legacy"}, limit)
	if err != nil {
		return nil, err
	}
	return toDomainCandidates(results), nil
}

func toDomainCandidates(candidates []Candidate) []ingredient.Candidate {
	out := make([]ingredient.Candidate, len(candidates))
	for i, c := range candidates {
		out[i] = ingredient.Candidate{
			Name:        c.Name,
			Source:      ingredient.SourceFDC,
			FdcID:       c.FdcID,
			Kcal100g:    c.Kcal100g,
			Protein100g: c.Protein100g,
			Fat100g:     c.Fat100g,
			Carbs100g:   c.Carbs100g,
			Fiber100g:   c.Fiber100g,
			Sodium100g:  c.Sodium100g,
		}
	}
	return out
}
