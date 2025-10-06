package tests

import (
	"context"
	"testing"

	grpcpb "contestmanager/api/grpc"
	"contestmanager/internal/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGRPCServerContestAndSubmission(t *testing.T) {
	env := SetupTestEnvironment(t)

	env.SetupGRPCServer(t, "50052")
	env.SetupGRPCClient(t, "50052")

	t.Run("Create Contest", func(t *testing.T) {
		ctx := context.Background()
		
		problem := env.CreateTestProblem(t, "sum_two_numbers")
		require.NotNil(t, problem)
		
		req := &grpcpb.CreateContestRequest{
			NumProblems:      1,
			ParticipantModels: []string{"test-agent"},
		}

		resp, err := env.GRPCClient.CreateContest(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Contest.Id)

		contestID := resp.Contest.Id
		t.Logf("Created contest with ID: %s", contestID)

		t.Run("Create Submission", func(t *testing.T) {
			participantID := ""
			for _, p := range resp.Contest.Participants {
				participantID = p.Id
				break
			}
			require.NotEmpty(t, participantID)

			problemID := ""
			for _, p := range resp.Contest.Problems {
				problemID = p.Id
				break
			}
			require.NotEmpty(t, problemID)

			submitReq := &grpcpb.SubmitSolutionRequest{
				ContestId:    contestID,
				ParticipantId: participantID,
				ProblemId:    problemID,
				Code:         "def solve():\n    return 'test'",
				Language:     grpcpb.Language_LANGUAGE_PYTHON,
			}

			submitResp, err := env.GRPCClient.SubmitSolution(ctx, submitReq)
			require.NoError(t, err)
			require.NotNil(t, submitResp)
			require.NotEmpty(t, submitResp.SubmissionId)

			t.Logf("Created submission with ID: %s", submitResp.SubmissionId)

			t.Run("Get Submissions", func(t *testing.T) {
				getSubmissionsReq := &grpcpb.GetSubmissionsRequest{
					ContestId:    contestID,
					ParticipantId: participantID,
					ProblemId:    problemID,
				}

				getSubmissionsResp, err := env.GRPCClient.GetSubmissions(ctx, getSubmissionsReq)
				require.NoError(t, err)
				require.NotNil(t, getSubmissionsResp)
				require.NotEmpty(t, getSubmissionsResp.Submissions)

				t.Logf("Found %d submissions", len(getSubmissionsResp.Submissions))

				submission := getSubmissionsResp.Submissions[0]
				assert.Equal(t, contestID, submission.ContestId)
				assert.Equal(t, participantID, submission.ParticipantId)
				assert.Equal(t, problemID, submission.ProblemId)
				assert.Equal(t, "def solve():\n    return 'test'", submission.Code)
				assert.Equal(t, grpcpb.Language_LANGUAGE_PYTHON, submission.Language)

				t.Logf("Submission verification successful")
			})
		})
	})
}

func TestGRPCServerWithSampleData(t *testing.T) {
	
	env := SetupTestEnvironment(t)

	env.SetupGRPCServer(t, "50053")
	env.SetupGRPCClient(t, "50053")

			t.Run("Test with Sample Problem and Contest", func(t *testing.T) {
		ctx := context.Background()
		
		problem := env.CreateTestProblem(t, "coffee")
		require.NotNil(t, problem)

		contest := env.CreateSampleContest(t, problem.ID)
		require.NotNil(t, contest)

		participants, err := env.ParticipantRepo.GetParticipantsByContest(context.Background(), contest.ID)
		require.NoError(t, err, "Failed to get participants")
		require.Len(t, participants, 2, "Should have 2 participants from contest creation")

		cppSubmission := env.CreateSampleSubmission(t, contest.ID, participants[0].ID, problem.ID, "coffee", "cpp_correct")
		require.NotNil(t, cppSubmission)

		pythonSubmission := env.CreateSampleSubmission(t, contest.ID, participants[1].ID, problem.ID, "coffee", "python_correct")
		require.NotNil(t, pythonSubmission)

		getSubmissionsReq := &grpcpb.GetSubmissionsRequest{
			ContestId:    contest.ID.String(),
			ParticipantId: participants[0].ID.String(),
			ProblemId:    problem.ID.String(),
		}

		getSubmissionsResp, err := env.GRPCClient.GetSubmissions(ctx, getSubmissionsReq)
		require.NoError(t, err)
		require.NotNil(t, getSubmissionsResp)
		require.NotEmpty(t, getSubmissionsResp.Submissions)

		t.Logf("Found %d submissions for participant %s", len(getSubmissionsResp.Submissions), participants[0].ID.String())

		found := false
		for _, sub := range getSubmissionsResp.Submissions {
			if sub.Id == cppSubmission.ID.String() {
				assert.Equal(t, contest.ID.String(), sub.ContestId)
				assert.Equal(t, participants[0].ID.String(), sub.ParticipantId)
				assert.Equal(t, problem.ID.String(), sub.ProblemId)
				assert.Equal(t, grpcpb.Language_LANGUAGE_CPP, sub.Language)
				found = true
				break
			}
		}
		require.True(t, found, "C++ submission should be found in gRPC response")

		t.Logf("gRPC submission retrieval test successful")
	})
}

func TestContestCreationDataSetup(t *testing.T) {
	env := SetupTestEnvironment(t)

	t.Run("Verify_Contest_Creation_Data_Setup", func(t *testing.T) {

		env.SetupGRPCServer(t, "50054")
		env.SetupGRPCClient(t, "50054")

		req := &grpcpb.CreateContestRequest{
			NumProblems:      1,
			ParticipantModels: []string{"test-agent"},
		}

		resp, err := env.GRPCClient.CreateContest(context.Background(), req)
		require.NoError(t, err, "Failed to create contest")
		require.NotNil(t, resp.Contest, "Contest response should not be nil")

		contestID := resp.Contest.Id
		t.Logf("Created contest with ID: %s", contestID)

		contest, err := env.ContestRepo.GetContest(context.Background(), uuid.MustParse(contestID))
		require.NoError(t, err, "Failed to get contest from database")
		require.NotNil(t, contest, "Contest should exist in database")
		require.Equal(t, models.ContestStateRunning, contest.State, "Contest should be in RUNNING state")

		require.Len(t, contest.Problems, 1, "Contest should have exactly 1 problem")
		contestProblem := contest.Problems[0]
		require.NotNil(t, contestProblem, "Contest should have a problem")

		require.Len(t, contest.Participants, 1, "Contest should have exactly 1 participant")
		participant := contest.Participants[0]
		require.NotEmpty(t, participant.ModelName, "Participant should have a model name")

		require.Len(t, participant.ProblemResults, 1, "Participant should have exactly 1 problem result")
		problemResult := participant.ProblemResults[0]
		require.Equal(t, contestProblem.ID, problemResult.ProblemID, "Problem result should be for the correct problem")
		require.Equal(t, models.ProblemStatusNonTried, problemResult.Status, "Problem result should be initialized as NonTried")
		require.Equal(t, int32(0), problemResult.PenaltyCount, "Problem result should have 0 penalty count")
		require.Equal(t, int32(0), problemResult.PenaltySeconds, "Problem result should have 0 penalty seconds")

		testCases, err := env.TestCaseRepo.GetTestCasesByProblem(context.Background(), contestProblem.ID)
		require.NoError(t, err, "Failed to get test cases")
		require.Len(t, testCases, 3, "Problem should have exactly 3 test cases")

		require.Equal(t, models.ContestStateRunning, contest.State, "Contest should be in RUNNING state")

		t.Logf("Contest creation data setup verification successful")
	})
}

