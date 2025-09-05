package main

import (
	"log"
	"net"

	"contestmanager/api/grpc"
	"contestmanager/internal/config"
	"contestmanager/internal/coordinator"
	"contestmanager/internal/database"
	"contestmanager/internal/services"

	"github.com/redis/go-redis/v9"
	grpcServer "google.golang.org/grpc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Loaded configuration - Log level: %s", cfg.Logging.Level)

	db, err := database.NewGormConnection(database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run auto-migration: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	contestRepo := database.NewContestRepository(db)
	problemRepo := database.NewProblemRepository(db)
	participantRepo := database.NewParticipantRepository(db)
	submissionRepo := database.NewSubmissionRepository(db)
	problemResultRepo := database.NewProblemResultRepository(db)
	testCaseRepo := database.NewTestCaseRepository(db)

	contestCoordinator := coordinator.NewContestCoordinator(
		cfg.Contest.MaxConcurrentContests,
		contestRepo,
		submissionRepo,
		participantRepo,
		problemResultRepo,
		testCaseRepo,
		redisClient,
	)

	contestService := services.NewContestService(
		contestRepo,
		problemRepo,
		participantRepo,
		submissionRepo,
		problemResultRepo,
		testCaseRepo,
		contestCoordinator,
	)

	server := grpcServer.NewServer()
	grpc.RegisterContestServiceServer(server, contestService)

	listener, err := net.Listen("tcp", ":"+cfg.Server.GRPCPort)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", cfg.Server.GRPCPort, err)
	}

	log.Printf("ContestManager gRPC server starting on port %s", cfg.Server.GRPCPort)
	log.Printf("Database: %s:%s/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	log.Printf("Redis: %s", cfg.Redis.Addr)
	log.Printf("Max concurrent contests: %d", cfg.Contest.MaxConcurrentContests)

	if err := server.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC server: %v", err)
	}
}
