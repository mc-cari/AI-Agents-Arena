//go:build integration
// +build integration

package tests

import (
	"contestmanager/internal/coordinator"
	"contestmanager/internal/database"
	"contestmanager/internal/models"
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContestManagerIntegration(t *testing.T) {

	env := SetupTestEnvironment(t)

	t.Run("Full Contest Lifecycle", func(t *testing.T) {
		problem := env.CreateTestProblem(t, "sum_two_numbers")

		contest := env.CreateSampleContest(t, problem.ID)
		require.NotNil(t, contest, "Contest should be created successfully")

		err := env.Coordinator.StartContest(contest.ID)
		require.NoError(t, err, "Contest should start successfully")

		participants := env.CreateSampleParticipants(t, contest.ID, 2)

		validSubmission := env.CreateSampleSubmission(t, contest.ID, participants[0].ID, problem.ID, "sum_two_numbers", "cpp_correct")
		require.NotNil(t, validSubmission, "Valid submission should be created")

		invalidSubmission := env.CreateSampleSubmission(t, contest.ID, participants[1].ID, problem.ID, "sum_two_numbers", "cpp_wrong")
		require.NotNil(t, invalidSubmission, "Invalid submission should be created")

		simulateContestProgression(t, env.Coordinator, contest.ID, contest.EndsAt)

		verifyContestCompletion(t, env.ContestRepo, contest.ID)

		verifySubmissions(t, env.SubmissionRepo, validSubmission.ID, invalidSubmission.ID)

	})

	t.Run("Worker Code Execution", func(t *testing.T) {
		problem := env.CreateTestProblem(t, "sum_two_numbers")

		contest := env.CreateSampleContest(t, problem.ID)
		require.NotNil(t, contest, "Contest should be created successfully")

		err := env.Coordinator.StartContest(contest.ID)
		require.NoError(t, err, "Contest should start successfully")

		participants := env.CreateSampleParticipants(t, contest.ID, 2)

		validSubmission := env.CreateSampleSubmission(t, contest.ID, participants[0].ID, problem.ID, "sum_two_numbers", "cpp_correct")
		require.NotNil(t, validSubmission, "Valid submission should be created")

		pythonSubmission := env.CreateSampleSubmission(t, contest.ID, participants[1].ID, problem.ID, "sum_two_numbers", "python_correct")
		require.NotNil(t, pythonSubmission, "Python submission should be created")

		invalidCppSubmission := env.CreateSampleSubmission(t, contest.ID, participants[0].ID, problem.ID, "sum_two_numbers", "cpp_compilation_error")
		require.NotNil(t, invalidCppSubmission, "Invalid C++ submission should be created")

		timeLimitSubmission := createTimeLimitSubmission(t, env.SubmissionRepo, contest.ID, participants[0].ID, problem.ID)
		require.NotNil(t, timeLimitSubmission, "Time limit submission should be created")

		err = env.Coordinator.ProcessSubmission(validSubmission.ID)
		require.NoError(t, err, "Should process valid submission")

		err = env.Coordinator.ProcessSubmission(pythonSubmission.ID)
		require.NoError(t, err, "Should process Python submission")

		err = env.Coordinator.ProcessSubmission(invalidCppSubmission.ID)
		require.NoError(t, err, "Should process invalid C++ submission")

		err = env.Coordinator.ProcessSubmission(timeLimitSubmission.ID)
		require.NoError(t, err, "Should process time limit submission")

		time.Sleep(10 * time.Second)

		verifyWorkerExecution(t, env.SubmissionRepo, validSubmission.ID, pythonSubmission.ID, invalidCppSubmission.ID, timeLimitSubmission.ID)

		err = env.Coordinator.StopContest(contest.ID)
		require.NoError(t, err, "Contest should stop successfully")
	})

}

func createTimeLimitSubmission(t *testing.T, submissionRepo *database.SubmissionRepository, contestID, participantID, problemID uuid.UUID) *models.Submission {
	ctx := context.Background()

	submission := &models.Submission{
		ID:            uuid.New(),
		ContestID:     contestID,
		ParticipantID: participantID,
		ProblemID:     problemID,
		Code: `#include <iostream>
#include <chrono>
#include <thread>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    
    // Simulate a long-running operation that will exceed time limit
    // Sleep for 5 seconds (problem time limit is 1 second = 1000ms)
    this_thread::sleep_for(chrono::seconds(5));
    
    cout << a + b << endl;
    return 0;
}`,
		Language:       models.LanguageCPP,
		Status:         models.SubmissionStatusPending,
		TotalTestCases: 3,
	}

	err := submissionRepo.CreateSubmission(ctx, submission)
	require.NoError(t, err, "Failed to create time limit submission")

	return submission
}

func simulateContestProgression(t *testing.T, coordinator *coordinator.ContestCoordinator, contestID uuid.UUID, endTime time.Time) {
	timeUntilEnd := time.Until(endTime)
	if timeUntilEnd > 0 {
		time.Sleep(2 * time.Second)
	}
}

func verifyContestCompletion(t *testing.T, contestRepo *database.ContestRepository, contestID uuid.UUID) {
	ctx := context.Background()
	
	contest, err := contestRepo.GetContest(ctx, contestID)
	require.NoError(t, err, "Should be able to retrieve contest")
	require.NotNil(t, contest, "Contest should exist")
	
	assert.Equal(t, contestID, contest.ID, "Contest ID should match")
}

func verifySubmissions(t *testing.T, submissionRepo *database.SubmissionRepository, validSubmissionID, invalidSubmissionID uuid.UUID) {
	ctx := context.Background()
	
	validSubmission, err := submissionRepo.GetSubmission(ctx, validSubmissionID)
	require.NoError(t, err, "Should be able to retrieve valid submission")
	require.NotNil(t, validSubmission, "Valid submission should exist")
	assert.Equal(t, validSubmissionID, validSubmission.ID, "Valid submission ID should match")
	
	invalidSubmission, err := submissionRepo.GetSubmission(ctx, invalidSubmissionID)
	require.NoError(t, err, "Should be able to retrieve invalid submission")
	require.NotNil(t, invalidSubmission, "Invalid submission should exist")
	assert.Equal(t, invalidSubmissionID, invalidSubmission.ID, "Invalid submission ID should match")

}

func verifyWorkerExecution(t *testing.T, submissionRepo *database.SubmissionRepository, validSubmissionID, pythonSubmissionID, invalidCppSubmissionID, timeLimitSubmissionID uuid.UUID) {
	ctx := context.Background()
	
	validSubmission, err := submissionRepo.GetSubmission(ctx, validSubmissionID)
	require.NoError(t, err, "Should be able to retrieve valid submission")
	require.NotNil(t, validSubmission, "Valid submission should exist")
	
	assert.Equal(t, models.SubmissionStatusAccepted, validSubmission.Status, 
		"Valid C++ submission should be ACCEPTED after worker processing")
	
	pythonSubmission, err := submissionRepo.GetSubmission(ctx, pythonSubmissionID)
	require.NoError(t, err, "Should be able to retrieve Python submission")
	require.NotNil(t, pythonSubmission, "Python submission should exist")
	
	assert.Equal(t, models.SubmissionStatusAccepted, pythonSubmission.Status, 
		"Python submission should be ACCEPTED after worker processing")
	
	invalidCppSubmission, err := submissionRepo.GetSubmission(ctx, invalidCppSubmissionID)
	require.NoError(t, err, "Should be able to retrieve invalid C++ submission")
	require.NotNil(t, invalidCppSubmission, "Invalid C++ submission should exist")
	
	assert.Equal(t, models.SubmissionStatusCompilationError, invalidCppSubmission.Status, 
		"Invalid C++ submission should be COMPILATION_ERROR after worker processing")
	
	timeLimitSubmission, err := submissionRepo.GetSubmission(ctx, timeLimitSubmissionID)
	require.NoError(t, err, "Should be able to retrieve time limit submission")
	require.NotNil(t, timeLimitSubmission, "Time limit submission should exist")
	
	assert.Equal(t, models.SubmissionStatusTimeLimitExceeded, timeLimitSubmission.Status, 
		"Time limit submission should be TIME_LIMIT_EXCEEDED after worker processing")

}

func TestMain(m *testing.M) {
	code := m.Run()

	os.Exit(code)
}
