# 接口实现进度

跟踪 [api.md](./api.md) 中各接口的实现状态。

**Legend**: ✅ 完成 · 🚧 进行中 · ⬜ 待开始

**最后更新**: 2026-04-30

---

## 总览

| 模块 | Proto | Handler | Service | Storage | 测试 |
|---|:-:|:-:|:-:|:-:|:-:|
| 1. Auth | ✅ | 🚧 | 🚧 | 🚧 | ⬜ |
| 2. Users / Profile | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 3. Connections | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 4. Posts & Feed | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 4.1 互动 (Like/Bookmark/Share) | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 4.2 Comments | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 5. Media | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 6. Communities | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 7. Discovery | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 7.1 Search | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 8. Notifications | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 9. Messages | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 10. Dashboard | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 11. Settings | ✅ | ⬜ | ⬜ | ⬜ | ⬜ |
| 系统接口 (`/health` `/ready` `/api/v1/ping`) | — | ✅ | — | — | ⬜ |

---

## 基础设施

| 项 | 状态 | 备注 |
|---|:-:|---|
| 项目结构 (cmd/internal/pkg/proto) | ✅ | 参考 `kongken/kapi` |
| buf 工具链 (`buf.yaml` / `buf.gen.yaml`) | ✅ | Go + gRPC + Connect |
| butterfly core 接入 | ✅ | `app.New(...).Run()` |
| Gin Router 注册入口 | ✅ | `internal/http/routes.go` |
| Storage - Redis / S3 | ✅ | 由 butterfly core 提供 |
| Storage - Postgres + ent | ✅ | `internal/dao/`，底层连接由 butterfly `store/sqldb` 提供（pgx/v5），ent 包装在其上 |
| ent 自动建表 (`Schema.Create`) | ✅ | 启动时执行；生产建议改 atlas 版本化迁移 |
| CORS 中间件 | ✅ | `corsMiddleware()` |
| 健康检查 / 探针 | ✅ | `/health` `/ready` |
| 配置管理 (yaml) | ✅ | `config.yaml` + `internal/config` |
| Dockerfile | ✅ | 多阶段构建 |
| Makefile (tidy/proto/ent/build/run) | ✅ | |
| 全局 JWT 鉴权中间件 | ✅ | `internal/auth/middleware.go` (`RequireAuth(issuer)` + `UserID(c)`) |
| 统一错误响应 (`common.v1.Error`) | ✅ | `internal/httpx/errors.go` |
| 分页参数解析 helper | ✅ | `internal/httpx/pagination.go` (`ParsePage`) |
| Idempotency-Key 中间件 | ⬜ | 写操作（POST 帖子/消息） |
| 限流中间件 | ⬜ | 登录、注册、发帖 |
| WebSocket 实时通道 (`/ws`) | ⬜ | message.new / notification.new |
| OpenAPI / Swagger 输出 | ⬜ | 可由 buf 插件生成 |

### 存储分层决策

| 实体 / 数据类 | 存储 | 理由 |
|---|---|---|
| User | Postgres (ent) | 鉴权关键、强一致 |
| Post 主记录 | Postgres (ent) | feed JOIN、hashtag GIN 索引、计数事务 |
| Comment | Postgres (ent) | 楼中楼递归 CTE |
| Connection / Follow / ConnectionRequest | Postgres (ent) | 关系表，需事务保证 |
| Community + Membership | Postgres (ent) | 关系表 |
| Bookmark | Postgres (ent) | 关系表 |
| Block | Postgres (ent) | 关系表 |
| Privacy / Settings | Postgres (ent) | 行级配置 |
| Session (登录会话) | Postgres (ent) | 与 User 强绑定 |
| **Notification** | **Mongo** | 8 种类型 payload 形状不同 + TTL 自动清理 + 写多读少 |
| **Conversation / Message** | **Mongo** | fan-out 写多、按 conversation 分片、附件嵌套 |
| Device (push token) | Mongo | 形状随平台扩展，TTL 友好 |
| 头像 / 封面 / 帖子图 / 视频 / 私信附件 / Photo bytes | **S3** | 二进制 blob，Postgres 只存 `media_id` + metadata |
| Feed 时间线 / 热点计数（likes_count, unread_count）/ 在线状态 | Redis | butterfly 已接 |
| 限流计数 / Idempotency-Key 缓存 | Redis | |

