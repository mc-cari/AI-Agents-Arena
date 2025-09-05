package types

import (
	"time"

	"github.com/google/uuid"
)

// APIError represents a standard API error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Error implements the error interface
func (e APIError) Error() string {
	return e.Message
}

// ContestCreateRequest represents a request to create a contest
type ContestCreateRequest struct {
	NumProblems       int      `json:"num_problems" validate:"min=2,max=10"`
	ParticipantModels []string `json:"participant_models" validate:"min=2"`
}

// ContestResponse represents a contest response
type ContestResponse struct {
	ID           uuid.UUID             `json:"id"`
	State        string                `json:"state"`
	StartedAt    time.Time             `json:"started_at"`
	EndsAt       time.Time             `json:"ends_at"`
	Problems     []ProblemResponse     `json:"problems"`
	Participants []ParticipantResponse `json:"participants"`
}

// ProblemResponse represents a problem response
type ProblemResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	TimeLimitSec   int32     `json:"time_limit_sec"`
	MemoryLimitMiB int32     `json:"memory_limit_mib"`
}

// ParticipantResponse represents a participant response
type ParticipantResponse struct {
	ID                  uuid.UUID                        `json:"id"`
	ModelName           string                           `json:"model_name"`
	Solved              int32                            `json:"solved"`
	TotalPenaltySeconds int32                            `json:"total_penalty_seconds"`
	ProblemResults      map[string]ProblemResultResponse `json:"problem_results"`
}

// ProblemResultResponse represents a problem result response
type ProblemResultResponse struct {
	Status         string `json:"status"`
	PenaltyCount   int32  `json:"penalty_count"`
	PenaltySeconds int32  `json:"penalty_seconds"`
}

// SubmissionRequest represents a submission request
type SubmissionRequest struct {
	ContestID     uuid.UUID `json:"contest_id" validate:"required"`
	ParticipantID uuid.UUID `json:"participant_id" validate:"required"`
	ProblemID     uuid.UUID `json:"problem_id" validate:"required"`
	Code          string    `json:"code" validate:"required"`
	Language      string    `json:"language" validate:"required"`
}

// SubmissionResponse represents a submission response
type SubmissionResponse struct {
	ID             uuid.UUID  `json:"id"`
	ContestID      uuid.UUID  `json:"contest_id"`
	ParticipantID  uuid.UUID  `json:"participant_id"`
	ProblemID      uuid.UUID  `json:"problem_id"`
	Code           string     `json:"code"`
	Language       string     `json:"language"`
	Status         string     `json:"status"`
	VerdictMessage string     `json:"verdict_message"`
	SubmittedAt    time.Time  `json:"submitted_at"`
	JudgedAt       *time.Time `json:"judged_at"`
}

// LeaderboardResponse represents a leaderboard response
type LeaderboardResponse struct {
	Participants []ParticipantResponse `json:"participants"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string      `json:"type"`
	ContestID uuid.UUID   `json:"contest_id"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// Constants for WebSocket message types
const (
	MessageTypeLeaderboardUpdate = "leaderboard_update"
	MessageTypeSubmissionUpdate  = "submission_update"
	MessageTypeContestEnd        = "contest_end"
)
