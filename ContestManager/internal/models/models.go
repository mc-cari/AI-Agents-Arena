package models

import (
	"time"

	"github.com/google/uuid"
)

type ContestState string

const (
	ContestStateRunning  ContestState = "RUNNING"
	ContestStateFinished ContestState = "FINISHED"
)

type Language int32

const (
	LanguageUnspecified Language = 0
	LanguageCPP        Language = 1
	LanguagePython     Language = 2
)

type SubmissionStatus string

const (
	SubmissionStatusPending             SubmissionStatus = "PENDING"
	SubmissionStatusCompiling           SubmissionStatus = "COMPILING"
	SubmissionStatusRunning             SubmissionStatus = "RUNNING"
	SubmissionStatusAccepted            SubmissionStatus = "ACCEPTED"
	SubmissionStatusWrongAnswer         SubmissionStatus = "WRONG_ANSWER"
	SubmissionStatusPresentationError   SubmissionStatus = "PRESENTATION_ERROR"
	SubmissionStatusTimeLimitExceeded   SubmissionStatus = "TIME_LIMIT_EXCEEDED"
	SubmissionStatusMemoryLimitExceeded SubmissionStatus = "MEMORY_LIMIT_EXCEEDED"
	SubmissionStatusRuntimeError        SubmissionStatus = "RUNTIME_ERROR"
	SubmissionStatusCompilationError    SubmissionStatus = "COMPILATION_ERROR"
	SubmissionStatusOutputLimitExceeded SubmissionStatus = "OUTPUT_LIMIT_EXCEEDED"
	SubmissionStatusJudgementFailed     SubmissionStatus = "JUDGEMENT_FAILED"
)

type ProblemTag string

const (
	ProblemTagStrings            ProblemTag = "STRINGS"
	ProblemTagGeometry           ProblemTag = "GEOMETRY"
	ProblemTagDynamicProgramming ProblemTag = "DYNAMIC_PROGRAMMING"
	ProblemTagGraphs            ProblemTag = "GRAPHS"
	ProblemTagGreedy            ProblemTag = "GREEDY"
	ProblemTagMath               ProblemTag = "MATH"
	ProblemTagDataStructures     ProblemTag = "DATA_STRUCTURES"
	ProblemTagSorting            ProblemTag = "SORTING"
	ProblemTagBinarySearch       ProblemTag = "BINARY_SEARCH"
	ProblemTagTwoPointers        ProblemTag = "TWO_POINTERS"
	ProblemTagSlidingWindow      ProblemTag = "SLIDING_WINDOW"
	ProblemTagBacktracking       ProblemTag = "BACKTRACKING"
	ProblemTagBitManipulation    ProblemTag = "BIT_MANIPULATION"
	ProblemTagTree               ProblemTag = "TREE"
	ProblemTagHeap               ProblemTag = "HEAP"
	ProblemTagStack              ProblemTag = "STACK"
	ProblemTagQueue              ProblemTag = "QUEUE"
	ProblemTagHashTable          ProblemTag = "HASH_TABLE"
	ProblemTagArray              ProblemTag = "ARRAY"
	ProblemTagLinkedList         ProblemTag = "LINKED_LIST"
	ProblemTagRecursion          ProblemTag = "RECURSION"
	ProblemTagDivideAndConquer   ProblemTag = "DIVIDE_AND_CONQUER"
	ProblemTagSimulation         ProblemTag = "SIMULATION"
	ProblemTagImplementation     ProblemTag = "IMPLEMENTATION"
	ProblemTagBruteForce         ProblemTag = "BRUTE_FORCE"
)

type ProblemStatus string

const (
	ProblemStatusAccepted ProblemStatus = "ACCEPTED"
	ProblemStatusTried    ProblemStatus = "TRIED"
	ProblemStatusNonTried ProblemStatus = "NON_TRIED"
)

