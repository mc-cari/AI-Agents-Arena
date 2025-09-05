#!/bin/bash
"""
Test runner for AgentManager contest integration tests.

This script runs various tests to validate the AgentManager integration
with ContestManager server.

Usage:
    ./run_tests.sh [test_type]

Test types:
    simple     - Run simple contest creation and agent initialization test
    full       - Run comprehensive integration test
    all        - Run all tests (default)
"""

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if Python environment is activated
    if [[ -z "${VIRTUAL_ENV}" ]]; then
        print_warning "Virtual environment not detected"
        print_status "Activating virtual environment..."
        if [[ -d "./venv" ]]; then
            source ./venv/bin/activate
            print_success "Virtual environment activated"
        else
            print_error "Virtual environment not found. Run: python -m venv venv && source venv/bin/activate"
            exit 1
        fi
    else
        print_success "Virtual environment is active: $VIRTUAL_ENV"
    fi
    
    # Check if requirements are installed
    print_status "Checking Python dependencies..."
    if python -c "import grpc, langchain, langgraph" 2>/dev/null; then
        print_success "Required Python packages are installed"
    else
        print_warning "Some required packages may be missing"
        print_status "Installing requirements..."
        pip install -r requirements.txt
    fi
    
    # Check environment file
    if [[ -f ".env" ]]; then
        print_success "Environment file (.env) found"
    else
        print_warning "No .env file found"
        if [[ -f ".env.example" ]]; then
            print_status "Creating .env from .env.example..."
            cp .env.example .env
            print_warning "Please edit .env file with your API keys"
        fi
    fi
    
    # Check ContestManager server
    print_status "Checking ContestManager server..."
    if timeout 5 bash -c "</dev/tcp/localhost/50051" 2>/dev/null; then
        print_success "ContestManager server is running on localhost:50051"
        return 0
    else
        print_warning "ContestManager server not reachable on localhost:50051"
        print_status "Tests will run in offline mode where possible"
        return 1
    fi
}

# Function to run simple test
run_simple_test() {
    print_status "Running simple contest integration test..."
    echo "=================================="
    
    if python test_simple_contest.py; then
        print_success "Simple test completed successfully"
        return 0
    else
        print_error "Simple test failed"
        return 1
    fi
}

# Function to run full integration test
run_full_test() {
    print_status "Running full integration test..."
    echo "=================================="
    
    if python test_contest_integration.py; then
        print_success "Full integration test completed"
        return 0
    else
        print_error "Full integration test failed"
        return 1
    fi
}

# Main execution
main() {
    local test_type="${1:-all}"
    local server_available=0
    
    echo "AgentManager Test Runner"
    echo "======================="
    echo
    
    # Check prerequisites
    if check_prerequisites; then
        server_available=1
    fi
    
    echo
    
    case "$test_type" in
        "simple")
            run_simple_test
            ;;
        "full")
            run_full_test
            ;;
        "all")
            echo "Running all tests..."
            echo
            
            local simple_result=0
            local full_result=0
            
            # Run simple test
            if run_simple_test; then
                simple_result=1
            fi
            echo
            
            # Run full test
            if run_full_test; then
                full_result=1
            fi
            echo
            
            # Summary
            echo "TEST SUMMARY"
            echo "============"
            [[ $simple_result -eq 1 ]] && print_success "✓ Simple integration test passed" || print_error "✗ Simple integration test failed"
            [[ $full_result -eq 1 ]] && print_success "✓ Full integration test passed" || print_error "✗ Full integration test failed"
            
            if [[ $server_available -eq 1 ]]; then
                print_success "✓ ContestManager server is available"
            else
                print_warning "⚠ ContestManager server not available"
            fi
            
            echo
            if [[ $simple_result -eq 1 && $full_result -eq 1 ]]; then
                print_success "🎉 All tests passed! AgentManager is ready for contest participation."
                if [[ $server_available -eq 1 ]]; then
                    echo
                    print_status "To run an agent in a contest:"
                    echo "  python -m src.main --contest-id <contest_id> --participant-id <participant_id>"
                fi
                return 0
            else
                print_error "❌ Some tests failed. Check the output above for details."
                return 1
            fi
            ;;
        *)
            print_error "Unknown test type: $test_type"
            echo "Usage: $0 [simple|full|all]"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
