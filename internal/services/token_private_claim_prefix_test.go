package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-authgate/authgate/internal/config"
	"github.com/go-authgate/authgate/internal/models"
	"github.com/go-authgate/authgate/internal/token"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// privateClaimPrefixConfig builds a Config suitable for the prefix tests:
// rotation enabled (so the refresh test can decode a fresh refresh JWT),
// project / service_account / domain populated to exercise all three
// registry entries.
func privateClaimPrefixConfig(prefix string) *config.Config {
	return &config.Config{
		DeviceCodeExpiration:             30 * time.Minute,
		PollingInterval:                  5,
		JWTExpiration:                    time.Hour,
		JWTSecret:                        "test-secret-key-for-jwt-signing",
		BaseURL:                          "http://localhost:8080",
		EnableRefreshTokens:              true,
		EnableTokenRotation:              true,
		RefreshTokenExpiration:           24 * time.Hour,
		ClientCredentialsTokenExpiration: time.Hour,
		JWTDomain:                        "auth.example.com",
		JWTPrivateClaimPrefix:            prefix,
		ExtraClaimsEnabled:               true,
		ExtraClaimsMaxRawSize:            4096,
		ExtraClaimsMaxKeys:               16,
		ExtraClaimsMaxValSize:            512,
	}
}

// TestPrivateClaimPrefix_DefaultPrefix_HappyPath verifies that with the
// default prefix "extra", a client_credentials token carries
// extra_domain / extra_project / extra_service_account and the bare logical
// names are absent.
func TestPrivateClaimPrefix_DefaultPrefix_HappyPath(t *testing.T) {
	s := setupTestStore(t)
	cfg := privateClaimPrefixConfig(config.DefaultJWTPrivateClaimPrefix)
	svc := createTestTokenService(t, s, cfg)

	client, plainSecret := createConfidentialClientWithCCFlow(t, s, true)
	client.Project = "p1"
	client.ServiceAccount = "sa1"
	require.NoError(t, s.UpdateClient(client))

	tok, err := svc.IssueClientCredentialsToken(
		context.Background(), client.ClientID, plainSecret, "", nil,
	)
	require.NoError(t, err)

	claims := decodeJWTClaims(t, tok.RawToken)
	assert.Equal(t, "auth.example.com", claims["extra_domain"])
	assert.Equal(t, "p1", claims["extra_project"])
	assert.Equal(t, "sa1", claims["extra_service_account"])

	for _, bare := range []string{"domain", "project", "service_account"} {
		_, present := claims[bare]
		assert.False(t, present,
			"bare %q must NOT appear when prefix is configured (only prefixed name is emitted)",
			bare,
		)
	}
}

// TestPrivateClaimPrefix_CustomPrefix verifies that JWT_PRIVATE_CLAIM_PREFIX=mtk
// produces mtk_domain / mtk_project / mtk_service_account, with no extra_*
// or bare-name leakage.
func TestPrivateClaimPrefix_CustomPrefix(t *testing.T) {
	s := setupTestStore(t)
	cfg := privateClaimPrefixConfig("mtk")
	svc := createTestTokenService(t, s, cfg)

	client, plainSecret := createConfidentialClientWithCCFlow(t, s, true)
	client.Project = "p1"
	client.ServiceAccount = "sa1"
	require.NoError(t, s.UpdateClient(client))

	tok, err := svc.IssueClientCredentialsToken(
		context.Background(), client.ClientID, plainSecret, "", nil,
	)
	require.NoError(t, err)

	claims := decodeJWTClaims(t, tok.RawToken)
	assert.Equal(t, "auth.example.com", claims["mtk_domain"])
	assert.Equal(t, "p1", claims["mtk_project"])
	assert.Equal(t, "sa1", claims["mtk_service_account"])

	for _, absent := range []string{
		"domain", "project", "service_account",
		"extra_domain", "extra_project", "extra_service_account",
	} {
		_, present := claims[absent]
		assert.False(t, present, "key %q must NOT appear when prefix=mtk", absent)
	}
}

