# Go Boilerplate

Production-ready REST API boilerplate built with Go, following the [Standard Go Project Layout](https://github.com/golang-standards/project-layout).

## Stack

| Layer      | Library                                                       |
|------------|---------------------------------------------------------------|
| HTTP       | `github.com/gofiber/fiber/v2`                                 |
| Logging    | `go.uber.org/zap` (wrapped as `AppLogger`)                    |
| Database   | `go.mongodb.org/mongo-driver/v2`                              |
| Auth       | `firebase.google.com/go/v4` + `github.com/golang-jwt/jwt/v4` |
| RBAC       | `github.com/casbin/casbin/v2`                                 |
| Cache      | `github.com/redis/go-redis/v9`                                |
| Messaging  | `cloud.google.com/go/pubsub`                                  |
| Validation | `github.com/go-playground/validator/v10`                      |
| Config     | `github.com/joho/godotenv`                                    |
| Swagger    | `github.com/gofiber/swagger` + `github.com/swaggo/swag`      |

## Project Layout

```
cmd/
  server/
    main.go                     ← Fiber bootstrap, DI wiring, graceful shutdown
  generate/
    main.go                     ← CRUD generator CLI (--domain, --file, --out)
    main_test.go                ← Unit tests for generator helpers and field parser
    templates/                  ← entity / repository / service / handler .tmpl files
configs/
  config.go                     ← AppConfig struct, LoadConfig(), global Cfg
  mongodb.go                    ← ConnectDB(), global MongoClient
  redis.go                      ← InitRedis(), CacheSet/Get/Del helpers
  firebase.go                   ← InitFirebase(), global FirebaseAuth
  pubsub.go                     ← InitPubSub(), global PubSubClient
internal/
  utils/
    logger.go                   ← InitLogger(), AppLogger, LogInfo/Warn/Error/Fatal/Sync
    casbin.go                   ← SetPolicyLoader(), CheckPermissions()
    token.go                    ← SignToken(), ValidateToken() — HS256 JWT
    error_handler.go            ← GlobalErrorHandler (Fiber error handler)
    apperror/
      error.go                  ← AppError type
      http_error.go             ← apperror.New("msg").Unauthorized() builder
      lookup_error.go           ← LookupError field-level validation helpers
  modules/
    common/
      entity.go                 ← Filter, Response, ListResponse, Pagination, AuthUser
      validator.go              ← XValidator (wraps go-playground/validator)
      middleware.go             ← GlobalAuthMiddleware, AuthTokenMiddleware, HybridTokenMiddleware
      permission.go             ← PermissionMiddleware (Casbin RBAC)
      logging.go                ← LoggingMiddleware, GetTraceID()
      pagination.go             ← CalculatePagination()
    {domain}/
      entity.go                 ← Mongo struct, Request/Response types
      repository.go             ← Interface + impl, raw Mongo queries
      service.go                ← Interface + impl, business logic
      handler.go                ← Fiber routes, Swagger annotations
      subscriber.go             ← (optional) PubSub subscription handler
deployments/
  Dockerfile                    ← Multi-stage Docker build
docs/                           ← Swagger output (generated — do not edit)
scripts/                        ← Build, migration, tooling scripts
```

Every domain module is self-contained: entity → repository → service → handler. No circular imports across modules.

## Getting Started

### 1. Clone and install dependencies

```bash
git clone <repo-url>
cd go-boilerplate
go mod tidy
```

### 2. Configure environment

```bash
cp configs/.env.example .env
# Edit .env with your values — never commit real credentials
```

| Variable                        | Description                                        |
|---------------------------------|----------------------------------------------------|
| `PORT`                          | HTTP port (default `3000`)                         |
| `MONGO_URI`                     | MongoDB connection string                          |
| `DB_NAME`                       | Database name                                      |
| `SECRET_KEY`                    | HS256 JWT signing secret                           |
| `GOOGLE_APPLICATION_CREDENTIALS`| Path to Google service account JSON                |
| `GOOGLE_SERVICE_ACCOUNT`        | Used for RS256 token fallback                      |
| `REDIS_ENABLE`                  | `true` to enable Redis cache                       |
| `REDIS_URL`                     | Redis connection URL                               |
| `STATIC_TOKEN`                  | Optional bypass token (dev/testing only)           |
| `SKIP_PERMISSION`               | `true` to skip Casbin checks (dev only)            |
| `AUTH_API_URL`                  | Base URL for refresh-token validation              |
| `PUBSUB_PROJECT_ID`             | GCP project ID (leave empty to disable PubSub)     |

### 3. Generate Swagger docs

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/server/main.go --output docs
```

Docs are served at `http://localhost:3000/swagger/`.

### 4. Run

```bash
go run cmd/server/main.go
```

### 5. Docker

```bash
docker build -f deployments/Dockerfile -t go-boilerplate .
docker run -p 3000:3000 --env-file .env go-boilerplate
```

## CRUD Generator

Generate a complete module from an entity struct file:

```bash
# Parse fields from an existing struct, generate all four files, auto-wire in main.go
go run cmd/generate/main.go --domain=Product --file=product.go

# Scaffold an empty entity.go to fill in manually
go run cmd/generate/main.go --domain=Product

# Custom output directory
go run cmd/generate/main.go --domain=Product --file=product.go --out=internal/modules/catalog/product
```

The generator:
1. Parses field names and types from your entity struct using `go/ast`.
2. Always generates all four files — `entity.go`, `repository.go`, `service.go`, `handler.go`.
3. Derives camelCase JSON tags from bson snake_case tags automatically.
4. Removes the source file after extraction.
5. Auto-wires the new module into `cmd/server/main.go`.

### Entity file format

Place the file anywhere (e.g. `cmd/generate/product.go`) and point `--file` at it:

```go
package main

import (
    "time"
    "go.mongodb.org/mongo-driver/v2/bson"
)

type Product struct {
    ID        bson.ObjectID `bson:"_id"`
    Name      string        `bson:"name"`
    Price     float64       `bson:"price"`
    IsActive  bool          `bson:"is_active"`
    CreatedAt time.Time     `bson:"created_at"`
    UpdatedAt time.Time     `bson:"updated_at"`
}
```

Fields `_id`, `created_at`, `updated_at` are treated as system fields and handled automatically by the templates. All other fields are included in `ProductRequest` and `ProductResponse`.

Generated `ProductResponse` uses camelCase JSON keys:

```json
{
  "id": "...",
  "name": "...",
  "price": 0,
  "isActive": true,
  "createdAt": "2026-06-18T00:00:00Z",
  "updatedAt": "2026-06-18T00:00:00Z"
}
```

### Run generator tests

```bash
go test ./cmd/generate/...
```

## Logging

`AppLogger` wraps zap with a structured, dual-output logger:

- **Console** — colored, human-readable, all levels ≥ Debug
- **File** — JSON, info+, written to `logs/YYYY-M-D_api.log` daily

```go
utils.LogInfo("GetAll", "ProductService", traceId)
utils.LogWarn("Create", "ProductService", traceId, "item already exists")
utils.LogError("Update", "ProductService", traceId, err.Error())
utils.LogFatal("main", "app", "", "cannot connect to database")
```

Every request gets a UUID trace ID from `LoggingMiddleware`. Retrieve it in any handler or service:

```go
traceId := common.GetTraceID(c)
utils.LogInfo("Create", "ProductHandler", traceId)
```

Sample log output for a `GET /platforms` request:

```
INFO  [GET] URL: "/platforms" Queries: map[] Params: map[] Body: map[]  {"function":"LoggingMiddleware","class":"LoggingMiddleware","trace_id":"..."}
INFO  call GetAll from PlatformHandler                                   {"function":"GetAll","class":"PlatformHandler","trace_id":"..."}
```

## Auth Middleware

### `common.GlobalAuthMiddleware`

Firebase + RS256 service-account JWT hybrid:

1. Requires `Authorization: Bearer <token>`.
2. `STATIC_TOKEN` bypasses all checks.
3. Parses optional 4th JWT segment carrying `platform`, `currentRole`, `staffId`, `workunitId`.
4. Verifies via Firebase Admin SDK; falls back to RS256 service-account JWT.
5. Sets `c.Locals("auth_user", common.AuthUser{...})`.

### `common.AuthTokenMiddleware`

HS256 internal JWT — sets `c.Locals("user_id")` and `c.Locals("email")`.

### `common.PermissionMiddleware`

Casbin RBAC check placed after `GlobalAuthMiddleware`:

1. `STATIC_TOKEN` bypass.
2. `SKIP_PERMISSION=true` bypass.
3. `SUPERADMIN` role bypasses policy.
4. Checks `(role, path, method)` against the loaded Casbin policy.
5. Returns `403` if not allowed.

Register a policy loader at startup in `cmd/server/main.go`:

```go
utils.SetPolicyLoader(func(ctx context.Context) (string, error) {
    // load CSV from MongoDB or Redis cache
    // format: role,/path,METHOD  (one per line)
    return myRoleAdapter.ToCasbinCSV(ctx)
})
```

Protect a route group:

```go
api := app.Group("/v1", common.GlobalAuthMiddleware, common.PermissionMiddleware)
```

## Error Handling

Use `apperror.New` for structured errors:

```go
return apperror.New("user not found").NotFound()           // 404
return apperror.New("email already exists").Conflict()     // 409
return apperror.New("").InternalServerError()              // 500
return apperror.New("invalid token").Unauthorized()        // 401

// Field-level validation errors
return apperror.ValidationFailed("validation failed", apperror.LookupErrors{
    apperror.NewRequiredError("email"),
    apperror.NewDuplicateError("username", "john"),
})
```

`GlobalErrorHandler` catches all `*apperror.AppError` values and returns:

```json
{
  "success": false,
  "error": {
    "type": "APP",
    "code": "NOT_FOUND",
    "statusCode": 404,
    "message": "user not found"
  }
}
```

## Module Conventions

- `entity.go` — Mongo document struct (bson tags), Request struct (`validate` + camelCase json tags), Response struct (string ID, RFC3339 timestamps).
- `repository.go` — interface + `*RepositoryImpl`; all methods take `ctx context.Context` first; uses `bson.ObjectIDFromHex`.
- `service.go` — interface + `*ServiceImpl`; private `mapToResponse` converts entity → response; injects `*utils.AppLogger`.
- `handler.go` — `New{Domain}Handler(app, service)` registers routes; each method logs with `utils.LogInfo`; every handler has full Swagger annotations.
- Constructors always return the **interface**, not the concrete type.
- No logic in `main()` beyond wiring and lifecycle management.
