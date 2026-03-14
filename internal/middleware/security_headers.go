package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders returns a middleware that sets HTTP security headers
// to protect against common web vulnerabilities. HSTS is only applied
// when useHSTS is true (i.e. when BaseURL uses HTTPS), so local HTTP
// development is unaffected.
func SecurityHeaders(useHSTS bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()

		// Prevent MIME-sniffing (e.g. treating uploads as executable scripts)
		h.Set("X-Content-Type-Options", "nosniff")

		// Deny framing to prevent clickjacking on login/consent pages
		h.Set("X-Frame-Options", "DENY")

		// Restrict resource origins — only allow same-origin by default
		h.Set(
			"Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'",
		)

		// Disable referrer to avoid leaking OAuth parameters
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features that an OAuth server doesn't need
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Force HTTPS only when the server is actually served over TLS
		if useHSTS {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}
