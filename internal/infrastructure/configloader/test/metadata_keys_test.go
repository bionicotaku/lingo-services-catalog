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
    - x-apigateway-api-userinfo
    - x-md-
    - x-md-idempotency-key
    - x-md-if-match
    - x-md-if-none-match

data:
  postgres:
    dsn: postgres://user:pass@localhost:5432/postgres?sslmode=disable
    max_open_conns: 1
    min_open_conns: 0
  grpc_client:
    metadata_keys:
      - x-apigateway-api-userinfo
      - x-md-
`

	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	runtimeCfg, err := configloader.Load(configloader.Params{ConfPath: cfgPath})
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	serverExpected := []string{
		"x-apigateway-api-userinfo",
		"x-md-",
		"x-md-idempotency-key",
		"x-md-if-match",
		"x-md-if-none-match",
	}
	if got := runtimeCfg.Server.MetadataKeys; !equalStrings(got, serverExpected) {
		t.Fatalf("server metadata keys mismatch: got %v want %v", got, serverExpected)
	}
	clientExpected := []string{"x-apigateway-api-userinfo", "x-md-"}
	if got := runtimeCfg.GRPCClient.MetadataKeys; !equalStrings(got, clientExpected) {
		t.Fatalf("client metadata keys mismatch: got %v want %v", got, clientExpected)
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
