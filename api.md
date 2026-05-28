# UCHome API 设计

基于 Stitch 设计稿（UCHome Social Network / "Connect" 社交中心）整理的后端 API 清单。涵盖 Sign In / Sign Up / Dashboard / Social Feed / Discovery / User Profile / Settings 七个屏幕所需的全部接口。

- **Base URL**: `/api/v1`
- **认证方式**: `Authorization: Bearer <access_token>`（除注册/登录/刷新 token 外均需要）
- **数据格式**: `application/json`，时间统一使用 RFC3339 (UTC)
- **分页**: 基于游标，查询参数 `cursor`、`limit`（默认 20，最大 100）；响应包含 `next_cursor`
- **错误响应**: `{"code": "string", "message": "string", "details": object}`，HTTP 状态码遵循语义化（400/401/403/404/409/422/429/5xx）

---

## 1. 认证与会话 (Auth)

对应屏幕：**Sign In**、**Sign Up**

| Method | Path | 说明 |
|---|---|---|
| `POST` | `/auth/register` | 注册账号 |
| `POST` | `/auth/login` | 邮箱 + 密码登录 |
| `POST` | `/auth/logout` | 登出（撤销当前 token） |
| `POST` | `/auth/refresh` | 刷新 access_token |
| `POST` | `/auth/password/forgot` | 发送找回密码邮件 |
| `POST` | `/auth/password/reset` | 使用重置令牌设置新密码 |
| `POST` | `/auth/password/change` | 已登录用户修改密码 |
| `POST` | `/auth/email/verify-request` | 重发邮箱验证邮件 |
| `POST` | `/auth/email/verify` | 校验邮箱验证 token |
| `GET`  | `/auth/me` | 获取当前登录用户信息 |

**注册请求**（来自 Sign Up 页面字段）：

```json
POST /auth/register
{
  "full_name": "Jane Doe",
  "email": "jane@example.com",
  "username": "janedoe",
  "password": "********",
  "accept_terms": true
}
```

**登录请求**（Sign In 页面）：

```json
POST /auth/login
{
  "email": "you@example.com",
  "password": "********",
  "remember_me": true
}
```

**登录/刷新响应**：

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "user": { "id": "...", "username": "...", "display_name": "...", "avatar_url": "..." }
}
```

---

## 2. 用户与个人资料 (Users / Profile)

对应屏幕：**User Profile**、**Settings (Profile)**、Top Nav 头像菜单

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/users/me` | 当前用户完整资料 |
| `PATCH`  | `/users/me` | 更新自己的资料（display_name / bio / interests 等） |
| `DELETE` | `/users/me` | 注销账号 |
| `POST`   | `/users/me/avatar` | 上传/更换头像（multipart） |
| `POST`   | `/users/me/cover` | 上传/更换封面图 |
| `GET`    | `/users/{username}` | 查看他人公开主页 |
| `GET`    | `/users/{username}/stats` | 主页统计：followers / following / projects |
| `GET`    | `/users/{username}/posts` | 主页 Posts Tab |
| `GET`    | `/users/{username}/photos` | 主页 Photos Tab（Bento 网格） |
| `GET`    | `/users/{username}/friends` | 主页 Friends Tab / 右侧好友栏 |
| `GET`    | `/users/me/interests` | 兴趣标签列表 |
| `PUT`    | `/users/me/interests` | 覆盖式更新兴趣标签 |
| `POST`   | `/users/me/interests` | 添加单个兴趣标签 |
| `DELETE` | `/users/me/interests/{tag}` | 删除某个标签 |

**User 实体（公开字段）**：

```json
{
  "id": "u_123",
  "username": "alexsmith99",
  "display_name": "Alex Smith",
  "title": "Product Designer",
  "bio": "...",
  "avatar_url": "https://...",
  "cover_url": "https://...",
  "location": "San Francisco, CA",
  "interests": ["Web Design", "Photography"],
  "stats": { "followers": 1248, "following": 312, "projects": 24 },
  "is_following": false,
  "is_self": false,
  "created_at": "2025-01-12T08:30:00Z"
}
```

---

## 3. 关注 / 好友 / 连接 (Follows & Connections)

对应屏幕：**User Profile**（Follow/Unfollow、Friends 列表）、**Dashboard**（Connection Requests）、**Discovery**（Add Friend）