### DAO / Schema 实施进度

#### Postgres (ent schemas)

| 实体 | Schema | 状态 |
|---|---|:-:|
| User | `internal/dao/ent/schema/user.go` | ✅ 基础字段 + followers/following 自关联 |
| Post |  | ⬜ |
| Comment |  | ⬜ |
| Community |  | ⬜ |
| Membership (User↔Community) |  | ⬜ |
| ConnectionRequest |  | ⬜ |
| Bookmark |  | ⬜ |
| Block |  | ⬜ |
| Photo (metadata only) |  | ⬜ |
| Session |  | ⬜ |
| Privacy |  | ⬜ |

#### Mongo (collections)

| Collection | 文档形状 | 索引 | 状态 |
|---|---|---|:-:|
| `notifications` | `{_id, user_id, type, actor, target_*, payload, created_at, read_at}` | `(user_id, created_at desc)`、`read_at` TTL | ⬜ |
| `conversations` | `{_id, participants[], last_message, unread:{user_id:int}}` | `participants` | ⬜ |
| `messages` | `{_id, conversation_id, sender_id, content, attachments[], created_at, read_by[]}` | `(conversation_id, created_at desc)` | ⬜ |
| `devices` | `{_id, user_id, platform, token, last_seen_at}` | `user_id`、`token` 唯一 | ⬜ |

DAO helper 入口：`internal/dao/mongo.go` (`NotificationsColl()` / `MessagesColl()` / `ConversationsColl()`)

#### S3 (media bucket)

| 用途 | Key 约定 | 状态 |
|---|---|:-:|
| 头像 | `avatars/{user_id}/{media_id}.{ext}` | ⬜ |
| 封面 | `covers/{user_id}/{media_id}.{ext}` | ⬜ |
| 帖子图片 / 视频 | `posts/{user_id}/{yyyy-mm}/{media_id}.{ext}` | ⬜ |
| Photo (相册照片) | `photos/{user_id}/{media_id}.{ext}` | ⬜ |
| 私信附件 | `messages/{conversation_id}/{media_id}.{ext}` | ⬜ |

DAO helper 入口：`internal/dao/media.go` (`MediaClient()` / `MediaBucketName()`)；上传通过 `MediaService.Presign` 返回预签名 PUT URL，前端直传。

---

## 1. 认证与会话 (Auth)

| Method | Path | 状态 |
|---|---|:-:|
| POST | `/auth/register` | ✅ |
| POST | `/auth/login` | ✅ |
| POST | `/auth/logout` | ⬜ 后续与 session/blacklist 一起做 |
| POST | `/auth/refresh` | ✅ |
| POST | `/auth/password/forgot` | ⬜ 依赖邮件服务 |
| POST | `/auth/password/reset` | ⬜ 依赖邮件服务 |
| POST | `/auth/password/change` | ⬜ |
| POST | `/auth/email/verify-request` | ⬜ 依赖邮件服务 |
| POST | `/auth/email/verify` | ⬜ |
| GET  | `/auth/me` | ✅ |

实现：`internal/auth/{handler,jwt,password,middleware}.go`；JWT 用 HS256（`github.com/golang-jwt/jwt/v5`），密码 bcrypt cost 12。

---

