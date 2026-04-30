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
internal/config/         service config struct (yaml-bound)
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
make proto          # buf generate
make proto-lint     # buf lint
make build          # binary -> bin/ohome
make docker-build   # docker image
```