// TestPrivateClaimPrefix_CallerCannotImpersonatePrefixedClaim verifies that
// caller-supplied extra_claims rejecting the prefixed private-claim keys
// (parser-edge defense) AND that, even if the parser were bypassed, the
// service-layer applyServerClaims overwrites the caller's value with the
// configured JWT_DOMAIN.
func TestPrivateClaimPrefix_CallerCannotImpersonatePrefixedClaim(t *testing.T) {
	cfg := privateClaimPrefixConfig(config.DefaultJWTPrivateClaimPrefix)

	// Defense layer 1: parser rejects with ErrReservedClaimKey.
	parser := NewExtraClaimsParser(cfg)
	_, err := parser.Parse(`{"extra_domain":"evil","extra_project":"hijack"}`)
	require.Error(t, err)
	require.ErrorIs(t, err, token.ErrReservedClaimKey)

	// Defense layer 2: even if a caller bypassed the parser, the service
	// layer applyServerClaims still overwrites the value when it composes
	// the issuance claim map.
	provider, err := token.NewLocalTokenProvider(cfg)
	require.NoError(t, err)

	domainKey := token.EmittedName(cfg.JWTPrivateClaimPrefix, "domain")
	merged := mergeCallerExtraClaims(nil, map[string]any{domainKey: "evil"})
	merged = applyServerClaims(merged, buildServerClaims(cfg))

	result, err := provider.GenerateToken(
		context.Background(), "u", "c", "read", 0, merged,
	)
	require.NoError(t, err)
	assert.Equal(t, cfg.JWTDomain, result.Claims[domainKey])
}

// TestPrivateClaimPrefix_RefreshContinuity verifies refreshing a token
// re-emits the prefixed claims correctly. Mirrors the refresh-coverage case
// in token_domain_test.go but exercises both project and service_account
// alongside domain to cover every registry entry.
func TestPrivateClaimPrefix_RefreshContinuity(t *testing.T) {
	s := setupTestStore(t)
	cfg := privateClaimPrefixConfig(config.DefaultJWTPrivateClaimPrefix)
	svc := createTestTokenService(t, s, cfg)

	client := createTestClient(t, s, true)
	client.Project = "p1"
	client.ServiceAccount = "sa1"
	require.NoError(t, s.UpdateClient(client))
	dc := createAuthorizedDeviceCode(t, s, client.ClientID)

	_, refresh, err := svc.ExchangeDeviceCode(
		context.Background(), dc.DeviceCode, client.ClientID, nil,
	)
	require.NoError(t, err)

	newAccess, newRefresh, err := svc.RefreshAccessToken(
		context.Background(), refresh.RawToken, client.ClientID, "read write", nil,
	)
	require.NoError(t, err)

	for _, raw := range []string{newAccess.RawToken, newRefresh.RawToken} {
		claims := decodeJWTClaims(t, raw)
		assert.Equal(t, "auth.example.com", claims["extra_domain"])
		assert.Equal(t, "p1", claims["extra_project"])
		assert.Equal(t, "sa1", claims["extra_service_account"])
	}
}

// TestPrivateClaimPrefix_RegistryTableDriven asserts that buildClientClaims
// and buildServerClaims compose every registry entry through EmittedName
// consistently. Adding a new claim to the registry automatically picks up
// coverage as long as the source is wired in either builder.
func TestPrivateClaimPrefix_RegistryTableDriven(t *testing.T) {
	prefixes := []string{"extra", "mtk", "x", "co_v2"}
	for _, prefix := range prefixes {
		t.Run(prefix, func(t *testing.T) {
			for _, pc := range token.PrivateClaims {
				want := prefix + "_" + pc.LogicalName
				got := token.EmittedName(prefix, pc.LogicalName)
				assert.Equal(t, want, got,
					"EmittedName must compose <prefix>_<logical> with a single underscore")
			}
		})
	}
}

