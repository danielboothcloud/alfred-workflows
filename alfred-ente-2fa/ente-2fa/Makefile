# Ente Auth Go - Alfred Workflow Makefile

BINARY_NAME=alfred-ente-auth-go
WORKFLOW_DIR=workflow
PACKAGE_NAME=ente-auth-go.alfredworkflow

.PHONY: all build package clean install test deps help

# Default target
all: build package

# Build the Go binary
build:
	@echo "🔨 Building Go binary..."
	@go build -ldflags="-s -w" -o $(WORKFLOW_DIR)/$(BINARY_NAME)
	@echo "✅ Binary built successfully"

# Download Go dependencies
deps:
	@echo "📦 Downloading dependencies..."
	@go mod tidy
	@go mod download

# Create Alfred workflow package
package: build
	@echo "📦 Creating workflow package..."
	@cd $(WORKFLOW_DIR) && zip -r ../$(PACKAGE_NAME) . > /dev/null
	@echo "✅ Package created: $(PACKAGE_NAME)"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f $(WORKFLOW_DIR)/$(BINARY_NAME)
	@rm -f $(PACKAGE_NAME)
	@echo "✅ Clean complete"

# Install workflow (macOS only)
install: package
	@echo "🚀 Installing workflow..."
	@open $(PACKAGE_NAME)

# Run tests
test:
	@echo "🧪 Running tests..."
	@go test ./...

# Build with debug info (for development)
debug:
	@echo "🔨 Building debug binary..."
	@go build -o $(WORKFLOW_DIR)/$(BINARY_NAME)

# Build for release (optimized)
release: clean deps
	@echo "🚀 Building release version..."
	@go build -ldflags="-s -w -X main.version=$$(date +%Y.%m.%d)" -o $(WORKFLOW_DIR)/$(BINARY_NAME)
	@cd $(WORKFLOW_DIR) && zip -r ../ente-auth-go-v$$(date +%Y.%m.%d).alfredworkflow . > /dev/null
	@echo "✅ Release package created: ente-auth-go-v$$(date +%Y.%m.%d).alfredworkflow"

# Show file information
info:
	@echo "📊 Build Information:"
	@echo "Binary: $$(du -h $(WORKFLOW_DIR)/$(BINARY_NAME) 2>/dev/null | cut -f1 || echo 'Not built')"
	@echo "Package: $$(du -h $(PACKAGE_NAME) 2>/dev/null | cut -f1 || echo 'Not created')"
	@file $(WORKFLOW_DIR)/$(BINARY_NAME) 2>/dev/null || echo "Binary not found"

# Show help
help:
	@echo "Ente Auth Go - Alfred Workflow Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  build     - Build the Go binary"
	@echo "  package   - Create Alfred workflow package"
	@echo "  clean     - Remove build artifacts"
	@echo "  install   - Build, package, and install workflow"
	@echo "  test      - Run tests"
	@echo "  deps      - Download dependencies"
	@echo "  debug     - Build with debug information"
	@echo "  release   - Build optimized release version"
	@echo "  info      - Show build information"
	@echo "  help      - Show this help message"
	@echo ""
	@echo "Quick start: make install"