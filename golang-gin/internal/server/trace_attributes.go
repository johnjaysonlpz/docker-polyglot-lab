package server

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func RequestIDSpanAttrMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rid := c.GetString(requestIDContextKey); rid != "" {
			span := trace.SpanFromContext(c.Request.Context())
			if span.SpanContext().IsValid() {
				span.SetAttributes(attribute.String("request_id", rid))
			}
		}

		c.Next()
	}
}