type Problem struct {
	ID            uuid.UUID  `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	Name          string     `json:"name" gorm:"size:255;not null"`
	Description   string     `json:"description" gorm:"type:text;not null"`
	TimeLimitMs   int32      `json:"time_limit_ms" gorm:"not null;default:1000"`
	MemoryLimitMb int32      `json:"memory_limit_mb" gorm:"not null;default:256"`
	Tag           ProblemTag `json:"tag" gorm:"type:problem_tag;not null;default:'IMPLEMENTATION'"`
  Source        string     `json:"source" gorm:"size:255;not null;default:'UNKNOWN'"`
	CreatedAt     time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	TestCases     []TestCase `json:"test_cases,omitempty" gorm:"foreignKey:ProblemID;constraint:OnDelete:CASCADE"`
}

type Contest struct {
	ID           uuid.UUID     `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	State        ContestState  `json:"state" gorm:"size:20;not null;default:'RUNNING'"`
	StartedAt    time.Time     `json:"started_at" gorm:"not null"`
	EndsAt       time.Time     `json:"ends_at" gorm:"not null"`
	NumProblems  int32         `json:"num_problems" gorm:"not null"`
	CreatedAt    time.Time     `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time     `json:"updated_at" gorm:"autoUpdateTime"`
	Problems     []Problem     `json:"problems,omitempty" gorm:"many2many:contest_problems;joinForeignKey:ContestID;joinReferences:ProblemID;"`
	Participants []Participant `json:"participants,omitempty" gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE"`
}

type Participant struct {
	ID                  uuid.UUID       `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	ContestID           uuid.UUID       `json:"contest_id" gorm:"type:uuid;not null"`
	ModelName           string          `json:"model_name" gorm:"size:100;not null"`
	Solved              int32           `json:"solved" gorm:"default:0"`
	TotalPenaltySeconds int32           `json:"total_penalty_seconds" gorm:"default:0"`
	CreatedAt           time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
	Contest             Contest         `json:"contest,omitempty" gorm:"foreignKey:ContestID"`
	ProblemResults      []ProblemResult `json:"problem_results,omitempty" gorm:"foreignKey:ParticipantID;constraint:OnDelete:CASCADE"`
}

type Submission struct {
	ID                 uuid.UUID        `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	ContestID          uuid.UUID        `json:"contest_id" gorm:"type:uuid;not null"`
	ParticipantID      uuid.UUID        `json:"participant_id" gorm:"type:uuid;not null"`
	ProblemID          uuid.UUID        `json:"problem_id" gorm:"type:uuid;not null"`
	Code               string           `json:"code" gorm:"type:text;not null"`
	Language           Language         `json:"language" gorm:"type:smallint;not null"`
	Status             SubmissionStatus `json:"status" gorm:"size:30;not null;default:'PENDING'"`
	VerdictMessage     string           `json:"verdict_message" gorm:"type:text"`
	TotalTestCases     int32            `json:"total_test_cases" gorm:"default:0"`
	ProcessedTestCases int32            `json:"processed_test_cases" gorm:"default:0"`
	SubmittedAt        time.Time        `json:"submitted_at" gorm:"autoCreateTime"`
	JudgedAt           *time.Time       `json:"judged_at"`
	Contest            Contest          `json:"contest,omitempty" gorm:"foreignKey:ContestID"`
	Participant        Participant      `json:"participant,omitempty" gorm:"foreignKey:ParticipantID"`
	Problem            Problem          `json:"problem,omitempty" gorm:"foreignKey:ProblemID"`
}

type TestCase struct {
	ID             uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	ProblemID      uuid.UUID `json:"problem_id" gorm:"type:uuid;not null"`
	Input          string    `json:"input" gorm:"type:text;not null"`
	ExpectedOutput string    `json:"expected_output" gorm:"type:text;not null"`
	TestOrder      int32     `json:"test_order" gorm:"not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	Problem        Problem   `json:"problem,omitempty" gorm:"foreignKey:ProblemID"`
}

type ProblemResult struct {
	ParticipantID  uuid.UUID     `json:"participant_id" gorm:"type:uuid;primaryKey"`
	ProblemID      uuid.UUID     `json:"problem_id" gorm:"type:uuid;primaryKey"`
	Status         ProblemStatus `json:"status" gorm:"size:20;not null;default:'NON_TRIED'"`
	PenaltyCount   int32         `json:"penalty_count" gorm:"default:0"`
	PenaltySeconds int32         `json:"penalty_seconds" gorm:"default:0"`
	Participant    Participant   `json:"participant,omitempty" gorm:"foreignKey:ParticipantID"`
	Problem        Problem       `json:"problem,omitempty" gorm:"foreignKey:ProblemID"`
}

type ContestProblem struct {
	ContestID    uuid.UUID `json:"contest_id" gorm:"type:uuid;primaryKey"`
	ProblemID    uuid.UUID `json:"problem_id" gorm:"type:uuid;primaryKey"`
	ProblemOrder int32     `json:"problem_order" gorm:"not null"`
}

func (ContestProblem) TableName() string {
	return "contest_problems"
}
