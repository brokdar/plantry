package food

// FillMissingKcal derives Kcal100g from macros for a single candidate, using
// the same rule the resolver applies to lookup results.
func FillMissingKcal(c *Candidate) {
	cs := []Candidate{*c}
	fillMissingKcal(cs)
	*c = cs[0]
}
