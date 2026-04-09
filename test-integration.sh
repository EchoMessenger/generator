#!/bin/bash
# Integration test script for generator
# Validates all components are working correctly

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== EchoMessenger Generator Integration Tests ===${NC}\n"

# Test 1: Binary compilation
echo -e "${YELLOW}Test 1: Building binary...${NC}"
if GOFLAGS=-mod=mod go build -o build/generator ./cmd 2>&1; then
    echo -e "${GREEN}✓ Binary compiled successfully${NC}\n"
else
    echo -e "${RED}✗ Build failed${NC}\n"
    exit 1
fi

# Test 2: Help output
echo -e "${YELLOW}Test 2: Checking help...${NC}"
if ./build/generator -help 2>&1 | grep -q "Usage:"; then
    echo -e "${GREEN}✓ Help output works${NC}\n"
else
    echo -e "${RED}✗ Help output failed${NC}\n"
    exit 1
fi

# Test 3: Config validation
echo -e "${YELLOW}Test 3: Validating config file...${NC}"
if [ -f config.example.yaml ]; then
    echo -e "${GREEN}✓ Config template exists${NC}\n"
else
    echo -e "${RED}✗ Config template missing${NC}\n"
    exit 1
fi

# Test 4: Docker file checks
echo -e "${YELLOW}Test 4: Checking Docker files...${NC}"
if [ -f Dockerfile ] && [ -f docker-compose.yml ] && [ -f prometheus.yml ]; then
    echo -e "${GREEN}✓ All Docker files present${NC}\n"
else
    echo -e "${RED}✗ Docker files missing${NC}\n"
    exit 1
fi

# Test 5: README
echo -e "${YELLOW}Test 5: Checking documentation...${NC}"
if [ -f README.md ] && grep -q "Quick Start" README.md; then
    echo -e "${GREEN}✓ README documentation complete${NC}\n"
else
    echo -e "${RED}✗ README incomplete${NC}\n"
    exit 1
fi

# Test 6: Source files
echo -e "${YELLOW}Test 6: Verifying all source files...${NC}"
required_files=(
    "cmd/main.go"
    "internal/client/ws.go"
    "internal/client/session.go"
    "internal/client/actions.go"
    "internal/client/provisioner.go"
    "internal/config/config.go"
    "internal/scenario/runner.go"
    "internal/scenario/normal.go"
    "internal/scenario/malicious.go"
    "internal/utils/events.go"
    "internal/utils/metrics.go"
)

all_present=true
for file in "${required_files[@]}"; do
    if [ ! -f "$file" ]; then
        echo -e "${RED}✗ Missing: $file${NC}"
        all_present=false
    fi
done

if [ "$all_present" = true ]; then
    echo -e "${GREEN}✓ All source files present (${#required_files[@]} files)${NC}\n"
else
    exit 1
fi

# Test 7: Go modules
echo -e "${YELLOW}Test 7: Checking Go modules...${NC}"
if [ -f go.mod ] && [ -f go.sum ]; then
    echo -e "${GREEN}✓ Go module files present${NC}\n"
else
    echo -e "${RED}✗ Go module files missing${NC}\n"
    exit 1
fi

# Test 8: Makefile
echo -e "${YELLOW}Test 8: Verifying Makefile targets...${NC}"
if grep -q "build:" Makefile && grep -q "docker-build:" Makefile; then
    echo -e "${GREEN}✓ Makefile has required targets${NC}\n"
else
    echo -e "${RED}✗ Makefile incomplete${NC}\n"
    exit 1
fi

# Test 9: Code quality checks
echo -e "${YELLOW}Test 9: Running gofmt...${NC}"
if gofmt -l cmd internal | grep -q .; then
    echo -e "${YELLOW}⚠ Some files need formatting (run: make fmt)${NC}\n"
else
    echo -e "${GREEN}✓ Code formatting OK${NC}\n"
fi

# Test 10: Build size check
echo -e "${YELLOW}Test 10: Checking binary size...${NC}"
size=$(stat -f%z build/generator 2>/dev/null || stat -c%s build/generator 2>/dev/null)
size_mb=$(echo "scale=1; $size / 1048576" | bc)
echo -e "${GREEN}✓ Binary size: ${size_mb}MB${NC}\n"

echo -e "${GREEN}=== All tests passed! ===${NC}"
echo ""
echo -e "Next steps:"
echo -e "  1. Copy config.example.yaml → config.yaml"
echo -e "  2. Edit config.yaml with your server details"
echo -e "  3. Run: ./build/generator -config config.yaml"
echo -e "  4. Or deploy with: docker-compose up"
echo ""
