package database

import (
	"contestmanager/internal/models"
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ParticipantRepository struct {
	db *GormDB
}

func NewParticipantRepository(db *GormDB) *ParticipantRepository {
	return &ParticipantRepository{db: db}
}

func (r *ParticipantRepository) CreateParticipant(ctx context.Context, participant *models.Participant) error {
	return r.db.WithContext(ctx).Create(participant).Error
}
func (r *ParticipantRepository) GetParticipant(ctx context.Context, id uuid.UUID) (*models.Participant, error) {
	var participant models.Participant
	err := r.db.WithContext(ctx).
		Preload("Contest").
		Preload("ProblemResults").
		First(&participant, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &participant, nil
}

func (r *ParticipantRepository) GetParticipantsByContest(
	ctx context.Context,
	contestID uuid.UUID,
) ([]models.Participant, error) {
	var participants []models.Participant
	err := r.db.WithContext(ctx).
		Where("contest_id = ?", contestID).
		Preload("ProblemResults").
		Order("solved DESC, total_penalty_seconds ASC").
		Find(&participants).Error

	if err != nil {
		return nil, err
	}

	return participants, nil
}

func (r *ParticipantRepository) UpdateParticipantStats(
	ctx context.Context,
	id uuid.UUID,
	solved int32,
	totalPenalty int32,
) error {
	return r.db.WithContext(ctx).
		Model(&models.Participant{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"solved":                solved,
			"total_penalty_seconds": totalPenalty,
			"updated_at":            time.Now(),
		}).Error
}

func (r *ParticipantRepository) DeleteParticipant(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Participant{}, "id = ?", id).Error
}
func (r *ParticipantRepository) CountParticipantsByContest(
	ctx context.Context,
	contestID uuid.UUID,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Participant{}).
		Where("contest_id = ?", contestID).
		Count(&count).Error

	return count, err
}
