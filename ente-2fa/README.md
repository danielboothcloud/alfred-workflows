# Ente Auth Alfred Workflow (Go)

A fast, native Go implementation of the Ente Auth Alfred workflow for managing TOTP codes.

## Features

- **Native Performance**: Single binary with no runtime dependencies
- **Secure Storage**: Uses macOS Keychain for encrypted account storage
- **Fuzzy Search**: Fast searching through your TOTP accounts
- **Icon Support**: Automatic icon downloading for services
- **Real-time Updates**: Live TOTP code updates in Alfred
- **Concurrent Operations**: Faster icon downloads and processing

## Setup

### Prerequisites

1. **Ente CLI**: Download from [Ente releases](https://github.com/ente-io/ente/releases?q=tag%3Acli-v0)
   ```bash
   sudo mv /path/to/ente /usr/local/bin/ente
   sudo chmod +x /usr/local/bin/ente
   ente version  # Verify installation
   ```

2. **Configure Ente CLI**:
   ```bash
   ente account add
   # Choose 'auth' as app type
   # Set export directory (must exist)
   # Provide Ente login credentials
   ```

### Installation

1. **Build the workflow**:
   ```bash
   cd alfred-ente-auth-go
   go build -o workflow/alfred-ente-auth-go
   ```

2. **Install in Alfred**:
   - Double-click the generated `.alfredworkflow` file, or
   - Use the `alfred link` command if you have go-alfred CLI tool

3. **Configure environment variables** in Alfred workflow settings:
   - `ENTE_EXPORT_DIR`: Path to your Ente export directory
   - `ENTE_AUTH_BINARY_PATH`: Path to Ente CLI binary (optional)
   - `USERNAME_IN_TITLE`: Show username in title (optional)
   - `USERNAME_IN_SUBTITLE`: Show username in subtitle (default: true)
   - `OVERWRITE_EXPORT`: Overwrite existing exports (default: true)

## Usage

### Import Accounts
```
ente import
```
Exports TOTP accounts from Ente CLI and imports them into the workflow.

### Search & Use Accounts
```
ente
ente github
```
- Lists all TOTP accounts or searches for specific ones
- Press Enter to copy current TOTP code
- Press Cmd+Enter to copy current code and paste it
- Press Alt+Enter to paste current code
- Press Cmd+Alt+Enter to copy next code
- Press Shift+Cmd+Alt+Enter to paste next code

## Architecture

```
alfred-ente-auth-go/
├── main.go                 # CLI entry point & Alfred integration
├── internal/
│   ├── ente/              # Ente CLI interactions
│   ├── keychain/          # macOS Keychain operations
│   ├── totp/              # TOTP code generation
│   ├── models/            # Data structures
│   └── utils/             # Search & utilities
└── workflow/              # Alfred workflow configuration
```

## Improvements over Python Version

- **Performance**: Native Go binary vs Python interpreter
- **Dependencies**: Self-contained vs Python + virtualenv
- **Concurrency**: Parallel icon downloads
- **Memory**: Lower memory footprint
- **Distribution**: Single binary vs complex Python setup

## Development

```bash
# Install dependencies
go mod tidy

# Build
go build -o workflow/alfred-ente-auth-go

# Test locally
./workflow/alfred-ente-auth-go get_accounts
./workflow/alfred-ente-auth-go import
```

## License

Same as original Python version - check the original repository for license details.