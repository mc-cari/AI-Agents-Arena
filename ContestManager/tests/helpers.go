package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	grpcpb "contestmanager/api/grpc"
	"contestmanager/internal/config"
	"contestmanager/internal/coordinator"
	"contestmanager/internal/database"
	"contestmanager/internal/models"
	"contestmanager/internal/services"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type TestConfig struct {
	Database config.DatabaseConfig
	Redis    config.RedisConfig
	Contest  config.ContestConfig
}

type TestEnvironment struct {
	Config            *TestConfig
	DB                *database.GormDB
	RedisClient       *redis.Client
	ContestRepo       *database.ContestRepository
	ProblemRepo       *database.ProblemRepository
	ParticipantRepo   *database.ParticipantRepository
	SubmissionRepo    *database.SubmissionRepository
	ProblemResultRepo *database.ProblemResultRepository
	TestCaseRepo      *database.TestCaseRepository
	Coordinator       *coordinator.ContestCoordinator
	ContestService    *services.ContestService
	GRPCServer        *grpc.Server
	GRPCClient        grpcpb.ContestServiceClient
	GRPCConn          *grpc.ClientConn
}

func NewTestConfig() *TestConfig {
	return &TestConfig{
		Database: config.DatabaseConfig{
			Host:     getEnv("DB_HOST", "postgres-test"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "contestmanager"),
			Password: getEnv("DB_PASSWORD", "contestmanager_password"),
			DBName:   getEnv("DB_NAME", "contestmanager_test"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: config.RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "redis-test:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       1,
		},
		Contest: config.ContestConfig{
			MaxConcurrentContests: 3,
			DurationSeconds:       300,
			MaxTokensPerContest:   200000,
		},
	}
}

func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	cfg := NewTestConfig()

	db, err := database.NewGormConnection(database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	require.NoError(t, err, "Failed to connect to test database")

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx := context.Background()
	_, err = redisClient.Ping(ctx).Result()
	require.NoError(t, err, "Failed to connect to Redis")

	contestRepo := database.NewContestRepository(db)
	problemRepo := database.NewProblemRepository(db)
	participantRepo := database.NewParticipantRepository(db)
	submissionRepo := database.NewSubmissionRepository(db)
	problemResultRepo := database.NewProblemResultRepository(db)
	testCaseRepo := database.NewTestCaseRepository(db)

	coord := coordinator.NewContestCoordinator(
		cfg.Contest.MaxConcurrentContests,
		contestRepo,
		submissionRepo,
		participantRepo,
		problemResultRepo,
		testCaseRepo,
		redisClient,
	)

	configConfig := &config.Config{
		Database: cfg.Database,
		Redis:    cfg.Redis,
		Contest:  cfg.Contest,
	}

	contestService := services.NewContestService(
		contestRepo,
		problemRepo,
		participantRepo,
		submissionRepo,
		problemResultRepo,
		testCaseRepo,
		coord,
		configConfig,
	)

	err = db.AutoMigrate()
	require.NoError(t, err, "Failed to run database migrations")

	return &TestEnvironment{
		Config:            cfg,
		DB:                db,
		RedisClient:       redisClient,
		ContestRepo:       contestRepo,
		ProblemRepo:       problemRepo,
		ParticipantRepo:   participantRepo,
		SubmissionRepo:    submissionRepo,
		ProblemResultRepo: problemResultRepo,
		TestCaseRepo:      testCaseRepo,
		Coordinator:       coord,
		ContestService:    contestService,
	}
}

func (env *TestEnvironment) SetupGRPCServer(t *testing.T, port string) {
	env.GRPCServer = grpc.NewServer()
	grpcpb.RegisterContestServiceServer(env.GRPCServer, env.ContestService)

	go func() {
		lis, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}
	
		if err := env.GRPCServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	time.Sleep(2 * time.Second)
}

func (env *TestEnvironment) SetupGRPCClient(t *testing.T, port string) {
	conn, err := grpc.NewClient("localhost:"+port, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "Failed to connect to gRPC server")

	env.GRPCConn = conn
	env.GRPCClient = grpcpb.NewContestServiceClient(conn)
}

func (env *TestEnvironment) CleanupTestEnvironment() {
	if env.GRPCServer != nil {
		env.GRPCServer.Stop()
	}
	if env.GRPCConn != nil {
		env.GRPCConn.Close()
	}
	if env.RedisClient != nil {
		env.RedisClient.Close()
	}
	if env.DB != nil {
		env.DB.Close()
	}
}

func (env *TestEnvironment) CreateTestProblem(t *testing.T, problemKey string) *models.Problem {
	ctx := context.Background()
	
	problemTemplate, exists := SampleProblems[problemKey]
	require.True(t, exists, "Problem key %s not found", problemKey)

	problem := &models.Problem{
		ID:            uuid.New(),
		Name:          problemTemplate.Name,
		Description:   problemTemplate.Description,
		TimeLimitMs:   problemTemplate.TimeLimitMs,
		MemoryLimitMb: problemTemplate.MemoryLimitMb,
		Tag:           problemTemplate.Tag,
	}
	
	err := env.ProblemRepo.CreateProblem(ctx, problem)
	require.NoError(t, err, "Failed to create problem %s", problemKey)
	
	testCases := SampleTestCases[problemKey]
	for i, testCase := range testCases {
		newTestCase := &models.TestCase{
			ID:             uuid.New(),
			ProblemID:      problem.ID,
			Input:          testCase.Input,
			ExpectedOutput: testCase.ExpectedOutput,
			TestOrder:      testCase.TestOrder,
		}
		
		err := env.TestCaseRepo.CreateTestCase(ctx, newTestCase)
		require.NoError(t, err, "Failed to create test case %d for problem %s", i, problemKey)
	}
	
	t.Logf("Created problem %s with ID %s and %d test cases", problemKey, problem.ID, len(testCases))
	return problem
}

func (env *TestEnvironment) CleanupTestData(t *testing.T) {
	ctx := context.Background()
	
	tables := []string{
		"problem_results",
		"submissions", 
		"participants",
		"contest_problems",
		"test_cases",
		"problems",
		"contests",
	}
	
	for _, table := range tables {
		err := env.DB.WithContext(ctx).Exec(fmt.Sprintf("DELETE FROM %s", table)).Error
		require.NoError(t, err, "Failed to cleanup table %s", table)
	}
}

func (env *TestEnvironment) CreateSampleProblem(t *testing.T, problemKey string) *models.Problem {
	ctx := context.Background()
	
	problem, exists := SampleProblems[problemKey]
	require.True(t, exists, "Problem key %s not found", problemKey)

	problem.ID = uuid.New()
	
	err := env.ProblemRepo.CreateProblem(ctx, problem)
	require.NoError(t, err, "Failed to create sample problem")

	testCases, exists := SampleTestCases[problemKey]
	require.True(t, exists, "Test cases for problem key %s not found", problemKey)
	
	for i := range testCases {
		testCases[i].ID = uuid.New()
		testCases[i].ProblemID = problem.ID
		err := env.TestCaseRepo.CreateTestCase(ctx, &testCases[i])
		require.NoError(t, err, "Failed to create test case")
	}

	return problem
}

func (env *TestEnvironment) CreateSampleContest(t *testing.T, problemID uuid.UUID) *models.Contest {
	ctx := context.Background()

	contestID := uuid.New()
	startTime := time.Now()
	duration := 5 * time.Minute
	endTime := startTime.Add(duration)

	contest := &models.Contest{
		ID:          contestID,
		State:       models.ContestStateRunning,
		StartedAt:   startTime,
		EndsAt:      endTime,
		NumProblems: 1,
	}

	err := env.ContestRepo.CreateContest(ctx, contest)
	require.NoError(t, err, "Failed to create sample contest")

	err = env.ProblemRepo.AddProblemToContest(ctx, contestID, problemID)
	require.NoError(t, err, "Failed to add problem to contest")

	return contest
}

func (env *TestEnvironment) CreateSampleParticipants(t *testing.T, contestID uuid.UUID, count int) []models.Participant {
	ctx := context.Background()

	participants := make([]models.Participant, count)
	for i := 0; i < count; i++ {
		participants[i] = models.Participant{
			ID:                  uuid.New(),
			ContestID:           contestID,
			ModelName:           fmt.Sprintf("test-model-%d", i+1),
			Solved:              0,
			TotalPenaltySeconds: 0,
		}

		err := env.ParticipantRepo.CreateParticipant(ctx, &participants[i])
		require.NoError(t, err, "Failed to create sample participant")
	}

	return participants
}

func (env *TestEnvironment) CreateSampleSubmission(t *testing.T, contestID, participantID, problemID uuid.UUID, problemKey, codeKey string) *models.Submission {
	ctx := context.Background()

	code, exists := SampleCode[problemKey][codeKey]
	require.True(t, exists, "Code for problem %s and key %s not found", problemKey, codeKey)

	var language models.Language
	if codeKey == "python_correct" || codeKey == "python_wrong" {
		language = models.LanguagePython
	} else {
		language = models.LanguageCPP
	}

	submission := &models.Submission{
		ID:            uuid.New(),
		ContestID:     contestID,
		ParticipantID: participantID,
		ProblemID:     problemID,
		Code:          code,
		Language:      language,
		Status:        models.SubmissionStatusPending,
		TotalTestCases: 3,
	}

	err := env.SubmissionRepo.CreateSubmission(ctx, submission)
	require.NoError(t, err, "Failed to create sample submission")

	return submission
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

var SampleProblems = map[string]*models.Problem{
	"sum_two_numbers": {
		ID:            uuid.New(),
		Name:          "Sum of Two Numbers",
		Description:   "Given two integers a and b, return their sum.\n\n**Input:** Two integers a and b\n**Output:** The sum of a and b\n\n**Example:**\nInput: 2 3\nOutput: 5",
		TimeLimitMs:   1000,
		MemoryLimitMb: 256,
		Tag:           models.ProblemTagImplementation,
	},
	"coffee": {
		ID:            uuid.New(),
		Name:          "Coffee",
		Description:   "You are given a string of length 6. Check if the 3rd and 4th characters are equal, and if the 5th and 6th characters are equal.\n\n**Input:** A string of length 6\n**Output:** 'YES' if both conditions are met, 'NO' otherwise\n\n**Example:**\nInput: 'coffee'\nOutput: 'YES' (3rd=4th='f', 5th=6th='e')",
		TimeLimitMs:   1000,
		MemoryLimitMb: 256,
		Tag:           models.ProblemTagImplementation,
	},
}

var SampleTestCases = map[string][]models.TestCase{
	"sum_two_numbers": {
		{
			ID:             uuid.New(),
			Input:          "2 3",
			ExpectedOutput: "5",
			TestOrder:      1,
		},
		{
			ID:             uuid.New(),
			Input:          "-1 1",
			ExpectedOutput: "0",
			TestOrder:      2,
		},
		{
			ID:             uuid.New(),
			Input:          "100 200",
			ExpectedOutput: "300",
			TestOrder:      3,
		},
	},
	"coffee": {
		{
			ID:             uuid.New(),
			Input:          "coffee",
			ExpectedOutput: "YES",
			TestOrder:      1,
		},
		{
			ID:             uuid.New(),
			Input:          "abcdef",
			ExpectedOutput: "NO",
			TestOrder:      2,
		},
		{
			ID:             uuid.New(),
			Input:          "aabbaa",
			ExpectedOutput: "YES",
			TestOrder:      3,
		},
	},
	"make_even": {
		{
			ID:             uuid.New(),
			Input:          "1 2 3 4",
			ExpectedOutput: "2",
			TestOrder:      1,
		},
		{
			ID:             uuid.New(),
			Input:          "2 4 6 8",
			ExpectedOutput: "0",
			TestOrder:      2,
		},
		{
			ID:             uuid.New(),
			Input:          "1 3 5 7",
			ExpectedOutput: "4",
			TestOrder:      3,
		},
	},
}

var SampleCode = map[string]map[string]string{
	"sum_two_numbers": {
		"cpp_correct": `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    cout << a + b << endl;
    return 0;
}`,
		"cpp_wrong": `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    cout << a - b << endl;  // Wrong: should be addition
    return 0;
}`,
		"cpp_compilation_error": `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    cout << a + b << endl
    return 0;  // Missing semicolon
}`,
		"python_correct": `def solve():
    a, b = map(int, input().split())
    return str(a + b)

if __name__ == "__main__":
    print(solve())`,
		"python_wrong": `def solve():
    a, b = map(int, input().split())
    return str(a - b)  # Wrong: should be addition

if __name__ == "__main__":
    print(solve())`,
	},
	"coffee": {
		"cpp_correct": `#include <iostream>
#include <string>
using namespace std;

int main() {
    string s;
    cin >> s;
    if (s[2] == s[3] && s[4] == s[5]) {
        cout << "YES" << endl;
    } else {
        cout << "NO" << endl;
    }
    return 0;
}`,
		"python_correct": `def solve():
    s = input()
    if s[2] == s[3] and s[4] == s[5]:
        return "YES"
    else:
        return "NO"

if __name__ == "__main__":
    print(solve())`,
	},
}

func (env *TestEnvironment) ImportProblemFromScript(t *testing.T, problemPath string) *models.Problem {
	absPath, err := filepath.Abs(problemPath)
	require.NoError(t, err, "Failed to get absolute path for %s", problemPath)
	
	_, err = os.Stat(absPath)
	require.NoError(t, err, "Problem directory does not exist: %s", absPath)
	

	importScriptPath, err := filepath.Abs("/app/bin/import-problems")
	require.NoError(t, err, "Failed to get absolute path for import-problems binary")
	

	_, err = os.Stat(importScriptPath)
	require.NoError(t, err, "import-problems binary does not exist: %s", importScriptPath)
	

	envVars := []string{
		fmt.Sprintf("DB_HOST=%s", env.Config.Database.Host),
		fmt.Sprintf("DB_PORT=%s", env.Config.Database.Port),
		fmt.Sprintf("DB_USER=%s", env.Config.Database.User),
		fmt.Sprintf("DB_PASSWORD=%s", env.Config.Database.Password),
		fmt.Sprintf("DB_NAME=%s", env.Config.Database.DBName),
		fmt.Sprintf("DB_SSLMODE=%s", env.Config.Database.SSLMode),
	}
	

	cmd := exec.Command(importScriptPath, absPath)
	cmd.Env = append(os.Environ(), envVars...)
	
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to run import-problems script: %s\nOutput: %s", err, string(output))
	
	t.Logf("Import-problems output: %s", string(output))
	

	metadataPath := filepath.Join(absPath, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	require.NoError(t, err, "Failed to read metadata.json")
	

	var metadata struct {
		Name string `json:"name"`
	}
	err = json.Unmarshal(metadataData, &metadata)
	require.NoError(t, err, "Failed to parse metadata.json")
	

	ctx := context.Background()
	var problems []models.Problem
	err = env.DB.DB.WithContext(ctx).Find(&problems).Error
	require.NoError(t, err, "Failed to get all problems")
	
	var importedProblem *models.Problem
	for _, problem := range problems {
		if problem.Name == metadata.Name {
			importedProblem = &problem
			break
		}
	}
	
	require.NotNil(t, importedProblem, "Failed to find imported problem with name: %s", metadata.Name)
	t.Logf("Successfully imported problem: %s (ID: %s)", importedProblem.Name, importedProblem.ID)
	
	return importedProblem
}
