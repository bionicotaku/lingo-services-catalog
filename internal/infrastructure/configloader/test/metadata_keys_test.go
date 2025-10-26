package configloader_test

import (
	"os"
	"path/filepath"
	"testing"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
)

func TestLoadMetadataKeys(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	configYAML := `server:
  grpc:
    addr: ":9000"
    timeout: 5s
  handlers:
    default_timeout: 5s
    command_timeout: 5s
    query_timeout: 5s
  metadata_keys:
    - x-md-global-user-id
    - x-md-idempotency-key
    - x-md-if-match
    - x-md-if-none-match
    - x-md-actor-type
    - x-md-actor-id

data:
  postgres:
    dsn: postgres://user:pass@localhost:5432/postgres?sslmode=disable
    max_open_conns: 1
    min_open_conns: 0
`

	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	runtimeCfg, err := configloader.Load(configloader.Params{ConfPath: cfgPath})
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	expected := []string{
		"x-md-global-user-id",
		"x-md-idempotency-key",
		"x-md-if-match",
		"x-md-if-none-match",
		"x-md-actor-type",
		"x-md-actor-id",
	}
	if got := runtimeCfg.Server.MetadataKeys; !equalStrings(got, expected) {
		t.Fatalf("server metadata keys mismatch: got %v want %v", got, expected)
	}
	if got := runtimeCfg.GRPCClient.MetadataKeys; !equalStrings(got, expected) {
		t.Fatalf("client metadata keys mismatch: got %v want %v", got, expected)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
