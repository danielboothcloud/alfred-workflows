package ente

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"alfred-ente-auth-go/internal/models"
)

// Client handles interactions with the Ente CLI
type Client struct {
	binaryPath string
}

// NewClient creates a new Ente CLI client
func NewClient() (*Client, error) {
	binaryPath := os.Getenv("ENTE_AUTH_BINARY_PATH")
	if binaryPath == "" {
		// Try to find ente in PATH
		path, err := exec.LookPath("ente")
		if err != nil {
			return nil, fmt.Errorf("ente binary not found: %w", err)
		}
		binaryPath = path
	}

	// Verify the binary works
	if err := checkEnteBinary(binaryPath); err != nil {
		return nil, fmt.Errorf("ente binary check failed: %w", err)
	}

	return &Client{binaryPath: binaryPath}, nil
}

// checkEnteBinary verifies the ente binary is working
func checkEnteBinary(path string) error {
	cmd := exec.Command(path, "version")
	return cmd.Run()
}

// ExportAccounts exports TOTP accounts from Ente
func (c *Client) ExportAccounts(exportPath string, overwrite bool) error {
	exportPath = filepath.Clean(exportPath)

	// Check if file exists and handle overwrite
	if _, err := os.Stat(exportPath); err == nil {
		if !overwrite {
			return nil // File exists and overwrite is false
		}
		if err := os.Remove(exportPath); err != nil {
			return fmt.Errorf("failed to remove existing export file: %w", err)
		}
	}

	// Ensure export directory exists
	exportDir := filepath.Dir(exportPath)
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return fmt.Errorf("failed to create export directory: %w", err)
	}

	// Run ente export command
	cmd := exec.Command(c.binaryPath, "export")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ente export failed: %w, output: %s", err, string(output))
	}

	// Verify export file was created
	if _, err := os.Stat(exportPath); err != nil {
		return fmt.Errorf("export file not found after export: %w", err)
	}

	return nil
}

// ParseExport parses an Ente export file and returns TOTP accounts
func (c *Client) ParseExport(exportPath string) (models.TotpAccounts, error) {
	file, err := os.Open(exportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open export file: %w", err)
	}
	defer file.Close()

	var accounts models.TotpAccounts
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		account, err := parseOTPAuthURI(line)
		if err != nil {
			// Log the error but continue parsing other lines
			continue
		}

		accounts = append(accounts, account)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading export file: %w", err)
	}

	return accounts, nil
}

// parseOTPAuthURI parses an otpauth:// URI into a TotpAccount
func parseOTPAuthURI(uri string) (models.TotpAccount, error) {
	// Clean up the URI
	uri = strings.Replace(uri, "=sha1", "=SHA1", -1)
	if strings.Contains(uri, "codeDisplay") {
		parts := strings.Split(uri, "codeDisplay")
		if len(parts) > 0 {
			uri = strings.TrimSuffix(parts[0], "&")
		}
	}

	parsedURL, err := url.Parse(uri)
	if err != nil {
		return models.TotpAccount{}, fmt.Errorf("failed to parse URI: %w", err)
	}

	if parsedURL.Scheme != "otpauth" {
		return models.TotpAccount{}, fmt.Errorf("invalid scheme: %s", parsedURL.Scheme)
	}

	// Parse path to get service name and username (URL decode like Python version)
	decodedPath, err := url.QueryUnescape(strings.Trim(parsedURL.Path, "/"))
	if err != nil {
		decodedPath = strings.Trim(parsedURL.Path, "/")
	}
	pathParts := strings.Split(decodedPath, ":")
	var serviceName, username string

	if len(pathParts) >= 2 {
		serviceName = pathParts[0]
		username = pathParts[1]
	} else if len(pathParts) == 1 {
		serviceName = strings.TrimSuffix(pathParts[0], ":")
		username = ""
	} else {
		return models.TotpAccount{}, fmt.Errorf("invalid path format")
	}

	// Parse query parameters
	query := parsedURL.Query()
	secret := query.Get("secret")
	if secret == "" {
		return models.TotpAccount{}, fmt.Errorf("missing secret parameter")
	}

	period := 30 // default period
	if periodStr := query.Get("period"); periodStr != "" && periodStr != "null" {
		if p, err := strconv.Atoi(periodStr); err == nil {
			period = p
		}
	}

	return models.TotpAccount{
		ServiceName: serviceName,
		Username:    username,
		Secret:      secret,
		Period:      period,
	}, nil
}

// DeleteExport removes the export file
func (c *Client) DeleteExport(exportPath string) error {
	return os.Remove(exportPath)
}