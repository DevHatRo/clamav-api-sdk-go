package clamav

// ScanResult represents the result of a virus scan.
type ScanResult struct {
	// Status is "OK" (clean), "FOUND" (infected), or "ERROR".
	Status string `json:"status"`
	// Message contains the virus name if infected, error description if error, or empty if clean.
	Message string `json:"message"`
	// ScanTime is the scan duration in seconds.
	ScanTime float64 `json:"time"`
	// Filename is the scanned file's name, if provided.
	Filename string `json:"filename,omitempty"`
}

// IsInfected returns true if the scan found a virus.
func (r *ScanResult) IsInfected() bool {
	return r.Status == "FOUND"
}

// IsClean returns true if the file is clean.
func (r *ScanResult) IsClean() bool {
	return r.Status == "OK"
}

// HealthCheckResult represents the health status of the ClamAV service.
type HealthCheckResult struct {
	// Healthy is true when the ClamAV service is operational.
	Healthy bool
	// Message contains the raw status message from the server.
	Message string
}

// VersionResult contains the ClamAV API server version info.
type VersionResult struct {
	// Version is the server version string.
	Version string `json:"version"`
	// Commit is the git commit hash of the server build.
	Commit string `json:"commit"`
	// Build is the build timestamp.
	Build string `json:"build"`
}

// FileInput represents a file to scan (used by ScanMultiple).
type FileInput struct {
	// Data is the file content.
	Data []byte
	// Filename is the name of the file.
	Filename string
}
