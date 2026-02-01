package utils

import (
	sharedUtils "github.com/EasterCompany/dex-go-utils/utils"
)

// SetVersion populates the package-level version variables.
func SetVersion(versionStr, branchStr, commitStr, buildDateStr, archStr string) {
	sharedUtils.SetVersion(versionStr, branchStr, commitStr, buildDateStr, archStr)
}

// GetVersion constructs and returns the version information for the service.
func GetVersion() Version {
	return sharedUtils.GetVersion()
}
