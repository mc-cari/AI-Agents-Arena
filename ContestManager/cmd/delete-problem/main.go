package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"contestmanager/internal/database"
	"contestmanager/internal/models"
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <problem_name>")
		fmt.Println("Example: go run main.go 'Make Them Even'")
		os.Exit(1)
	}

	problemName := os.Args[1]
	
	config := database.Config{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		User:     getEnvOrDefault("DB_USER", "postgres"),
		Password: getEnvOrDefault("DB_PASSWORD", "password"),
		DBName:   getEnvOrDefault("DB_NAME", "contestmanager"),
		SSLMode:  getEnvOrDefault("DB_SSLMODE", "disable"),
	}

	db, err := database.NewGormConnection(config)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run auto-migration: %v", err)
	}

	ctx := context.Background()
	problemRepo := database.NewProblemRepository(db)

	// Find the problem by name
	var problem models.Problem
	err = db.WithContext(ctx).Where("name = ?", problemName).First(&problem).Error
	if err != nil {
		log.Fatalf("Failed to find problem '%s': %v", problemName, err)
	}

	fmt.Printf("Found problem: %s (ID: %s)\n", problem.Name, problem.ID)

	// Check if problem is used in any contests
	var contestCount int64
	err = db.WithContext(ctx).Model(&models.ContestProblem{}).Where("problem_id = ?", problem.ID).Count(&contestCount).Error
	if err != nil {
		log.Fatalf("Failed to check contest usage: %v", err)
	}

	if contestCount > 0 {
		fmt.Printf("Warning: Problem is used in %d contests. Proceeding with deletion...\n", contestCount)
	}

	// Delete the problem (this will also delete associated test cases due to CASCADE)
	err = problemRepo.DeleteProblem(ctx, problem.ID)
	if err != nil {
		log.Fatalf("Failed to delete problem: %v", err)
	}

	fmt.Printf("Successfully deleted problem '%s' and all associated test cases\n", problemName)
}
