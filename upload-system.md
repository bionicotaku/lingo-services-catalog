下面给你一份**可落地的端到端设计**，覆盖架构、接口、GCS 配置、前端直传（断点续传/分片）、后端签名与回调、数据库模型与扩展处理（转码/缩略图/回放）。

场景假设：**前端 → 后端注册上传 → 后端向 GCS 申请可控上传权限 → 前端直传到 GCS → 上传完成后 GCS 回调后端**。

---

## **1) 总体架构 & 时序**

```
Browser (Front-end)
   │ 1. POST /api/uploads (注册上传：文件名/大小/MIME/策略等)
   ▼
API Server (Back-end)
   │ 2. 生成对象名、写DB为 pending
   │ 3. 用服务账号生成“v4 已签名的 Resumable 初始化URL”
   │    （action=resumable）
   │    并返回给前端
   ▼
Google Cloud Storage (GCS)
   │ 4. 前端对“已签名URL”发 POST + x-goog-resumable:start
   │    ← Location 头里返回“会话URI（session URI）”
   │ 5. 前端用该会话URI分片 PUT 上传（支持 308/Range 续传）
   │    上传完成返回 200/201
   │
   ├─(事件) 对象完成 → 触发 Pub/Sub 通知（OBJECT_FINALIZE）
   │
   ▼
Pub/Sub (push)
   │ 6. push 到后端 /_/gcs/callback 端点（OIDC JWT）
   ▼
API Server (Back-end)
   │ 7. 校验JWT与消息，更新DB为 completed，
   │    可异步发转码/缩略图任务
   ▼
DB/Transcoder/处理流水线
```

> 关键事实：

- > Resumable 上传会先返回 Location 里的 **session URI**，之后用 PUT 上传数据；上传分片成功会返回 **308 + Range 头**，直到最后 200/201；**会话有效期约 1 周**。这些细节以官方文档为准。

- > GCS 的“对象变更”通过 **Pub/Sub Notifications**（如 OBJECT_FINALIZE 事件），支持 **至少一次**投递（需幂等处理）。

- > 生成 **v4 签名的 Resumable 初始化 URL** 时，客户端发起初始化必须带 X-Goog-Resumable: start 头。

- > **分片大小**建议为 **8MiB+** 且必须是 **256KiB 的整数倍**（最后一片除外）。

- > 配置 **CORS** 要用 gcloud/配置文件（控制台不可直接设置），并显式 **Expose** Location、Range 等响应头以便前端可读。

---

## **2) 数据模型（建议）**

uploads 表（记录一次上传会话）：

```
CREATE TABLE uploads (
  id UUID PRIMARY KEY,
  user_id TEXT NOT NULL,
  bucket TEXT NOT NULL,
  object_name TEXT NOT NULL,
  original_filename TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','uploading','completed','failed')),
  session_url TEXT,                  -- 可选：若由后端创建会话并下发（备选方案）
  gcs_generation TEXT,               -- 完成后回填
  gcs_etag TEXT,                     -- 完成后回填
  md5_hash TEXT,                     -- 可选：校验
  crc32c TEXT,                       -- 可选：校验
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);
```

对象命名：videos/{user_id}/{uuid}/{slug}.{ext}，避免用户可控文件名直接作为对象路径（防穿越/碰撞）。

---

## **3) 后端 API 设计**

### **3.1 注册上传（生成已签名的 Resumable 初始化 URL）**

**POST** /api/uploads

- Request JSON：

```
{
  "filename": "movie.mp4",
  "size": 734003200,
  "contentType": "video/mp4",
  "visibility": "private"   // 可扩展：private/public/internal
}
```

-
- Response JSON：

```
{
  "uploadId": "uuid",
  "bucket": "your-bucket",
  "objectName": "videos/u123/8d2.../movie.mp4",
  "resumableInitUrl": "https://storage.googleapis.com/....(已签名v4，action=resumable)",
  "expiresAt": 1730112345000
}
```

> 说明：用 v4 签名 URL（action=resumable），前端对该 URL 发送 POST 并带 X-Goog-Resumable: start，GCS 返回 Location 头即 **session URI**。

### **3.2 查询上传状态**

**GET** /api/uploads/{uploadId} → 返回 status、bucket、objectName、处理进度等。

### **3.3 GCS 回调（Pub/Sub 推送）**

**POST** /\_/gcs/callback

- Header: Authorization: Bearer <OIDC JWT>（Pub/Sub 代表服务账号签名）
- Body: Pub/Sub push 消息（JSON 包含 message.data base64 的对象元数据字符串）。
- 处理：验证 JWT（iss=https://accounts.google.com、aud 为你配置的 audience、email_verified true 等），解析 OBJECT_FINALIZE 消息，幂等更新记录为 completed，可触发异步转码/缩略图。

---

## **4) 后端关键实现（Node.js/TypeScript 示例）**

### **4.1 生成 v4 签名的 Resumable 初始化 URL**

