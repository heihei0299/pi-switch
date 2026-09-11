package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error envelopes are split by surface (system-contract 2.8). They live here so
// each shape has one owner instead of a copy per handler: management answers a
// bare message, inference the OpenAI error object.

// managementError is the management (/api) envelope.
func managementError(message string) gin.H {
	return gin.H{"error": message}
}

// inferenceError is the inference (/v1) envelope.
func inferenceError(message, errType string) gin.H {
	return gin.H{"error": gin.H{"message": message, "type": errType}}
}

func managementUnauthorized() gin.H { return managementError("Unauthorized") }

func inferenceUnauthorized() gin.H {
	return inferenceError("Unauthorized", "invalid_request_error")
}

// managementRecovery answers a recovered panic with the management envelope.
func managementRecovery(c *gin.Context, _ any) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, managementError("internal server error"))
}

// inferenceRecovery answers a recovered panic with the inference envelope, so a
// /v1 client never sees gin's empty 500 body.
func inferenceRecovery(c *gin.Context, _ any) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, inferenceError("internal server error", "internal_error"))
}

// inferenceNoRoute answers unmatched /v1 routes (including a wrong method, which
// gin treats as NoRoute) with the inference envelope.
func inferenceNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, inferenceError("not found: "+c.Request.URL.Path, "invalid_request_error"))
}