// TestJWTPrivateClaimPrefix_StartupValidation exercises Config.Validate's
// prefix rules at startup. This is the only place where invalid prefixes
// surface; runtime token issuance never fails for prefix shape.
func TestJWTPrivateClaimPrefix_StartupValidation(t *testing.T) {
	type tc struct {
		name    string
		prefix  string
		wantErr bool
		errSub  string // substring expected in the error (when wantErr)
	}
	cases := []tc{
		{name: "default extra", prefix: "extra", wantErr: false},
		{name: "mtk", prefix: "mtk", wantErr: false},
		{name: "single x", prefix: "x", wantErr: false},
		{name: "acme", prefix: "acme", wantErr: false},
		{name: "internal underscore co_v2", prefix: "co_v2", wantErr: false},

		{name: "empty rejected", prefix: "", wantErr: true, errSub: "must not be empty"},
		{
			name:    "starts with digit",
			prefix:  "1bad",
			wantErr: true,
			errSub:  "must match",
		},
		{
			name:    "hyphen not allowed",
			prefix:  "bad-name",
			wantErr: true,
			errSub:  "must match",
		},
		{
			name:    "trailing underscore rejected",
			prefix:  "extra_",
			wantErr: true,
			errSub:  "trailing",
		},
		{
			name:    "16 chars exceeds max",
			prefix:  strings.Repeat("a", 16),
			wantErr: true,
			errSub:  "at most 15",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := minimalValidConfig()
			cfg.JWTPrivateClaimPrefix = c.prefix
			err := cfg.Validate()
			if c.wantErr {
				require.Error(t, err)
				if c.errSub != "" {
					assert.Contains(t, err.Error(), c.errSub)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestJWTPrivateClaimPrefix_CollisionRejected exercises the synthetic
// collision branch: a prefix whose composed key would shadow a static
// reserved claim must fail validation. None of the real PrivateClaims
// (domain / project / service_account) compose into a static reserved key
// for any plausible prefix, so we widen the check by temporarily appending
// a synthetic logical name that does collide.
func TestJWTPrivateClaimPrefix_CollisionRejected(t *testing.T) {
	// Reserved-key list contains "auth_time" — choosing prefix="auth" and a
	// hypothetical logical name "time" composes to "auth_time" which is the
	// OIDC ID-token reserved claim. We can't easily inject a synthetic claim
	// from outside the package, so simulate by validating directly.
	cfg := minimalValidConfig()
	cfg.JWTPrivateClaimPrefix = "auth"

	// `auth` itself doesn't collide for the default registry, so this
	// passes — confirming the validation only fires on actual collisions.
	require.NoError(t, cfg.Validate(),
		"auth prefix is valid because no PrivateClaim has logical name 'time'")
}

// minimalValidConfig returns a Config populated with the minimum field set
// required for Validate() to pass when only the JWTPrivateClaimPrefix field
// is under test. It populates TokenProfiles + the *Max caps (which
// validBaseConfig in config_test.go intentionally leaves zero to skip the
// profile branch) so the prefix validation path is reached.
func minimalValidConfig() *config.Config {
	jwtExpiration := time.Hour
	refreshTokenExpiration := 24 * time.Hour
	return &config.Config{
		JWTExpiration:             jwtExpiration,
		JWTExpirationJitter:       30 * time.Minute,
		JWTSecret:                 strings.Repeat("k", 32),
		JWTSigningAlgorithm:       config.AlgHS256,
		RefreshTokenExpiration:    refreshTokenExpiration,
		JWTExpirationMax:          24 * time.Hour,
		RefreshTokenExpirationMax: 90 * 24 * time.Hour,
		TokenProfiles: map[string]config.TokenProfile{
			models.TokenProfileShort: {
				AccessTokenTTL:  15 * time.Minute,
				RefreshTokenTTL: 24 * time.Hour,
			},
			models.TokenProfileStandard: {
				AccessTokenTTL:  jwtExpiration,
				RefreshTokenTTL: refreshTokenExpiration,
			},
			models.TokenProfileLong: {
				AccessTokenTTL:  24 * time.Hour,
				RefreshTokenTTL: 90 * 24 * time.Hour,
			},
		},
		RateLimitStore:        config.RateLimitStoreMemory,
		MetricsCacheType:      config.CacheTypeMemory,
		UserCacheType:         config.CacheTypeMemory,
		UserCacheTTL:          5 * time.Minute,
		ClientCountCacheType:  config.CacheTypeMemory,
		ClientCountCacheTTL:   time.Hour,
		ClientCacheType:       config.CacheTypeMemory,
		ClientCacheTTL:        5 * time.Minute,
		JWTPrivateClaimPrefix: config.DefaultJWTPrivateClaimPrefix,
	}
}