| Method | Path | 说明 |
|---|---|---|
| `POST`   | `/users/{username}/follow` | 关注 |
| `DELETE` | `/users/{username}/follow` | 取关 |
| `GET`    | `/users/{username}/followers` | 粉丝列表 |
| `GET`    | `/users/{username}/following` | 关注列表 |
| `GET`    | `/connections` | 我的好友列表（双向连接） |
| `DELETE` | `/connections/{user_id}` | 删除好友 |
| `GET`    | `/connections/requests` | 收到的连接请求（Dashboard 小部件） |
| `POST`   | `/connections/requests` | 发送连接请求 `{to_user_id}` |
| `POST`   | `/connections/requests/{id}/accept` | 接受请求 |
| `POST`   | `/connections/requests/{id}/reject` | 拒绝请求 |
| `POST`   | `/users/me/contacts/sync` | 通讯录同步以推荐好友 |

---

## 4. 动态 / 帖子 (Posts & Feed)

对应屏幕：**Social Feed**（Composer + Feed Cards）、**Dashboard**（Recent Activity）、**User Profile**（Posts Tab）

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/feed` | 主信息流（默认 Following，可选 `scope=for_you\|following\|community`） |
| `GET`    | `/posts/{id}` | 单条帖子详情 |
| `POST`   | `/posts` | 发帖 |
| `PATCH`  | `/posts/{id}` | 编辑帖子 |
| `DELETE` | `/posts/{id}` | 删除帖子 |
| `POST`   | `/posts/{id}/report` | 举报 |
| `POST`   | `/posts/{id}/hide` | 隐藏（"More" 菜单） |

**发帖请求**（Composer 支持 photo / video / event）：

```json
POST /posts
{
  "content": "Just shipped a new design...",
  "attachments": [
    { "type": "image", "media_id": "m_abc" }
  ],
  "hashtags": ["#design", "#minimalism"],
  "community_id": "c_design",      // 可选
  "event_id": "e_42",              // 可选（attach event）
  "visibility": "public"           // public | followers | private
}
```

**Post 响应实体**：

```json
{
  "id": "p_123",
  "author": { "id": "u_1", "name": "Marcus Thorne", "avatar_url": "...", "title": "..." },
  "community": { "id": "c_design", "name": "Design Community", "icon_url": "..." },
  "content": "...",
  "media": [{ "type": "image", "url": "...", "width": 1200, "height": 800 }],
  "hashtags": ["#design"],
  "created_at": "2026-04-30T10:00:00Z",
  "stats": { "likes": 124, "comments": 28, "shares": 6 },
  "viewer": { "liked": false, "bookmarked": false, "shared": false }
}
```

### 4.1 互动（点赞 / 收藏 / 分享）

| Method | Path | 说明 |
|---|---|---|
| `POST`   | `/posts/{id}/like` | 点赞 |
| `DELETE` | `/posts/{id}/like` | 取消点赞 |
| `GET`    | `/posts/{id}/likes` | 点赞用户列表 |
| `POST`   | `/posts/{id}/share` | 转发 / 分享 |
| `POST`   | `/posts/{id}/bookmark` | 收藏（侧边栏 Bookmarks） |
| `DELETE` | `/posts/{id}/bookmark` | 取消收藏 |
| `GET`    | `/users/me/bookmarks` | 我的收藏列表 |

### 4.2 评论 (Comments)

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/posts/{id}/comments` | 评论列表（支持 `parent_id` 过滤楼中楼） |
| `POST`   | `/posts/{id}/comments` | 发表评论 / 回复 |
| `PATCH`  | `/comments/{id}` | 编辑 |
| `DELETE` | `/comments/{id}` | 删除 |
| `POST`   | `/comments/{id}/like` | 评论点赞 |
| `DELETE` | `/comments/{id}/like` | 取消评论点赞 |

---

## 5. 媒体上传 (Media)

支持 Composer 的 photo / video，以及头像、封面、相册。

| Method | Path | 说明 |
|---|---|---|
| `POST` | `/media/uploads` | 直接上传 multipart，返回 `media_id`、`url` |
| `POST` | `/media/uploads/presign` | 申请预签名上传 URL（大文件直传 OSS） |
| `GET`  | `/media/{id}` | 查询媒体元信息 |

**上传响应**：

```json
{
  "media_id": "m_abc",
  "url": "https://cdn.../m_abc.jpg",
  "type": "image",
  "width": 1600, "height": 1200,
  "size_bytes": 234567
}
```

### 5.1 个人相册（Profile - Photos Tab）

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/users/{username}/albums` | 相册列表 |
| `GET`    | `/users/me/photos` | 我的全部照片 |
| `POST`   | `/users/me/photos` | 添加照片到相册（含 `media_id`、`title`、`alt_text`） |
| `DELETE` | `/photos/{id}` | 删除照片 |

---

## 6. 社区 (Communities)

对应屏幕：**Discovery**（Popular Communities）、**Social Feed**（Community Post Variant）、侧栏 "Communities"

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/communities` | 社区列表（支持 `q`、`category`、`sort=members\|trending`） |
| `GET`    | `/communities/{id}` | 社区详情 |
| `GET`    | `/communities/{id}/posts` | 社区帖子流 |
| `POST`   | `/communities/{id}/join` | 加入 |
| `DELETE` | `/communities/{id}/join` | 退出 |
| `GET`    | `/communities/{id}/members` | 成员列表 |
| `GET`    | `/users/me/communities` | 我加入的社区 |

