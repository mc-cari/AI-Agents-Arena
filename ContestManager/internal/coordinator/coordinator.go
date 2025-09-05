package coordinator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"contestmanager/internal/database"
	"contestmanager/internal/models"
	"contestmanager/internal/queue"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type LeaderboardUpdate struct {
	ContestID    uuid.UUID
	Participants []*models.Participant
	UpdatedAt    time.Time
}

type ContestCoordinator struct {
	mu                    sync.RWMutex
	activeContests        map[uuid.UUID]*ContestInstance
	maxConcurrentContests int

	contestRepo       *database.ContestRepository
	submissionRepo    *database.SubmissionRepository
	participantRepo   *database.ParticipantRepository
	problemResultRepo *database.ProblemResultRepository
	testCaseRepo      *database.TestCaseRepository

	redisClient *redis.Client
	execQueue   *queue.ExecutionQueue

	leaderboardSubscribers map[uuid.UUID][]chan LeaderboardUpdate

	resultsChan <-chan *models.ExecutionResult
}

type ContestInstance struct {
	ID        uuid.UUID
	StartedAt time.Time
	EndsAt    time.Time
	State     models.ContestState

	submissionChan chan uuid.UUID
	stopChan       chan struct{}

	coordinator *ContestCoordinator
}

func NewContestCoordinator(
	maxConcurrentContests int,
	contestRepo *database.ContestRepository,
	submissionRepo *database.SubmissionRepository,
	participantRepo *database.ParticipantRepository,
	problemResultRepo *database.ProblemResultRepository,
	testCaseRepo *database.TestCaseRepository,
	redisClient *redis.Client,
) *ContestCoordinator {
	execQueue := queue.NewExecutionQueue(redisClient)

	coordinator := &ContestCoordinator{
		activeContests:         make(map[uuid.UUID]*ContestInstance),
		maxConcurrentContests:  maxConcurrentContests,
		contestRepo:            contestRepo,
		submissionRepo:         submissionRepo,
		participantRepo:        participantRepo,
		problemResultRepo:      problemResultRepo,
		testCaseRepo:           testCaseRepo,
		redisClient:            redisClient,
		execQueue:              execQueue,
		leaderboardSubscribers: make(map[uuid.UUID][]chan LeaderboardUpdate),
	}

	resultsChan, err := execQueue.SubscribeToResults(context.Background())
	if err != nil {
		log.Fatalf("Failed to subscribe to execution results: %v", err)
	}
	coordinator.resultsChan = resultsChan

	go coordinator.processExecutionResults(context.Background())

	return coordinator
}

func (c *ContestCoordinator) StartContest(contestID uuid.UUID) error {
	c.mu.Lock()

	if len(c.activeContests) >= c.maxConcurrentContests {
		c.mu.Unlock()
		return fmt.Errorf("maximum concurrent contests (%d) reached", c.maxConcurrentContests)
	}

	contest, err := c.contestRepo.GetContest(context.Background(), contestID)
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to get contest: %w", err)
	}

	instance := &ContestInstance{
		ID:             contestID,
		StartedAt:      contest.StartedAt,
		EndsAt:         contest.EndsAt,
		State:          contest.State,
		submissionChan: make(chan uuid.UUID, 100),
		stopChan:       make(chan struct{}),
		coordinator:    c,
	}

	c.activeContests[contestID] = instance

	c.mu.Unlock()
	
	if err := c.broadcastLeaderboardUpdateForContest(contestID); err != nil {
		log.Printf("Failed to broadcast leaderboard update: %v", err)
		c.mu.Lock()
		delete(c.activeContests, contestID)
		c.mu.Unlock()
		return fmt.Errorf("failed to broadcast leaderboard update: %w", err)
	}

	go instance.run()


	return nil
}

func (c *ContestCoordinator) StopContest(contestID uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	instance, exists := c.activeContests[contestID]
	if !exists {
		return fmt.Errorf("contest %s not found", contestID)
	}

	close(instance.stopChan)


	if err := c.contestRepo.UpdateContestState(context.Background(), contestID, models.ContestStateFinished); err != nil {
		log.Printf("Failed to update contest state: %v", err)
	}

	delete(c.activeContests, contestID)
	delete(c.leaderboardSubscribers, contestID)


	return nil
}

