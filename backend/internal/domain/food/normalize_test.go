package food

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase umlaut", "möhre", "mohre"},
		{"capitalised umlaut", "Möhre", "mohre"},
		{"uppercase umlaut", "MÖHRE", "mohre"},
		{"already folded", "mohre", "mohre"},
		{"all german umlauts", "Äpfel Öl Über", "apfel ol uber"},
		{"eszett", "Weißkohl", "weisskohl"},
		{"other latin diacritics", "Crème Brûlée Jalapeño Façade", "creme brulee jalapeno facade"},
		{"trim and collapse spaces", "  Rote \t  Bete  ", "rote bete"},
		{"empty", "", ""},
		{"only spaces", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeName(tt.in); got != tt.want {
				t.Errorf("normalizeName(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}
