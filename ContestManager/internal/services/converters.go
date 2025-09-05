package services

import (
	"contestmanager/api/grpc"
	"contestmanager/internal/database"
	"contestmanager/internal/models"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertContestToGRPC(contest *models.Contest, problems []models.Problem, participants []models.Participant, problemResultRepo *database.ProblemResultRepository) (*grpc.Contest, error) {
	grpcProblems := make([]*grpc.Problem, len(problems))
	for i, problem := range problems {
		grpcProblems[i] = &grpc.Problem{
			Id:            problem.ID.String(),
			Name:          problem.Name,
			Description:   problem.Description,
			TimeLimitMs:   problem.TimeLimitMs,
			MemoryLimitMb: problem.MemoryLimitMb,
		}
	}

	grpcParticipants := make([]*grpc.Participant, len(participants))
	for i, participant := range participants {
		p, err := ConvertParticipantToGRPC(&participant, int32(i+1))
		if err != nil {
			return nil, err
		}
		grpcParticipants[i] = p
	}

	return &grpc.Contest{
		Id:           contest.ID.String(),
		State:        ConvertContestStateToGRPC(contest.State),
		StartedAt:    timestamppb.New(contest.StartedAt),
		EndsAt:       timestamppb.New(contest.EndsAt),
		Problems:     grpcProblems,
		Participants: grpcParticipants,
	}, nil
}

func ConvertParticipantToGRPC(participant *models.Participant, rank int32) (*grpc.Participant, error) {
	problemResults := make(map[string]*grpc.ProblemResult)
	for _, result := range participant.ProblemResults {
		problemResults[result.ProblemID.String()] = &grpc.ProblemResult{
			Status:         ConvertProblemStatusToGRPC(result.Status),
			PenaltyCount:   result.PenaltyCount,
			PenaltySeconds: result.PenaltySeconds,
		}
	}

	participantResult := &grpc.ParticipantResult{
		Solved:              participant.Solved,
		TotalPenaltySeconds: participant.TotalPenaltySeconds,
		ProblemResults:      problemResults,
		Rank:                rank,
	}

	return &grpc.Participant{
		Id:         participant.ID.String(),
		ModelName:  participant.ModelName,
		Result:     participantResult,
	}, nil
}

func ConvertSubmissionToGRPC(submission *models.Submission) *grpc.Submission {
	language := grpc.Language_LANGUAGE_UNSPECIFIED
	switch submission.Language {
	case models.LanguageCPP:
		language = grpc.Language_LANGUAGE_CPP
	case models.LanguagePython:
		language = grpc.Language_LANGUAGE_PYTHON
	}

	return &grpc.Submission{
		Id:                 submission.ID.String(),
		ContestId:          submission.ContestID.String(),
		ParticipantId:      submission.ParticipantID.String(),
		ProblemId:          submission.ProblemID.String(),
		Code:               submission.Code,
		Language:           language,
		Status:             ConvertSubmissionStatusToGRPC(submission.Status),
		SubmittedAt:        timestamppb.New(submission.SubmittedAt),
		VerdictMessage:     submission.VerdictMessage,
		TotalTestCases:     submission.TotalTestCases,
		ProcessedTestCases: submission.ProcessedTestCases,
	}
}

func ConvertContestStateToGRPC(state models.ContestState) grpc.ContestState {
	switch state {
	case models.ContestStateRunning:
		return grpc.ContestState_CONTEST_STATE_RUNNING
	case models.ContestStateFinished:
		return grpc.ContestState_CONTEST_STATE_FINISHED
	default:
		return grpc.ContestState_CONTEST_STATE_UNSPECIFIED
	}
}

func ConvertSubmissionStatusToGRPC(status models.SubmissionStatus) grpc.SubmissionStatus {
	switch status {
	case models.SubmissionStatusPending:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_PENDING
	case models.SubmissionStatusCompiling:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_COMPILING
	case models.SubmissionStatusRunning:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_RUNNING
	case models.SubmissionStatusAccepted:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_ACCEPTED
	case models.SubmissionStatusWrongAnswer:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_WRONG_ANSWER
	case models.SubmissionStatusPresentationError:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_PRESENTATION_ERROR
	case models.SubmissionStatusTimeLimitExceeded:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_TIME_LIMIT_EXCEEDED
	case models.SubmissionStatusMemoryLimitExceeded:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_MEMORY_LIMIT_EXCEEDED
	case models.SubmissionStatusRuntimeError:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_RUNTIME_ERROR
	case models.SubmissionStatusCompilationError:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_COMPILATION_ERROR
	case models.SubmissionStatusOutputLimitExceeded:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_OUTPUT_LIMIT_EXCEEDED
	case models.SubmissionStatusJudgementFailed:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_JUDGEMENT_FAILED
	default:
		return grpc.SubmissionStatus_SUBMISSION_STATUS_UNSPECIFIED
	}
}

func ConvertProblemTagToGRPC(tag models.ProblemTag) string {
	switch tag {
	case models.ProblemTagMath:
		return "MATH"
	case models.ProblemTagGeometry:
		return "GEOMETRY"
	case models.ProblemTagStrings:
		return "STRINGS"
	case models.ProblemTagGraphs:
		return "GRAPHS"
	case models.ProblemTagGreedy:
		return "GREEDY"
	case models.ProblemTagDynamicProgramming:
		return "DYNAMIC_PROGRAMMING"
	case models.ProblemTagDataStructures:
		return "DATA_STRUCTURES"
	case models.ProblemTagSorting:
		return "SORTING"
	case models.ProblemTagBinarySearch:
		return "BINARY_SEARCH"
	case models.ProblemTagTwoPointers:
		return "TWO_POINTERS"
	case models.ProblemTagSlidingWindow:
		return "SLIDING_WINDOW"
	case models.ProblemTagBacktracking:
		return "BACKTRACKING"
	case models.ProblemTagBitManipulation:
		return "BIT_MANIPULATION"
	case models.ProblemTagTree:
		return "TREE"
	case models.ProblemTagHeap:
		return "HEAP"
	case models.ProblemTagStack:
		return "STACK"
	case models.ProblemTagQueue:
		return "QUEUE"
	case models.ProblemTagHashTable:
		return "HASH_TABLE"
	case models.ProblemTagArray:
		return "ARRAY"
	case models.ProblemTagLinkedList:
		return "LINKED_LIST"
	case models.ProblemTagRecursion:
		return "RECURSION"
	case models.ProblemTagDivideAndConquer:
		return "DIVIDE_AND_CONQUER"
	case models.ProblemTagSimulation:
		return "SIMULATION"
	case models.ProblemTagImplementation:
		return "IMPLEMENTATION"
	case models.ProblemTagBruteForce:
		return "BRUTE_FORCE"
	default:
		return "UNSPECIFIED"
	}
}

func ConvertProblemStatusToGRPC(status models.ProblemStatus) grpc.ProblemStatus {
	switch status {
	case models.ProblemStatusAccepted:
		return grpc.ProblemStatus_PROBLEM_STATUS_ACCEPTED
	case models.ProblemStatusTried:
		return grpc.ProblemStatus_PROBLEM_STATUS_TRIED
	case models.ProblemStatusNonTried:
		return grpc.ProblemStatus_PROBLEM_STATUS_NON_TRIED
	default:
		return grpc.ProblemStatus_PROBLEM_STATUS_UNSPECIFIED
	}
}
