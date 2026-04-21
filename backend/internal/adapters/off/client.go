package off

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultBaseURL = "https://world.openfoodfacts.org"
	userAgent      = "Plantry/1.0 (https://github.com/jaltszeimer/plantry)"
	productFields  = "product_name,product_name_de,product_name_en,brands,nutriments,serving_size,serving_quantity,product_quantity,product_quantity_unit,image_front_small_url"
)

// Client is an HTTP client for the Open Food Facts API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// Option configures a Client.
type Option func(*Client)

// Candidate represents a product found via the Open Food Facts API.
type Candidate struct {
	Name        string
	Brand       string
	Barcode     string
	ImageURL    string
	Kcal100g    *float64
	Protein100g *float64
	Fat100g     *float64
	Carbs100g   *float64
	Fiber100g   *float64
	Sodium100g  *float64

	SaturatedFat100g *float64
	TransFat100g     *float64
	Cholesterol100g  *float64 // mg
	Sugar100g        *float64
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

	// ServingQuantityG is the OFF-reported grams per serving, if the product
	// exposes one. Used to seed a "serving" portion so users can pick
	// "1 serving" as a natural unit for packaged foods.
	ServingQuantityG *float64
}

// New creates a Client with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		httpClient: http.DefaultClient,
		baseURL:    defaultBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the default Open Food Facts base URL.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// LookupBarcode fetches a single product by barcode.
// Returns an empty slice (no error) when the barcode is not found.
func (c *Client) LookupBarcode(ctx context.Context, barcode string, lang string) ([]Candidate, error) {
	u := fmt.Sprintf("%s/api/v2/product/%s.json?lc=%s&fields=%s",
		c.baseURL, url.PathEscape(barcode), url.QueryEscape(lang), productFields)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("off: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("off: barcode lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("off: barcode lookup: unexpected status %d", resp.StatusCode)
	}

	var body offResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("off: decode barcode response: %w", err)
	}

	if body.Status != 1 || body.Product == nil {
		return []Candidate{}, nil
	}

	cand := mapProduct(*body.Product, lang)
	cand.Barcode = barcode
	return []Candidate{cand}, nil
}

// SearchByName searches for products by free-text query.
// Returns an empty slice when no products match.
func (c *Client) SearchByName(ctx context.Context, query string, lang string, limit int) ([]Candidate, error) {
	u := fmt.Sprintf("%s/api/v2/search?search_terms=%s&lc=%s&fields=%s&page_size=%s&page=1&sort_by=score",
		c.baseURL, url.QueryEscape(query), url.QueryEscape(lang), productFields, strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("off: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("off: search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("off: search: unexpected status %d", resp.StatusCode)
	}

	var body offSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("off: decode search response: %w", err)
	}

	candidates := make([]Candidate, 0, len(body.Products))
	for _, p := range body.Products {
		candidates = append(candidates, mapProduct(p, lang))
	}
	return candidates, nil
}

func mapProduct(p offProduct, lang string) Candidate {
	var serving *float64
	if p.ServingQuantity > 0 {
		v := p.ServingQuantity
		serving = &v
	}
	return Candidate{
		ServingQuantityG: serving,
		Name:             localizedName(p, lang),
		Brand:            p.Brands,
		ImageURL:         p.ImageFrontSmallURL,
		Kcal100g:         p.Nutriments.EnergyKcal100g,
		Protein100g:      p.Nutriments.Proteins100g,
		Fat100g:          p.Nutriments.Fat100g,
		Carbs100g:        p.Nutriments.Carbohydrates100g,
		Fiber100g:        p.Nutriments.Fiber100g,
		Sodium100g:       p.Nutriments.Sodium100g,
		SaturatedFat100g: p.Nutriments.SaturatedFat100g,
		TransFat100g:     p.Nutriments.TransFat100g,
		Cholesterol100g:  p.Nutriments.Cholesterol100g,
		Sugar100g:        p.Nutriments.Sugars100g,
		Potassium100g:    p.Nutriments.Potassium100g,
		Calcium100g:      p.Nutriments.Calcium100g,
		Iron100g:         p.Nutriments.Iron100g,
		Magnesium100g:    p.Nutriments.Magnesium100g,
		Phosphorus100g:   p.Nutriments.Phosphorus100g,
		Zinc100g:         p.Nutriments.Zinc100g,
		VitaminA100g:     p.Nutriments.VitaminA100g,
		VitaminC100g:     p.Nutriments.VitaminC100g,
		VitaminD100g:     p.Nutriments.VitaminD100g,
		VitaminB12100g:   p.Nutriments.VitaminB12100g,
		VitaminB6100g:    p.Nutriments.VitaminB6100g,
		Folate100g:       p.Nutriments.VitaminB9100g,
	}
}

func localizedName(p offProduct, lang string) string {
	switch lang {
	case "de":
		if p.ProductNameDe != "" {
			return p.ProductNameDe
		}
	case "en":
		if p.ProductNameEn != "" {
			return p.ProductNameEn
		}
	}
	return p.ProductName
}

// --- internal OFF API response types ---

type offResponse struct {
	Code          string      `json:"code"`
	Status        int         `json:"status"`
	StatusVerbose string      `json:"status_verbose"`
	Product       *offProduct `json:"product,omitempty"`
}

type offSearchResponse struct {
	Count    int          `json:"count"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Products []offProduct `json:"products"`
}

type offProduct struct {
	ProductName         string        `json:"product_name"`
	ProductNameDe       string        `json:"product_name_de"`
	ProductNameEn       string        `json:"product_name_en"`
	Brands              string        `json:"brands"`
	ServingSize         string        `json:"serving_size"`
	ServingQuantity     float64       `json:"serving_quantity"`
	ProductQuantity     float64       `json:"product_quantity"`
	ProductQuantityUnit string        `json:"product_quantity_unit"`
	Nutriments          offNutriments `json:"nutriments"`
	ImageFrontSmallURL  string        `json:"image_front_small_url"`
}

type offNutriments struct {
	EnergyKcal100g    *float64 `json:"energy-kcal_100g"`
	Proteins100g      *float64 `json:"proteins_100g"`
	Fat100g           *float64 `json:"fat_100g"`
	Carbohydrates100g *float64 `json:"carbohydrates_100g"`
	Fiber100g         *float64 `json:"fiber_100g"`
	Sodium100g        *float64 `json:"sodium_100g"`

	SaturatedFat100g *float64 `json:"saturated-fat_100g"`
	TransFat100g     *float64 `json:"trans-fat_100g"`
	Cholesterol100g  *float64 `json:"cholesterol_100g"`
	Sugars100g       *float64 `json:"sugars_100g"`
	Potassium100g    *float64 `json:"potassium_100g"`
	Calcium100g      *float64 `json:"calcium_100g"`
	Iron100g         *float64 `json:"iron_100g"`
	Magnesium100g    *float64 `json:"magnesium_100g"`
	Phosphorus100g   *float64 `json:"phosphorus_100g"`
	Zinc100g         *float64 `json:"zinc_100g"`
	VitaminA100g     *float64 `json:"vitamin-a_100g"`
	VitaminC100g     *float64 `json:"vitamin-c_100g"`
	VitaminD100g     *float64 `json:"vitamin-d_100g"`
	VitaminB12100g   *float64 `json:"vitamin-b12_100g"`
	VitaminB6100g    *float64 `json:"vitamin-b6_100g"`
	VitaminB9100g    *float64 `json:"vitamin-b9_100g"`
}