## 2. 用户与个人资料 (Users)

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/users/me` | ⬜ |
| PATCH  | `/users/me` | ⬜ |
| DELETE | `/users/me` | ⬜ |
| POST   | `/users/me/avatar` | ⬜ |
| POST   | `/users/me/cover` | ⬜ |
| GET    | `/users/{username}` | ⬜ |
| GET    | `/users/{username}/stats` | ⬜ |
| GET    | `/users/{username}/posts` | ⬜ |
| GET    | `/users/{username}/photos` | ⬜ |
| GET    | `/users/{username}/friends` | ⬜ |
| GET    | `/users/me/interests` | ⬜ |
| PUT    | `/users/me/interests` | ⬜ |
| POST   | `/users/me/interests` | ⬜ |
| DELETE | `/users/me/interests/{tag}` | ⬜ |

---

## 3. 关注 / 好友 (Connections)

| Method | Path | 状态 |
|---|---|:-:|
| POST   | `/users/{username}/follow` | ⬜ |
| DELETE | `/users/{username}/follow` | ⬜ |
| GET    | `/users/{username}/followers` | ⬜ |
| GET    | `/users/{username}/following` | ⬜ |
| GET    | `/connections` | ⬜ |
| DELETE | `/connections/{user_id}` | ⬜ |
| GET    | `/connections/requests` | ⬜ |
| POST   | `/connections/requests` | ⬜ |
| POST   | `/connections/requests/{id}/accept` | ⬜ |
| POST   | `/connections/requests/{id}/reject` | ⬜ |
| POST   | `/users/me/contacts/sync` | ⬜ |

---

## 4. 动态 / 帖子 (Posts)

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/feed` | ⬜ |
| GET    | `/posts/{id}` | ⬜ |
| POST   | `/posts` | ⬜ |
| PATCH  | `/posts/{id}` | ⬜ |
| DELETE | `/posts/{id}` | ⬜ |
| POST   | `/posts/{id}/report` | ⬜ |
| POST   | `/posts/{id}/hide` | ⬜ |

### 4.1 互动

| Method | Path | 状态 |
|---|---|:-:|
| POST   | `/posts/{id}/like` | ⬜ |
| DELETE | `/posts/{id}/like` | ⬜ |
| GET    | `/posts/{id}/likes` | ⬜ |
| POST   | `/posts/{id}/share` | ⬜ |
| POST   | `/posts/{id}/bookmark` | ⬜ |
| DELETE | `/posts/{id}/bookmark` | ⬜ |
| GET    | `/users/me/bookmarks` | ⬜ |

### 4.2 评论

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/posts/{id}/comments` | ⬜ |
| POST   | `/posts/{id}/comments` | ⬜ |
| PATCH  | `/comments/{id}` | ⬜ |
| DELETE | `/comments/{id}` | ⬜ |
| POST   | `/comments/{id}/like` | ⬜ |
| DELETE | `/comments/{id}/like` | ⬜ |

---

## 5. 媒体 (Media)

| Method | Path | 状态 |
|---|---|:-:|
| POST | `/media/uploads` | ⬜ |
| POST | `/media/uploads/presign` | ⬜ |
| GET  | `/media/{id}` | ⬜ |
| GET  | `/users/{username}/albums` | ⬜ |
| GET  | `/users/me/photos` | ⬜ |
| POST | `/users/me/photos` | ⬜ |
| DELETE | `/photos/{id}` | ⬜ |

依赖：butterfly core `store/s3` 客户端。

---

## 6. 社区 (Communities)

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/communities` | ⬜ |
| GET    | `/communities/{id}` | ⬜ |
| GET    | `/communities/{id}/posts` | ⬜ |
| POST   | `/communities/{id}/join` | ⬜ |
| DELETE | `/communities/{id}/join` | ⬜ |
| GET    | `/communities/{id}/members` | ⬜ |
| GET    | `/users/me/communities` | ⬜ |

---

## 7. 发现 (Discovery)

