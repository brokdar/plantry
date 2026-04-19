package settings

// EnvSnapshot captures the env-driven values known at process startup. It is
// immutable after construction so the service never needs to read os.Getenv
// at request time.
type EnvSnapshot struct {
	values map[string]string
}

// NewEnvSnapshot builds a snapshot from a key→value map. Entries with empty
// values are retained so callers can distinguish "set to empty" from "unset"
// — though for our purposes an empty env value is treated as unset.
func NewEnvSnapshot(values map[string]string) EnvSnapshot {
	copy := make(map[string]string, len(values))
	for k, v := range values {
		copy[k] = v
	}
	return EnvSnapshot{values: copy}
}

// Lookup returns the value and whether it is set (non-empty).
func (s EnvSnapshot) Lookup(envVar string) (string, bool) {
	if envVar == "" {
		return "", false
	}
	v, ok := s.values[envVar]
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
