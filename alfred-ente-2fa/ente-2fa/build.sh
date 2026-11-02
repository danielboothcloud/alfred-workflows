#!/bin/bash

# Ente Auth Go - Alfred Workflow Build Script

set -e  # Exit on any error

echo "🚀 Building Ente Auth Go Alfred Workflow..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if we're in the right directory
if [[ ! -f "main.go" ]]; then
    echo -e "${RED}Error: main.go not found. Run this script from the project root.${NC}"
    exit 1
fi

# Step 1: Download dependencies
echo -e "${BLUE}📦 Downloading Go dependencies...${NC}"
go mod tidy

# Step 2: Copy icons if they exist
if [[ -f "ente.png" ]]; then
    echo -e "${BLUE}🎨 Copying workflow icon...${NC}"
    cp ente.png workflow/icon.png
fi

if [[ -f "2fa-fallback.svg" ]]; then
    echo -e "${BLUE}🔒 Copying 2FA fallback icon...${NC}"
    cp 2fa-fallback.svg workflow/2fa-fallback.svg
fi

# Step 3: Build the binary
echo -e "${BLUE}🔨 Building Go binary...${NC}"
go build -o workflow/alfred-ente-auth-go

# Check if build was successful
if [[ ! -f "workflow/alfred-ente-auth-go" ]]; then
    echo -e "${RED}❌ Build failed: Binary not created${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Binary built successfully${NC}"

# Step 4: Create workflow package
echo -e "${BLUE}📦 Creating Alfred workflow package...${NC}"
cd workflow
zip -r ../ente-auth-go.alfredworkflow . > /dev/null
cd ..

echo -e "${GREEN}✅ Workflow package created: ente-auth-go.alfredworkflow${NC}"

# Step 5: Show installation instructions
echo ""
echo -e "${YELLOW}🎯 Installation Instructions:${NC}"
echo -e "1. Install workflow: ${BLUE}open ente-auth-go.alfredworkflow${NC}"
echo -e "2. Configure settings in Alfred Preferences"
echo -e "3. Import accounts: Type ${BLUE}ente import${NC} in Alfred"
echo -e "4. Use TOTP codes: Type ${BLUE}ente${NC} in Alfred"
echo ""
echo -e "${GREEN}🎉 Build complete!${NC}"

# Optional: Show file sizes
echo -e "${YELLOW}📊 File Sizes:${NC}"
echo -e "Binary: $(du -h workflow/alfred-ente-auth-go | cut -f1)"
echo -e "Workflow: $(du -h ente-auth-go.alfredworkflow | cut -f1)"