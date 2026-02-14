// Package common provides shared constants, types, utilities, and interfaces
// used throughout the VPN Manager application.
//
// This package serves as the foundation for cross-cutting concerns:
//
//   - Constants: Application-wide constants like timeouts, file names, and UI dimensions
//   - Errors: Sentinel errors for consistent error handling across packages
//   - Interfaces: Abstractions for VPN connections, credential storage, and logging
//   - Logger: Structured logging with multiple output destinations
//   - Utils: Common utility functions for file operations and string manipulation
//
// # Usage
//
// Import the package to access shared functionality:
//
//	import "vpn-manager/internal/common"
//
//	// Use constants
//	timeout := common.ConnectionTimeout
//
//	// Use logger
//	common.LogInfo("Starting connection to %s", profileName)
//
//	// Check errors
//	if errors.Is(err, common.ErrProfileNotFound) {
//	    // Handle missing profile
//	}
//
// # Design Principles
//
// This package follows several design principles:
//
//   - Single Responsibility: Each file handles one concern
//   - Interface Segregation: Small, focused interfaces
//   - Open/Closed: Extensible through interfaces, not modification
//   - Dependency Inversion: High-level modules depend on abstractions
package common
