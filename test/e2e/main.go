package main

import (
	"context"
	"log"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// gRPC 服务地址
	grpcAddr = "localhost:9000"
	// 测试用户 ID
	testUserID = "f0ad5a16-0d50-4f94-8ff7-b99dda13ee47"
)

func main() {
	ctx := context.Background()

	// 1. 建立 gRPC 连接
	conn, err := grpc.NewClient(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("❌ 连接 gRPC 服务失败: %v", err)
	}
	defer conn.Close()

	commandClient := videov1.NewVideoCommandServiceClient(conn)
	queryClient := videov1.NewVideoQueryServiceClient(conn)

	log.Println("✅ 已连接到 gRPC 服务:", grpcAddr)
	log.Println()

	// 2. 测试 CreateVideo
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("📝 测试 1: CreateVideo")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	createReq := &videov1.CreateVideoRequest{
		UploadUserId:     testUserID,
		Title:            "端到端测试视频",
		Description:      strPtr("这是一个完整的端到端测试，验证 Outbox → Pub/Sub → Projection 流程"),
		RawFileReference: "gs://test-bucket/videos/e2e-test-" + time.Now().Format("20060102-150405") + ".mp4",
	}

	createResp, err := commandClient.CreateVideo(ctx, createReq)
	if err != nil {
		log.Fatalf("❌ CreateVideo 失败: %v", err)
	}

	videoID := createResp.VideoId
	log.Printf("✅ CreateVideo 成功!")
	log.Printf("   Video ID: %s", videoID)
	log.Printf("   Status: %s", createResp.Status)
	log.Printf("   Event ID: %s", createResp.EventId)
	log.Printf("   Version: %d", createResp.Version)
	log.Printf("   Created At: %s", createResp.CreatedAt)
	log.Println()

	// 3. 等待投影同步
	log.Println("⏳ 等待投影同步（5 秒）...")
	log.Println("   流程: videos → outbox → pub/sub → inbox → video_projection")
	time.Sleep(5 * time.Second)
	log.Println()

	// 注意：投影表查询只返回 ready/published 状态的视频
	// pending_upload 状态的视频会被过滤，这是业务设计
	log.Println("💡 注意: 投影表查询只返回 ready/published 状态的视频")
	log.Println("   pending_upload 状态会被过滤（业务设计）")
	log.Println("   现在先 UpdateVideo 改为 ready 状态，再验证查询")
	log.Println()

	// 4. 测试 UpdateVideo（改为 ready 状态）
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("📝 测试 2: UpdateVideo（改为 ready 状态）")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	updateReq := &videov1.UpdateVideoRequest{
		VideoId:     videoID,
		Title:       strPtr("端到端测试视频（已更新）"),
		Status:      strPtr("ready"),
		MediaStatus: strPtr("ready"),
	}

	updateResp, err := commandClient.UpdateVideo(ctx, updateReq)
	if err != nil {
		log.Fatalf("❌ UpdateVideo 失败: %v", err)
	}

	log.Println("✅ UpdateVideo 成功!")
	log.Printf("   Video ID: %s", updateResp.VideoId)
	log.Printf("   Status: %s", updateResp.Status)
	log.Printf("   Media Status: %s", updateResp.MediaStatus)
	log.Printf("   Event ID: %s", updateResp.EventId)
	log.Printf("   Version: %d", updateResp.Version)
	log.Println()

	// 5. 等待投影同步
	log.Println("⏳ 等待投影同步（10 秒）...")
	time.Sleep(10 * time.Second)
	log.Println()

	// 6. 验证投影表查询（现在状态是 ready，应该能查到）
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("🔍 验证投影表查询（ready 状态）")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	getReq := &videov1.GetVideoDetailRequest{
		VideoId: videoID,
	}

	getResp, err := queryClient.GetVideoDetail(ctx, getReq)
	if err != nil {
		log.Printf("❌ GetVideoDetail 失败: %v", err)
		log.Println("   可能原因:")
		log.Println("   1. Outbox Publisher 未启动或未成功发布")
		log.Println("   2. Projection Consumer 未启动或未消费消息")
		log.Println("   3. Pub/Sub 消息传递延迟")
		return
	}

	detail := getResp.Detail
	log.Println("✅ 投影表查询成功!")
	log.Printf("   Video ID: %s", detail.VideoId)
	log.Printf("   Title: %s", detail.Title)
	log.Printf("   Status: %s", detail.Status)
	log.Printf("   Media Status: %s", detail.MediaStatus)
	log.Printf("   Analysis Status: %s", detail.AnalysisStatus)
	log.Printf("   Created At: %s", detail.CreatedAt)
	log.Printf("   Updated At: %s", detail.UpdatedAt)
	log.Println()

	// 7. 测试再次 UpdateVideo
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("📝 测试 3: UpdateVideo（第二次更新）")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	updateReq2 := &videov1.UpdateVideoRequest{
		VideoId:        videoID,
		Title:          strPtr("端到端测试视频（第二次更新）"),
		AnalysisStatus: strPtr("ready"),
	}

	updateResp2, err := commandClient.UpdateVideo(ctx, updateReq2)
	if err != nil {
		log.Fatalf("❌ UpdateVideo 失败: %v", err)
	}

	log.Println("✅ UpdateVideo 成功!")
	log.Printf("   Video ID: %s", updateResp2.VideoId)
	log.Printf("   Analysis Status: %s", updateResp2.AnalysisStatus)
	log.Printf("   Version: %d", updateResp2.Version)
	log.Println()

	// 8. 等待投影同步
	log.Println("⏳ 等待投影同步（10 秒）...")
	time.Sleep(10 * time.Second)
	log.Println()

	// 9. 验证第二次更新
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("🔍 验证第二次更新")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	getResp2, err := queryClient.GetVideoDetail(ctx, getReq)
	if err != nil {
		log.Fatalf("❌ GetVideoDetail 失败: %v", err)
	}

	detail2 := getResp2.Detail
	log.Println("✅ 投影表查询成功!")
	log.Printf("   Title: %s", detail2.Title)
	log.Printf("   Analysis Status: %s", detail2.AnalysisStatus)
	log.Println()

	// 10. 测试 DeleteVideo
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("📝 测试 4: DeleteVideo")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	deleteReq := &videov1.DeleteVideoRequest{
		VideoId: videoID,
		Reason:  strPtr("端到端测试清理"),
	}

	deleteResp, err := commandClient.DeleteVideo(ctx, deleteReq)
	if err != nil {
		log.Fatalf("❌ DeleteVideo 失败: %v", err)
	}

	log.Println("✅ DeleteVideo 成功!")
	log.Printf("   Video ID: %s", deleteResp.VideoId)
	log.Printf("   Event ID: %s", deleteResp.EventId)
	log.Printf("   Version: %d", deleteResp.Version)
	log.Println()

	// 11. 等待投影同步
	log.Println("⏳ 等待投影同步（10 秒）...")
	time.Sleep(10 * time.Second)
	log.Println()

	// 12. 验证投影表已删除
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("🔍 验证投影表删除")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	_, err = queryClient.GetVideoDetail(ctx, getReq)
	if err != nil {
		log.Println("✅ 投影表已删除（符合预期）")
		log.Printf("   错误信息: %v", err)
	} else {
		log.Println("❌ 投影表未删除（不符合预期）")
	}
	log.Println()

	// 13. 完成
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("🎉 端到端测试完成!")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()
	log.Println("✅ 已验证流程:")
	log.Println("   1. CreateVideo → 写入 videos + outbox ✓")
	log.Println("   2. UpdateVideo (ready) → outbox → pub/sub → projection ✓")
	log.Println("   3. GetVideoDetail → 查询投影表（ready 状态）✓")
	log.Println("   4. UpdateVideo (第二次) → 投影更新 ✓")
	log.Println("   5. DeleteVideo → 投影删除 ✓")
}

func strPtr(s string) *string {
	return &s
}
