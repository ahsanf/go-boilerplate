# CLAUDE.md — Go Boilerplate Guidelines

## Stack

| Layer       | Library                              |
|-------------|--------------------------------------|
| HTTP        | `github.com/gofiber/fiber/v2`        |
| Logging     | `go.uber.org/zap`                    |
| Database    | `go.mongodb.org/mongo-driver/v2`     |
| Messaging   | `cloud.google.com/go/pubsub`         |
| Validation  | `github.com/go-playground/validator/v10` |
| JWT         | `github.com/golang-jwt/jwt/v4`       |
| Config      | `github.com/joho/godotenv`           |
| Swagger     | `github.com/gofiber/swagger` + `github.com/swaggo/swag` |

---

## Project Layout

```
app.go                          ← Fiber bootstrap, DI wiring, graceful shutdown
utils/
  mongodb.go                    ← ConnectDB(), global MongoClient
  zap.go                        ← InitLogger(), global Logger (*zap.Logger)
  pubsub.go                     ← InitPubSub(), global PubSubClient
  error_handler.go              ← GlobalErrorHandler (Fiber error handler)
  pagination.go                 ← CalculatePagination()
  token.go                      ← JWT sign/validate helpers
modules/
  common/
    entity.go                   ← Filter, Response, ListResponse, Pagination, StandardError
    validator.go                ← XValidator (wraps go-playground/validator)
    middleware.go               ← AuthTokenMiddleware, HybridTokenMiddleware
  {domain}/
    {subdomain}/
      entity.go                 ← Mongo structs, Request/Response types
      repository.go             ← Interface + impl, raw Mongo queries
      service.go                ← Interface + impl, business logic
      handler.go                ← Fiber routes, Swagger annotations
      subscriber.go             ← (optional) PubSub subscription handler
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
- If a query joins collections (e.g. lookup), add a separate `VFoo` view struct with the extra joined fields. The view struct reads from a Mongo view (`v_foos`), the write operations target the real collection (`foos`).

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
- For bulk writes use `mongo.BulkWrite` with `WriteModel` slice.
- Collection name constants live at the top of repository.go:
  ```go
  const collectionName = "foos"
  ```

---

### service.go

```go
type FooService interface {
    GetAll(ctx context.Context, filter common.Filter) ([]FooResponse, error)
    GetByID(ctx context.Context, id string) (FooResponse, error)
    Create(ctx context.Context, req *FooRequest) (FooResponse, error)
    Update(ctx context.Context, id string, req *FooRequest) (FooResponse, error)
    Delete(ctx context.Context, id string) error
}

type FooServiceImpl struct {
    repo FooRepository
}

func NewFooService(repo FooRepository) FooService {
    return &FooServiceImpl{repo: repo}
}
```

- Service constructors accept **interfaces**, not concrete types. This keeps modules decoupled.
- Mapping from domain struct to response happens in a private `mapToResponse(f Foo) FooResponse` method on the service.
- Business logic (stock calculations, status transitions, cross-module calls) belongs here, not in the repository.
- When injecting dependencies from another module, accept that module's **service interface** — never its repository.

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
- Return errors with `fiber.NewError(statusCode, err.Error())` — GlobalErrorHandler formats them.
- Use standard Fiber JSON response shape:
  ```go
  return c.JSON(fiber.Map{"message": "...", "data": data})
  return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "...", "data": data})
  ```
- All handlers must have Swagger `// @Summary`, `// @Tags`, `// @Param`, `// @Success`, `// @Failure`, `// @Security ApiKeyAuth`, `// @Router` annotations.

---

### subscriber.go (PubSub pattern)

```go
type FooSubscriber struct {
    client  *pubsub.Client
    service FooService
    logger  *zap.Logger
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
- Use `zap.Logger` for structured logging — never `log.Println` in subscriber code.
- Start subscribers in goroutines inside `app.go`, stopped via context cancellation on shutdown.

---

## Logging with Zap

Initialize a global logger in `utils/zap.go`:

```go
var Logger *zap.Logger

