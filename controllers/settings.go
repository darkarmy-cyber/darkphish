package controllers

import "github.com/darkarmy-cyber/darkphish/internal/update"

func WithUpdates(service *update.Service) AdminServerOption {
	return func(as *AdminServer) { as.updates = service }
}

func settingsTabAllowed(tab string) bool {
	switch tab {
	case "account", "ui", "reporting", "api", "users", "webhooks", "audit", "update":
		return true
	}
	return false
}

func adminSettingsTab(tab string) bool {
	return tab == "users" || tab == "webhooks" || tab == "audit" || tab == "update"
}
