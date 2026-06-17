package apperror

// HTTPError is the builder returned by New(). Call a status-code method on it.
//
//	apperror.New("token expired").Unauthorized()
//	apperror.New("").NotFound()
type HTTPError struct {
	msg          string
	lookupErrors LookupErrors
}

// New starts an HTTPError builder with an optional custom message.
func New(msg string, lookups ...LookupErrors) *HTTPError {
	h := &HTTPError{msg: msg}
	if len(lookups) > 0 {
		h.lookupErrors = lookups[0]
	}
	return h
}

func (h *HTTPError) build(status int, code, defaultMsg, errType string) *AppError {
	msg := h.msg
	if msg == "" {
		msg = defaultMsg
	}
	return &AppError{
		Type:         errType,
		Code:         code,
		StatusCode:   status,
		Message:      msg,
		LookupErrors: h.lookupErrors,
	}
}

// ── 4xx ──────────────────────────────────────────────────────────────────────

func (h *HTTPError) BadRequest() *AppError {
	return h.build(400, "BAD_REQUEST", "Bad request", ErrTypeNetwork)
}
func (h *HTTPError) Unauthorized() *AppError {
	return h.build(401, "UNAUTHORIZED", "Unauthorized", ErrTypeNetwork)
}
func (h *HTTPError) Forbidden() *AppError {
	return h.build(403, "FORBIDDEN", "Forbidden", ErrTypeNetwork)
}
func (h *HTTPError) NotFound() *AppError {
	return h.build(404, "NOT_FOUND", "Not found", ErrTypeNetwork)
}
func (h *HTTPError) Conflict() *AppError {
	return h.build(409, "CONFLICT", "Conflict", ErrTypeNetwork)
}
func (h *HTTPError) UnprocessableEntity() *AppError {
	return h.build(422, "UNPROCESSABLE_ENTITY", "Unprocessable entity", ErrTypeInvalid)
}
func (h *HTTPError) TooManyRequests() *AppError {
	return h.build(429, "TOO_MANY_REQUESTS", "Too many requests", ErrTypeNetwork)
}

// ── 5xx ──────────────────────────────────────────────────────────────────────

func (h *HTTPError) InternalServerError() *AppError {
	return h.build(500, "INTERNAL_SERVER_ERROR", "Internal server error", ErrTypeNetwork)
}
func (h *HTTPError) NotImplemented() *AppError {
	return h.build(501, "NOT_IMPLEMENTED", "Not implemented", ErrTypeNetwork)
}
func (h *HTTPError) BadGateway() *AppError {
	return h.build(502, "BAD_GATEWAY", "Bad gateway", ErrTypeNetwork)
}
func (h *HTTPError) ServiceUnavailable() *AppError {
	return h.build(503, "SERVICE_UNAVAILABLE", "Service unavailable", ErrTypeNetwork)
}

// ── App-specific helpers ──────────────────────────────────────────────────────

// AppErr creates a domain-specific error with ErrTypeApp.
func AppErr(status int, code, msg string) *AppError {
	return &AppError{
		Type:       ErrTypeApp,
		Code:       code,
		StatusCode: status,
		Message:    msg,
	}
}

// ValidationFailed is a convenience for 400 VALIDATION_FAILED with lookup errors.
func ValidationFailed(msg string, lookups LookupErrors) *AppError {
	return (&HTTPError{msg: msg, lookupErrors: lookups}).
		build(400, "VALIDATION_FAILED", "Validation failed", ErrTypeApp)
}

// Duplicate is a convenience for 400 DUPLICATE.
func Duplicate(msg string, lookups ...LookupErrors) *AppError {
	h := &HTTPError{msg: msg}
	if len(lookups) > 0 {
		h.lookupErrors = lookups[0]
	}
	return h.build(400, "DUPLICATE", "Duplicate data", ErrTypeApp)
}
