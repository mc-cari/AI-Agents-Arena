package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"contestmanager/api/grpc"
	"contestmanager/internal/coordinator"
	"contestmanager/internal/database"
	"contestmanager/internal/models"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ContestService struct {
	grpc.UnimplementedContestServiceServer
	contestRepo       *database.ContestRepository
	problemRepo       *database.ProblemRepository
	participantRepo   *database.ParticipantRepository
	submissionRepo    *database.SubmissionRepository
	problemResultRepo *database.ProblemResultRepository
	testCaseRepo      *database.TestCaseRepository
	coordinator       *coordinator.ContestCoordinator
}

func NewContestService(
	contestRepo *database.ContestRepository,
	problemRepo *database.ProblemRepository,
	participantRepo *database.ParticipantRepository,
	submissionRepo *database.SubmissionRepository,
	problemResultRepo *database.ProblemResultRepository,
	testCaseRepo *database.TestCaseRepository,
	coordinator *coordinator.ContestCoordinator,
) *ContestService {
	return &ContestService{
		contestRepo:       contestRepo,
		problemRepo:       problemRepo,
		participantRepo:   participantRepo,
		submissionRepo:    submissionRepo,
		problemResultRepo: problemResultRepo,
		testCaseRepo:      testCaseRepo,
		coordinator:       coordinator,
	}
}

func (cs *ContestService) CreateContest(ctx context.Context, req *grpc.CreateContestRequest) (*grpc.CreateContestResponse, error) {


	if req.NumProblems <= 0 {
		return nil, fmt.Errorf("invalid num_problems: %d", req.NumProblems)
	}
	if len(req.ParticipantModels) == 0 {
		return nil, fmt.Errorf("no participant models provided")
	}

	startTime := time.Now()
	endTime := startTime.Add(5 * time.Minute)



	contest := &models.Contest{
		State:       models.ContestStateRunning,
		StartedAt:   startTime,
		EndsAt:      endTime,
		NumProblems: req.NumProblems,
	}


	err := cs.contestRepo.CreateContest(ctx, contest)
	if err != nil {
		return nil, fmt.Errorf("failed to create contest: %w", err)
	}



	randomProblems, err := cs.problemRepo.GetRandomProblems(ctx, int(req.NumProblems))
	if err != nil {
		return nil, fmt.Errorf("failed to get random problems: %w", err)
	}


	if len(randomProblems) < int(req.NumProblems) {
		return nil, fmt.Errorf("not enough problems available")
	}


		for _, p := range randomProblems {
		
		if err := cs.problemRepo.AddProblemToContest(ctx, contest.ID, p.ID); err != nil {
			return nil, fmt.Errorf("failed to add problem to contest: %w", err)
		}
	}



	var participants []*models.Participant
	for _, modelName := range req.ParticipantModels {

		participant := &models.Participant{
			ContestID:           contest.ID,
			ModelName:           modelName,
			Solved:              0,
			TotalPenaltySeconds: 0,
		}

		err := cs.participantRepo.CreateParticipant(ctx, participant)
		if err != nil {
			return nil, fmt.Errorf("failed to create participant: %w", err)
		}


		participants = append(participants, participant)
	}



	for _, participant := range participants {
		for _, problem := range randomProblems {
			problemResult := &models.ProblemResult{
				ParticipantID:  participant.ID,
				ProblemID:      problem.ID,
				Status:         models.ProblemStatusNonTried,
				PenaltyCount:   0,
				PenaltySeconds: 0,
			}
			if err := cs.problemResultRepo.UpsertProblemResult(ctx, problemResult); err != nil {
				return nil, fmt.Errorf("failed to create problem result: %w", err)
			}
		}
	}





	if err := cs.coordinator.StartContest(contest.ID); err != nil {
		return nil, fmt.Errorf("failed to start contest in coordinator: %w", err)
	}


	grpcContest, err := cs.getCompleteContestData(ctx, contest.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get complete contest data: %w", err)
	}

	response := &grpc.CreateContestResponse{
		Contest: grpcContest,
	}

	return response, nil
}