func (c *ContestCoordinator) ProcessSubmission(submissionID uuid.UUID) error {
	submission, err := c.submissionRepo.GetSubmission(context.Background(), submissionID)
	if err != nil {
		return fmt.Errorf("failed to get submission: %w", err)
	}

	c.mu.RLock()
	_, exists := c.activeContests[submission.ContestID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("contest %s not active", submission.ContestID)
	}

	testCases, err := c.testCaseRepo.GetTestCasesByProblem(context.Background(), submission.ProblemID)
	if err != nil {
		return fmt.Errorf("failed to get test cases: %w", err)
	}

	execTestCases := make([]models.TestCaseData, len(testCases))
	for i, tc := range testCases {
		execTestCases[i] = models.TestCaseData{
			Input:          tc.Input,
			ExpectedOutput: tc.ExpectedOutput,
			TestOrder:      tc.TestOrder,
		}
	}

	execRequest := &models.ExecutionRequest{
		JobID:         uuid.New(),
		SubmissionID:  submissionID,
		ContestID:     submission.ContestID,
		ParticipantID: submission.ParticipantID,
		ProblemID:     submission.ProblemID,
		Code:          submission.Code,
		Language:      submission.Language,
		TestCases:     execTestCases,
		TimeLimitMs:   submission.Problem.TimeLimitMs,
		MemoryLimitMb: submission.Problem.MemoryLimitMb,
		CreatedAt:     time.Now(),
	}

	if err := c.submissionRepo.UpdateSubmissionStatus(
		context.Background(),
		submissionID,
		models.SubmissionStatusPending,
		"Queued for execution",
	); err != nil {
		return fmt.Errorf("failed to update submission status: %w", err)
	}

	if err := c.execQueue.QueueExecution(context.Background(), execRequest); err != nil {
		c.submissionRepo.UpdateSubmissionStatus(
			context.Background(),
			submissionID,
			models.SubmissionStatusJudgementFailed,
			"Failed to queue for execution",
		)
		return fmt.Errorf("failed to queue execution: %w", err)
	}


	return nil
}

func (c *ContestCoordinator) SubscribeToLeaderboardUpdates(contestID uuid.UUID) <-chan LeaderboardUpdate {
	c.mu.Lock()
	defer c.mu.Unlock()

	updateChan := make(chan LeaderboardUpdate, 10)
	c.leaderboardSubscribers[contestID] = append(c.leaderboardSubscribers[contestID], updateChan)

	return updateChan
}

func (c *ContestCoordinator) broadcastLeaderboardUpdate(update LeaderboardUpdate) {
	c.mu.RLock()
	subscribers, exists := c.leaderboardSubscribers[update.ContestID]
	c.mu.RUnlock()

	if !exists {
		return
	}

	for _, subscriber := range subscribers {
		select {
		case subscriber <- update:
		default:
		}
	}
}

func (ci *ContestInstance) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ci.stopChan:
			log.Printf("Contest %s stopped", ci.ID)
			return

		case <-ticker.C:
			if time.Now().After(ci.EndsAt) {
				ci.coordinator.StopContest(ci.ID)
				return
			}
		}
	}
}

func (c *ContestCoordinator) processExecutionResults(ctx context.Context) {
	log.Println("Started execution results processor")

	for {
		select {
		case <-ctx.Done():
			log.Println("Execution results processor stopped")
			return

		case result, ok := <-c.resultsChan:
			if !ok {
				log.Println("Results channel closed")
				return
			}

			if err := c.handleExecutionResult(ctx, result); err != nil {
				log.Printf("Failed to handle execution result for job %s: %v", result.JobID, err)
			}
		}
	}
}

