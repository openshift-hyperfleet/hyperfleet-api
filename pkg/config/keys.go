package config

// EnvPrefix is the environment variable prefix for all HyperFleet configuration.
// All configuration can be set via environment variables using the format:
//
//	HYPERFLEET_<SECTION>_<KEY>=value
//
// For example:
//   - HYPERFLEET_SERVER_HOST=0.0.0.0
//   - HYPERFLEET_DATABASE_PASSWORD=secret
//   - HYPERFLEET_LOGGING_LEVEL=debug
const EnvPrefix = "HYPERFLEET"
