// Package service provides pprof support for performance profiling.
// This file adds HTTP endpoints for Go's pprof tool to enable performance analysis.
package service

import (
	"net/http"
	_ "net/http/pprof" // Import pprof for side effects (registers HTTP handlers)

	"injective-explorer/internal/global"
)

// EnablePprof enables pprof endpoints on the default HTTP server.
// This should only be enabled in development or staging environments.
//
// The pprof endpoints will be available at:
//   - http://localhost:6060/debug/pprof/
//
// Usage:
//   - To profile CPU: go tool pprof http://localhost:6060/debug/pprof/profile
//   - To profile memory: go tool pprof http://localhost:6060/debug/pprof/heap
//   - To profile goroutines: go tool pprof http://localhost:6060/debug/pprof/goroutine
//
// Parameters:
//   - addr: The address to listen on (e.g., "localhost:6060")
//
// Returns:
//   - error: Error if the HTTP server fails to start
func EnablePprof(addr string) error {
	// Start pprof server in a separate goroutine
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			// Log error but don't panic - pprof is optional
			global.Logger.Errorf("pprof server error: %v", err)
		}
	}()

	global.Logger.Infof("pprof server started on %s", addr)
	return nil
}
