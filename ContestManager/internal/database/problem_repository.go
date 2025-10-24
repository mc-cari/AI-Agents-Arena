package database

import (
	"contestmanager/internal/models"
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProblemRepository struct {
	db *GormDB
}

func NewProblemRepository(db *GormDB) *ProblemRepository {
	return &ProblemRepository{db: db}
}

func (r *ProblemRepository) CreateProblem(ctx context.Context, problem *models.Problem) error {
	return r.db.WithContext(ctx).Create(problem).Error
}

func (r *ProblemRepository) GetProblemByID(ctx context.Context, id uuid.UUID) (*models.Problem, error) {
	var problem models.Problem
	err := r.db.WithContext(ctx).
		Preload("TestCases", func(db *gorm.DB) *gorm.DB {
			return db.Order("test_order ASC")
		}).
		First(&problem, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &problem, nil
}

func (r *ProblemRepository) GetProblemsByContest(ctx context.Context, contestID uuid.UUID) ([]models.Problem, error) {
	var problems []models.Problem
	err := r.db.WithContext(ctx).
		Joins("JOIN contest_problems ON problems.id = contest_problems.problem_id").
		Where("contest_problems.contest_id = ?", contestID).
		Find(&problems).Error

	if err != nil {
		return nil, err
	}

	return problems, nil
}

func (r *ProblemRepository) AddProblemToContest(ctx context.Context, contestID, problemID uuid.UUID) error {
	var maxOrder int32
	err := r.db.WithContext(ctx).
		Model(&models.ContestProblem{}).
		Select("COALESCE(MAX(problem_order), -1)").
		Where("contest_id = ?", contestID).
		Scan(&maxOrder).Error
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(&models.ContestProblem{
		ContestID:    contestID,
		ProblemID:    problemID,
		ProblemOrder: maxOrder + 1,
	}).Error
}

func (r *ProblemRepository) DeleteProblem(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("problem_id = ?", id).Delete(&models.TestCase{}).Error; err != nil {
			return err
		}

		if err := tx.Where("problem_id = ?", id).Delete(&models.ProblemResult{}).Error; err != nil {
			return err
		}

		if err := tx.Where("problem_id = ?", id).Delete(&models.Submission{}).Error; err != nil {
			return err
		}

		if err := tx.Where("problem_id = ?", id).Delete(&models.ContestProblem{}).Error; err != nil {
			return err
		}

		return tx.Delete(&models.Problem{}, "id = ?", id).Error
	})
}

func (r *ProblemRepository) CountProblems(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Problem{}).Count(&count).Error
	return count, err
}

func (r *ProblemRepository) GetRandomProblems(ctx context.Context, n int) ([]*models.Problem, error) {
	var problems []*models.Problem
	err := r.db.WithContext(ctx).
		Model(&models.Problem{}).
		Order("RANDOM()").
		Limit(n).
		Find(&problems).Error

	if err != nil {
		return nil, err
	}

	return problems, nil
}
