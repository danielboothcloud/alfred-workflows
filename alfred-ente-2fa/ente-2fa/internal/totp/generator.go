package totp

import (
	"fmt"
	"time"

	"alfred-ente-auth-go/internal/models"

	"github.com/pquerna/otp/totp"
)

// GenerateCode generates current and next TOTP codes for an account
func GenerateCode(account models.TotpAccount) (models.TotpCode, error) {
	now := time.Now()

	// Generate current TOTP using simple method (matches Python pyotp defaults)
	current, err := totp.GenerateCode(account.Secret, now)
	if err != nil {
		return models.TotpCode{}, fmt.Errorf("failed to generate current TOTP: %w", err)
	}

	// Generate next TOTP
	nextTime := now.Add(time.Duration(account.Period) * time.Second)
	next, err := totp.GenerateCode(account.Secret, nextTime)
	if err != nil {
		return models.TotpCode{}, fmt.Errorf("failed to generate next TOTP: %w", err)
	}

	// Calculate time remaining until next code
	timeRemaining := calculateTimeRemaining(account.Period)

	return models.TotpCode{
		Current:       current,
		Next:          next,
		TimeRemaining: timeRemaining,
	}, nil
}

// calculateTimeRemaining calculates how much time is left before the next TOTP code
func calculateTimeRemaining(period int) time.Duration {
	now := time.Now().Unix()
	remaining := int64(period) - (now % int64(period))
	return time.Duration(remaining) * time.Second
}