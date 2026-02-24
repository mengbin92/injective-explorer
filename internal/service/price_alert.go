// Package service provides price alert functionality.
package service

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/bytedance/sonic"

	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// PriceAlertService provides price alert functionality.
type PriceAlertService struct {
	db     *gorm.DB
	logger *log.Helper

	// Cache for active alerts.
	activeAlerts   map[string][]*models.PriceAlert
	alertsMutex    sync.RWMutex

	// HTTP client for notifications.
	httpClient *http.Client
}

// NewPriceAlertService creates a new price alert service.
func NewPriceAlertService(logger *log.Helper) *PriceAlertService {
	return &PriceAlertService{
		db:           db.Get(),
		logger:       logger,
		activeAlerts: make(map[string][]*models.PriceAlert),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CreateAlert creates a new price alert.
func (s *PriceAlertService) CreateAlert(ctx context.Context, alert *models.PriceAlert) error {
	// Validate parameters.
	if err := s.validateAlert(alert); err != nil {
		return err
	}

	// Set default values.
	if alert.Status == "" {
		alert.Status = models.PriceAlertStatusActive
	}
	if alert.CooldownMinutes == 0 {
		alert.CooldownMinutes = 60 // Default cooldown is 1 hour.
	}

	// Save to database.
	if err := s.db.WithContext(ctx).Create(alert).Error; err != nil {
		return errors.Wrap(err, "failed to create alert")
	}

	// Add to active cache.
	s.addAlertToCache(alert)

	s.logger.Infow("price alert created",
		"id", alert.ID,
		"symbol", alert.Symbol,
		"type", alert.AlertType,
	)

	return nil
}

// UpdateAlert updates a price alert.
func (s *PriceAlertService) UpdateAlert(ctx context.Context, alert *models.PriceAlert) error {
	if err := s.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert")
	}

	// Update cache.
	s.updateAlertInCache(alert)

	return nil
}

// DeleteAlert deletes a price alert.
func (s *PriceAlertService) DeleteAlert(ctx context.Context, alertID uint) error {
	var alert models.PriceAlert
	if err := s.db.WithContext(ctx).First(&alert, alertID).Error; err != nil {
		return errors.Wrap(err, "alert not found")
	}

	if err := s.db.WithContext(ctx).Delete(&alert).Error; err != nil {
		return errors.Wrap(err, "failed to delete alert")
	}

	// Remove from cache.
	s.removeAlertFromCache(&alert)

	return nil
}

// GetAlert gets a single alert by ID.
func (s *PriceAlertService) GetAlert(ctx context.Context, alertID uint) (*models.PriceAlert, error) {
	var alert models.PriceAlert
	if err := s.db.WithContext(ctx).First(&alert, alertID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Errorf("alert not found: %d", alertID)
		}
		return nil, errors.Wrap(err, "failed to get alert")
	}
	return &alert, nil
}

// ListAlerts queries the list of price alerts.
func (s *PriceAlertService) ListAlerts(ctx context.Context, query *models.PriceAlertQuery) ([]*models.PriceAlert, int64, error) {
	dbQuery := s.db.WithContext(ctx).Model(&models.PriceAlert{})

	if query.Symbol != "" {
		dbQuery = dbQuery.Where("symbol = ?", query.Symbol)
	}
	if query.Status != "" {
		dbQuery = dbQuery.Where("status = ?", query.Status)
	}
	if query.AlertType != "" {
		dbQuery = dbQuery.Where("alert_type = ?", query.AlertType)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrap(err, "failed to count alerts")
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	var alerts []*models.PriceAlert
	offset := (page - 1) * pageSize
	if err := dbQuery.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&alerts).Error; err != nil {
		return nil, 0, errors.Wrap(err, "failed to list alerts")
	}

	return alerts, total, nil
}

// CheckPriceAndTrigger checks price and triggers alerts.
func (s *PriceAlertService) CheckPriceAndTrigger(ctx context.Context, symbol string, currentPrice float64) error {
	alerts := s.getAlertsForSymbol(symbol)

	for _, alert := range alerts {
		if !alert.ShouldTrigger(currentPrice) {
			continue
		}

		// Trigger the alert.
		if err := s.triggerAlert(ctx, alert, currentPrice); err != nil {
			s.logger.Warnw("failed to trigger alert",
				"alert_id", alert.ID,
				"error", err,
			)
		}
	}

	return nil
}

// triggerAlert triggers a single alert.
func (s *PriceAlertService) triggerAlert(ctx context.Context, alert *models.PriceAlert, currentPrice float64) error {
	// Record trigger.
	alert.RecordTrigger(currentPrice)

	// Update database.
	if err := s.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert trigger count")
	}

	// Create history record.
	history := &models.PriceAlertHistory{
		AlertID:        alert.ID,
		Symbol:         alert.Symbol,
		AlertType:      string(alert.AlertType),
		TriggerPrice:   currentPrice,
		ReferencePrice: alert.ReferencePrice,
	}

	if alert.ReferencePrice > 0 {
		history.ChangePercent = (currentPrice - alert.ReferencePrice) / alert.ReferencePrice * 100
	}

	// Send notification.
	if err := s.sendNotification(ctx, alert, currentPrice); err != nil {
		history.NotificationError = err.Error()
		s.logger.Warnw("notification failed",
			"alert_id", alert.ID,
			"error", err,
		)
	} else {
		history.NotificationSent = true
	}

	// Save history record.
	if err := s.db.WithContext(ctx).Create(history).Error; err != nil {
		s.logger.Warnw("failed to save alert history", "error", err)
	}

	s.logger.Infow("alert triggered",
		"alert_id", alert.ID,
		"symbol", alert.Symbol,
		"price", currentPrice,
		"type", alert.AlertType,
	)

	return nil
}

