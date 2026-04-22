package settings

import "context"

// Row is the stored representation of a setting (encrypted or plaintext).
type Row struct {
	Key       string
	Value     string
	Encrypted bool
}

// Repository persists raw setting rows. Encryption is handled by the service
// layer before calls to Upsert and after calls to Get.
type Repository interface {
	Get(ctx context.Context, key string) (Row, bool, error)
	List(ctx context.Context) ([]Row, error)
	Upsert(ctx context.Context, r Row) error
	Delete(ctx context.Context, key string) error
}