**Community 实体**：

```json
{
  "id": "c_design_systems",
  "name": "Design Systems",
  "description": "...",
  "icon_url": "...",
  "cover_url": "...",
  "members_count": 12400,
  "is_member": false
}
```

---

## 7. 发现 (Discovery)

对应屏幕：**Discovery**（顶部 Bento 网格 + 推荐 + 热门社区）

| Method | Path | 说明 |
|---|---|---|
| `GET` | `/discover/highlights` | Trending Highlights（精选合集 + Trending Designs + Live Discussion） |
| `GET` | `/discover/trending/posts` | 热门帖子 |
| `GET` | `/discover/trending/topics` | 趋势话题 / 标签 |
| `GET` | `/discover/suggested-users` | 好友推荐（带头像、姓名、职位） |
| `GET` | `/discover/communities` | 推荐社区 |
| `GET` | `/discover/discussions` | 直播 / 实时讨论列表 |
| `GET` | `/discover/categories` | 分类入口（Events / Market / Articles / Jobs 来自 Dashboard Quick Links） |

### 7.1 全局搜索

| Method | Path | 说明 |
|---|---|---|
| `GET` | `/search` | 综合搜索 `?q=&type=all\|users\|posts\|communities\|tags` |
| `GET` | `/search/suggest` | 输入联想（Top Nav 搜索） |

---

## 8. 通知 (Notifications)

对应屏幕：所有页面顶栏的通知图标、移动端 Bottom Nav "Alerts"

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/notifications` | 通知列表（支持 `unread=true`、`type=` 过滤） |
| `GET`    | `/notifications/unread-count` | 未读小红点 |
| `POST`   | `/notifications/{id}/read` | 标记单条已读 |
| `POST`   | `/notifications/read-all` | 全部已读 |
| `DELETE` | `/notifications/{id}` | 删除 |

通知类型：`like` / `comment` / `mention` / `follow` / `connection_request` / `community_invite` / `post_share` / `system`。

---

## 9. 私信 / 聊天 (Messages)

对应屏幕：顶栏 chat 图标、Profile 中 Message Friend

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/messages/conversations` | 会话列表 |
| `POST`   | `/messages/conversations` | 新建私聊 / 群聊 `{participant_ids: []}` |
| `GET`    | `/messages/conversations/{id}` | 会话元信息 |
| `GET`    | `/messages/conversations/{id}/messages` | 历史消息（分页） |
| `POST`   | `/messages/conversations/{id}/messages` | 发送消息（文本 / 图片 / 文件） |
| `POST`   | `/messages/conversations/{id}/read` | 标记已读 |
| `DELETE` | `/messages/{id}` | 撤回消息 |

实时通道（可选）：`GET /ws` —— WebSocket，订阅消息、通知、在线状态。

---

## 10. 仪表盘 (Dashboard)

对应屏幕：**Dashboard**

| Method | Path | 说明 |
|---|---|---|
| `GET` | `/dashboard/summary` | 顶部欢迎语所需的当前用户简要 |
| `GET` | `/dashboard/metrics` | 四宫格指标 |
| `GET` | `/dashboard/activity` | Recent Activity（聚合自我和好友动态） |
| `GET` | `/dashboard/quick-links` | Events / Market / Articles / Jobs 入口配置（CMS 化） |

**Metrics 响应**：

```json
{
  "profile_views": { "value": 1248, "change_pct": 0.12, "period": "week" },
  "post_impressions": { "value": 45200, "change_abs": 5400, "period": "today" },
  "new_followers": { "value": 124, "since": "last_login" },
  "analytics_url": "/analytics"
}
```

---

## 11. 设置 (Settings)

对应屏幕：**Settings**（侧边菜单：Profile / Account / Privacy / Notifications）

### 11.1 Profile（已在 §2）

通过 `PATCH /users/me` 更新 display_name / username / bio / interests / avatar。

### 11.2 Account

| Method | Path | 说明 |
|---|---|---|
| `GET`    | `/settings/account` | 邮箱、用户名、登录方式、2FA 状态 |
| `PATCH`  | `/settings/account/email` | 修改邮箱（触发验证邮件） |
| `POST`   | `/settings/account/2fa/enable` | 开启 2FA |
| `POST`   | `/settings/account/2fa/disable` | 关闭 2FA |
| `GET`    | `/settings/account/sessions` | 登录历史 / 活跃会话 |
| `DELETE` | `/settings/account/sessions/{id}` | 注销某会话 |

