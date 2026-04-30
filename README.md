# ohome

UCHome / "Connect" social hub backend. Built on top of [butterfly.orx.me/core](https://butterfly.orx.me/core), with API contracts managed by **protobuf + buf**.

See [`api.md`](./api.md) for the full REST API specification derived from the design.

## Quick Start

```bash
make tidy        # install Go deps
make proto       # generate Go / gRPC / Connect from .proto files
make run         # start the service on :8080
```

Verify:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8080/api/v1/ping
```

## Layout

```
cmd/service/             application entrypoint (butterfly core app)
internal/config/         ohome-specific config (Redis/S3 handled by butterfly core)
internal/dao/            persistence layer (ent ORM, Postgres)
internal/dao/ent/        ent typed client (regenerate with `make ent`)
internal/dao/ent/schema/ ent schema source files
internal/http/           gin route registration
internal/<domain>/       per-domain handlers / services (auth, users, ...)
proto/                   protobuf source ("v1" packages, one per domain)
pkg/proto/               generated Go code (do not edit)
buf.yaml / buf.gen.yaml  buf module + codegen config
config.yaml              local file-based config
Makefile                 build / run / proto targets
Dockerfile               multi-stage container build
```

## Proto Domains

Each domain lives at `proto/<domain>/v1/` and generates into `pkg/proto/<domain>/v1/`:

| Domain | Service |
|---|---|
| `auth` | `AuthService` — register / login / refresh / password |
| `users` | `UserService` — profile, avatar, photos, friends |
| `connections` | `ConnectionService` — follow / connection requests |
| `posts` | `PostService` — feed, posts, likes, comments |
| `media` | `MediaService` — uploads, presign |
| `communities` | `CommunityService` — list / join / posts |
| `discovery` | `DiscoveryService` — highlights / suggestions / search |
| `notifications` | `NotificationService` |
| `messages` | `MessageService` |
| `dashboard` | `DashboardService` |
| `settings` | `SettingsService` |
| `common` | shared types (pagination, error) |

## Common Commands

```bash
make proto          # buf generate (Go + gRPC + Connect)
make proto-lint     # buf lint
make ent            # regenerate ent client from internal/dao/ent/schema
make build          # binary -> bin/ohome
make docker-build   # docker image
```

## Persistence

四套存储各司其职，详见 [task.md](./task.md) 的「存储分层决策」表。

- **Postgres + ent** (`internal/dao/dao.go`, `internal/dao/ent/`) — 主关系数据：users / posts / comments / follows / communities / bookmarks / blocks / privacy / sessions。底层 `*sql.DB` 由 butterfly `store/sqldb` 提供（v0.0.0-20260430+ 起内置 pgx/v5 Postgres 支持），ent client 在 `dao.Init()` 里基于它构建。
- **Mongo** (`internal/dao/mongo.go`，butterfly `store/mongo`) — `notifications` / `conversations` / `messages` / `devices`。形状灵活、写多读少、TTL 索引自动清理。
- **S3** (`internal/dao/media.go`，butterfly `store/s3`) — 二进制媒体（头像 / 封面 / 帖子图视频 / 相册 / 私信附件）。Postgres 只保存 `media_id` 和 metadata。
- **Redis** (butterfly `store/redis`) — 热点计数、feed 时间线缓存、限流、Idempotency-Key。

Postgres schema 迁移在启动时自动执行 (`ent.Client.Schema.Create()`)。生产环境切换到版本化迁移：`ent migrate` 或 atlas。
