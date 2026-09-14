package auth

// SettingsTabAllowed is shared by login return targets and Settings routing.
func SettingsTabAllowed(tab string) bool {
	switch tab {
	case "account", "ui", "reporting", "api", "users", "webhooks", "audit", "update":
		return true
	}
	return false
}