### 11.3 Privacy

| Method | Path | 说明 |
|---|---|---|
| `GET`   | `/settings/privacy` | 当前隐私配置 |
| `PATCH` | `/settings/privacy` | 更新可见性、私信范围、数据共享开关 |
| `GET`   | `/settings/privacy/blocked` | 屏蔽列表 |
| `POST`  | `/users/{username}/block` | 屏蔽 |
| `DELETE`| `/users/{username}/block` | 解除屏蔽 |

```json
PATCH /settings/privacy
{
  "profile_visibility": "public",   // public | followers | private
  "allow_messages_from": "everyone", // everyone | followers | none
  "show_online_status": true,
  "data_sharing": false
}
```

### 11.4 Notifications

| Method | Path | 说明 |
|---|---|---|
| `GET`   | `/settings/notifications` | 偏好（按渠道 × 类型矩阵） |
| `PATCH` | `/settings/notifications` | 更新偏好 |
| `POST`  | `/settings/notifications/devices` | 注册推送 device token |
| `DELETE`| `/settings/notifications/devices/{id}` | 解绑设备 |

```json
PATCH /settings/notifications
{
  "channels": {
    "email":  { "likes": false, "comments": true, "follows": true, "messages": true },
    "push":   { "likes": true,  "comments": true, "follows": true, "messages": true },
    "in_app": { "likes": true,  "comments": true, "follows": true, "messages": true }
  }
}
```

---

## 12. 上传与外链支持

| Method | Path | 说明 |
|---|---|---|
| `POST` | `/uploads/avatar` | 头像（裁剪后） |
| `POST` | `/uploads/cover` | 封面图 |
| `GET`  | `/oembed?url=` | 链接预览（Composer 自动卡片） |

---

## 13. 实体速查 (Schema Summary)

| 实体 | 关键字段 |
|---|---|
| **User** | id, username, display_name, title, bio, avatar_url, cover_url, interests[], stats{followers, following, projects} |
| **Post** | id, author_id, community_id?, content, media[], hashtags[], visibility, created_at, stats{likes,comments,shares} |
| **Comment** | id, post_id, parent_id?, author_id, content, created_at, likes |
| **Photo** | id, owner_id, media_id, title, alt_text, album_id?, created_at |
| **Community** | id, name, description, icon_url, members_count, category |
| **Notification** | id, type, actor, target_post?, target_user?, created_at, read_at |
| **Conversation** | id, participants[], last_message, unread_count |
| **Message** | id, conversation_id, sender_id, content, attachments[], created_at, read_by[] |
| **ConnectionRequest** | id, from_user, to_user, status (pending/accepted/rejected), created_at |
| **DiscoveryHighlight** | id, kind (collection / trend / discussion), title, description, image_url, link, meta |

---

## 14. 通用约定

- **分页响应**：
  ```json
  { "items": [...], "next_cursor": "...", "has_more": true }
  ```
- **限流**：登录、注册、密码相关接口 IP+账号双维度限流；社交写操作 60 req/min。
- **幂等**：`POST /posts`、`POST /messages/...` 支持 `Idempotency-Key` Header。
- **国际化**：响应根据 `Accept-Language` 返回本地化错误消息。
- **WebSocket 事件**：`message.new`、`notification.new`、`post.like`、`presence.update`。

---

## 15. 屏幕 → 接口映射

| 屏幕 | 关键接口 |
|---|---|
| Sign In | `POST /auth/login`、`POST /auth/password/forgot` |
| Sign Up | `POST /auth/register`、`POST /auth/email/verify-request` |
| Dashboard | `GET /dashboard/summary` `metrics` `activity` `quick-links`、`GET /connections/requests` `POST .../accept\|reject` |
| Social Feed | `GET /feed`、`POST /posts`、`POST /media/uploads`、`POST /posts/{id}/like\|share`、`GET /posts/{id}/comments`、`POST /posts/{id}/comments` |
| Discovery | `GET /discover/highlights\|suggested-users\|communities\|discussions`、`POST /communities/{id}/join`、`POST /connections/requests`、`POST /users/me/contacts/sync`、`GET /search` |
| User Profile | `GET /users/{username}` `stats` `posts` `photos` `friends`、`POST /users/{username}/follow`、`POST /messages/conversations` |
| Settings | `PATCH /users/me`、`POST /users/me/avatar`、`GET\|PATCH /settings/account\|privacy\|notifications` |
