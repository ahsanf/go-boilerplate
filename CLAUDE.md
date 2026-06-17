# CLAUDE.md — Go Boilerplate Guidelines

## Security Rules

> **NEVER share, paste, or attach any `.env` file, secret key, connection string, credential file, or token value when prompting an AI assistant.**
>
> - Use `.env.example` with placeholder values as the only reference for environment variable names.
> - If a real value is accidentally included in a prompt, rotate the secret immediately.
> - AI assistants must not ask for, suggest logging, or echo back any environment variable values.

---

## Stack

| Layer       | Library                                  |
|-------------|------------------------------------------|
| HTTP        | `github.com/gofiber/fiber/v2`            |
| Logging     | `go.uber.org/zap` (wrapped as `AppLogger`) |
| Database    | `go.mongodb.org/mongo-driver/v2`         |
| Auth        | `firebase.google.com/go/v4` + `github.com/golang-jwt/jwt/v4` |
| RBAC        | `github.com/casbin/casbin/v2`            |
| Cache       | `github.com/redis/go-redis/v9`           |
| Messaging   | `cloud.google.com/go/pubsub`             |
| Validation  | `github.com/go-playground/validator/v10` |
| Config      | `github.com/joho/godotenv`               |
| Swagger     | `github.com/gofiber/swagger` + `github.com/swaggo/swag` |

---

## Project Layout

```
app.go                          ← Fiber bootstrap, DI wiring, graceful shutdown
utils/
  config.go                     ← AppConfig struct, LoadConfig(), global Cfg
  logger.go                     ← InitLogger(), AppLogger, LogInfo/Warn/Error/Fatal/Sync helpers
  mongodb.go                    ← ConnectDB(), global MongoClient
  redis.go                      ← InitRedis(), CacheSet/Get/Del helpers
  firebase.go                   ← InitFirebase(), global FirebaseAuth
  pubsub.go                     ← InitPubSub(), global PubSubClient
  casbin.go                     ← SetPolicyLoader(), CheckPermissions()
  token.go                      ← SignToken(), ValidateToken() — HS256 JWT
  error_handler.go              ← GlobalErrorHandler (Fiber error handler)
  apperror/
    error.go                    ← AppError type
    http_error.go               ← apperror.New("msg").Unauthorized() builder
    lookup_error.go             ← LookupError field-level validation helpers
modules/
  common/
    entity.go                   ← Filter, Response, ListResponse, Pagination, AuthUser
    validator.go                ← XValidator (wraps go-playground/validator)
    middleware.go               ← GlobalAuthMiddleware, AuthTokenMiddleware, HybridTokenMiddleware
    permission.go               ← PermissionMiddleware (Casbin RBAC)
    logging.go                  ← LoggingMiddleware, GetTraceID()
    pagination.go               ← CalculatePagination()
  {domain}/
    entity.go                   ← Mongo struct, Request/Response types
    repository.go               ← Interface + impl, raw Mongo queries
    service.go                  ← Interface + impl, business logic
    handler.go                  ← Fiber routes, Swagger annotations
    subscriber.go               ← (optional) PubSub subscription handler
cmd/
  generate/
    main.go                     ← CRUD generator CLI (--domain, --file, --out)
    templates/                  ← entity / repository / service / handler .tmpl files
docs/                           ← Swagger output (generated — do not edit)
```

Every domain module is **self-contained**: entity → repository → service → handler. No circular imports across modules (pass interfaces, not concrete types).

---

## Module Conventions

### entity.go

```go
// Mongo document (bson tags, ObjectID for _id)
type Foo struct {
    ID        bson.ObjectID `bson:"_id"`
    Name      string        `bson:"name"`
    CreatedAt time.Time     `bson:"created_at"`
    UpdatedAt time.Time     `bson:"updated_at"`
}

// JSON request (validate tags on every field)
type FooRequest struct {
    Name string `validate:"required,min=3,max=100" json:"name"`
}

// JSON response (string ID, RFC3339 timestamps)
type FooResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    CreatedAt string `json:"created_at"`
    UpdatedAt string `json:"updated_at"`
}
```

- Use `bson.ObjectID` for `_id` — never `primitive.ObjectID` from v1.
- Response IDs are always `string` (`oid.Hex()`). Never expose raw `bson.ObjectID` in JSON.
- Timestamps in responses use `time.RFC3339`.
- If a query joins collections, add a separate `VFoo` view struct. The view struct reads from a Mongo view (`v_foos`); write operations target the real collection (`foos`).

---

### repository.go