// sendNotification sends notifications.
func (s *PriceAlertService) sendNotification(ctx context.Context, alert *models.PriceAlert, currentPrice float64) error {
	// Send webhook notification.
	if alert.WebhookURL != "" {
		if err := s.sendWebhookNotification(ctx, alert, currentPrice); err != nil {
			return err
		}
	}

	// Can be extended with other notification channels (email, Slack, etc.).

	return nil
}

// sendWebhookNotification sends a webhook notification.
func (s *PriceAlertService) sendWebhookNotification(ctx context.Context, alert *models.PriceAlert, currentPrice float64) error {
	payload := map[string]interface{}{
		"alert_id":        alert.ID,
		"symbol":          alert.Symbol,
		"alert_type":      alert.AlertType,
		"current_price":   currentPrice,
		"threshold_price": alert.ThresholdPrice,
		"threshold_pct":   alert.ThresholdPercent,
		"reference_price": alert.ReferencePrice,
		"triggered_at":    time.Now().Unix(),
		"description":     alert.Description,
	}

	jsonData, err := sonic.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal webhook payload")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", alert.WebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return errors.Wrap(err, "failed to create webhook request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "webhook request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.Errorf("webhook returned error status: %d", resp.StatusCode)
	}

	return nil
}

// LoadActiveAlerts loads active alerts from database into cache.
func (s *PriceAlertService) LoadActiveAlerts(ctx context.Context) error {
	var alerts []*models.PriceAlert
	if err := s.db.WithContext(ctx).
		Where("status = ?", models.PriceAlertStatusActive).
		Find(&alerts).Error; err != nil {
		return errors.Wrap(err, "failed to load active alerts")
	}

	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	s.activeAlerts = make(map[string][]*models.PriceAlert)
	for _, alert := range alerts {
		s.activeAlerts[alert.Symbol] = append(s.activeAlerts[alert.Symbol], alert)
	}

	s.logger.Infow("loaded active alerts", "count", len(alerts))
	return nil
}

// getAlertsForSymbol gets alerts for a specific symbol.
func (s *PriceAlertService) getAlertsForSymbol(symbol string) []*models.PriceAlert {
	s.alertsMutex.RLock()
	defer s.alertsMutex.RUnlock()

	return s.activeAlerts[symbol]
}

// addAlertToCache adds an alert to the cache.
func (s *PriceAlertService) addAlertToCache(alert *models.PriceAlert) {
	if alert.Status != models.PriceAlertStatusActive {
		return
	}

	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	s.activeAlerts[alert.Symbol] = append(s.activeAlerts[alert.Symbol], alert)
}

// updateAlertInCache updates an alert in the cache.
func (s *PriceAlertService) updateAlertInCache(alert *models.PriceAlert) {
	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	alerts := s.activeAlerts[alert.Symbol]
	for i, a := range alerts {
		if a.ID == alert.ID {
			if alert.Status == models.PriceAlertStatusActive {
				alerts[i] = alert
			} else {
				// Remove non-active alerts.
				s.activeAlerts[alert.Symbol] = append(alerts[:i], alerts[i+1:]...)
			}
			return
		}
	}

	// Add to cache if it's a new active alert.
	if alert.Status == models.PriceAlertStatusActive {
		s.activeAlerts[alert.Symbol] = append(s.activeAlerts[alert.Symbol], alert)
	}
}

// removeAlertFromCache removes an alert from the cache.
func (s *PriceAlertService) removeAlertFromCache(alert *models.PriceAlert) {
	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	alerts := s.activeAlerts[alert.Symbol]
	for i, a := range alerts {
		if a.ID == alert.ID {
			s.activeAlerts[alert.Symbol] = append(alerts[:i], alerts[i+1:]...)
			return
		}
	}
}

// validateAlert validates alert parameters.
func (s *PriceAlertService) validateAlert(alert *models.PriceAlert) error {
	if alert.Symbol == "" {
		return errors.New("symbol is required")
	}

	if alert.AlertType == "" {
		return errors.New("alert_type is required")
	}

	switch alert.AlertType {
	case models.PriceAlertTypeAbove, models.PriceAlertTypeBelow:
		if alert.ThresholdPrice <= 0 {
			return errors.New("threshold_price is required for price alerts")
		}
	case models.PriceAlertTypeChangeUp, models.PriceAlertTypeChangeDown:
		if alert.ThresholdPercent <= 0 {
			return errors.New("threshold_percent is required for change alerts")
		}
		if alert.ReferencePrice <= 0 {
			return errors.New("reference_price is required for change alerts")
		}
	default:
		return errors.Errorf("invalid alert_type: %s", alert.AlertType)
	}

	return nil
}

// GetAlertHistory gets the alert trigger history.
func (s *PriceAlertService) GetAlertHistory(ctx context.Context, alertID uint, limit int) ([]*models.PriceAlertHistory, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var history []*models.PriceAlertHistory
	if err := s.db.WithContext(ctx).
		Where("alert_id = ?", alertID).
		Order("created_at DESC").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, errors.Wrap(err, "failed to get alert history")
	}

	return history, nil
}