```
import {Storage} from '@google-cloud/storage';
import {v4 as uuid} from 'uuid';

const storage = new Storage(); // 使用服务账号
const BUCKET = 'your-bucket';

export async function createUploadSession(req, res) {
  const {filename, size, contentType} = req.body;
  const uploadId = uuid();
  const objectName = `videos/${req.user.id}/${uploadId}/${filename}`;

  // 写入DB：status=pending（略）

  const file = storage.bucket(BUCKET).file(objectName);

  // 生成 v4 Signed URL（action: "resumable"）
  const [signedUrl] = await file.getSignedUrl({
    version: 'v4',
    action: 'resumable',             // 关键
    expires: Date.now() + 15 * 60 * 1000,
    // 建议带contentType，后续对象元数据更准确
    contentType: contentType || 'application/octet-stream',
  });

  res.json({
    uploadId,
    bucket: BUCKET,
    objectName,
    resumableInitUrl: signedUrl,
    expiresAt: Date.now() + 15 * 60 * 1000
  });
}
```

> 备注：客户端**必须**在发起初始化请求时带 X-Goog-Resumable: start；该约束见官方 SDK 文档注释。

### **4.2 验证 Pub/Sub push（OIDC）**

```
import * as jwt from 'jsonwebtoken';
import {OAuth2Client} from 'google-auth-library';

const client = new OAuth2Client();

export async function gcsCallback(req, res) {
  const auth = req.header('Authorization') || '';
  const token = auth.replace(/^Bearer\s+/i, '');
  if (!token) return res.status(401).send('missing token');

  // 验证签名、iss、aud 等（示例仅演示aud检查，生产环境请充分校验）
  const ticket = await client.verifyIdToken({
    idToken: token,
    audience: 'https://api.example.com/_/gcs/callback', // 你配置的 audience
  });
  const payload = ticket.getPayload();
  if (payload?.iss !== 'https://accounts.google.com') return res.status(401).send('bad iss');

  // 解析 Pub/Sub 消息
  const msg = req.body?.message;
  const data = msg?.data ? JSON.parse(Buffer.from(msg.data, 'base64').toString()) : null;

  // 只处理 OBJECT_FINALIZE
  if (req.body?.message?.attributes?.eventType === 'OBJECT_FINALIZE' && data?.name) {
    // 幂等更新 uploads 表记录为 completed，记录 generation/etag/size/contentType……
    // 可投入转码/缩略图任务（见下文）
  }

  // 2xx/102 即视为成功
  res.status(204).end();
}
```

> Pub/Sub push JWT 的验证与配置（包括 --push-auth-service-account 和可选 audience）详见官方文档。

---

## **5) 前端直传实现（断点续传/分片）**

### **5.1 初始化 & 拿到 session URI**

```
// 1) 调后端注册
const reg = await fetch('/api/uploads', {method: 'POST', body: JSON.stringify({...})}).then(r=>r.json());

// 2) 用已签名URL发起Resumable会话
const initResp = await fetch(reg.resumableInitUrl, {
  method: 'POST',
  headers: {
    'X-Goog-Resumable': 'start',
    'X-Upload-Content-Type': file.type,      // 可选，但推荐
    'X-Upload-Content-Length': String(file.size),
    'Origin': window.location.origin          // 配合CORS
  }
});
const sessionUri = initResp.headers.get('Location'); // 这是后续上传用的“会话URL”
localStorage.setItem(`upload:${reg.uploadId}:sessionUri`, sessionUri);
```

> Location 里返回的是 **session URI**；其有效期**约一周**。

### **5.2 分片上传（建议 8MiB/片，必须是 256KiB 的倍数）**

```
async function uploadInChunks(file: File, sessionUri: string, onProgress: (n:number)=>void) {
  const CHUNK_SIZE = 8 * 1024 * 1024; // 8MiB，且满足256KiB倍数
  let offset = await queryOffset(sessionUri, file.size); // 断点续传

  while (offset < file.size) {
    const next = Math.min(offset + CHUNK_SIZE, file.size);
    const chunk = file.slice(offset, next);

    const resp = await fetch(sessionUri, {
      method: 'PUT',
      headers: {
        'Content-Length': String(chunk.size),
        'Content-Range': `bytes ${offset}-${next-1}/${file.size}`
      },
      body: chunk
    });

    if (resp.status === 308) {
      const range = resp.headers.get('Range');          // e.g. "bytes=0-8388607"
      const end = range ? Number(range.split('-')[1]) : offset - 1;
      offset = end + 1;
      onProgress(offset / file.size);
    } else if (resp.ok) {
      onProgress(1);
      return; // 完成
    } else {
      throw new Error(`Upload failed: ${resp.status}`);
    }
  }
}

async function queryOffset(sessionUri: string, total: number) {
  const resp = await fetch(sessionUri, {
    method: 'PUT',
    headers: {'Content-Length': '0', 'Content-Range': `bytes */${total}`}
  });
  if (resp.status === 308) {
    const rng = resp.headers.get('Range'); // 可能为null（尚未持久化任何字节）
    return rng ? (Number(rng.split('-')[1]) + 1) : 0;
  }
  if (resp.ok) return total; // 已完成
  return 0;
}
```

> 分片细节（308 Resume Incomplete、Range 头、Content-Range 规范），以及**分片大小需为 256KiB 的倍数**等规则，均见官方文档。

