package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func WriteError(c *gin.Context, status int, code, msg string) {
	rid := c.GetString(requestIDContextKey)

	c.AbortWithStatusJSON(status, ErrorResponse{
		Error:     msg,
		Code:      code,
		RequestID: rid,
	})
}

func NotFoundHandler(c *gin.Context) {
	WriteError(c, http.StatusNotFound, "not_found", "not found")
}

func MethodNotAllowedHandler(c *gin.Context) {
	WriteError(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}
