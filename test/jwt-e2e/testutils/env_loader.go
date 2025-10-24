// Package testutils 提供测试环境配置加载工具。
package testutils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 存储 E2E 测试配置。
type Config struct {
	ProjectID          string
	ServiceAccountEmail string
	Audience           string
	TestTimeout        string
	Verbose            string
}

// LoadTestEnv 从 configs/.env.test 加载测试环境变量。
//
// 如果文件不存在或加载失败，会返回错误。
// 已存在的环境变量不会被覆盖。
func LoadTestEnv() (*Config, error) {
	// 查找项目根目录
	rootDir, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("find project root: %w", err)
	}

	envFile := filepath.Join(rootDir, "configs", ".env.test")

	// 检查文件是否存在
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("配置文件不存在: %s\n"+
			"请复制 configs/.env.test.example 为 configs/.env.test 并填写实际值", envFile)
	}

	// 加载环境变量
	if err := loadEnvFile(envFile); err != nil {
		return nil, fmt.Errorf("load env file: %w", err)
	}

	// 构造配置对象
	cfg := &Config{
		ProjectID:           os.Getenv("GCP_PROJECT_ID"),
		ServiceAccountEmail: os.Getenv("JWT_TEST_SERVICE_ACCOUNT"),
		Audience:            os.Getenv("JWT_TEST_AUDIENCE"),
		TestTimeout:         getEnvOrDefault("E2E_TEST_TIMEOUT", "30"),
		Verbose:             getEnvOrDefault("E2E_TEST_VERBOSE", "true"),
	}

	// 验证必需字段
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("GCP_PROJECT_ID 未设置")
	}
	if cfg.ServiceAccountEmail == "" {
		return nil, fmt.Errorf("JWT_TEST_SERVICE_ACCOUNT 未设置")
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("JWT_TEST_AUDIENCE 未设置")
	}

	return cfg, nil
}

// loadEnvFile 加载 .env 文件并设置环境变量。
// 已存在的环境变量不会被覆盖。
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 KEY=VALUE 格式
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除值两边的引号（支持单引号和双引号）
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// 只设置未存在的环境变量
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// findProjectRoot 查找项目根目录（包含 go.mod 的目录）。
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 向上查找直到找到 go.mod
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// 已经到达根目录
			return "", fmt.Errorf("go.mod not found (not in a Go project?)")
		}
		dir = parent
	}
}

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值。
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
