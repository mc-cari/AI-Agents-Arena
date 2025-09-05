package main

import (
	"context"
	"log"
	"time"

	pb "contestmanager/api/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to the gRPC server
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create client
	client := pb.NewContestServiceClient(conn)
	ctx := context.Background()

	// Example 1: Create a contest
	log.Println("Creating a new contest...")
	createResp, err := client.CreateContest(ctx, &pb.CreateContestRequest{
		NumProblems:       3,
		ParticipantModels: []string{"gpt-4o-mini", "claude-3-sonnet", "gemini-pro"},
	})
	if err != nil {
		log.Fatalf("Failed to create contest: %v", err)
	}

	contestID := createResp.ContestId
	log.Printf("Created contest: %s", contestID)

	// Example 2: Get contest details
	log.Println("Getting contest details...")
	getResp, err := client.GetContest(ctx, &pb.GetContestRequest{
		ContestId: contestID,
	})
	if err != nil {
		log.Fatalf("Failed to get contest: %v", err)
	}

	contest := getResp.Contest
	log.Printf("Contest state: %s", contest.State)
	log.Printf("Problems: %d", len(contest.Problems))
	log.Printf("Participants: %d", len(contest.Participants))

	// Example 3: Submit a solution
	if len(contest.Participants) > 0 && len(contest.Problems) > 0 {
		log.Println("Submitting a solution...")

		sampleCode := `
def two_sum(nums, target):
    seen = {}
    for i, num in enumerate(nums):
        complement = target - num
        if complement in seen:
            return [seen[complement], i]
        seen[num] = i
    return []
`

		submitResp, err := client.SubmitSolution(ctx, &pb.SubmitSolutionRequest{
			ContestId:     contestID,
			ParticipantId: contest.Participants[0].Id,
			ProblemId:     contest.Problems[0].Id,
			Code:          sampleCode,
			Language:      pb.Language_LANGUAGE_PYTHON,
		})
		if err != nil {
			log.Fatalf("Failed to submit solution: %v", err)
		}

		log.Printf("Submitted solution: %s", submitResp.SubmissionId)

		// Wait a bit for processing
		time.Sleep(2 * time.Second)

		// Example 4: Get submissions
		log.Println("Getting submissions...")
		submissionsResp, err := client.GetSubmissions(ctx, &pb.GetSubmissionsRequest{
			ContestId: contestID,
		})
		if err != nil {
			log.Fatalf("Failed to get submissions: %v", err)
		}

		for _, submission := range submissionsResp.Submissions {
			log.Printf("Submission %s: %s - %s",
				submission.Id, submission.Status, submission.VerdictMessage)
		}
	}

	// Example 5: Get leaderboard
	log.Println("Getting leaderboard...")
	leaderboardResp, err := client.GetLeaderboard(ctx, &pb.GetLeaderboardRequest{
		ContestId: contestID,
	})
	if err != nil {
		log.Fatalf("Failed to get leaderboard: %v", err)
	}

	log.Println("Leaderboard:")
	for i, participant := range leaderboardResp.Participants {
		log.Printf("%d. %s - Solved: %d, Penalty: %d seconds",
			i+1, participant.ModelName,
			participant.Result.Solved, participant.Result.TotalPenaltySeconds)
	}

	// Example 6: Stream leaderboard updates (for a short time)
	log.Println("Streaming leaderboard updates for 10 seconds...")
	stream, err := client.StreamLeaderboard(ctx, &pb.StreamLeaderboardRequest{
		ContestId: contestID,
	})
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	// Create a timeout context for streaming
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		for {
			select {
			case <-streamCtx.Done():
				return
			default:
				update, err := stream.Recv()
				if err != nil {
					log.Printf("Stream ended: %v", err)
					return
				}
				log.Printf("Leaderboard update received with %d participants",
					len(update.Participants))
			}
		}
	}()

	// Wait for streaming to complete
	<-streamCtx.Done()
	log.Println("Client example completed!")
}
