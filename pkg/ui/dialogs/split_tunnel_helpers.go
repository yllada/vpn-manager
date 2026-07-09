// Package dialogs provides dialog components for VPN Manager.
// This file contains shared helpers for split tunneling configuration.
package dialogs

import (
	"net"
)

// =============================================================================
// Route Management Helpers
// =============================================================================

// ValidateRoute validates an IP address or CIDR network.
func ValidateRoute(route string) bool {
	// Try parsing as CIDR
	_, _, err := net.ParseCIDR(route)
	if err == nil {
		return true
	}

	// Try parsing as IP
	ip := net.ParseIP(route)
	return ip != nil
}

// AddRouteToSlice adds a route to the slice if not already present.
// Returns true if the route was added, false if duplicate.
func AddRouteToSlice(routes *[]string, route string) bool {
	for _, r := range *routes {
		if r == route {
			return false
		}
	}
	*routes = append(*routes, route)
	return true
}

// RemoveRouteFromSlice removes a route from the slice.
// Returns the new slice.
func RemoveRouteFromSlice(routes []string, route string) []string {
	newRoutes := make([]string, 0, len(routes))
	for _, r := range routes {
		if r != route {
			newRoutes = append(newRoutes, r)
		}
	}
	return newRoutes
}

// =============================================================================
// Mode Index Helpers
// =============================================================================

// FindModeIndex returns the index of a mode ID in the slice, or 0 if not found.
func FindModeIndex(modeID string, modeIDs []string) uint {
	if modeID == "" {
		return 0
	}
	for i, id := range modeIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}
