package bootstrap

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-authgate/authgate/internal/config"
)

// validateAllConfiguration validates all configuration settings
func validateAllConfiguration(cfg *config.Config) {
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}
	if err := validateAuthConfig(cfg); err != nil {
		log.Fatalf("Invalid authentication configuration: %v", err)
	}
}

// validateAuthConfig checks that required config is present for selected auth mode
func validateAuthConfig(cfg *config.Config) error {
	switch cfg.AuthMode {
	case config.AuthModeHTTPAPI:
		if cfg.HTTPAPIURL == "" {
			return errors.New("HTTP_API_URL is required when AUTH_MODE=http_api")
		}
	case config.AuthModeLocal:
		// No additional validation needed
	default:
		return fmt.Errorf("invalid AUTH_MODE: %s (must be: local, http_api)", cfg.AuthMode)
	}
	return nil
}
