package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"contestmanager/internal/models"
	"contestmanager/internal/queue"
	"contestmanager/internal/worker/executors"

	"github.com/google/uuid"
)

type WorkerService struct {
	workerID     string
	queue        *queue.ExecutionQueue
	executor     executors.Executor
	isRunning    bool
	jobsProcessed int64
}

func NewWorkerService(queue *queue.ExecutionQueue) *WorkerService {
	workerID := fmt.Sprintf("worker-%s", uuid.New().String()[:8])
	
	if hostname, err := os.Hostname(); err == nil {
		workerID = fmt.Sprintf("worker-%s-%s", hostname, uuid.New().String()[:8])
	}

	return &WorkerService{
		workerID: workerID,
		queue:    queue,
		executor: executors.NewDirectExecutor(),
	}
}

func (ws *WorkerService) Start(ctx context.Context) error {
	log.Printf("Starting worker %s", ws.workerID)
	
	if err := ws.queue.RegisterWorker(ctx, ws.workerID); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}
	
	ws.isRunning = true
	
	go ws.heartbeatLoop(ctx)
	
	return ws.processJobs(ctx)
}

func (ws *WorkerService) processJobs(ctx context.Context) error {

	
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %s shutting down", ws.workerID)
			ws.isRunning = false
			return nil
			
		default:
			job, err := ws.queue.DequeueExecution(ctx, ws.workerID, 30*time.Second)
			if err != nil {
				log.Printf("Error dequeuing job: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			
			if job == nil {
				continue
			}
			
			result := ws.processJob(ctx, job)
			
			if err := ws.queue.PublishResult(ctx, result); err != nil {
				log.Printf("Failed to publish result for job %s: %v", job.JobID, err)
			}
			
			if err := ws.queue.MarkWorkerIdle(ctx, ws.workerID); err != nil {
				log.Printf("Failed to mark worker as idle: %v", err)
			}
			
			ws.jobsProcessed++
		}
	}
}

func (ws *WorkerService) processJob(ctx context.Context, job *models.ExecutionRequest) *models.ExecutionResult {
	log.Printf("Worker %s processing job %s for submission %s", ws.workerID, job.JobID, job.SubmissionID)
	
	startTime := time.Now()
	
	execResult, err := ws.executor.Execute(ctx, job)
	
	if err != nil {
		log.Printf("Execution failed for job %s: %v", job.JobID, err)
		return &models.ExecutionResult{
			JobID:           job.JobID,
			SubmissionID:    job.SubmissionID,
			WorkerID:        ws.workerID,
			Status:          models.SubmissionStatusJudgementFailed,
			VerdictMessage:  fmt.Sprintf("Execution failed: %v", err),
			ProcessedAt:     time.Now(),
		}
	}
	
	execResult.WorkerID = ws.workerID
	execResult.ProcessedAt = time.Now()
	
	duration := time.Since(startTime)
	log.Printf("Worker %s completed job %s in %v (status: %s)", 
		ws.workerID, job.JobID, duration, execResult.Status)
	
	return execResult
}

func (ws *WorkerService) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ws.queue.HeartbeatWorker(ctx, ws.workerID); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		}
	}
}


