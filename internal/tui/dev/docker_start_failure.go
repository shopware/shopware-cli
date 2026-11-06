package dev

// failure_category values of the project.dev.docker_start event.
const (
	dockerStartFailurePortInUse  = "port_in_use"
	dockerStartFailureDiskFull   = "disk_full"
	dockerStartFailureImagePull  = "image_pull"
	dockerStartFailureMount      = "mount"
	dockerStartFailurePermission = "permission"
	dockerStartFailureUnknown    = "unknown"
)

// dockerStartFailureRules lists known `docker compose up` failure patterns.
// mount comes before permission, as a denied mount also says "permission denied".
var dockerStartFailureRules = []failureRule[string]{
	{
		category: dockerStartFailurePortInUse,
		patterns: failurePatterns(
			`port is already allocated`,
			`address already in use`,
			`ports are not available`,
		),
	},
	{
		category: dockerStartFailureDiskFull,
		patterns: failurePatterns(
			`no space left on device`,
		),
	},
	{
		category: dockerStartFailureImagePull,
		patterns: failurePatterns(
			`pull access denied`,
			`manifest unknown`,
			`manifest for .* not found`,
			`no matching manifest`,
			`toomanyrequests`,
			`failed to resolve reference`,
			`tls handshake timeout`,
		),
	},
	{
		category: dockerStartFailureMount,
		patterns: failurePatterns(
			`mounts denied`,
			`is not shared from the host`,
			`error while creating mount source path`,
			`invalid mount config`,
		),
	},
	{
		category: dockerStartFailurePermission,
		patterns: failurePatterns(
			`permission denied`,
			`operation not permitted`,
		),
	},
}

// classifyDockerStartFailure returns the failure_category of a failed
// `docker compose up` from its output. The output itself is never sent.
func classifyDockerStartFailure(output []string) string {
	if category, _, ok := matchFailure(cleanInstallOutput(output), dockerStartFailureRules); ok {
		return category
	}
	return dockerStartFailureUnknown
}
