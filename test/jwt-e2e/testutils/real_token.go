// Package testutils 提供测试环境的 Token 获取工具。
package testutils

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

// NewTokenSource 创建用于测试的 ID Token Source。
//
// 优先尝试以下方法（按顺序）：
// 1. 使用 gcloud CLI 直接获取 ID Token（最简单，推荐）
// 2. Service Account Impersonation（需要权限）
func NewTokenSource(ctx context.Context, serviceAccount, audience string) (oauth2.TokenSource, error) {
	// 方法 1：尝试使用 gcloud CLI 获取 Token
	token, err := getTokenFromGcloud(ctx, serviceAccount, audience)
	if err == nil {
		// 成功获取，返回静态 TokenSource
		return oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
		}), nil
	}

	// 方法 2：使用 Impersonation（需要额外权限）
	ts, err := impersonate.IDTokenSource(ctx, impersonate.IDTokenConfig{
		TargetPrincipal: serviceAccount,
		Audience:        audience,
		IncludeEmail:    true,
	}, option.WithScopes("https://www.googleapis.com/auth/cloud-platform"))
	if err != nil {
		return nil, fmt.Errorf("create token source failed (gcloud and impersonation both failed): %w", err)
	}

	return ts, nil
}

// getTokenFromGcloud 使用 gcloud CLI 获取 ID Token。
//
// 这是最简单的方法，直接调用：
// gcloud auth print-identity-token --impersonate-service-account=SA --audiences=AUDIENCE
func getTokenFromGcloud(ctx context.Context, serviceAccount, audience string) (string, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "print-identity-token",
		"--impersonate-service-account="+serviceAccount,
		"--audiences="+audience,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gcloud command failed: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("gcloud returned empty token")
	}

	return token, nil
}

// NewTokenSourceWithUserCredentials 使用用户凭证直接创建 TokenSource（如果支持）。
// 注意：用户凭证通常不支持 ID Token，此函数仅用于测试 Access Token。
func NewTokenSourceWithUserCredentials(ctx context.Context) (oauth2.TokenSource, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("find default credentials: %w", err)
	}

	return creds.TokenSource, nil
}