func (c *ContestCoordinator) handleExecutionResult(ctx context.Context, result *models.ExecutionResult) error {


	if err := c.submissionRepo.UpdateSubmissionStatus(
		ctx,
		result.SubmissionID,
		result.Status,
		result.VerdictMessage,
	); err != nil {
		return fmt.Errorf("failed to update submission status: %w", err)
	}

	if err := c.submissionRepo.UpdateSubmissionTestCaseProgress(
		ctx,
		result.SubmissionID,
		result.TotalTestCases,
		result.PassedTestCases,
	); err != nil {
		log.Printf("Failed to update test case progress: %v", err)
	}

	submission, err := c.submissionRepo.GetSubmission(ctx, result.SubmissionID)
	if err != nil {
		return fmt.Errorf("failed to get submission: %w", err)
	}

	c.mu.RLock()
	instance, contestActive := c.activeContests[submission.ContestID]
	c.mu.RUnlock()

	if !contestActive {
		log.Printf("Contest %s no longer active, skipping stats update", submission.ContestID)
		return nil
	}

	if result.Status != models.SubmissionStatusAccepted {
		return nil
	}


	if err := c.updateParticipantStats(submission, result.Status, instance.StartedAt); err != nil {
		log.Printf("Failed to update participant stats: %v", err)
	}

	if err := c.broadcastLeaderboardUpdateForContest(submission.ContestID); err != nil {
		log.Printf("Failed to broadcast leaderboard update: %v", err)
	}

	return nil
}


func (c *ContestCoordinator) updateParticipantStats(submission *models.Submission, status models.SubmissionStatus, contestStartTime time.Time) error {
	participant, err := c.participantRepo.GetParticipant(context.Background(), submission.ParticipantID)
	if err != nil {
		return fmt.Errorf("failed to get participant: %w", err)
	}
	if participant == nil {
		return fmt.Errorf("participant not found")
	}

	var problemResult *models.ProblemResult
	for i := range participant.ProblemResults {
		if participant.ProblemResults[i].ProblemID == submission.ProblemID {
			problemResult = &participant.ProblemResults[i]
			break
		}
	}

	if problemResult == nil {
		return fmt.Errorf("problem result not found for participant %s and problem %s", submission.ParticipantID, submission.ProblemID)
	}

	if status == models.SubmissionStatusAccepted && problemResult.Status != models.ProblemStatusAccepted {
		problemResult.Status = models.ProblemStatusAccepted
		problemResult.PenaltySeconds = int32(submission.SubmittedAt.Sub(contestStartTime).Seconds())
	} else if status != models.SubmissionStatusAccepted {
		if problemResult.Status == models.ProblemStatusNonTried {
			problemResult.Status = models.ProblemStatusTried
		}
		problemResult.PenaltyCount++
	}

	if err := c.problemResultRepo.UpsertProblemResult(context.Background(), problemResult); err != nil {
		return fmt.Errorf("failed to update problem result: %w", err)
	}

	participant.Solved = 0
	participant.TotalPenaltySeconds = 0

	for _, result := range participant.ProblemResults {
		if result.Status == models.ProblemStatusAccepted {
			participant.Solved++
			participant.TotalPenaltySeconds += result.PenaltySeconds + (result.PenaltyCount * 60)
		}
	}

	if err := c.participantRepo.UpdateParticipantStats(
		context.Background(),
		submission.ParticipantID,
		participant.Solved,
		participant.TotalPenaltySeconds,
	); err != nil {
		return fmt.Errorf("failed to update participant stats: %w", err)
	}

	return nil
}

func (c *ContestCoordinator) broadcastLeaderboardUpdateForContest(contestID uuid.UUID) error {
	participants, err := c.participantRepo.GetParticipantsByContest(context.Background(), contestID)
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}

	participantPtrs := make([]*models.Participant, len(participants))
	for i := range participants {
		participantPtrs[i] = &participants[i]
	}

	update := LeaderboardUpdate{
		ContestID:    contestID,
		Participants: participantPtrs,
		UpdatedAt:    time.Now(),
	}

	c.broadcastLeaderboardUpdate(update)
	return nil
}


