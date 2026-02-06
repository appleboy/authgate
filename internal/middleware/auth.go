package middleware

import (
	"net/http"
	"time"

	"github.com/appleboy/authgate/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	SessionUserID       = "user_id"
	SessionLastActivity = "last_activity"
)

// RequireAuth is a middleware that requires the user to be logged in
func RequireAuth(userService *services.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get(SessionUserID)

		if userID == nil {
			// Redirect to login with return URL
			redirectURL := c.Request.URL.String()
			c.Redirect(http.StatusFound, "/login?redirect="+redirectURL)
			c.Abort()
			return
		}

		c.Set("user_id", userID)

		// Load user object for audit logging and other purposes
		user, err := userService.GetUserByID(userID.(string))
		if err == nil {
			c.Set("user", user)
		}

		c.Next()
	}
}

// SessionIdleTimeout checks if the session has been idle for too long
// and clears it if necessary. Set idleTimeoutSeconds to 0 to disable.
func SessionIdleTimeout(idleTimeoutSeconds int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if idle timeout is disabled
		if idleTimeoutSeconds <= 0 {
			c.Next()
			return
		}

		session := sessions.Default(c)
		userID := session.Get(SessionUserID)

		// Only check idle timeout for authenticated sessions
		if userID != nil {
			lastActivity := session.Get(SessionLastActivity)

			if lastActivity != nil {
				lastActivityTime, ok := lastActivity.(int64)
				if ok {
					idleSeconds := time.Now().Unix() - lastActivityTime
					if idleSeconds > int64(idleTimeoutSeconds) {
						// Session idle timeout exceeded, clear session
						session.Clear()
						_ = session.Save()

						// Redirect to login with timeout message
						redirectURL := c.Request.URL.String()
						c.Redirect(
							http.StatusFound,
							"/login?redirect="+redirectURL+"&error=session_timeout",
						)
						c.Abort()
						return
					}
				}
			}

			// Update last activity timestamp
			session.Set(SessionLastActivity, time.Now().Unix())
			_ = session.Save()
		}

		c.Next()
	}
}

// RequireAdmin is a middleware that requires the user to have admin role
// This middleware should be used after RequireAuth
func RequireAdmin(userService *services.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.HTML(http.StatusForbidden, "error.html", gin.H{
				"error": "Unauthorized access",
			})
			c.Abort()
			return
		}

		user, err := userService.GetUserByID(userID.(string))
		if err != nil {
			c.HTML(http.StatusForbidden, "error.html", gin.H{
				"error": "User not found",
			})
			c.Abort()
			return
		}

		if !user.IsAdmin() {
			c.HTML(http.StatusForbidden, "error.html", gin.H{
				"error": "Admin access required",
			})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}
