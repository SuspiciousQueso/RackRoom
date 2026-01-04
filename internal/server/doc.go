// Package server implements the RackRoom HTTP API surface.
//
// Owns:
//   - HTTP routing, handlers, and request/response contracts
//   - Authentication wrappers (service key + agent signature verification)
//   - API-level invariants and behavior
//
// Does not own:
//   - Storage internals (Store implementations)
//   - Agent-side inventory collection logic
//
// Invariants:
//   - JSON responses are consistent via writeJSON (except raw inventory payload passthrough)
//   - Admin endpoints must be protected by RequireServiceKey
//   - Mutating agent endpoints require signed requests (RequireAgentAuth)
package server
