package database

import (
	"contestmanager/internal/models"
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ContestRepository struct {
	db *GormDB
}

func NewContestRepository(db *GormDB) *ContestRepository {
	return &ContestRepository{db: db}
}

func (r *ContestRepository) CreateContest(ctx context.Context, contest *models.Contest) error {
	return r.db.WithContext(ctx).Create(contest).Error
}

func (r *ContestRepository) GetContest(ctx context.Context, id uuid.UUID) (*models.Contest, error) {
	var contest models.Contest
	err := r.db.WithContext(ctx).
		Preload("Problems").
		Preload("Participants", func(db *gorm.DB) *gorm.DB {
			return db.Order("solved DESC, total_penalty_seconds ASC")
		}).
		Preload("Participants.ProblemResults").
		First(&contest, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &contest, nil
}

func (r *ContestRepository) ListContests(ctx context.Context, limit, offset int) ([]models.Contest, error) {
	var contests []models.Contest
	err := r.db.WithContext(ctx).
		Preload("Problems").
		Preload("Participants", func(db *gorm.DB) *gorm.DB {
			return db.Order("solved DESC, total_penalty_seconds ASC")
		}).
		Preload("Participants.ProblemResults").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&contests).Error

	if err != nil {
		return nil, err
	}

	return contests, nil
}

func (r *ContestRepository) UpdateContestState(ctx context.Context, id uuid.UUID, state models.ContestState) error {
	return r.db.WithContext(ctx).
		Model(&models.Contest{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"state":     state,
			"updated_at": time.Now(),
		}).Error
}

func (r *ContestRepository) GetActiveContests(ctx context.Context) ([]models.Contest, error) {
	var contests []models.Contest
	err := r.db.WithContext(ctx).
		Preload("Problems").
		Preload("Participants").
		Preload("Participants.ProblemResults").
		Where("state = ?", models.ContestStateRunning).
		Find(&contests).Error

	if err != nil {
		return nil, err
	}

	return contests, nil
}

func (r *ContestRepository) DeleteContest(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Select("Problems", "Problems.TestCases").
		Delete(&models.Contest{ID: id}).Error
}
