package store

import "errors"

var (
	// ErrUsernameConflict is returned when a username already exists
	ErrUsernameConflict = errors.New("username already exists")

	// ErrExternalUserMissingIdentity is returned by UpsertExternalUser when
	// one of the required identity fields (username, externalID, authSource,
	// or — on create — email) is blank after trimming. Without a stable
	// identity we cannot safely create or link a user.
	ErrExternalUserMissingIdentity = errors.New(
		"external user missing required identity field (username, external_id, auth_source, or email)",
	)

	// ErrAmbiguousEmail is returned when GetUserByEmail's whitespace-tolerant
	// fallback lookup matches more than one row — a signal that legacy data
	// contains duplicate emails differing only in whitespace, which must be
	// deduped manually before the caller can proceed.
	ErrAmbiguousEmail = errors.New(
		"multiple users match the normalized email; manual deduplication required",
	)

	// ErrAuthCodeAlreadyUsed is returned by MarkAuthorizationCodeUsed when the
	// code was already consumed by a concurrent request (0 rows updated).
	ErrAuthCodeAlreadyUsed = errors.New("authorization code already used")

	// ErrDeviceCodeAlreadyAuthorized is returned by AuthorizeDeviceCode when the
	// device code was already authorized by a concurrent request (0 rows updated).
	ErrDeviceCodeAlreadyAuthorized = errors.New("device code already authorized")
)
