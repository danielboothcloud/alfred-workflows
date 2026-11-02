package keychain

import (
	"fmt"

	"alfred-ente-auth-go/internal/models"

	"github.com/keybase/go-keychain"
)

const (
	serviceName = "alfred-ente-auth-go"
	accountName = "totp-accounts"
)

// Store handles keychain operations for TOTP accounts
type Store struct{}

// NewStore creates a new keychain store
func NewStore() *Store {
	return &Store{}
}

// SaveAccounts saves TOTP accounts to the keychain
func (s *Store) SaveAccounts(accounts models.TotpAccounts) error {
	jsonData, err := accounts.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize accounts: %w", err)
	}

	// Create keychain item
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(serviceName)
	item.SetAccount(accountName)
	item.SetData([]byte(jsonData))
	item.SetAccessible(keychain.AccessibleWhenUnlocked)
	item.SetSynchronizable(keychain.SynchronizableNo)

	// Delete existing item first (if any)
	s.deleteExisting()

	// Add new item
	err = keychain.AddItem(item)
	if err != nil {
		return fmt.Errorf("failed to add item to keychain: %w", err)
	}

	return nil
}

// LoadAccounts loads TOTP accounts from the keychain
func (s *Store) LoadAccounts() (models.TotpAccounts, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(serviceName)
	query.SetAccount(accountName)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		if err == keychain.ErrorItemNotFound {
			return models.TotpAccounts{}, nil // Return empty slice if not found
		}
		return nil, fmt.Errorf("failed to query keychain: %w", err)
	}

	if len(results) == 0 {
		return models.TotpAccounts{}, nil
	}

	jsonData := results[0].Data
	accounts, err := models.FromJSON(string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize accounts: %w", err)
	}

	return accounts, nil
}

// DeleteAccounts removes TOTP accounts from the keychain
func (s *Store) DeleteAccounts() error {
	return s.deleteExisting()
}

// deleteExisting removes existing keychain item
func (s *Store) deleteExisting() error {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(serviceName)
	query.SetAccount(accountName)

	err := keychain.DeleteItem(query)
	if err != nil && err != keychain.ErrorItemNotFound {
		return fmt.Errorf("failed to delete existing keychain item: %w", err)
	}

	return nil
}