---

## **6) GCS 侧配置**

### **6.1 CORS（允许前端跨域直传，并暴露必要响应头）**

cors.json 示例：

```
[
  {
    "origin": ["https://your-frontend.example.com"],
    "method": ["POST", "PUT", "GET", "HEAD", "OPTIONS"],
    "responseHeader": [
      "Content-Type",
      "X-Goog-Resumable",
      "X-Upload-Content-Type",
      "X-Upload-Content-Length",
      "Content-Range",
      "Range",
      "Location",
      "ETag",
      "x-goog-hash"
    ],
    "maxAgeSeconds": 3600
  }
]
```

应用：

```
gcloud storage buckets update gs://your-bucket --cors-file=cors.json
```

> CORS 需用 gcloud/配置文件管理；前端若需读取非 safelist 响应头，需要 Access-Control-Expose-Headers（上面通过 responseHeader 配置实现）。

### **6.2 桶权限**

- **启用** Uniform bucket-level access（UBLA），用 IAM 管控、禁用 ACL，简化且更安全。
- 后端服务账号最小权限：写入对象可用 roles/storage.objectCreator（只创建不读不删）；若需要读回对象元数据/触发器等，可加 roles/storage.objectUser。

### **6.3 Pub/Sub 通知 + push**

```
# 1) 创建topic
gcloud pubsub topics create video-uploads

# 2) 桶绑定通知：只监听对象完成
gcloud storage buckets notifications create gs://your-bucket \
  --topic=video-uploads \
  --event-types=OBJECT_FINALIZE \
  --payload-format=json

# 3) 创建 push 订阅，携带OIDC token推送到你的后端
gcloud pubsub subscriptions create video-uploads-push \
  --topic=video-uploads \
  --push-endpoint=https://api.example.com/_/gcs/callback \
  --push-auth-service-account=push-notifier@your-proj.iam.gserviceaccount.com \
  --push-auth-token-audience=https://api.example.com/_/gcs/callback
```

> Pub/Sub Notifications 的事件种类/负载格式与**至少一次**投递语义见文档；push 认证采用 OIDC JWT（后端需要校验 iss/aud/email_verified 等）。

---

## **7) 安全与健壮性**

- **对象名只在后端生成**，不要信任前端文件名。
- **签名 URL 过期**：建议 10–15 分钟。session URI 有效期 ~1 周；不要把 session URI 永久存储到客户端可被他人获取的地方。
- **大小/类型限制**：在注册上传时校验 size/contentType，并将期望值通过 X-Upload-Content-\* 传入初始化请求，回调时再次比对。
- **幂等回调**：Pub/Sub 至少一次投递，按 uploadId + generation 去重更新。
- **重试与续传**：前端断网/刷新后持久化 sessionUri 到本地存储，PUT 0 字节+Content-Range: \*/size 查询偏移再续传。

---

## **8) 上传后处理（可选）**

- **视频转码/自适应码流**：用 **Transcoder API** 生成 HLS/DASH/MP4，支持丰富的 JobConfig；回调完成后触发一个转码作业（Cloud Tasks/WorkQueue 驱动）。
- **缩略图**：Cloud Run/Functions 拉起 ffmpeg 从关键帧导出海报图与预览 sprites。

---

## **9) 备选实现：由后端初始化会话并直接返回 session URI**

如果你不希望把“初始化签名 URL”暴露给前端，也可由后端使用服务账号直接发起初始化（拿到 Location 会话 URI）并**将该 session URI 直接返回给前端**，前端随后用它 PUT 上传（GCS 接受持有该 URL 的任何客户端继续上传）。官方对分享 session URI 有提示，需要注意保密与时效管理。

---

## **10) 关键点清单（上线 checklist）**

- GCS 桶：区域、UBLA 开启、版本策略（可选）
- CORS：允许前端域、方法 POST/PUT/OPTIONS、Expose Location/Range/ETag/x-goog-hash
- 后端：/api/uploads 生成 v4 action=resumable 签名 URL；DB 记录
- 前端：先 POST 初始化拿 Location，再分片 PUT；支持 308/Range 续传
- Pub/Sub：bucket notification（OBJECT_FINALIZE→topic），push 订阅到后端，OIDC 验证
- 幂等：重复消息/重复回调处理
- 处理流水线：转码/缩略图/内容审核（可选）
- 监控：错误率、上传时延、失败重试、对象大小分布

---

### **参考（核心文档）**

- Resumable 上传流程（Location/308/Range/有效期一周/可查询偏移/分片必须 256KiB 倍数）：
- Pub/Sub Notifications for GCS（事件类型、负载、至少一次语义）：
- v4 已签名 URL（resumable 需要 X-Goog-Resumable: start）：
- CORS 配置与暴露响应头（控制台不可配置 CORS、Expose-Headers 机制）：
- 分片大小应为 256KiB 整数倍（建议至少 8MiB）：

---

如果你愿意，我可以把上面的**后端与前端代码片段补全成最小可运行项目的骨架（Express + 小型上传页面）**，并附上 gcloud 一键化脚本与示例 cors.json。
