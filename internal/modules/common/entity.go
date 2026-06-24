package common

type Filter struct {
	Page     int    `json:"page"`
	Limit    int    `json:"limit"`
	Search   string `json:"search"`
	SortBy   string `json:"sortBy"`
	SortType string `json:"sortType"` // "asc" | "desc"
}

type Response struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ListResponse struct {
	Message    string      `json:"message"`
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"totalItems"`
	TotalPages int64 `json:"totalPages"`
}

type StandardError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// AuthUser is stored in c.Locals("auth_user") after GlobalAuthMiddleware.
type AuthUser struct {
	Email      string
	Role       string
	UserId     string
	ModuleId   string
	WorkunitId string
	Token      string // the verified JWT (without the custom 4th segment)
}

type JwtPayload struct {
	UserId     string
	Role       string
	ModuleId   string
	WorkunitId string
}

type DBName string
type CollectionName string
type HttpMethod string


const (
	YOUR_DB_NAME DBName = "your_db_name"
)

const (
	YOUR_COLLECTION_NAME CollectionName = "your_collection_name"
)

const (
	METHOD_GET    HttpMethod = "GET"
	METHOD_POST   HttpMethod = "POST"
	METHOD_PUT    HttpMethod = "PUT"
	METHOD_DELETE HttpMethod = "DELETE"
	METHOD_PATCH  HttpMethod = "PATCH"
)


