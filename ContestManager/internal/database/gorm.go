package database

import (
	"context"
	"fmt"
	"os"

	"contestmanager/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type GormDB struct {
	DB *gorm.DB
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func NewGormConnection(config Config) (*GormDB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode)

	logLevel := logger.Info
	if gormLogLevel := os.Getenv("GORM_LOG_LEVEL"); gormLogLevel != "" {
		switch gormLogLevel {
		case "silent":
			logLevel = logger.Silent
		case "error":
			logLevel = logger.Error
		case "warn":
			logLevel = logger.Warn
		case "info":
			logLevel = logger.Info
		}
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &GormDB{DB: db}, nil
}

func (db *GormDB) AutoMigrate() error {
	if err := db.createCustomTypes(); err != nil {
		return fmt.Errorf("failed to create custom types: %w", err)
	}

	if err := db.verifyCustomTypes(); err != nil {
		return fmt.Errorf("failed to verify custom types: %w", err)
	}

	err := db.DB.AutoMigrate(
		&models.Problem{},
		&models.Contest{},
		&models.Participant{},
		&models.Submission{},
		&models.TestCase{},
		&models.ProblemResult{},
		&models.ContestProblem{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto-migrate: %w", err)
	}

	err = db.DB.SetupJoinTable(&models.Contest{}, "Problems", &models.ContestProblem{})
	if err != nil {
		return fmt.Errorf("failed to setup join table: %w", err)
	}

	return nil
}

func (db *GormDB) WithContext(ctx context.Context) *gorm.DB {
	return db.DB.WithContext(ctx)
}

func (db *GormDB) Exec(sql string, values ...interface{}) *gorm.DB {
	return db.DB.Exec(sql, values...)
}

func (db *GormDB) Transaction(fc func(tx *gorm.DB) error) error {
	return db.DB.Transaction(fc)
}


func (db *GormDB) createCustomTypes() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	

	_, err = sqlDB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	if err != nil {
		return fmt.Errorf("failed to create UUID extension: %w", err)
	}
	
	// Create enum types only if they don't exist

	problemTagValues := []string{
		string(models.ProblemTagStrings),
		string(models.ProblemTagGeometry),
		string(models.ProblemTagDynamicProgramming),
		string(models.ProblemTagGraphs),
		string(models.ProblemTagGreedy),
		string(models.ProblemTagMath),
		string(models.ProblemTagDataStructures),
		string(models.ProblemTagSorting),
		string(models.ProblemTagBinarySearch),
		string(models.ProblemTagTwoPointers),
		string(models.ProblemTagSlidingWindow),
		string(models.ProblemTagBacktracking),
		string(models.ProblemTagBitManipulation),
		string(models.ProblemTagTree),
		string(models.ProblemTagHeap),
		string(models.ProblemTagStack),
		string(models.ProblemTagQueue),
		string(models.ProblemTagHashTable),
		string(models.ProblemTagArray),
		string(models.ProblemTagLinkedList),
		string(models.ProblemTagRecursion),
		string(models.ProblemTagDivideAndConquer),
		string(models.ProblemTagSimulation),
		string(models.ProblemTagImplementation),
		string(models.ProblemTagBruteForce),
	}
	
	err = db.createEnumType("problem_tag", problemTagValues)
	if err != nil {
		return fmt.Errorf("failed to create problem_tag type: %w", err)
	}

	contestStateValues := []string{
		string(models.ContestStateRunning),
		string(models.ContestStateFinished),
	}
	
	err = db.createEnumType("contest_state", contestStateValues)
	if err != nil {
		return fmt.Errorf("failed to create contest_state type: %w", err)
	}

	submissionStatusValues := []string{
		string(models.SubmissionStatusPending),
		string(models.SubmissionStatusCompiling),
		string(models.SubmissionStatusRunning),
		string(models.SubmissionStatusAccepted),
		string(models.SubmissionStatusWrongAnswer),
		string(models.SubmissionStatusPresentationError),
		string(models.SubmissionStatusTimeLimitExceeded),
		string(models.SubmissionStatusMemoryLimitExceeded),
		string(models.SubmissionStatusRuntimeError),
		string(models.SubmissionStatusCompilationError),
		string(models.SubmissionStatusOutputLimitExceeded),
		string(models.SubmissionStatusJudgementFailed),
	}
	
	err = db.createEnumType("submission_status", submissionStatusValues)
	if err != nil {
		return fmt.Errorf("failed to create submission_status type: %w", err)
	}

	problemStatusValues := []string{
		string(models.ProblemStatusAccepted),
		string(models.ProblemStatusTried),
		string(models.ProblemStatusNonTried),
	}
	
	err = db.createEnumType("problem_status", problemStatusValues)
	if err != nil {
		return fmt.Errorf("failed to create problem_status type: %w", err)
	}

	return nil
}

func (db *GormDB) createEnumType(typeName string, values []string) error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}

	// Check if type already exists
	var exists bool
	checkQuery := `SELECT EXISTS (SELECT 1 FROM pg_type WHERE typname = $1)`
	err = sqlDB.QueryRow(checkQuery, typeName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if type %s exists: %w", typeName, err)
	}

	// Only create if it doesn't exist
	if !exists {
		enumValues := ""
		for i, value := range values {
			if i > 0 {
				enumValues += ", "
			}
			enumValues += "'" + value + "'"
		}

		createQuery := fmt.Sprintf(`CREATE TYPE %s AS ENUM (%s)`, typeName, enumValues)
		_, err = sqlDB.Exec(createQuery)
		if err != nil {
			return fmt.Errorf("failed to create type %s: %w", typeName, err)
		}
	}

	return nil
}

func (db *GormDB) verifyCustomTypes() error {
	
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	
	types := []string{"problem_tag", "contest_state", "submission_status", "problem_status"}
	
	for _, typeName := range types {
		var exists bool
		query := `
			SELECT EXISTS (
				SELECT 1 FROM pg_type 
				WHERE typname = $1
			)
		`
		
		row := sqlDB.QueryRow(query, typeName)
		err := row.Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check if type %s exists: %w", typeName, err)
		}
		
		if !exists {
			return fmt.Errorf("custom type %s was not created successfully", typeName)
		}
		
	}
	
	return nil
}

func (db *GormDB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}