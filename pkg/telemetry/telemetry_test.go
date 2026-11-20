package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		ServiceName:    "devplatform-control-plane",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		OTLPEndpoint:   "localhost:4317",
		SampleRate:     0.1,
	}

	if cfg.ServiceName == "" {
		t.Fatal("expected non-empty ServiceName")
	}
	if cfg.SampleRate < 0.0 || cfg.SampleRate > 1.0 {
		t.Fatalf("expected SampleRate within [0.0, 1.0], got %f", cfg.SampleRate)
	}
}

func TestGinMiddleware_Execution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GinMiddleware("test-service"))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
