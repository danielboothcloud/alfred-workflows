package utils

import (
	"strings"

	"alfred-ente-auth-go/internal/models"

	"github.com/sahilm/fuzzy"
)

// FuzzySearchAccounts performs fuzzy search on TOTP accounts
func FuzzySearchAccounts(query string, accounts models.TotpAccounts) models.TotpAccounts {
	if query == "" {
		return accounts
	}

	// Create search targets
	var targets []string
	for _, account := range accounts {
		// Combine service name and username for searching
		searchText := account.ServiceName
		if account.Username != "" {
			searchText += " " + account.Username
		}
		targets = append(targets, searchText)
	}

	// Perform fuzzy search
	matches := fuzzy.Find(query, targets)

	// Return matching accounts
	var results models.TotpAccounts
	for _, match := range matches {
		if match.Index < len(accounts) {
			results = append(results, accounts[match.Index])
		}
	}

	return results
}

// SanitizeServiceName sanitizes service name for file operations
func SanitizeServiceName(name string) string {
	// Remove or replace invalid characters for filenames
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)

	sanitized := replacer.Replace(strings.ToLower(name))

	// Remove multiple consecutive dashes
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	// Trim dashes from start and end
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}