```go
type FooRepository interface {
    GetAll(ctx context.Context, filter common.Filter) ([]Foo, error)
    Count(ctx context.Context, filter common.Filter) (int64, error)
    GetByID(ctx context.Context, id string) (Foo, error)
    Create(ctx context.Context, foo *Foo) error
    Update(ctx context.Context, foo *Foo) error
    Delete(ctx context.Context, id string) error
}

type FooRepositoryImpl struct {
    db *mongo.Database
}

func NewFooRepository(db *mongo.Database) FooRepository {
    return &FooRepositoryImpl{db: db}
}
```

- Always define an interface; pass the interface to service constructors.
- All methods receive `ctx context.Context` as first arg — propagate it to every Mongo call.
- Use `bson.ObjectIDFromHex(id)` and return an error immediately if it fails.
- Return empty slice (not nil) when no documents found: `if results == nil { return []Foo{}, nil }`.
- Collection name constant at the top of repository.go:
  ```go
  const collectionName = "foos"
  ```
- Search filter: only set `$or` when you have actual conditions — an empty `bson.A{}` causes a MongoDB error.
- Sorting: pull `bson.D{{Key: field, Value: dir}}` into a named variable before passing to `SetSort`.

---

### service.go

```go
type FooService interface {
    GetAll(ctx context.Context, filter common.Filter) ([]FooResponse, common.Pagination, error)
    GetByID(ctx context.Context, id string) (FooResponse, error)
    Create(ctx context.Context, req *FooRequest) (FooResponse, error)
    Update(ctx context.Context, id string, req *FooRequest) (FooResponse, error)
    Delete(ctx context.Context, id string) error
}

type FooServiceImpl struct {
    repo   FooRepository
    logger *utils.AppLogger
}

func NewFooService(repo FooRepository, logger *utils.AppLogger) FooService {
    return &FooServiceImpl{repo: repo, logger: logger}
}
```

- Service constructors accept **interfaces**, not concrete types.
- Inject `*utils.AppLogger` (not `*zap.Logger`) for structured logging in services.
- Mapping from domain struct to response happens in a private `mapToResponse(f Foo) FooResponse` method.
- Business logic belongs here, not in the repository.
- When injecting from another module, accept that module's **service interface** — never its repository.
- Wire with `utils.Log`: `foo.NewFooService(fooRepo, utils.Log)`.

---

### handler.go

```go
func NewFooHandler(app *fiber.App, service FooService) {
    handler := &FooHandlerImpl{
        service:   service,
        validator: common.NewXValidator(),
    }

    api := app.Group("/foos", common.AuthTokenMiddleware)
    api.Get("", handler.GetAll)
    api.Get("/:id", handler.GetByID)
    api.Post("", handler.Create)
    api.Put("/:id", handler.Update)
    api.Delete("/:id", handler.Delete)
}
```

- Parse body with `c.BodyParser(&req)`, then validate with `h.validator.ValidateAndReturnError(&req)`.
- Return errors with `apperror.New("msg").NotFound()` (or the relevant builder method) — `GlobalErrorHandler` formats the structured envelope automatically.
- Standard Fiber JSON response shape:
  ```go
  return c.JSON(fiber.Map{"message": "...", "data": data})
  return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "...", "data": data})
  ```
- All handlers must have full Swagger annotations: `@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Security ApiKeyAuth`, `@Router`.

---

### subscriber.go (PubSub pattern)

```go
type FooSubscriber struct {
    client  *pubsub.Client
    service FooService
    logger  *zap.Logger   // raw *zap.Logger for library compat; use utils.Logger
}

func NewFooSubscriber(client *pubsub.Client, service FooService, logger *zap.Logger) *FooSubscriber {
    return &FooSubscriber{client: client, service: service, logger: logger}
}

func (s *FooSubscriber) Listen(ctx context.Context, subscriptionID string) error {
    sub := s.client.Subscription(subscriptionID)
    return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
        var payload FooEvent
        if err := json.Unmarshal(msg.Data, &payload); err != nil {
            s.logger.Error("failed to unmarshal pubsub message", zap.Error(err))
            msg.Nack()
            return
        }
        if err := s.service.HandleEvent(ctx, &payload); err != nil {
            s.logger.Error("failed to handle event", zap.Error(err), zap.String("msg_id", msg.ID))
            msg.Nack()
            return
        }
        msg.Ack()
    })
}
```

- **Always Ack or Nack** — never silently drop a message.
- Nack on unmarshal failure and handler error so the message is redelivered (ensure idempotency in the service layer).
- Subscribers accept `*zap.Logger` (use `utils.Logger`) because PubSub callbacks are goroutines without a Fiber context.
- Start subscribers in goroutines inside `app.go`, stopped via context cancellation on shutdown.

---

## Logging (`utils/logger.go`)

`AppLogger` wraps zap and exposes the same API as the TypeScript logger:

