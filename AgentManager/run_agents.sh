#!/bin/bash

# Script to run multiple agents for a contest

set -e

CONTEST_ID=""
CONTEST_MANAGER_HOST="localhost:50051"
LOG_LEVEL="INFO"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --contest-id)
            CONTEST_ID="$2"
            shift 2
            ;;
        --host)
            CONTEST_MANAGER_HOST="$2"
            shift 2
            ;;
        --log-level)
            LOG_LEVEL="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$CONTEST_ID" ]; then
    echo "Usage: $0 --contest-id <contest_id> [--host <host>] [--log-level <level>]"
    exit 1
fi

echo "Starting agents for contest: $CONTEST_ID"

# Activate virtual environment if it exists
if [ -d "venv" ]; then
    source venv/bin/activate
fi

# Load environment variables
if [ -f ".env" ]; then
    set -a
    source .env
    set +a
fi

# Define agent configurations with 10 different models
declare -a MODELS=(
    "gpt-4"
    "gpt-4-turbo" 
    "gpt-3.5-turbo"
    "gpt-4o"
    "claude-3-5-sonnet-20241022"
    "claude-3-haiku-20240307"
    "claude-3-opus-20240229"
    "gemini-1.5-pro"
    "gemini-1.5-flash"
    "gemini-pro"
)

# Start agents in background
PIDS=()

for model in "${MODELS[@]}"; do
    participant_id="${model}-agent-$(date +%s)"
    
    echo "Starting $model agent (participant: $participant_id)"
    
    python -m src.main \
        --contest-id "$CONTEST_ID" \
        --participant-id "$participant_id" \
        --model "$model" \
        --host "$CONTEST_MANAGER_HOST" \
        --log-level "$LOG_LEVEL" &
    
    PIDS+=($!)
    
    # Small delay between starting agents
    sleep 2
done

echo "Started ${#PIDS[@]} agents with PIDs: ${PIDS[*]}"

# Function to cleanup on exit
cleanup() {
    echo "Stopping all agents..."
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
        fi
    done
    wait
    echo "All agents stopped"
}

# Set trap for cleanup
trap cleanup EXIT INT TERM

# Wait for all agents to complete
echo "Waiting for agents to complete..."
for pid in "${PIDS[@]}"; do
    wait "$pid"
done

echo "All agents completed"