| Method | Path | 状态 |
|---|---|:-:|
| GET | `/discover/highlights` | ⬜ |
| GET | `/discover/trending/posts` | ⬜ |
| GET | `/discover/trending/topics` | ⬜ |
| GET | `/discover/suggested-users` | ⬜ |
| GET | `/discover/communities` | ⬜ |
| GET | `/discover/discussions` | ⬜ |
| GET | `/discover/categories` | ⬜ |

### 7.1 搜索

| Method | Path | 状态 |
|---|---|:-:|
| GET | `/search` | ⬜ |
| GET | `/search/suggest` | ⬜ |

---

## 8. 通知 (Notifications)

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/notifications` | ⬜ |
| GET    | `/notifications/unread-count` | ⬜ |
| POST   | `/notifications/{id}/read` | ⬜ |
| POST   | `/notifications/read-all` | ⬜ |
| DELETE | `/notifications/{id}` | ⬜ |

---

## 9. 私信 (Messages)

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/messages/conversations` | ⬜ |
| POST   | `/messages/conversations` | ⬜ |
| GET    | `/messages/conversations/{id}` | ⬜ |
| GET    | `/messages/conversations/{id}/messages` | ⬜ |
| POST   | `/messages/conversations/{id}/messages` | ⬜ |
| POST   | `/messages/conversations/{id}/read` | ⬜ |
| DELETE | `/messages/{id}` | ⬜ |
| GET    | `/ws` | ⬜ |

---

## 10. 仪表盘 (Dashboard)

| Method | Path | 状态 |
|---|---|:-:|
| GET | `/dashboard/summary` | ⬜ |
| GET | `/dashboard/metrics` | ⬜ |
| GET | `/dashboard/activity` | ⬜ |
| GET | `/dashboard/quick-links` | ⬜ |

---

## 11. 设置 (Settings)

### 11.1 Account

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/settings/account` | ⬜ |
| PATCH  | `/settings/account/email` | ⬜ |
| POST   | `/settings/account/2fa/enable` | ⬜ |
| POST   | `/settings/account/2fa/disable` | ⬜ |
| GET    | `/settings/account/sessions` | ⬜ |
| DELETE | `/settings/account/sessions/{id}` | ⬜ |

### 11.2 Privacy

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/settings/privacy` | ⬜ |
| PATCH  | `/settings/privacy` | ⬜ |
| GET    | `/settings/privacy/blocked` | ⬜ |
| POST   | `/users/{username}/block` | ⬜ |
| DELETE | `/users/{username}/block` | ⬜ |

### 11.3 Notifications

| Method | Path | 状态 |
|---|---|:-:|
| GET    | `/settings/notifications` | ⬜ |
| PATCH  | `/settings/notifications` | ⬜ |
| POST   | `/settings/notifications/devices` | ⬜ |
| DELETE | `/settings/notifications/devices/{id}` | ⬜ |

---

## 推荐实现顺序

按依赖从浅到深：

1. **基础设施先行**：JWT 中间件 + 统一错误响应 + 分页 helper
2. **Auth + Users**（注册→登录→profile 读写） — 解锁所有需要登录态的接口
3. **Media** — 头像/封面/帖子图依赖
4. **Posts** + **Comments** + **互动** — Social Feed 核心
5. **Connections** — Profile 关注 + Dashboard 请求
6. **Communities** — Feed 中的社区 Variant 依赖
7. **Notifications** — 异步事件触发器（点赞/评论/关注产生）
8. **Discovery + Search**
9. **Messages** + WebSocket
10. **Dashboard** — 聚合层，最后
11. **Settings** — 横切配置

---

## 备注

- Proto 已为所有 11 个域生成 gRPC + Connect 代码（`pkg/proto/<domain>/v1/`），handler 可以直接复用 `*Request` / `*Response` 类型避免重复定义 DTO。
- 优先实现 REST handler；同一份 Service 实现可后续接入 Connect/gRPC server，而无需改业务逻辑。
- 详见 [api.md](./api.md) 的 §15「屏幕 → 接口映射」决定每个屏幕的 MVP 闭环顺序。
