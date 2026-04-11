package health

import "errors"

// Sentinel errors for health check probes.
var (
	// ErrProbeTimeout indicates the probe timed out waiting for a response.
	ErrProbeTimeout = errors.New("probe timed out")

	// ErrProbeConnectionRefused indicates the connection was refused by the target.
	ErrProbeConnectionRefused = errors.New("probe connection refused")

	// ErrAllProbesFailed indicates all probes in the fallback chain failed.
	ErrAllProbesFailed = errors.New("all probes failed")

	// ErrICMPNotAvailable indicates ICMP probing is not available due to permissions.
	ErrICMPNotAvailable = errors.New("ICMP not available: insufficient permissions")
)
