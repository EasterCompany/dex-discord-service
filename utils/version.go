package utils

import (
	"fmt"
	"strings"
)

// Version holds all version-related information for a service.
// This structure is UNIVERSAL across all services.
type Version struct {
	Str string         `json:"str"`
	Obj VersionDetails `json:"obj"`
}

// VersionDetails breaks down the version string into its components.
// This structure is UNIVERSAL across all services.
type VersionDetails struct {
	Major     string `json:"major"`
	Minor     string `json:"minor"`
	Patch     string `json:"patch"`
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Arch      string `json:"arch"`
	BuildHash string `json:"build_hash"`
}

var currentVersion Version

// SetVersion populates the package-level version variables.
func SetVersion(versionStr, branchStr, commitStr, buildDateStr, buildYearStr, buildHashStr, archStr string) {
	// Format the architecture: linux/amd64 -> linux-amd64
	formattedArch := strings.ReplaceAll(archStr, "/", "-")

	// Parse major, minor, patch from the full version string
	vParts := strings.Split(strings.TrimPrefix(versionStr, "v"), ".")
	major, minor, patch := "0", "0", "0"
	if len(vParts) >= 3 {
		major = vParts[0]
		minor = vParts[1]
		patch = vParts[2]
	}

	currentVersion = Version{
		Str: fmt.Sprintf("%s.%s.%s.%s.%s.%s.%s",
			major, minor, patch, branchStr, commitStr, buildDateStr, formattedArch),
		Obj: VersionDetails{
			Major:     major,
			Minor:     minor,
			Patch:     patch,
			Branch:    branchStr,
			Commit:    commitStr,
			BuildDate: buildDateStr,
			Arch:      formattedArch,
			BuildHash: buildHashStr,
		},
	}
}

// GetVersion constructs and returns the version information for the service.
func GetVersion() Version {
	return currentVersion
}
