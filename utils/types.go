package utils

// ServiceReport defines the UNIVERSAL structure for the /service endpoint response.
// ALL Dexter services MUST implement this exact structure.
type ServiceReport struct {
	Version Version                `json:"version"`
	Health  Health                 `json:"health"`
	Metrics map[string]interface{} `json:"metrics"`
}