```go
// Defaults message to "call {funcName} from {className}"
utils.LogInfo("GetAll", "ProductService", traceId)
utils.LogWarn("Create", "ProductService", traceId)
utils.LogError("Update", "ProductService", traceId, err.Error())
utils.LogFatal("main", "app", "", "cannot connect to database")
```

- **Console** — colored, human-readable, all levels ≥ Debug.
- **File** — JSON, info+ only, written to `logs/YYYY-M-D_api.log` (daily, created at startup).
- `utils.Logger` (`*zap.Logger`) is still set for libraries that expect raw zap (e.g. PubSub subscribers).
- Call `utils.LogSync()` during graceful shutdown.

### LoggingMiddleware

Place `common.LoggingMiddleware` **before** route groups in `app.go`. It:

1. Generates a UUID v4 trace ID per request.
2. Stores it in `c.Locals("trace_id")`.
3. Logs URL, query params, route params, and request body via `utils.LogInfo`.

Retrieve the trace ID in handlers or services:

```go
traceId := common.GetTraceID(c)
utils.LogInfo("Create", "ProductService", traceId, "creating product")
```

---

## Common Package (`modules/common`)

### entity.go — shared types

```go
type Filter struct {
    Page     int    `json:"page"`
    Limit    int    `json:"limit"`
    Search   string `json:"search"`
    SortBy   string `json:"sort_by"`
    SortType string `json:"sort_type"`  // "asc" | "desc"
}

type AuthUser struct {
    Email      string
    ObaRole    []string
    StaffObaId string
    WorkunitId string
    Platform   string
    Token      string
}

type Pagination struct {
    Page       int   `json:"page"`
    Limit      int   `json:"limit"`
    TotalItems int64 `json:"total_items"`
    TotalPages int64 `json:"total_pages"`
}
```

### validator.go

`XValidator` wraps `go-playground/validator`. Registers JSON field names via `RegisterTagNameFunc` so errors report JSON keys. Call `ValidateAndReturnError` in handlers before the service.

### middleware.go

- `GlobalAuthMiddleware` — Firebase ID token first, falls back to RS256 service-account JWT. Parses optional 4th JWT segment for platform metadata. Sets `c.Locals("auth_user", AuthUser{...})`.
- `AuthTokenMiddleware` — HS256 internal JWT. Sets `c.Locals("user_id", ...)` and `c.Locals("email", ...)`.
- `HybridTokenMiddleware` — alias for `GlobalAuthMiddleware`.

### permission.go

`PermissionMiddleware` — Casbin RBAC check placed after `GlobalAuthMiddleware`:
1. `STATIC_TOKEN` bypass.
2. `SKIP_PERMISSION=true` bypass.
3. `SUPERADMIN` role bypasses policy.
4. Checks `(role, path, method)` against the policy loaded via `utils.SetPolicyLoader`.
5. Returns `403` if denied.

### logging.go

`LoggingMiddleware` + `GetTraceID(c *fiber.Ctx) string` — see Logging section above.

### pagination.go

`CalculatePagination()` lives here (not in `utils`) to avoid import cycles.

---

## app.go — Bootstrap Pattern

```go
func main() {
    godotenv.Load()
    utils.LoadConfig()
    utils.InitLogger()
    defer utils.LogSync()

    utils.ConnectDB()
    utils.InitRedis()

    ctx := context.Background()

    if utils.Cfg.PubSubProjectID != "" {
        utils.InitPubSub(ctx)
        defer utils.PubSubClient.Close()
    }

    if utils.Cfg.GoogleCredPath != "" || utils.Cfg.ServiceAccount != "" {
        utils.InitFirebase(ctx)
    }

    db := utils.MongoClient.Database(utils.Cfg.DBName)

    // Register Casbin policy loader (load CSV from DB or Redis)
    // utils.SetPolicyLoader(func(ctx context.Context) (string, error) {
    //     return myAdapter.ToCasbinCSV(ctx)
    // })

    app := fiber.New(fiber.Config{ErrorHandler: utils.GlobalErrorHandler})
    app.Use(cors.New())
    app.Use(common.LoggingMiddleware)  // UUID trace ID + request log
    app.Get("/swagger/*", swagger.HandlerDefault)

    // Wire modules
    fooRepo := foo.NewFooRepository(db)
    fooSvc  := foo.NewFooService(fooRepo, utils.Log)
    foo.NewFooHandler(app, fooSvc)

    // Start PubSub subscribers
    go foo.NewFooSubscriber(utils.PubSubClient, fooSvc, utils.Logger).
        Listen(ctx, os.Getenv("FOO_SUBSCRIPTION_ID"))

    // Graceful shutdown
    go func() {
        port := utils.Cfg.Port
        if port == "" { port = "3000" }
        if err := app.Listen(":" + port); err != nil {
            log.Panicf("server error: %v", err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit

    shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutCancel()
    app.ShutdownWithContext(shutCtx)
    utils.MongoClient.Disconnect(shutCtx)
}
```

