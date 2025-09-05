package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"contestmanager/internal/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	ExecutionJobsQueue    = "execution:jobs"
	ExecutionResultsQueue = "execution:results"
	WorkerStatusHash      = "workers:status"
	
	JobTimeout = 5 * time.Minute
)

type ExecutionQueue struct {
	client *redis.Client
}

func NewExecutionQueue(client *redis.Client) *ExecutionQueue {
	return &ExecutionQueue{
		client: client,
	}
}

func (eq *ExecutionQueue) QueueExecution(ctx context.Context, req *models.ExecutionRequest) error {

	
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal execution request: %w", err)
	}

	err = eq.client.LPush(ctx, ExecutionJobsQueue, data).Err()
	if err != nil {
		return fmt.Errorf("failed to queue execution job: %w", err)
	}

	log.Printf("Queued execution job %s for submission %s", req.JobID, req.SubmissionID)
	return nil
}

func (eq *ExecutionQueue) DequeueExecution(ctx context.Context, workerID string, timeout time.Duration) (*models.ExecutionRequest, error) {
	result, err := eq.client.BRPop(ctx, timeout, ExecutionJobsQueue).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to dequeue execution job: %w", err)
	}

	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected BRPOP result length: %d", len(result))
	}

	var req models.ExecutionRequest
	err = json.Unmarshal([]byte(result[1]), &req)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal execution request: %w", err)
	}

	eq.updateWorkerStatus(ctx, workerID, "busy", &req.JobID)


	return &req, nil
}

func (eq *ExecutionQueue) PublishResult(ctx context.Context, result *models.ExecutionResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal execution result: %w", err)
	}

	err = eq.client.Publish(ctx, ExecutionResultsQueue, data).Err()
	if err != nil {
		return fmt.Errorf("failed to publish execution result: %w", err)
	}


	return nil
}

func (eq *ExecutionQueue) SubscribeToResults(ctx context.Context) (<-chan *models.ExecutionResult, error) {
	pubsub := eq.client.Subscribe(ctx, ExecutionResultsQueue)
	
	resultChan := make(chan *models.ExecutionResult, 10)
	
	go func() {
		defer close(resultChan)
		defer pubsub.Close()
		
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				
				var result models.ExecutionResult
				if err := json.Unmarshal([]byte(msg.Payload), &result); err != nil {
					log.Printf("Failed to unmarshal execution result: %v", err)
					continue
				}
				
				select {
				case resultChan <- &result:
				case <-ctx.Done():
					return
				default:
					log.Printf("Result channel full, dropping result for job %s", result.JobID)
				}
			}
		}
	}()
	
	return resultChan, nil
}

func (eq *ExecutionQueue) updateWorkerStatus(ctx context.Context, workerID, status string, currentJobID *uuid.UUID) {
	workerStatus := models.WorkerStatus{
		WorkerID:      workerID,
		Status:        status,
		LastPing:      time.Now(),
		CurrentJobID:  currentJobID,
	}
	
	data, err := json.Marshal(workerStatus)
	if err != nil {
		log.Printf("Failed to marshal worker status: %v", err)
		return
	}
	
	err = eq.client.HSet(ctx, WorkerStatusHash, workerID, data).Err()
	if err != nil {
		log.Printf("Failed to update worker status: %v", err)
	}
}

func (eq *ExecutionQueue) RegisterWorker(ctx context.Context, workerID string) error {
	eq.updateWorkerStatus(ctx, workerID, "idle", nil)
	log.Printf("Registered worker %s", workerID)
	return nil
}

func (eq *ExecutionQueue) HeartbeatWorker(ctx context.Context, workerID string) error {
	statusData, err := eq.client.HGet(ctx, WorkerStatusHash, workerID).Result()
	if err != nil {
		if err == redis.Nil {
			return eq.RegisterWorker(ctx, workerID)
		}
		return fmt.Errorf("failed to get worker status: %w", err)
	}
	
	var status models.WorkerStatus
	if err := json.Unmarshal([]byte(statusData), &status); err != nil {
		return fmt.Errorf("failed to unmarshal worker status: %w", err)
	}
	
	status.LastPing = time.Now()
	eq.updateWorkerStatusFromStruct(ctx, workerID, &status)
	return nil
}

func (eq *ExecutionQueue) updateWorkerStatusFromStruct(ctx context.Context, workerID string, status *models.WorkerStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		log.Printf("Failed to marshal worker status: %v", err)
		return
	}
	
	err = eq.client.HSet(ctx, WorkerStatusHash, workerID, data).Err()
	if err != nil {
		log.Printf("Failed to update worker status: %v", err)
	}
}

func (eq *ExecutionQueue) MarkWorkerIdle(ctx context.Context, workerID string) error {
	eq.updateWorkerStatus(ctx, workerID, "idle", nil)
	return nil
}

func (eq *ExecutionQueue) GetActiveWorkers(ctx context.Context) ([]models.WorkerStatus, error) {
	workers, err := eq.client.HGetAll(ctx, WorkerStatusHash).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get workers: %w", err)
	}
	
	var activeWorkers []models.WorkerStatus
	cutoff := time.Now().Add(-30 * time.Second)
	
	for _, workerData := range workers {
		var worker models.WorkerStatus
		if err := json.Unmarshal([]byte(workerData), &worker); err != nil {
			log.Printf("Failed to unmarshal worker status: %v", err)
			continue
		}
		
		if worker.LastPing.After(cutoff) {
			activeWorkers = append(activeWorkers, worker)
		}
	}
	
	return activeWorkers, nil
}

func (eq *ExecutionQueue) GetQueueLength(ctx context.Context) (int64, error) {
	return eq.client.LLen(ctx, ExecutionJobsQueue).Result()
}

func (eq *ExecutionQueue) CleanupStaleWorkers(ctx context.Context) error {
	workers, err := eq.client.HGetAll(ctx, WorkerStatusHash).Result()
	if err != nil {
		return fmt.Errorf("failed to get workers: %w", err)
	}
	
	cutoff := time.Now().Add(-2 * time.Minute)
	
	for workerID, workerData := range workers {
		var worker models.WorkerStatus
		if err := json.Unmarshal([]byte(workerData), &worker); err != nil {
			log.Printf("Failed to unmarshal worker status: %v", err)
			continue
		}
		
		if worker.LastPing.Before(cutoff) {
			err := eq.client.HDel(ctx, WorkerStatusHash, workerID).Err()
			if err != nil {
				log.Printf("Failed to remove stale worker %s: %v", workerID, err)
			} else {
				log.Printf("Removed stale worker %s", workerID)
			}
		}
	}
	
	return nil
}
