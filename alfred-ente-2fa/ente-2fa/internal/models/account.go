package models

import (
	"encoding/json"
	"time"
)

// TotpAccount represents a TOTP account with service details
type TotpAccount struct {
	ServiceName string `json:"service_name"`
	Username    string `json:"username"`
	Secret      string `json:"secret"`
	Period      int    `json:"period"`
}

// TotpAccounts represents a collection of TOTP accounts
type TotpAccounts []TotpAccount

// ToJSON converts TotpAccounts to JSON string
func (ta TotpAccounts) ToJSON() (string, error) {
	data, err := json.Marshal(ta)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON creates TotpAccounts from JSON string
func FromJSON(jsonStr string) (TotpAccounts, error) {
	var accounts TotpAccounts
	err := json.Unmarshal([]byte(jsonStr), &accounts)
	return accounts, err
}

// ImportResult represents the result of importing accounts
type ImportResult struct {
	Count    int          `json:"count"`
	Accounts TotpAccounts `json:"accounts"`
}

// TotpCode represents a generated TOTP code with timing info
type TotpCode struct {
	Current       string        `json:"current"`
	Next          string        `json:"next"`
	TimeRemaining time.Duration `json:"time_remaining"`
}