func InitLogger() {
    cfg := zap.NewProductionConfig()
    cfg.EncoderConfig.TimeKey = "timestamp"
    cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    Logger, _ = cfg.Build()
}
```

Usage rules:
- Pass `*zap.Logger` into constructors that need it (subscribers, background workers). Do **not** use the global in library code.
- Use structured fields: `zap.String("key", val)`, `zap.Error(err)`, `zap.Int("count", n)`.
- In Fiber handlers use `fiber/v2/log` for request-scoped logs; use `utils.Logger` for application-level events.
- Call `Logger.Sync()` during graceful shutdown.

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

type Response struct {
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
}

type ListResponse struct {
    Message    string      `json:"message"`
    Data       interface{} `json:"data"`
    Pagination Pagination  `json:"pagination"`
}

type Pagination struct {
    Page       int   `json:"page"`
    Limit      int   `json:"limit"`
    TotalItems int64 `json:"total_items"`
    TotalPages int64 `json:"total_pages"`
}

type StandardError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

### validator.go

`XValidator` wraps `go-playground/validator`. Register JSON field names via `RegisterTagNameFunc` so validation errors report JSON keys, not Go struct field names. Call `ValidateAndReturnError` in handlers before calling the service.

### middleware.go

- `AuthTokenMiddleware` — validates custom JWT, sets `user_id`, `email` in `c.Locals`.
- `GlobalAuthMiddleware` / `HybridTokenMiddleware` — Firebase ID token first, falls back to RS256 service-account JWT; sets `c.Locals("auth_user", common.AuthUser{...})`.
- All protected route groups pass a middleware: `app.Group("/resource", common.AuthTokenMiddleware)`.

---

## app.go — Bootstrap Pattern

```go
func main() {
    godotenv.Load()
    utils.InitLogger()
    defer utils.Logger.Sync()

    utils.ConnectDB()
    utils.InitPubSub(context.Background())
    defer utils.PubSubClient.Close()

    db := utils.MongoClient.Database(os.Getenv("DB_NAME"))
    app := fiber.New(fiber.Config{ErrorHandler: utils.GlobalErrorHandler})
    app.Use(cors.New())
    app.Use(logger.New())

    // Wire modules
    fooRepo := foo.NewFooRepository(db)
    fooSvc := foo.NewFooService(fooRepo)
    foo.NewFooHandler(app, fooSvc)

    // Start PubSub subscribers
    fooSub := foo.NewFooSubscriber(utils.PubSubClient, fooSvc, utils.Logger)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go fooSub.Listen(ctx, os.Getenv("FOO_SUBSCRIPTION_ID"))

    // Graceful shutdown
    go func() {
        port := os.Getenv("PORT")
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

## MongoDB Rules

- Use `go.mongodb.org/mongo-driver/v2` — **v2**, not v1. Import path is `go.mongodb.org/mongo-driver/v2/...`.
- `bson.ObjectID`, `bson.NewObjectID()`, `bson.ObjectIDFromHex()` — all from v2.
- Use MongoDB **views** (`v_collection`) for queries that require `$lookup` (joins). Views are read-only.
- Pagination: `SetSkip((page-1)*limit)` + `SetLimit(limit)`. Always do a separate `CountDocuments` call for total.
- Sorting: default `sort_by=updated_at`, `sort_type=desc`. Map `"asc"→1`, `"desc"→-1`.
- TLS via X.509 certificate: loaded from `MONGO_CREDENTIALS` env path.

---

## PubSub Rules

- Initialize client once at startup in `utils/pubsub.go`; store in `utils.PubSubClient`.
- Project ID from `PUBSUB_PROJECT_ID` env var.
- **Publishers** live in the service layer — call `topic.Publish(ctx, &pubsub.Message{Data: payload})`.
- **Subscribers** live in `subscriber.go` per module — started as goroutines in `app.go`.
- Always handle context cancellation in `sub.Receive` — it stops when ctx is cancelled (graceful shutdown).
- Payload structs use `json` tags; marshal/unmarshal with `encoding/json`.
- Log message ID (`msg.ID`) in error logs to aid debugging.

---

## Error Handling

- In handlers: `return fiber.NewError(fiber.StatusXxx, err.Error())` — GlobalErrorHandler catches it.
- In services/repos: return raw `error` — no wrapping needed unless adding context.
- Global error response shape (from `utils.GlobalErrorHandler`):
  ```json
  { "status": "error", "message": "...", "code": 400 }
  ```
- Validation errors return `400` with field-level detail from `XValidator`.
- `fiber.ErrNotFound` for missing documents, `fiber.ErrUnauthorized` for auth failures.

---

## Environment Variables

```
PORT=3000
MONGO_URI=mongodb+srv://...
MONGO_CREDENTIALS=/path/to/cert.pem
DB_NAME=mydb
SECRET_KEY=jwt-secret
PUBSUB_PROJECT_ID=gcp-project-id
GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json
```

---

## Code Style

- All constructors follow `New{Type}(deps...) Interface` — return interface, not concrete type.
- No global state except the four singletons: `MongoClient`, `Logger`, `PubSubClient`, `FirebaseApp`.
- No logic in `main()` beyond wiring and lifecycle management.
- `snake_case` for JSON and BSON tags; `PascalCase` for exported Go identifiers.
- Do not use `log.Println` / `fmt.Println` in business code — use `zap.Logger`.
- All exported functions must be reachable through their module's interface.

---

## Swagger

Generate docs with:
```bash
swag init
```

Every handler method needs these annotations before the function:
```go
// GetAll godoc
// @Summary ...
// @Description ...
// @Tags domain-subdomain
// @Accept json
// @Produce json
// @Param q query string false "Search term"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Limit" default(10)
// @Success 200 {object} common.ListResponse{data=[]FooResponse}
// @Failure 401 {object} common.StandardError
// @Failure 500 {object} common.StandardError
// @Security ApiKeyAuth
// @Router /foos [get]
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
