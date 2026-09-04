package repositories

import (
	"errors"
	"time"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"

	"gorm.io/gorm"
)

type monthlyReviewRepository struct {
	db *gorm.DB
}

func NewMonthlyReviewRepository(db *gorm.DB) domain.MonthlyReviewRepository {
	return &monthlyReviewRepository{db: db}
}

func toDomainMonthlyReview(m models.MonthlyReview) domain.MonthlyReview {
	return domain.MonthlyReview{
		ID:               m.ID,
		TenantID:         m.TenantID,
		Competencia:      m.Competencia,
		Status:           domain.MonthlyReviewStatus(m.Status),
		PayrollDone:      m.PayrollDone,
		OffsetsDone:      m.OffsetsDone,
		ClosedAt:         m.ClosedAt,
		ClosedByUserID:   m.ClosedByUserID,
		ReopenedAt:       m.ReopenedAt,
		ReopenedByUserID: m.ReopenedByUserID,
		ReopenReason:     m.ReopenReason,
	}
}

// getOrCreateTx busca a revisão da competência dentro de uma transação, criando-a em
// estado `aberta` se ainda não existir. Extraído porque SetManualConditions, Close e
// GetOrCreate repetiam o mesmo "First, se ErrRecordNotFound então Create" — a única
// diferença entre os três métodos está no que cada um faz DEPOIS de ter a linha em mãos.
func getOrCreateTx(tx *gorm.DB, tenantID int, competencia string) (models.MonthlyReview, error) {
	var m models.MonthlyReview
	err := tx.Where("tenant_id = ? AND competencia = ?", tenantID, competencia).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		m = models.MonthlyReview{TenantID: tenantID, Competencia: competencia, Status: string(domain.MonthlyReviewOpen)}
		if err := tx.Create(&m).Error; err != nil {
			return models.MonthlyReview{}, err
		}
		return m, nil
	}
	if err != nil {
		return models.MonthlyReview{}, err
	}
	return m, nil
}

func (r *monthlyReviewRepository) GetOrCreate(tenantID int, competencia string) (*domain.MonthlyReview, error) {
	const op = "monthlyReviewRepository.GetOrCreate"

	m, err := getOrCreateTx(r.db, tenantID, competencia)
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao buscar ou criar revisão mensal", err)
	}
	review := toDomainMonthlyReview(m)
	return &review, nil
}

func (r *monthlyReviewRepository) SetManualConditions(tenantID int, competencia string, payrollDone, offsetsDone *bool) (*domain.MonthlyReview, error) {
	const op = "monthlyReviewRepository.SetManualConditions"

	var result models.MonthlyReview
	err := r.db.Transaction(func(tx *gorm.DB) error {
		m, err := getOrCreateTx(tx, tenantID, competencia)
		if err != nil {
			return err
		}
		if m.Status == string(domain.MonthlyReviewClosed) {
			return domain.NewConflict(op, "competência encerrada", nil).
				WithDetails("reabra a competência antes de alterar as condições manuais")
		}

		updates := map[string]interface{}{}
		if payrollDone != nil {
			updates["payroll_done"] = *payrollDone
		}
		if offsetsDone != nil {
			updates["offsets_done"] = *offsetsDone
		}
		if len(updates) > 0 {
			if err := tx.Model(&m).Updates(updates).Error; err != nil {
				return err
			}
		}
		result = m
		if payrollDone != nil {
			result.PayrollDone = *payrollDone
		}
		if offsetsDone != nil {
			result.OffsetsDone = *offsetsDone
		}
		return nil
	})

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return nil, err
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao atualizar condições manuais", err)
	}
	review := toDomainMonthlyReview(result)
	return &review, nil
}

func (r *monthlyReviewRepository) Close(tenantID int, competencia string, actorUserID int) (*domain.MonthlyReview, error) {
	const op = "monthlyReviewRepository.Close"

	var result models.MonthlyReview
	err := r.db.Transaction(func(tx *gorm.DB) error {
		m, err := getOrCreateTx(tx, tenantID, competencia)
		if err != nil {
			return err
		}
		if m.Status == string(domain.MonthlyReviewClosed) {
			return domain.NewConflict(op, "competência já está encerrada", nil)
		}

		now := time.Now()
		if err := tx.Model(&m).Updates(map[string]interface{}{
			"status":            string(domain.MonthlyReviewClosed),
			"closed_at":         now,
			"closed_by_user_id": actorUserID,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&models.MonthlyReviewEvent{
			MonthlyReviewID: m.ID,
			TenantID:        tenantID,
			Type:            string(domain.MonthlyReviewEventClosed),
			ActorUserID:     actorUserID,
			CreatedAt:       now,
		}).Error; err != nil {
			return err
		}

		m.Status = string(domain.MonthlyReviewClosed)
		m.ClosedAt = &now
		m.ClosedByUserID = &actorUserID
		result = m
		return nil
	})

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return nil, err
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao encerrar competência", err)
	}
	review := toDomainMonthlyReview(result)
	return &review, nil
}

func (r *monthlyReviewRepository) Reopen(tenantID int, competencia string, actorUserID int, reason string) (*domain.MonthlyReview, error) {
	const op = "monthlyReviewRepository.Reopen"

	var result models.MonthlyReview
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var m models.MonthlyReview
		if err := tx.Where("tenant_id = ? AND competencia = ?", tenantID, competencia).First(&m).Error; err != nil {
			return err
		}
		if m.Status == string(domain.MonthlyReviewOpen) {
			return domain.NewConflict(op, "competência já está aberta", nil)
		}

		now := time.Now()
		if err := tx.Model(&m).Updates(map[string]interface{}{
			"status":              string(domain.MonthlyReviewOpen),
			"reopened_at":         now,
			"reopened_by_user_id": actorUserID,
			"reopen_reason":       reason,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&models.MonthlyReviewEvent{
			MonthlyReviewID: m.ID,
			TenantID:        tenantID,
			Type:            string(domain.MonthlyReviewEventReopen),
			Reason:          reason,
			ActorUserID:     actorUserID,
			CreatedAt:       now,
		}).Error; err != nil {
			return err
		}

		m.Status = string(domain.MonthlyReviewOpen)
		m.ReopenedAt = &now
		m.ReopenedByUserID = &actorUserID
		m.ReopenReason = reason
		result = m
		return nil
	})

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "revisão mensal não encontrada", err)
	}
	if err != nil {
		return nil, domain.NewInternal(op, "falha ao reabrir competência", err)
	}
	review := toDomainMonthlyReview(result)
	return &review, nil
}

// IsClosedAt confere se a competência de `date` está encerrada para o tenant. Ausência de
// registro significa "nunca foi encerrada" (aberta por omissão) — não é erro.
func (r *monthlyReviewRepository) IsClosedAt(tenantID int, date time.Time) (bool, error) {
	const op = "monthlyReviewRepository.IsClosedAt"

	competencia := domain.CompetenciaOf(date)
	var m models.MonthlyReview
	err := r.db.Where("tenant_id = ? AND competencia = ?", tenantID, competencia).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, domain.NewInternal(op, "falha ao verificar competência", err)
	}
	return m.Status == string(domain.MonthlyReviewClosed), nil
}
