package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"contestmanager/internal/database"
	"contestmanager/internal/models"

	"github.com/google/uuid"
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

type AtCoderMetadata struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Constraints   string `json:"constraints"`
	TimeLimitMs   int32  `json:"time_limit_ms"`
	MemoryLimitMb int32  `json:"memory_limit_mb"`
	SourceURL     string `json:"source_url"`
	ContestID     string `json:"contest_id"`
	ProblemLetter string `json:"problem_letter"`
	Tag           string `json:"tag,omitempty"`
}

type AtCoderTestCase struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	TestOrder      int32  `json:"test_order"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <problem_folder_path>")
		fmt.Println("Example: go run main.go /path/to/ContestManager/data/coffee")
		os.Exit(1)
	}

	problemPath := os.Args[1]
	config := database.Config{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		User:     getEnvOrDefault("DB_USER", "postgres"),
		Password: getEnvOrDefault("DB_PASSWORD", "password"),
		DBName:   getEnvOrDefault("DB_NAME", "contestmanager"),
		SSLMode:  getEnvOrDefault("DB_SSLMODE", "disable"),
	}

	db, err := database.NewGormConnection(config)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run auto-migration: %v", err)
	}
	if err := importProblem(problemPath, db); err != nil {
		log.Fatalf("Failed to import problem: %v", err)
	}

	fmt.Printf("Successfully imported problem from %s\n", problemPath)
}

func importProblem(problemPath string, db *database.GormDB) error {
	metadataPath := filepath.Join(problemPath, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata.json: %v", err)
	}

	var metadata AtCoderMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata.json: %v", err)
	}

	testCasesPath := filepath.Join(problemPath, "test_cases.json")
	testCasesData, err := os.ReadFile(testCasesPath)
	if err != nil {
		return fmt.Errorf("failed to read test_cases.json: %v", err)
	}

	var testCases []AtCoderTestCase
	if err := json.Unmarshal(testCasesData, &testCases); err != nil {
		return fmt.Errorf("failed to parse test_cases.json: %v", err)
	}

	problemTag := models.ProblemTagImplementation
	if metadata.Tag != "" {
		switch strings.ToUpper(metadata.Tag) {
		case "ARRAY":
			problemTag = models.ProblemTagArray
		case "STRINGS":
			problemTag = models.ProblemTagStrings
		case "MATH":
			problemTag = models.ProblemTagMath
		case "DYNAMIC_PROGRAMMING", "DP":
			problemTag = models.ProblemTagDynamicProgramming
		case "GREEDY":
			problemTag = models.ProblemTagGreedy
		case "GRAPH":
			problemTag = models.ProblemTagGraphs
		case "TREE":
			problemTag = models.ProblemTagTree
		case "LINKED_LIST":
			problemTag = models.ProblemTagLinkedList
		case "STACK":
			problemTag = models.ProblemTagStack
		case "QUEUE":
			problemTag = models.ProblemTagQueue
		case "HEAP":
			problemTag = models.ProblemTagHeap
		case "HASH_TABLE":
			problemTag = models.ProblemTagHashTable
		case "TWO_POINTERS":
			problemTag = models.ProblemTagTwoPointers
		case "SLIDING_WINDOW":
			problemTag = models.ProblemTagSlidingWindow
		case "BINARY_SEARCH":
			problemTag = models.ProblemTagBinarySearch
		case "BACKTRACKING":
			problemTag = models.ProblemTagBacktracking
		case "GEOMETRY":
			problemTag = models.ProblemTagGeometry
		case "BIT_MANIPULATION":
			problemTag = models.ProblemTagBitManipulation
		case "SORTING":
			problemTag = models.ProblemTagSorting
		case "SIMULATION":
			problemTag = models.ProblemTagSimulation
		case "DATA_STRUCTURES":
			problemTag = models.ProblemTagDataStructures
		case "BRUTE_FORCE":
			problemTag = models.ProblemTagBruteForce
		}
	}

	problem := &models.Problem{
		ID:            uuid.New(),
		Name:          metadata.Name,
		Description:   metadata.Description,
		TimeLimitMs:   metadata.TimeLimitMs,
		MemoryLimitMb: metadata.MemoryLimitMb,
		Tag:           problemTag,
	}

	if err := db.DB.Create(&problem).Error; err != nil {
		return fmt.Errorf("failed to create problem: %v", err)
	}

	fmt.Printf("Created problem: %s (ID: %s)\n", problem.Name, problem.ID.String())

	for _, tc := range testCases {
		testCase := &models.TestCase{
			ID:             uuid.New(),
			ProblemID:      problem.ID,
			Input:          tc.Input,
			ExpectedOutput: tc.ExpectedOutput,
			TestOrder:      tc.TestOrder,
		}

		if err := db.DB.Create(&testCase).Error; err != nil {
			return fmt.Errorf("failed to create test case %d: %v", tc.TestOrder, err)
		}
	}

	fmt.Printf("Created %d test cases for problem %s\n", len(testCases), problem.Name)
	return nil
}
