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

// ErrDNSAccountRequired is returned when dns_provider_account_id is missing.
var ErrDNSAccountRequired = errors.New("dns provider account is required")

// ErrCertificateIssuerRequired is returned when certificate_issuer_id is missing.
var ErrCertificateIssuerRequired = errors.New("certificate issuer is required")

// ErrManagedCertificateNameRequired is returned when name is empty.
var ErrManagedCertificateNameRequired = errors.New("managed certificate name is required")

// ErrInvalidDomains is returned when domain normalization or validation fails.
var ErrInvalidDomains = errors.New("invalid domains")

// ErrTooManyDomains is returned when more than 100 domains are supplied.
var ErrTooManyDomains = errors.New("at most 100 domains are allowed")

// ErrInvalidKeyType is returned for unsupported key types.
var ErrInvalidKeyType = errors.New("invalid certificate key type")

// ErrIssuerNotReady is returned when the referenced issuer is not Ready.
var ErrIssuerNotReady = errors.New("certificate issuer is not ready for issuance")

// ErrIssueFieldsLocked is returned when issue fields are changed during a running operation.
var ErrIssueFieldsLocked = errors.New("issue fields cannot be changed while an operation is running")

// ErrNoActiveVersion is returned when Renew is requested without an active version.
var ErrNoActiveVersion = errors.New("managed certificate has no active version to renew")

// ErrIssuanceOperationInFlight is returned when another Issue or Renew operation is running.
var ErrIssuanceOperationInFlight = errors.New("another certificate issuance operation is already running")
