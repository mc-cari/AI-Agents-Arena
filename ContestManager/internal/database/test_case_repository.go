package database

import (
	"context"

	"contestmanager/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TestCaseRepository struct {
	db *GormDB
}

func NewTestCaseRepository(db *GormDB) *TestCaseRepository {
	return &TestCaseRepository{db: db}
}

func (r *TestCaseRepository) CreateTestCase(ctx context.Context, testCase *models.TestCase) error {
	return r.db.WithContext(ctx).Create(testCase).Error
}

func (r *TestCaseRepository) BatchCreateTestCases(
	ctx context.Context,
	testCases []*models.TestCase,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, tc := range testCases {
			if err := tx.Create(tc).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TestCaseRepository) GetTestCasesByProblem(
	ctx context.Context,
	problemID uuid.UUID,
) ([]models.TestCase, error) {
	var testCases []models.TestCase
	err := r.db.WithContext(ctx).
		Where("problem_id = ?", problemID).
		Order("test_order ASC").
		Find(&testCases).Error

	if err != nil {
		return nil, err
	}

	return testCases, nil
}

func (r *TestCaseRepository) DeleteTestCasesByProblem(
	ctx context.Context,
	problemID uuid.UUID,
) error {
	return r.db.WithContext(ctx).
		Where("problem_id = ?", problemID).
		Delete(&models.TestCase{}).Error
}

func (r *TestCaseRepository) CountTestCasesByProblem(
	ctx context.Context,
	problemID uuid.UUID,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.TestCase{}).
		Where("problem_id = ?", problemID).
		Count(&count).Error

	return count, err
}
