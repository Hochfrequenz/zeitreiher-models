// Package zeitreihermodels holds the hand-written public data models of the Zeitreiher API, version 1.
//
// Zeitreiher is a read-only API over SAP IS-U EDM profile values (Lastgänge). The contract is the
// JSON form of the types in this package, pinned by the shared fixtures in testdata/, which the Go
// and the Python models both parse and round-trip. The gRPC contract lives in proto/zeitreiher/v1.
//
// The package lives at the repository root; its major version follows Go module versioning, so a
// breaking v2 is the module github.com/hochfrequenz/zeitreiher-models/v2 (spec §3.4).
//
// Nothing in this package is generated. Every type validates itself when it is unmarshalled and
// refuses to marshal when it is invalid, so a value that leaves or enters a program through these
// types always satisfies the contract:
//
//   - Request timestamps are RFC 3339 with whole seconds and an explicit offset (Z or ±hh:mm).
//     Timestamps without an offset and the "unknown offset" -00:00 are rejected.
//   - Response timestamps are RFC 3339 with whole seconds in UTC (Z).
//   - Intervals are half-open: [from, to), and from must be before to.
//   - Values are decimal strings, already scaled (e.g. "0.022"), never JSON numbers.
//   - Requests reject unknown keys; responses ignore them, so additive server changes do not break
//     older clients.
package zeitreihermodels