func (cs *ContestService) CreateContestWithProblems(ctx context.Context, req *grpc.CreateContestWithProblemsRequest) (*grpc.CreateContestResponse, error) {
	if len(req.ProblemIds) == 0 {
		return nil, fmt.Errorf("no problem IDs provided")
	}
	if len(req.ParticipantModels) == 0 {
		return nil, fmt.Errorf("no participant models provided")
	}

	startTime := time.Now()
	endTime := startTime.Add(5 * time.Minute)

	contest := &models.Contest{
		State:       models.ContestStateRunning,
		StartedAt:   startTime,
		EndsAt:      endTime,
		NumProblems: int32(len(req.ProblemIds)),
	}

	err := cs.contestRepo.CreateContest(ctx, contest)
	if err != nil {
		return nil, fmt.Errorf("failed to create contest: %w", err)
	}

	var problems []*models.Problem
	for _, problemIDStr := range req.ProblemIds {
		problemID, err := uuid.Parse(problemIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid problem ID %s: %w", problemIDStr, err)
		}

		problem, err := cs.problemRepo.GetProblemByID(ctx, problemID)
		if err != nil {
			return nil, fmt.Errorf("failed to get problem %s: %w", problemIDStr, err)
		}

		problems = append(problems, problem)
	}

	for _, problem := range problems {
		if err := cs.problemRepo.AddProblemToContest(ctx, contest.ID, problem.ID); err != nil {
			return nil, fmt.Errorf("failed to add problem to contest: %w", err)
		}
	}

	var participants []*models.Participant
	for _, modelName := range req.ParticipantModels {
		participant := &models.Participant{
			ContestID:           contest.ID,
			ModelName:           modelName,
			Solved:              0,
			TotalPenaltySeconds: 0,
		}

		err := cs.participantRepo.CreateParticipant(ctx, participant)
		if err != nil {
			return nil, fmt.Errorf("failed to create participant: %w", err)
		}

		participants = append(participants, participant)
	}

	for _, participant := range participants {
		for _, problem := range problems {
			problemResult := &models.ProblemResult{
				ParticipantID:  participant.ID,
				ProblemID:      problem.ID,
				Status:         models.ProblemStatusNonTried,
				PenaltyCount:   0,
				PenaltySeconds: 0,
			}
			if err := cs.problemResultRepo.UpsertProblemResult(ctx, problemResult); err != nil {
				return nil, fmt.Errorf("failed to create problem result: %w", err)
			}
		}
	}

	if err := cs.coordinator.StartContest(contest.ID); err != nil {
		return nil, fmt.Errorf("failed to start contest in coordinator: %w", err)
	}

	grpcContest, err := cs.getCompleteContestData(ctx, contest.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get complete contest data: %w", err)
	}

	response := &grpc.CreateContestResponse{
		Contest: grpcContest,
	}

	return response, nil
}
func (s *ContestService) GetContest(ctx context.Context, req *grpc.GetContestRequest) (*grpc.GetContestResponse, error) {


	contestID, err := uuid.Parse(req.ContestId)
	if err != nil {
		return nil, fmt.Errorf("invalid contest ID: %w", err)
	}


	_, err = s.contestRepo.GetContest(ctx, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contest: %w", err)
	}



	grpcContest, err := s.getCompleteContestData(ctx, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get complete contest data: %w", err)
	}

	return &grpc.GetContestResponse{
		Contest: grpcContest,
	}, nil
}

func (s *ContestService) GetLeaderboard(ctx context.Context, req *grpc.GetLeaderboardRequest) (*grpc.GetLeaderboardResponse, error) {

	
	contestID, err := uuid.Parse(req.ContestId)
	if err != nil {
		return nil, fmt.Errorf("invalid contest ID: %w", err)
	}


	participants, err := s.participantRepo.GetParticipantsByContest(ctx, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}


	grpcParticipants := make([]*grpc.Participant, len(participants))
	for i, participant := range participants {
		grpcParticipants[i], _ = ConvertParticipantToGRPC(&participant, int32(i+1))
	}


	return &grpc.GetLeaderboardResponse{
		Participants: grpcParticipants,
		UpdatedAt:    timestamppb.Now(),
	}, nil
}

func (s *ContestService) ListContests(ctx context.Context, req *grpc.ListContestsRequest) (*grpc.ListContestsResponse, error) {

	
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 5

	}

	offset := 0


	contests, err := s.contestRepo.ListContests(ctx, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list contests: %w", err)
	}


	grpcContests := make([]*grpc.Contest, len(contests))
	for i, contest := range contests {

		grpcContest, err := ConvertContestToGRPC(&contest, contest.Problems, contest.Participants, s.problemResultRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to convert contest to gRPC format: %w", err)
		}

		grpcContests[i] = grpcContest
	}


	return &grpc.ListContestsResponse{
		Contests: grpcContests,
	}, nil
}

