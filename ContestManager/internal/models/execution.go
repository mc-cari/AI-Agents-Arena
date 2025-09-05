package models

import (
	"time"

	"github.com/google/uuid"
)

type ExecutionRequest struct {
	JobID         uuid.UUID `json:"job_id"`
	SubmissionID  uuid.UUID `json:"submission_id"`
	ContestID     uuid.UUID `json:"contest_id"`
	ParticipantID uuid.UUID `json:"participant_id"`
	ProblemID     uuid.UUID `json:"problem_id"`
	Code          string    `json:"code"`
	Language      Language  `json:"language"`
	TestCases     []TestCaseData `json:"test_cases"`
	TimeLimitMs   int32     `json:"time_limit_ms"`
	MemoryLimitMb int32     `json:"memory_limit_mb"`
	CreatedAt     time.Time `json:"created_at"`
}

type TestCaseData struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	TestOrder      int32  `json:"test_order"`
}

type ExecutionResult struct {
	JobID            uuid.UUID           `json:"job_id"`
	SubmissionID     uuid.UUID           `json:"submission_id"`
	Status           SubmissionStatus    `json:"status"`
	VerdictMessage   string              `json:"verdict_message"`
	TestCaseResults  []TestCaseResult    `json:"test_case_results"`
	TotalTestCases   int32               `json:"total_test_cases"`
	PassedTestCases  int32               `json:"passed_test_cases"`
	ExecutionTimeMs  int32               `json:"execution_time_ms"`
	MemoryUsedKb     int32               `json:"memory_used_kb"`
	CompilerOutput   string              `json:"compiler_output"`
	ProcessedAt      time.Time           `json:"processed_at"`
	WorkerID         string              `json:"worker_id"`
}

type TestCaseResult struct {
	TestOrder      int32               `json:"test_order"`
	Status         TestCaseStatus      `json:"status"`
	ActualOutput   string              `json:"actual_output"`
	ExpectedOutput string              `json:"expected_output"`
	ExecutionTimeMs int32              `json:"execution_time_ms"`
	MemoryUsedKb   int32               `json:"memory_used_kb"`
	ErrorMessage   string              `json:"error_message,omitempty"`
}

type TestCaseStatus string

const (
	TestCaseStatusPassed           TestCaseStatus = "PASSED"
	TestCaseStatusWrongAnswer      TestCaseStatus = "WRONG_ANSWER"
	TestCaseStatusTimeLimitExceeded TestCaseStatus = "TIME_LIMIT_EXCEEDED"
	TestCaseStatusMemoryLimitExceeded TestCaseStatus = "MEMORY_LIMIT_EXCEEDED"
	TestCaseStatusRuntimeError     TestCaseStatus = "RUNTIME_ERROR"
	TestCaseStatusPresentationError TestCaseStatus = "PRESENTATION_ERROR"
)

type WorkerStatus struct {
	WorkerID     string    `json:"worker_id"`
	Status       string    `json:"status"`
	LastPing     time.Time `json:"last_ping"`
	JobsProcessed int64     `json:"jobs_processed"`
	CurrentJobID  *uuid.UUID `json:"current_job_id,omitempty"`
}

type ExecutionMetrics struct {
	TotalJobs       int64   `json:"total_jobs"`
	CompletedJobs   int64   `json:"completed_jobs"`
	FailedJobs      int64   `json:"failed_jobs"`
	AverageExecTime float64 `json:"average_exec_time_ms"`
	ActiveWorkers   int32   `json:"active_workers"`
}
