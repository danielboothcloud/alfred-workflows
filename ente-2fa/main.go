package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"alfred-ente-auth-go/internal/ente"
	"alfred-ente-auth-go/internal/keychain"
	"alfred-ente-auth-go/internal/models"
	"alfred-ente-auth-go/internal/totp"
	"alfred-ente-auth-go/internal/utils"

	aw "github.com/deanishe/awgo"
)

func main() {
	wf := aw.New()
	wf.Run(func() { run(wf) })
}

func run(wf *aw.Workflow) {
	args := wf.Args()

	// Debug: log the arguments received
	log.Printf("Args received: %v (length: %d)", args, len(args))

	if len(args) == 0 {
		wf.FatalError(fmt.Errorf("no command specified"))
		return
	}

	switch args[0] {
	case "import":
		handleImport(wf)
	case "get_accounts":
		query := ""
		if len(args) > 1 {
			query = args[1]
		}
		handleGetAccounts(wf, query)
	default:
		wf.FatalError(fmt.Errorf("unknown command: %s", args[0]))
	}
}

func handleImport(wf *aw.Workflow) {
	exportDir := os.Getenv("ENTE_EXPORT_DIR")
	if exportDir == "" {
		wf.FatalError(fmt.Errorf("ENTE_EXPORT_DIR not configured"))
		return
	}

	overwriteStr := os.Getenv("OVERWRITE_EXPORT")
	overwrite := overwriteStr == "" || strings.ToLower(overwriteStr) == "true"

	exportPath := filepath.Join(exportDir, "ente_auth.txt")

	// Initialize clients
	enteClient, err := ente.NewClient()
	if err != nil {
		wf.FatalError(fmt.Errorf("failed to initialize Ente client: %w", err))
		return
	}

	store := keychain.NewStore()

	// Export from Ente CLI
	if err := enteClient.ExportAccounts(exportPath, overwrite); err != nil {
		wf.FatalError(fmt.Errorf("failed to export from Ente: %w", err))
		return
	}

	// Parse the export file
	newAccounts, err := enteClient.ParseExport(exportPath)
	if err != nil {
		wf.FatalError(fmt.Errorf("failed to parse export: %w", err))
		return
	}

	// Load existing accounts and deduplicate
	existingAccounts, _ := store.LoadAccounts() // Ignore error if no existing accounts
	deduplicatedAccounts := deduplicateAccounts(existingAccounts, newAccounts)

	// Save deduplicated accounts to keychain
	if err := store.SaveAccounts(deduplicatedAccounts); err != nil {
		wf.FatalError(fmt.Errorf("failed to save to keychain: %w", err))
		return
	}

	accounts := deduplicatedAccounts // For icon downloads

	// Download icons concurrently
	iconsDir := filepath.Join(wf.DataDir(), "service_icons")
	downloadIcons(accounts, iconsDir)

	// Clean up export file
	enteClient.DeleteExport(exportPath)

	// Show success message
	wf.NewItem("Import Complete").
		Subtitle(fmt.Sprintf("Successfully imported %d TOTP accounts", len(accounts))).
		Icon(aw.IconInfo)

	wf.SendFeedback()
}

func handleGetAccounts(wf *aw.Workflow, query string) {
	store := keychain.NewStore()

	// Load accounts from keychain
	accounts, err := store.LoadAccounts()
	if err != nil {
		wf.FatalError(fmt.Errorf("failed to load accounts: %w", err))
		return
	}

	if len(accounts) == 0 {
		wf.NewItem("No TOTP accounts found").
			Subtitle("Try importing some accounts first").
			Icon(aw.IconWarning)
		wf.SendFeedback()
		return
	}

	// Apply search filter
	if query != "" {
		accounts = utils.FuzzySearchAccounts(query, accounts)
	}

	// Environment variables for display options
	usernameInTitle := os.Getenv("USERNAME_IN_TITLE") == "true"
	usernameInSubtitle := os.Getenv("USERNAME_IN_SUBTITLE") != "false" // Default to true

	iconsDir := filepath.Join(wf.DataDir(), "service_icons")

	// Generate TOTP codes and create Alfred items
	for _, account := range accounts {
		code, err := totp.GenerateCode(account)
		if err != nil {
			log.Printf("Failed to generate TOTP for %s: %v", account.ServiceName, err)
			continue
		}

		// Build title
		title := account.ServiceName
		if account.Username != "" && usernameInTitle {
			title = fmt.Sprintf("%s - %s", account.ServiceName, account.Username)
		}

		// Build subtitle
		subtitle := fmt.Sprintf("Current TOTP: %s | Next TOTP: %s, %d seconds left",
			code.Current, code.Next, int(code.TimeRemaining.Seconds()))
		if account.Username != "" && usernameInSubtitle {
			subtitle = fmt.Sprintf("%s - %s", subtitle, account.Username)
		}

		// Create icon path
		sanitizedName := utils.SanitizeServiceName(account.ServiceName)
		iconPath := filepath.Join(iconsDir, sanitizedName+".svg")

		item := wf.NewItem(title).
			Subtitle(subtitle).
			Arg(fmt.Sprintf("%s,%s", code.Current, code.Next)).
			Match(account.ServiceName).
			UID(account.ServiceName + account.Username).
			Valid(true)

		// Set icon if it exists
		if _, err := os.Stat(iconPath); err == nil {
			item.Icon(&aw.Icon{Value: iconPath})
		} else {
			item.Icon(aw.IconInfo)
		}
	}

	// Set cache variable for Alfred
	if jsonData, err := accounts.ToJSON(); err == nil {
		wf.Var("ENTE_ACCOUNTS_CACHE", jsonData)
	}

	// Rerun after 1 second to update TOTP codes
	wf.Rerun(1.0)
	wf.SendFeedback()
}

// downloadIcons downloads icons for all services concurrently
func downloadIcons(accounts models.TotpAccounts, iconsDir string) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent downloads

	for _, account := range accounts {
		wg.Add(1)
		go func(serviceName string) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release

			if err := utils.DownloadIcon(serviceName, iconsDir); err != nil {
				log.Printf("Failed to download icon for %s: %v", serviceName, err)
			}
		}(account.ServiceName)
	}

	wg.Wait()
}

// deduplicateAccounts merges existing and new accounts, removing duplicates
func deduplicateAccounts(existing, newAccounts models.TotpAccounts) models.TotpAccounts {
	// Create a map of existing accounts keyed by service+username
	existingMap := make(map[string]models.TotpAccount)
	for _, account := range existing {
		key := account.ServiceName + "|" + account.Username
		existingMap[key] = account
	}

	// Add new accounts, updating existing ones
	for _, account := range newAccounts {
		key := account.ServiceName + "|" + account.Username
		existingMap[key] = account // This will update existing or add new
	}

	// Convert back to slice
	var result models.TotpAccounts
	for _, account := range existingMap {
		result = append(result, account)
	}

	return result
}