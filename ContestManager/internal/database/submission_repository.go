package database

import (
	"contestmanager/internal/models"
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubmissionRepository struct {
	db *GormDB
}

func NewSubmissionRepository(db *GormDB) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

func (r *SubmissionRepository) CreateSubmission(ctx context.Context, submission *models.Submission) error {
	return r.db.WithContext(ctx).Create(submission).Error
}
func (r *SubmissionRepository) GetSubmission(ctx context.Context, id uuid.UUID) (*models.Submission, error) {
	var submission models.Submission
	err := r.db.WithContext(ctx).
		Preload("Problem").
		Preload("Participant").
		First(&submission, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &submission, nil
}

func (r *SubmissionRepository) GetSubmissionsByContest(ctx context.Context, contestID uuid.UUID) ([]models.Submission, error) {
	var submissions []models.Submission
	err := r.db.WithContext(ctx).
		Where("contest_id = ?", contestID).
		Order("submitted_at DESC").
		Limit(50).
		Find(&submissions).Error

	if err != nil {
		return nil, err
	}

	return submissions, nil
}

func (r *SubmissionRepository) GetSubmissions(
	ctx context.Context,
	contestID *uuid.UUID,
	participantID *uuid.UUID,
	problemID *uuid.UUID,
	limit, offset int,
) ([]models.Submission, error) {
	var submissions []models.Submission
	query := r.db.WithContext(ctx).Model(&models.Submission{}).
		Preload("Problem").
		Preload("Participant").
		Order("submitted_at DESC").
		Limit(limit).
		Offset(offset)

	if contestID != nil {
		query = query.Where("contest_id = ?", *contestID)
	}
	if participantID != nil {
		query = query.Where("participant_id = ?", *participantID)
	}
	if problemID != nil {
		query = query.Where("problem_id = ?", *problemID)
	}

	err := query.Find(&submissions).Error
	if err != nil {
		return nil, err
	}

	return submissions, nil
}

func (r *SubmissionRepository) CountSubmissions(
	ctx context.Context,
	contestID *uuid.UUID,
	participantID *uuid.UUID,
	problemID *uuid.UUID,
) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&models.Submission{})

	if contestID != nil {
		query = query.Where("contest_id = ?", *contestID)
	}
	if participantID != nil {
		query = query.Where("participant_id = ?", *participantID)
	}
	if problemID != nil {
		query = query.Where("problem_id = ?", *problemID)
	}

	err := query.Count(&count).Error
	return count, err
}

func (r *SubmissionRepository) UpdateSubmissionStatus(
	ctx context.Context,
	id uuid.UUID,
	status models.SubmissionStatus,
	verdict string,
) error {
	return r.db.WithContext(ctx).
		Model(&models.Submission{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          status,
			"verdict_message": verdict,
		}).Error
}

func (r *SubmissionRepository) UpdateSubmissionTestCaseProgress(
	ctx context.Context,
	id uuid.UUID,
	totalTestCases, processedTestCases int32,
) error {
	return r.db.WithContext(ctx).
		Model(&models.Submission{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"total_test_cases":     totalTestCases,
			"processed_test_cases": processedTestCases,
		}).Error
}

func (r *SubmissionRepository) GetLastSubmission(
	ctx context.Context,
	participantID, problemID uuid.UUID,
) (*models.Submission, error) {
	var submission models.Submission
	err := r.db.WithContext(ctx).
		Where("participant_id = ? AND problem_id = ?", participantID, problemID).
		Order("submitted_at DESC").
		First(&submission).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &submission, nil
}
