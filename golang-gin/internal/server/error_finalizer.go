package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorFinalizer() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Written() {
			return
		}
		if len(c.Errors) == 0 {
			return
		}

		last := c.Errors.Last()
		if last == nil || last.Err == nil {
			return
		}

		fallbackStatus := c.Writer.Status()
		if fallbackStatus < 400 {
			fallbackStatus = http.StatusInternalServerError
		}

		status, code, msg := ResolveErrorResponse(last.Err, fallbackStatus)
		WriteError(c, status, code, msg)
	}
}