---

## CRUD Generator

Generate a full module from an entity struct file:

```bash
go run cmd/generate/main.go --domain=Product --file=product.go
go run cmd/generate/main.go --domain=Product --file=product.go --out=modules/catalog/product
go run cmd/generate/main.go --domain=Product   # scaffold entity.go to fill in
```

The generator:
1. Parses field names and types from the entity struct using `go/ast`.
2. **Always generates all four files** (`entity.go`, `repository.go`, `service.go`, `handler.go`) from templates.
3. When `--file` is given, extracts fields then removes the source file.
4. Uses `[[` `]]` template delimiters so Go composite literals (`bson.D{{...}}`) pass through unmodified.
5. System fields (`_id`, `created_at`, `updated_at`) are excluded from `Request`/`Response` structs.

Wire in `app.go`:
```go
productRepo := product.NewProductRepository(db)
productSvc  := product.NewProductService(productRepo, utils.Log)
product.NewProductHandler(app, productSvc)
```

---

## MongoDB Rules

- Use `go.mongodb.org/mongo-driver/v2` — **v2**, not v1. Import path is `go.mongodb.org/mongo-driver/v2/...`.
- `bson.ObjectID`, `bson.NewObjectID()`, `bson.ObjectIDFromHex()` — all from v2 bson package.
- Use MongoDB **views** (`v_collection`) for queries that require `$lookup`. Views are read-only.
- Pagination: `SetSkip((page-1)*limit)` + `SetLimit(limit)`. Always do a separate `CountDocuments` call for total.
- Sorting: default `sort_by=updated_at`, `sort_type=desc`. Map `"asc"→1`, `"desc"→-1`.

---

## PubSub Rules

- Initialize client once at startup in `utils/pubsub.go`; only if `PUBSUB_PROJECT_ID` is set.
- **Publishers** live in the service layer — call `topic.Publish(ctx, &pubsub.Message{Data: payload})`.
- **Subscribers** live in `subscriber.go` per module — started as goroutines in `app.go`.
- Always handle context cancellation in `sub.Receive` — it stops when ctx is cancelled (graceful shutdown).
- Log `msg.ID` in error logs to aid debugging.

---

## Error Handling

Use `apperror.New` for all application errors:

```go
return apperror.New("user not found").NotFound()           // 404
return apperror.New("email already exists").Conflict()     // 409
return apperror.New("").InternalServerError()              // 500
return apperror.New("invalid or expired token").Unauthorized() // 401

// With field-level detail
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

---

## Environment Variables

Reference `.env.example` only — never paste real values into prompts.

```
PORT=3000
MONGO_URI=
MONGO_CREDENTIALS=
DB_NAME=
SECRET_KEY=
PUBSUB_PROJECT_ID=
GOOGLE_APPLICATION_CREDENTIALS=
GOOGLE_SERVICE_ACCOUNT=
REDIS_ENABLE=false
REDIS_URL=
STATIC_TOKEN=
SKIP_PERMISSION=false
AUTH_API_URL=
```

---

## Code Style

- All constructors follow `New{Type}(deps...) Interface` — return interface, not concrete type.
- No global state except the five singletons: `MongoClient`, `Logger`, `Log`, `PubSubClient`, `FirebaseAuth`.
- No logic in `main()` beyond wiring and lifecycle management.
- `snake_case` for JSON and BSON tags; `PascalCase` for exported Go identifiers.
- Never use `log.Println` / `fmt.Println` in business code — use `utils.LogInfo/LogError/...`.
- Services receive `*utils.AppLogger`; PubSub subscribers receive `*zap.Logger` (use `utils.Logger`).
- All exported functions must be reachable through their module's interface.

---

## Swagger

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init
```

Every handler method needs these annotations:

```go
// GetAll godoc
// @Summary      List foos
// @Tags         foo
// @Accept       json
// @Produce      json
// @Param        page   query int    false "Page"   default(1)
// @Param        limit  query int    false "Limit"  default(10)
// @Success      200    {object} common.ListResponse{data=[]FooResponse}
// @Failure      401    {object} common.StandardError
// @Failure      500    {object} common.StandardError
// @Security     ApiKeyAuth
// @Router       /foos [get]
```

Swagger is served at `/swagger/*` via `github.com/gofiber/swagger`.

---

## Docker

```dockerfile
FROM golang:1.25 AS builder
COPY . /app
WORKDIR /app
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine
COPY --from=builder /app/server /server
CMD ["/server"]
```
