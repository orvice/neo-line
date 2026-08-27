package certmanager

import "errors"

// ErrTokenRequired is returned when create is attempted without an API token.
var ErrTokenRequired = errors.New("api token is required")

// ErrIssuerNameRequired is returned when an issuer name is missing.
var ErrIssuerNameRequired = errors.New("issuer name is required")

// ErrIssuerEmailRequired is returned when an issuer email is missing.
var ErrIssuerEmailRequired = errors.New("issuer email is required")

// ErrTermsOfServiceRequired is returned when ToS agreement is missing.
var ErrTermsOfServiceRequired = errors.New("terms of service agreement is required")

// ErrEABRequired is returned when EAB credentials are required but missing.
var ErrEABRequired = errors.New("external account binding credentials are required")

// ErrCustomDirectoryRequired is returned when custom ca_type lacks a directory URL.
var ErrCustomDirectoryRequired = errors.New("custom directory URL is required")

// ErrInvalidDirectoryURL is returned when a custom directory URL is invalid.
var ErrInvalidDirectoryURL = errors.New("invalid custom directory URL")

// ErrIssuerRegistrationPending is returned when mutating a pending issuer.
var ErrIssuerRegistrationPending = errors.New("issuer registration is still pending")
