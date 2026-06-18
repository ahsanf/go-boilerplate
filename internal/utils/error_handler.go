package utils

import (
	"go-boilerplate/internal/utils/apperror"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type errorBody struct {
	Type         string                `json:"type"`
	Code         string                `json:"code"`
	StatusCode   int                   `json:"statusCode"`
	Message      string                `json:"message"`
	LookupErrors apperror.LookupErrors `json:"lookupErrors,omitempty"`
	Details      any                   `json:"details,omitempty"`
}

type errorResponse struct {
	Success bool      `json:"success"`
	Error   errorBody `json:"error"`
}

func GlobalErrorHandler(c *fiber.Ctx, err error) error {
	Logger.Error("request error", zap.String("path", c.Path()), zap.Error(err))

	if appErr, ok := err.(*apperror.AppError); ok {
		return c.Status(appErr.StatusCode).JSON(errorResponse{
			Success: false,
			Error: errorBody{
				Type:         appErr.Type,
				Code:         appErr.Code,
				StatusCode:   appErr.StatusCode,
				Message:      appErr.Message,
				LookupErrors: appErr.LookupErrors,
				Details:      appErr.Details,
			},
		})
	}

	code := fiber.StatusInternalServerError
	msg := "internal server error"
	if fErr, ok := err.(*fiber.Error); ok {
		code = fErr.Code
		msg = fErr.Message
	}

	return c.Status(code).JSON(errorResponse{
		Success: false,
		Error: errorBody{
			Type:       "APP",
			Code:       "ERROR",
			StatusCode: code,
			Message:    msg,
		},
	})
}
