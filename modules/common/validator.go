package common

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type XValidator struct {
	v *validator.Validate
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func NewXValidator() *XValidator {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &XValidator{v: v}
}

func (x *XValidator) ValidateAndReturnError(s interface{}) []ValidationError {
	errs := x.v.Struct(s)
	if errs == nil {
		return nil
	}
	var out []ValidationError
	for _, err := range errs.(validator.ValidationErrors) {
		out = append(out, ValidationError{
			Field:   err.Field(),
			Message: err.Tag(),
		})
	}
	return out
}
