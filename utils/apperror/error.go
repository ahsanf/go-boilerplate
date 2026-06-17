package apperror

const (
	ErrTypeApp     = "APP"
	ErrTypeNetwork = "NETWORK"
	ErrTypeInvalid = "INVALID DATA"
)

// AppError is the structured application error used throughout the codebase.
type AppError struct {
	Type         string       `json:"type"`
	Code         string       `json:"code"`
	StatusCode   int          `json:"statusCode"`
	Message      string       `json:"message"`
	LookupErrors LookupErrors `json:"lookupErrors,omitempty"`
	Details      any          `json:"details,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

// WithLookupErrors attaches field-level errors and returns the same AppError for chaining.
func (e *AppError) WithLookupErrors(l LookupErrors) *AppError {
	e.LookupErrors = l
	return e
}

// WithDetails attaches arbitrary detail data.
func (e *AppError) WithDetails(d any) *AppError {
	e.Details = d
	return e
}
