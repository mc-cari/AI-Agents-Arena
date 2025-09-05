package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"contestmanager/internal/config"
	"contestmanager/internal/queue"
	"contestmanager/internal/worker"

	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("Starting Code Execution Worker...")

	cfg := config.LoadConfig()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	execQueue := queue.NewExecutionQueue(redisClient)

	workerService := worker.NewWorkerService(execQueue)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := workerService.Start(ctx); err != nil {
			log.Fatalf("Worker failed to start: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Received shutdown signal, stopping worker...")

	cancel()
	
	time.Sleep(10 * time.Second)
	
	log.Println("Worker stopped")
}
