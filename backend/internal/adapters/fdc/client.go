package fdc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Client communicates with the USDA FoodData Central API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// Option configures a Client.
type Option func(*Client)

// Candidate holds a normalised search result from the FDC API.
type Candidate struct {
	Name        string
	FdcID       int
	DataType    string
	Category    string
	Kcal100g    *float64
	Protein100g *float64
	Fat100g     *float64
	Carbs100g   *float64
	Fiber100g   *float64
	Sodium100g  *float64 // converted from mg to g

	// Extended nutrients — units match FDC native unless noted.
	SaturatedFat100g *float64 // g
	TransFat100g     *float64 // g
	Cholesterol100g  *float64 // mg
	Sugar100g        *float64 // g
	Potassium100g    *float64 // mg
	Calcium100g      *float64 // mg
	Iron100g         *float64 // mg
	Magnesium100g    *float64 // mg
	Phosphorus100g   *float64 // mg
	Zinc100g         *float64 // mg
	VitaminA100g     *float64 // µg RAE
	VitaminC100g     *float64 // mg
	VitaminD100g     *float64 // µg
	VitaminB12100g   *float64 // µg
	VitaminB6100g    *float64 // mg
	Folate100g       *float64 // µg DFE
}

// New creates a Client for the given API key.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		httpClient: http.DefaultClient,
		baseURL:    "https://api.nal.usda.gov",
		apiKey:     apiKey,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithBaseURL overrides the default base URL.
func WithBaseURL(u string) Option {
	return func(cl *Client) { cl.baseURL = u }
}

// SearchByName queries the FDC food search endpoint and returns normalised candidates.
func (c *Client) SearchByName(ctx context.Context, query string, dataTypes []string, limit int) ([]Candidate, error) {
	u, err := url.Parse(c.baseURL + "/fdc/v1/foods/search")
	if err != nil {
		return nil, fmt.Errorf("fdc: parse base URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	q.Set("pageSize", fmt.Sprintf("%d", limit))
	q.Set("pageNumber", "1")
	q.Set("api_key", c.apiKey)
	for _, dt := range dataTypes {
		q.Add("dataType", dt)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("fdc: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fdc: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fdc: unexpected status %d", resp.StatusCode)
	}

	var sr fdcSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("fdc: decode response: %w", err)
	}

	candidates := make([]Candidate, 0, len(sr.Foods))
	for _, f := range sr.Foods {
		candidates = append(candidates, normaliseFDC(f))
	}
	return candidates, nil
}

// --- internal FDC response types ---

type fdcSearchResponse struct {
	TotalHits   int             `json:"totalHits"`
	CurrentPage int             `json:"currentPage"`
	TotalPages  int             `json:"totalPages"`
	Foods       []fdcSearchFood `json:"foods"`
}

type fdcSearchFood struct {
	FdcID         int                 `json:"fdcId"`
	Description   string              `json:"description"`
	DataType      string              `json:"dataType"`
	FoodCategory  string              `json:"foodCategory"`
	FoodNutrients []fdcSearchNutrient `json:"foodNutrients"`
}

type fdcSearchNutrient struct {
	NutrientID   int     `json:"nutrientId"`
	NutrientName string  `json:"nutrientName"`
	UnitName     string  `json:"unitName"`
	Value        float64 `json:"value"`
}

func normaliseFDC(f fdcSearchFood) Candidate {
	vals := make(map[int]float64, len(f.FoodNutrients))
	for _, n := range f.FoodNutrients {
		vals[n.NutrientID] = n.Value
	}
	ptr := func(id int) *float64 {
		if v, ok := vals[id]; ok {
			cp := v
			return &cp
		}
		return nil
	}

	// Convert sodium from mg to g.
	sodiumG := ptr(1093)
	if sodiumG != nil {
		v := *sodiumG / 1000
		sodiumG = &v
	}

	return Candidate{
		Name:             f.Description,
		FdcID:            f.FdcID,
		DataType:         f.DataType,
		Category:         f.FoodCategory,
		Kcal100g:         ptr(1008),
		Protein100g:      ptr(1003),
		Fat100g:          ptr(1004),
		Carbs100g:        ptr(1005),
		Fiber100g:        ptr(1079),
		Sodium100g:       sodiumG,
		SaturatedFat100g: ptr(1258),
		TransFat100g:     ptr(1257),
		Cholesterol100g:  ptr(1253), // mg
		Sugar100g:        ptr(2000),
		Potassium100g:    ptr(1092), // mg
		Calcium100g:      ptr(1087), // mg
		Iron100g:         ptr(1089), // mg
		Magnesium100g:    ptr(1090), // mg
		Phosphorus100g:   ptr(1091), // mg
		Zinc100g:         ptr(1095), // mg
		VitaminA100g:     ptr(1106), // µg RAE
		VitaminC100g:     ptr(1162), // mg
		VitaminD100g:     ptr(1114), // µg
		VitaminB12100g:   ptr(1178), // µg
		VitaminB6100g:    ptr(1175), // mg
		Folate100g:       ptr(1190), // µg DFE
	}
}
