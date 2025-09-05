package database

import (
	"context"

	"contestmanager/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProblemResultRepository struct {
	db *gorm.DB
}

func NewProblemResultRepository(db *GormDB) *ProblemResultRepository {
	return &ProblemResultRepository{db: db.DB}
}

func (r *ProblemResultRepository) UpsertProblemResult(ctx context.Context, result *models.ProblemResult) error {
	return r.db.WithContext(ctx).
		Save(result).
		Error
}

func (r *ProblemResultRepository) GetProblemResultsByParticipant(ctx context.Context, participantID uuid.UUID) (map[string]*models.ProblemResult, error) {
	var results []models.ProblemResult
	err := r.db.WithContext(ctx).
		Where("participant_id = ?", participantID).
		Find(&results).
		Error

	if err != nil {
		return nil, err
	}

	resultMap := make(map[string]*models.ProblemResult, len(results))
	for i := range results {
		resultMap[results[i].ProblemID.String()] = &results[i]
	}

	return resultMap, nil
}
