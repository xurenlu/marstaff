#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Port
PORT=15678

echo -e "${GREEN}=== Marstaff Restart Script ===${NC}"
echo -e "  ${YELLOW}(Using Docker? Run: docker compose build --no-cache && docker compose up -d)${NC}"
echo ""

# Kill processes on port
echo -e "${YELLOW}[1/2] Killing existing processes...${NC}"

for i in 1 2 3 4 5; do
    PID=$(lsof -ti:$PORT 2>/dev/null)
    if [ -n "$PID" ]; then
        echo -e "  ${RED}Killing process (PID: $PID) on port $PORT${NC}"
        kill -9 $PID 2>/dev/null
        sleep 2
    else
        break
    fi
done

# Verify port is free
if lsof -ti:$PORT >/dev/null 2>&1; then
    echo -e "  ${RED}WARNING: Port $PORT still in use after kill. Retrying...${NC}"
    sleep 3
    kill -9 $(lsof -ti:$PORT 2>/dev/null) 2>/dev/null
    sleep 2
fi

if lsof -ti:$PORT >/dev/null 2>&1; then
    echo -e "  ${RED}ERROR: Port $PORT still in use. Cannot start.${NC}"
    exit 1
fi

echo -e "  ${GREEN}Port $PORT is free${NC}"
echo ""

# Set environment variables
echo -e "${YELLOW}[2/2] Setting environment variables...${NC}"
# Source .env if exists (for FIRECRAWL_API_KEY etc., add FIRECRAWL_API_KEY=fc-xxx to .env for firecrawl tasks)
[ -f .env ] && set -a && source .env && set +a
export ALIYUN_ACCESS_KEY_ID="YOUR_ALIYUN_ACCESS_KEY_ID"
export ALIYUN_ACCESS_KEY_SECRET="YOUR_ALIYUN_ACCESS_KEY_SECRET"
export ZHIPU_API_KEY="YOUR_ZHIPU_API_KEY"
export QWEN_API_KEY="YOUR_QWEN_API_KEY"
echo -e "  ${GREEN}Environment variables set${NC}"
echo ""

# Start service
echo -e "${YELLOW}Starting Gateway (with embedded Agent)...${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Port: $PORT${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

ALIYUN_ACCESS_KEY_ID="$ALIYUN_ACCESS_KEY_ID" \
ALIYUN_ACCESS_KEY_SECRET="$ALIYUN_ACCESS_KEY_SECRET" \
ZHIPU_API_KEY="$ZHIPU_API_KEY" \
QWEN_API_KEY="$QWEN_API_KEY" \
FIRECRAWL_API_KEY="$FIRECRAWL_API_KEY" \
go run ./cmd/gateway/main.go -c configs/config.yaml

echo ""
echo -e "${RED}Service stopped.${NC}"
