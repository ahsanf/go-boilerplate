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
	Role       []string
	UserId     string
	PlatformId string
	WorkunitId string
	Token      string // the verified JWT (without the custom 4th segment)
}
