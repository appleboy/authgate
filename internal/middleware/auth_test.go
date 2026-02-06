package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Setup session middleware
	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test_session", store))

	return r
}

func TestSessionIdleTimeout_Disabled(t *testing.T) {
	r := setupTestRouter()

	// Add idle timeout middleware with 0 (disabled)
	r.Use(SessionIdleTimeout(0))

	r.GET("/test", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(SessionUserID, "user123")
		session.Set(SessionLastActivity, time.Now().Unix()-3600) // 1 hour ago
		_ = session.Save()
		c.String(http.StatusOK, "OK")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	// Should not redirect even though last activity was long ago (idle timeout disabled)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionIdleTimeout_ExceededTimeout(t *testing.T) {
	r := setupTestRouter()

	// Add idle timeout middleware (30 seconds)
	r.Use(SessionIdleTimeout(30))

	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Should not reach here")
	})

	// First request: set up an expired session
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)

	// Create session with user and expired last activity
	r2 := setupTestRouter()
	r2.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(SessionUserID, "user123")
		session.Set(SessionLastActivity, time.Now().Unix()-60) // 60 seconds ago
		_ = session.Save()
		c.Next()
	})
	r2.Use(SessionIdleTimeout(30))
	r2.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Should not reach here")
	})

	r2.ServeHTTP(w1, req1)

	// Should redirect to login with timeout error
	assert.Equal(t, http.StatusFound, w1.Code)
	location := w1.Header().Get("Location")
	assert.Contains(t, location, "/login")
	assert.Contains(t, location, "error=session_timeout")
}

func TestSessionIdleTimeout_UpdatesLastActivity(t *testing.T) {
	r := setupTestRouter()

	oldTimestamp := time.Now().Unix() - 10 // 10 seconds ago

	// Add idle timeout middleware (30 seconds)
	r.Use(SessionIdleTimeout(30))

	r.GET("/test", func(c *gin.Context) {
		session := sessions.Default(c)

		// Get updated last activity
		lastActivity := session.Get(SessionLastActivity)
		if lastActivity != nil {
			lastActivityAfter := lastActivity.(int64)
			// Last activity should be updated to current time
			assert.Greater(t, lastActivityAfter, oldTimestamp)
		}

		c.String(http.StatusOK, "OK")
	})

	// First request: set up session with old last activity
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)

	r2 := setupTestRouter()
	r2.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(SessionUserID, "user123")
		session.Set(SessionLastActivity, oldTimestamp)
		_ = session.Save()
		c.Next()
	})
	r2.Use(SessionIdleTimeout(30))
	r2.GET("/test", func(c *gin.Context) {
		session := sessions.Default(c)
		lastActivity := session.Get(SessionLastActivity)
		assert.NotNil(t, lastActivity)
		lastActivityAfter := lastActivity.(int64)
		assert.Greater(t, lastActivityAfter, oldTimestamp)
		c.String(http.StatusOK, "OK")
	})

	r2.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
}

func TestSessionIdleTimeout_NoSessionSkipped(t *testing.T) {
	r := setupTestRouter()

	// Add idle timeout middleware
	r.Use(SessionIdleTimeout(30))

	handlerCalled := false
	r.GET("/test", func(c *gin.Context) {
		handlerCalled = true
		c.String(http.StatusOK, "OK")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	// Should proceed normally (no session to check)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, handlerCalled)
}

func TestSessionIdleTimeout_WithinTimeout(t *testing.T) {
	r := setupTestRouter()

	// Set up session with recent activity (within timeout)
	r.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set(SessionUserID, "user123")
		session.Set(SessionLastActivity, time.Now().Unix()-10) // 10 seconds ago
		_ = session.Save()
		c.Next()
	})

	// Add idle timeout middleware (30 seconds)
	r.Use(SessionIdleTimeout(30))

	handlerCalled := false
	r.GET("/test", func(c *gin.Context) {
		handlerCalled = true
		c.String(http.StatusOK, "OK")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	// Should not redirect (within timeout)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, handlerCalled)
}
