package server

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

type PanicError struct {
	Value any
}

func (e PanicError) Error() string {
	return fmt.Sprintf("%v", e.Value)
}

func GinRecoveryWithSlog() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()

				path := c.FullPath()
				if path == "" {
					path = c.Request.URL.Path
				}

				_ = c.Error(PanicError{Value: rec}).SetType(gin.ErrorTypePrivate)

				LoggerFrom(c).Error("panic_recovered",
					"panic", rec,
					"stack", string(stack),
					"status", http.StatusInternalServerError,
					"method", c.Request.Method,
					"path", path,
					"raw_path", c.Request.URL.Path,
				)

				WriteError(c, http.StatusInternalServerError, "internal_server_error", "internal server error")
			}
		}()

		c.Next()
	}
}
