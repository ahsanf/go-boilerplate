# Go Boilerplate

Production-ready REST API boilerplate built with Go, following a clean hexagonal-style module layout.

## Stack

| Layer | Library |
|---|---|
| HTTP | `github.com/gofiber/fiber/v2` |
| Logging | `go.uber.org/zap` |
| Database | `go.mongodb.org/mongo-driver/v2` |
| Auth | `firebase.google.com/go/v4` + `github.com/golang-jwt/jwt/v4` |
| RBAC | `github.com/casbin/casbin/v2` |
| Cache | `github.com/redis/go-redis/v9` |
| Messaging | `cloud.google.com/go/pubsub` |
| Validation | `github.com/go-playground/validator/v10` |
| Config | `github.com/joho/godotenv` |
| Swagger | `github.com/gofiber/swagger` + `github.com/swaggo/swag` |

## Project Layout

```
app.go                          ← Fiber bootstrap, DI wiring, graceful shutdown
utils/
  config.go                     ← AppConfig struct, LoadConfig()
  mongodb.go                    ← ConnectDB(), MongoClient
  zap.go                        ← InitLogger(), Logger
  redis.go                      ← InitRedis(), CacheSet/Get/Del
  firebase.go                   ← InitFirebase(), FirebaseAuth
  pubsub.go                     ← InitPubSub(), PubSubClient
  token.go                      ← SignToken(), ValidateToken() — HS256 JWT
  casbin.go                     ← SetPolicyLoader(), CheckPermissions()
  error_handler.go              ← GlobalErrorHandler (Fiber error handler)
  apperror/
    error.go                    ← AppError type
    http_error.go               ← apperror.New("msg").Unauthorized() builder
    lookup_error.go             ← LookupError field-level validation helpers
modules/
  common/
    entity.go                   ← Filter, Response, ListResponse, Pagination, AuthUser
    validator.go                ← XValidator
    middleware.go               ← GlobalAuthMiddleware, AuthTokenMiddleware
    permission.go               ← PermissionMiddleware (Casbin RBAC)
    pagination.go               ← CalculatePagination()
  {domain}/
    entity.go                   ← Mongo struct, Request/Response types
    repository.go               ← Interface + impl, raw Mongo queries
    service.go                  ← Interface + impl, business logic
    handler.go                  ← Fiber routes, Swagger annotations
    subscriber.go               ← (optional) PubSub subscription handler
cmd/
  generate/
    main.go                     ← CRUD generator CLI
    templates/                  ← entity / repository / service / handler templates
docs/                           ← Swagger output (generated — do not edit)
```

## Getting Started

### 1. Clone and install dependencies

```bash
git clone <repo-url>
cd go-boilerplate
go mod tidy
```

### 2. Configure environment

```bash
cp .env.example .env
# Edit .env with your values
```

Key variables:

| Variable | Description |
|---|---|
| `PORT` | HTTP port (default `3000`) |
| `MONGO_URI` | MongoDB connection string |
| `DB_NAME` | Database name |
| `SECRET_KEY` | HS256 JWT signing secret |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to Google service account JSON |
| `GOOGLE_SERVICE_ACCOUNT` | Same file — used for RS256 token fallback |
| `REDIS_ENABLE` | `true` to enable Redis cache |
| `REDIS_URL` | Redis connection URL |
| `STATIC_TOKEN` | Optional bypass token (dev/testing) |
| `SKIP_PERMISSION` | `true` to skip Casbin checks (dev only) |
| `AUTH_API_URL` | Base URL for refresh-token validation |
| `PUBSUB_PROJECT_ID` | GCP project ID (optional) |

### 3. Generate Swagger docs

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init
```

Docs are served at `http://localhost:3000/swagger/`.

### 4. Run

```bash
go run app.go
```

### 5. Docker

```bash
docker build -t go-boilerplate .
docker run -p 3000:3000 --env-file .env go-boilerplate
```

## CRUD Generator

Generate a full module (entity + repository + service + handler) from an existing entity struct file:

```bash
go run cmd/generate/main.go --domain=Product --file=product.go
```

This moves `product.go` to `modules/product/entity.go` (rewriting the package declaration), parses the struct fields, and generates:

```
modules/product/
  entity.go       ← moved from --file, package rewritten, Request/Response added
  repository.go   ← CRUD interface + MongoDB implementation
  service.go      ← CRUD interface + implementation with mapToResponse
  handler.go      ← Fiber routes with full Swagger annotations
```

Without `--file`, a template `entity.go` is generated for you to fill in:

```bash
go run cmd/generate/main.go --domain=Product
```

Custom output directory:

```bash
go run cmd/generate/main.go --domain=Product --file=product.go --out=modules/catalog/product
```

### Entity file format

```go
package main

import (
    "time"
    "go.mongodb.org/mongo-driver/v2/bson"
)

type Product struct {
    ID          bson.ObjectID `bson:"_id"`
    Name        string        `bson:"name"`
    Price       float64       `bson:"price"`
    IsActive    bool          `bson:"is_active"`
    CreatedAt   time.Time     `bson:"created_at"`
    UpdatedAt   time.Time     `bson:"updated_at"`
}
```

Fields with bson tags `_id`, `created_at`, `updated_at` are treated as system fields and handled automatically. All other fields are included in `ProductRequest` and `ProductResponse`.

### Wire the generated module in `app.go`

```go
productRepo := product.NewProductRepository(db)
productSvc  := product.NewProductService(productRepo, utils.Logger)
product.NewProductHandler(app, productSvc)
```

## Auth Middleware

### `common.GlobalAuthMiddleware`

Mirrors the Firebase + service-account JWT hybrid flow:

1. Requires `Authorization: Bearer <token>`.
2. `STATIC_TOKEN` bypasses all checks.
3. Parses optional 4th JWT segment carrying `platform`, `currentRole`, `staffId`, `workunitId`.
4. Verifies via Firebase Admin SDK; falls back to RS256 service-account JWT.
5. Sets `c.Locals("auth_user", common.AuthUser{...})`.

### `common.PermissionMiddleware`

Casbin RBAC check placed after `GlobalAuthMiddleware`:

1. `STATIC_TOKEN` bypass.
2. `SKIP_PERMISSION=true` bypass.
3. `SUPERADMIN` role bypasses policy.
4. Checks `(role, path, method)` against the Casbin policy CSV.
5. Returns `403` if not allowed.

Register your policy loader once at startup in `app.go`:

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

Use `apperror.New` for structured errors that produce the standard envelope:

```go
// in a service or handler
return apperror.New("user not found").NotFound()
return apperror.New("email already exists").Conflict()
return apperror.New("").InternalServerError()

// with field-level lookup errors
return apperror.ValidationFailed("validation failed", apperror.LookupErrors{
    apperror.NewRequiredError("email"),
    apperror.NewDuplicateError("username", "john"),
})
```

All errors are caught by `GlobalErrorHandler` and returned as:

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

- `entity.go` — Mongo document struct, Request struct (`validate` tags), Response struct (string ID and timestamps).
- `repository.go` — interface + `*RepositoryImpl`; all methods take `ctx context.Context` first.
- `service.go` — interface + `*ServiceImpl`; private `mapToResponse` converts entity to response.
- `handler.go` — `New{Domain}Handler(app, service)` registers routes; every handler has full Swagger annotations.
- Constructors always return the **interface**, not the concrete type.
- No logic in `main()` beyond wiring and lifecycle.
