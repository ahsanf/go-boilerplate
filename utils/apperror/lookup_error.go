package apperror

type LookupErrorDetail struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

// LookupError maps a field name to its error detail.
type LookupError map[string]LookupErrorDetail

// LookupErrors is the list attached to an AppError.
type LookupErrors []LookupError

// NewLookupError creates a single field error entry.
func NewLookupError(field, title, message string) LookupError {
	return LookupError{
		field: {Field: field, Title: title, Message: message},
	}
}

// NewDuplicateError creates a lookup error for a duplicate value.
func NewDuplicateError(field, value string) LookupError {
	return NewLookupError(
		field,
		"Error: Duplicate "+field,
		field+" '"+value+"' already exists",
	)
}

// NewInvalidError creates a lookup error for an invalid value.
func NewInvalidError(field, reason string) LookupError {
	if reason == "" {
		reason = "Data " + field + " tidak sesuai dengan pilihan yang tersedia"
	}
	return NewLookupError(field, "Error: Invalid "+field, reason)
}

// NewRequiredError creates a lookup error for a missing required field.
func NewRequiredError(field string) LookupError {
	return NewLookupError(
		field,
		"Error: "+field+" is required",
		"Field "+field+" is required and cannot be empty",
	)
}

// MultiLookupErrors builds LookupErrors from a slice of field descriptors.
func MultiLookupErrors(errs []struct{ Field, Title, Message string }) LookupErrors {
	out := make(LookupErrors, 0, len(errs))
	for _, e := range errs {
		out = append(out, NewLookupError(e.Field, e.Title, e.Message))
	}
	return out
}
