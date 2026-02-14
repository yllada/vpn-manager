// Package vpn provides VPN connection management functionality for VPN Manager.
//
// This package implements the core VPN functionality including:
//
//   - Profile management: Creating, updating, and deleting VPN profiles
//   - Connection management: Establishing, monitoring, and terminating connections
//   - Split tunneling: Routing specific traffic through or around the VPN
//   - Configuration parsing: Reading and validating OpenVPN configuration files
//
// # Architecture
//
// The package is organized around three main types:
//
//   - Manager: Orchestrates VPN connections and maintains connection state
//   - ProfileManager: Handles persistence and management of VPN profiles
//   - Connection: Represents an active VPN connection with its process and state
//
// # Connection Flow
//
// A typical connection flow:
//
//  1. User selects a profile through the UI
//  2. UI calls Manager.Connect() with credentials
//  3. Manager creates a Connection and starts OpenVPN process
//  4. Connection monitors the process and updates status
//  5. UI receives status updates and displays to user
//
// # OpenVPN Integration
//
// The package supports both OpenVPN 2.x (openvpn command) and OpenVPN 3
// (openvpn3 command). It automatically detects available implementations
// and uses the appropriate one.
//
// # Thread Safety
//
// All types in this package are safe for concurrent use. The Manager
// uses internal locking to protect shared state.
package vpn