func (s *ContestService) SubmitSolution(ctx context.Context, req *grpc.SubmitSolutionRequest) (*grpc.SubmitSolutionResponse, error) {

	
	contestID, err := uuid.Parse(req.ContestId)
	if err != nil {
		return nil, fmt.Errorf("invalid contest ID: %w", err)
	}

	participantID, err := uuid.Parse(req.ParticipantId)
	if err != nil {
		return nil, fmt.Errorf("failed to parse participant ID: %w", err)
	}

	problemID, err := uuid.Parse(req.ProblemId)
	if err != nil {
		return nil, fmt.Errorf("failed to parse problem ID: %w", err)
	}


	contest, err := s.contestRepo.GetContest(ctx, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contest: %w", err)
	}

	if contest.State != models.ContestStateRunning || time.Now().After(contest.EndsAt) {
		return nil, fmt.Errorf("contest is not accepting submissions")
	}


	nTestCases, err := s.testCaseRepo.CountTestCasesByProblem(ctx, problemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get test cases: %w", err)
	}


	submissionID := uuid.New()
	var language models.Language
	switch req.Language {
	case grpc.Language_LANGUAGE_CPP:
		language = models.LanguageCPP
	case grpc.Language_LANGUAGE_PYTHON:
		language = models.LanguagePython
	default:
		language = models.LanguageUnspecified
	}

	if language == models.LanguageUnspecified {
		return nil, fmt.Errorf("invalid language")
	}

	submission := models.Submission{
		ID:                 submissionID,
		ContestID:          contestID,
		ParticipantID:      participantID,
		ProblemID:          problemID,
		Code:               req.Code,
		Language:           language,
		Status:             models.SubmissionStatusPending,
		SubmittedAt:        time.Now(),
		TotalTestCases:     int32(nTestCases),
		ProcessedTestCases: 0,
	}


	if err := s.submissionRepo.CreateSubmission(ctx, &submission); err != nil {
		return nil, fmt.Errorf("failed to create submission: %w", err)
	}


	if err := s.coordinator.ProcessSubmission(submissionID); err != nil {
		return nil, fmt.Errorf("failed to process submission: %w", err)
	}

	grpcSubmission := ConvertSubmissionToGRPC(&submission)

	log.Printf("SubmitSolution: Successfully submitted solution %s", submissionID.String())
	return &grpc.SubmitSolutionResponse{
		SubmissionId: submissionID.String(),
		Submission:   grpcSubmission,
	}, nil
}

func (s *ContestService) GetSubmissions(ctx context.Context, req *grpc.GetSubmissionsRequest) (*grpc.GetSubmissionsResponse, error) {

	
	var contestID, participantID, problemID *uuid.UUID

	if req.ContestId != "" {
		id, err := uuid.Parse(req.ContestId)
		if err != nil {
			return nil, fmt.Errorf("invalid contest ID: %w", err)
		}
		contestID = &id
	}

	if req.ParticipantId != "" {
		id, err := uuid.Parse(req.ParticipantId)
		if err != nil {
			return nil, fmt.Errorf("invalid participant ID: %w", err)
		}
		participantID = &id
	}

	if req.ProblemId != "" {
		id, err := uuid.Parse(req.ProblemId)
		if err != nil {
			return nil, fmt.Errorf("invalid problem ID: %w", err)
		}
		problemID = &id
	}


	
	submissions, err := s.submissionRepo.GetSubmissions(
		ctx,
		contestID,
		participantID,
		problemID,
		100, 
		0, 
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get submissions: %w", err)
	}



	grpcSubmissions := make([]*grpc.Submission, len(submissions))
	for i, submission := range submissions {
		grpcSubmissions[i] = ConvertSubmissionToGRPC(&submission)
	}

	return &grpc.GetSubmissionsResponse{
		Submissions: grpcSubmissions,
	}, nil
}



func (s *ContestService) StreamLeaderboard(req *grpc.StreamLeaderboardRequest, stream grpc.ContestService_StreamLeaderboardServer) error {
	contestID, err := uuid.Parse(req.ContestId)
	if err != nil {
		return fmt.Errorf("invalid contest ID: %w", err)
	}

	updateChan := s.coordinator.SubscribeToLeaderboardUpdates(contestID)

	for update := range updateChan {
		grpcParticipants := make([]*grpc.Participant, len(update.Participants))
		for i, coordParticipant := range update.Participants {
			rank := int32(i + 1) 
			grpcParticipants[i], _ = ConvertParticipantToGRPC(coordParticipant, rank)
		}

		grpcUpdate := &grpc.LeaderboardUpdate{
			Participants: grpcParticipants,
			UpdatedAt:    timestamppb.New(update.UpdatedAt),
		}

		if err := stream.Send(grpcUpdate); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ContestService) getCompleteContestData(ctx context.Context, contestID uuid.UUID) (*grpc.Contest, error) {

	completeContest, err := cs.contestRepo.GetContest(ctx, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch complete contest: %w", err)
	}

	grpcContest, err := ConvertContestToGRPC(completeContest, completeContest.Problems, completeContest.Participants, cs.problemResultRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert contest to gRPC: %w", err)
	}

	return grpcContest, nil
}
