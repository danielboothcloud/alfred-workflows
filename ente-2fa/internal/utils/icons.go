package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadIcon downloads an icon for a service, with 2FA fallback
func DownloadIcon(serviceName, iconsDir string) error {
	sanitizedName := SanitizeServiceName(serviceName)
	iconPath := filepath.Join(iconsDir, sanitizedName+".svg")

	// Check if icon already exists
	if _, err := os.Stat(iconPath); err == nil {
		return nil // Icon already exists
	}

	// Ensure icons directory exists
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		return fmt.Errorf("failed to create icons directory: %w", err)
	}

	// Try to download from simple-icons
	iconURL := fmt.Sprintf("https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/%s.svg",
		strings.ToLower(sanitizedName))

	err := downloadFile(iconURL, iconPath)
	if err != nil {
		// If download failed, use the 2FA fallback icon
		return copyFallbackIcon(iconPath)
	}

	return nil
}

// downloadFile downloads a file from URL to local path
func downloadFile(url, filepath string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download icon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download icon: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create icon file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save icon: %w", err)
	}

	return nil
}

// copyFallbackIcon creates a 2FA fallback icon for unrecognized services
func copyFallbackIcon(destPath string) error {
	// Embedded 2FA icon from SVG Repo - clean and simple
	fallbackSVG := `<svg fill="#000000" width="32" height="32" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
  <polygon points="11 23.18 9 21.179 7.589 22.589 11 26 17 20 15.59 18.59 11 23.18"/>
  <path d="M28,30H24V28h4V16H24V8a4.0045,4.0045,0,0,0-4-4V2a6.0067,6.0067,0,0,1,6,6v6h2a2.0021,2.0021,0,0,1,2,2V28A2.0021,2.0021,0,0,1,28,30Z"/>
  <path d="M20,14H18V8A6,6,0,0,0,6,8v6H4a2,2,0,0,0-2,2V28a2,2,0,0,0,2,2H20a2,2,0,0,0,2-2V16A2,2,0,0,0,20,14ZM8,8a4,4,0,0,1,8,0v6H8ZM20,28H4V16H20Z"/>
</svg>`

	return os.WriteFile(destPath, []byte(fallbackSVG), 0644)
}