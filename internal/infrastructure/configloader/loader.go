package configloader

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	configpb "github.com/bionicotaku/lingo-services-catalog/configs"

	"github.com/bufbuild/protovalidate-go"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/joho/godotenv"
)

// Params 控制配置加载的输入参数。
type Params struct {
	ConfPath string
}

const (
	defaultConfPath       = "configs/config.yaml"
	envConfPath           = "CONF_PATH"
	envDatabaseURL        = "DATABASE_URL"
	envPort               = "PORT"
	envServiceName        = "SERVICE_NAME"
	envServiceVersion     = "SERVICE_VERSION"
	envEnvironment        = "APP_ENV"
	defaultServiceName    = "template"
	defaultServiceVersion = "dev"
	defaultEnvironment    = "development"
)

// Load 解析配置文件并返回归一化的 RuntimeConfig。
func Load(params Params) (RuntimeConfig, error) {
	confPath := resolveConfPath(params.ConfPath)
	if err := loadEnvFiles(confPath); err != nil {
		return RuntimeConfig{}, fmt.Errorf("load env files: %w", err)
	}

	bootstrap, err := loadBootstrap(confPath)
	if err != nil {
		return RuntimeConfig{}, err
	}

	service := buildServiceInfo()
	runtime := fromProto(bootstrap)
	runtime.Service = service
	fillDefaults(&runtime)

	return runtime, nil
}

func resolveConfPath(explicit string) string {
	switch {
	case explicit != "":
		return explicit
	case os.Getenv(envConfPath) != "":
		return os.Getenv(envConfPath)
	default:
		return defaultConfPath
	}
}

func loadEnvFiles(confPath string) error {
	dirs := candidateDirs(confPath)
	var files []string
	seen := map[string]struct{}{}
	for _, dir := range dirs {
		for _, name := range []string{".env.local", ".env"} {
			fp := filepath.Join(dir, name)
			if _, err := os.Stat(fp); err != nil {
				continue
			}
			if _, ok := seen[fp]; ok {
				continue
			}
			files = append(files, fp)
			seen[fp] = struct{}{}
		}
	}
	if len(files) == 0 {
		return nil
	}
	return godotenv.Overload(files...)
}

func candidateDirs(confPath string) []string {
	var dirs []string
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		for _, exist := range dirs {
			if exist == clean {
				return
			}
		}
		dirs = append(dirs, clean)
	}

	if info, err := os.Stat(confPath); err == nil {
		if info.IsDir() {
			add(confPath)
		} else {
			add(filepath.Dir(confPath))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	return dirs
}

func loadBootstrap(confPath string) (*configpb.Bootstrap, error) {
	c := config.New(config.WithSource(file.NewSource(confPath)))
	if err := c.Load(); err != nil {
		return nil, fmt.Errorf("load config %q: %w", confPath, err)
	}
	defer c.Close()

	var bootstrap configpb.Bootstrap
	if err := c.Scan(&bootstrap); err != nil {
		return nil, fmt.Errorf("scan config %q: %w", confPath, err)
	}

	applyEnvOverrides(&bootstrap)

	validator, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("init validator: %w", err)
	}
	if err := validator.Validate(&bootstrap); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &bootstrap, nil
}

func buildServiceInfo() ServiceInfo {
	name := firstNonEmpty(os.Getenv(envServiceName), defaultServiceName)
	version := firstNonEmpty(os.Getenv(envServiceVersion), defaultServiceVersion)
	env := resolveEnvironment(os.Getenv(envEnvironment))
	instance := hostnameOrDefault()

	return ServiceInfo{
		Name:        name,
		Version:     version,
		Environment: env,
		InstanceID:  instance,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveEnvironment(raw string) string {
	if raw == "" {
		return defaultEnvironment
	}
	switch raw {
	case "dev", "development":
		return defaultEnvironment
	case "staging":
		return "staging"
	case "prod", "production":
		return "production"
	default:
		return raw
	}
}

func hostnameOrDefault() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "unknown-instance"
	}
	return host
}

func applyEnvOverrides(b *configpb.Bootstrap) {
	if b == nil {
		return
	}
	if dsn := os.Getenv(envDatabaseURL); dsn != "" {
		if data := b.GetData(); data != nil && data.Postgres != nil {
			data.Postgres.Dsn = dsn
		}
	}
	if port := os.Getenv(envPort); port != "" {
		if server := b.GetServer(); server != nil && server.Grpc != nil {
			server.Grpc.Addr = replacePort(server.Grpc.GetAddr(), port)
		}
	}
}

func replacePort(addr, port string) string {
	if addr == "" {
		return ":" + port
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}
