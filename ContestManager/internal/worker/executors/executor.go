package executors

import (
	"contestmanager/internal/models"
	"context"
)

type Executor interface {
	Execute(ctx context.Context, req *models.ExecutionRequest) (*models.ExecutionResult, error)
}

type ExecuteCode interface {
	ExecuteCode(ctx context.Context, req *models.ExecutionRequest) ([]*models.TestCaseResult, error)
}

