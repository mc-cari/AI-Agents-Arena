package executors

import (
	"contestmanager/internal/models"
	"context"
)

// Executor defines the interface for code execution
type Executor interface {
	Execute(ctx context.Context, req *models.ExecutionRequest) (*models.ExecutionResult, error)
}

// ExecuteCode defines the interface for executing code with test cases
type ExecuteCode interface {
	ExecuteCode(ctx context.Context, req *models.ExecutionRequest) ([]*models.TestCaseResult, error)
}

