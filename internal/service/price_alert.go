// Package service provides price alert functionality.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// PriceAlertService 价格预警服务
type PriceAlertService struct {
	db     *gorm.DB
	logger *log.Helper

	// 活跃预警缓存
	activeAlerts   map[string][]*models.PriceAlert
	alertsMutex    sync.RWMutex

	// 通知客户端
	httpClient *http.Client
}

// NewPriceAlertService 创建价格预警服务
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

// CreateAlert 创建价格预警
func (s *PriceAlertService) CreateAlert(ctx context.Context, alert *models.PriceAlert) error {
	// 验证参数
	if err := s.validateAlert(alert); err != nil {
		return err
	}

	// 设置默认值
	if alert.Status == "" {
		alert.Status = models.PriceAlertStatusActive
	}
	if alert.CooldownMinutes == 0 {
		alert.CooldownMinutes = 60 // 默认1小时冷却
	}

	// 保存到数据库
	if err := s.db.WithContext(ctx).Create(alert).Error; err != nil {
		return errors.Wrap(err, "failed to create alert")
	}

	// 添加到活跃缓存
	s.addAlertToCache(alert)

	s.logger.Infow("price alert created",
		"id", alert.ID,
		"symbol", alert.Symbol,
		"type", alert.AlertType,
	)

	return nil
}

// UpdateAlert 更新价格预警
func (s *PriceAlertService) UpdateAlert(ctx context.Context, alert *models.PriceAlert) error {
	if err := s.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert")
	}

	// 更新缓存
	s.updateAlertInCache(alert)

	return nil
}

// DeleteAlert 删除价格预警
func (s *PriceAlertService) DeleteAlert(ctx context.Context, alertID uint) error {
	var alert models.PriceAlert
	if err := s.db.WithContext(ctx).First(&alert, alertID).Error; err != nil {
		return errors.Wrap(err, "alert not found")
	}

	if err := s.db.WithContext(ctx).Delete(&alert).Error; err != nil {
		return errors.Wrap(err, "failed to delete alert")
	}

	// 从缓存移除
	s.removeAlertFromCache(&alert)

	return nil
}

// GetAlert 获取单个预警详情
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

// ListAlerts 查询预警列表
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

// CheckPriceAndTrigger 检查价格并触发预警
func (s *PriceAlertService) CheckPriceAndTrigger(ctx context.Context, symbol string, currentPrice float64) error {
	alerts := s.getAlertsForSymbol(symbol)

	for _, alert := range alerts {
		if !alert.ShouldTrigger(currentPrice) {
			continue
		}

		// 触发预警
		if err := s.triggerAlert(ctx, alert, currentPrice); err != nil {
			s.logger.Warnw("failed to trigger alert",
				"alert_id", alert.ID,
				"error", err,
			)
		}
	}

	return nil
}

// triggerAlert 触发单个预警
func (s *PriceAlertService) triggerAlert(ctx context.Context, alert *models.PriceAlert, currentPrice float64) error {
	// 记录触发
	alert.RecordTrigger(currentPrice)

	// 更新数据库
	if err := s.db.WithContext(ctx).Save(alert).Error; err != nil {
		return errors.Wrap(err, "failed to update alert trigger count")
	}

	// 创建历史记录
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

	// 发送通知
	if err := s.sendNotification(ctx, alert, currentPrice); err != nil {
		history.NotificationError = err.Error()
		s.logger.Warnw("notification failed",
			"alert_id", alert.ID,
			"error", err,
		)
	} else {
		history.NotificationSent = true
	}

	// 保存历史记录
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

// sendNotification 发送通知
func (s *PriceAlertService) sendNotification(ctx context.Context, alert *models.PriceAlert, currentPrice float64) error {
	// 发送 Webhook 通知
	if alert.WebhookURL != "" {
		if err := s.sendWebhookNotification(ctx, alert, currentPrice); err != nil {
			return err
		}
	}

	// 可以扩展其他通知渠道（邮件、Slack等）

	return nil
}

// sendWebhookNotification 发送 Webhook 通知
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

	jsonData, err := json.Marshal(payload)
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

// LoadActiveAlerts 从数据库加载活跃预警到缓存
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

// getAlertsForSymbol 获取某个币种的预警
func (s *PriceAlertService) getAlertsForSymbol(symbol string) []*models.PriceAlert {
	s.alertsMutex.RLock()
	defer s.alertsMutex.RUnlock()

	return s.activeAlerts[symbol]
}

// addAlertToCache 添加预警到缓存
func (s *PriceAlertService) addAlertToCache(alert *models.PriceAlert) {
	if alert.Status != models.PriceAlertStatusActive {
		return
	}

	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	s.activeAlerts[alert.Symbol] = append(s.activeAlerts[alert.Symbol], alert)
}

// updateAlertInCache 更新缓存中的预警
func (s *PriceAlertService) updateAlertInCache(alert *models.PriceAlert) {
	s.alertsMutex.Lock()
	defer s.alertsMutex.Unlock()

	alerts := s.activeAlerts[alert.Symbol]
	for i, a := range alerts {
		if a.ID == alert.ID {
			if alert.Status == models.PriceAlertStatusActive {
				alerts[i] = alert
			} else {
				// 移除非活跃预警
				s.activeAlerts[alert.Symbol] = append(alerts[:i], alerts[i+1:]...)
			}
			return
		}
	}

	// 如果是新的活跃预警，添加到缓存
	if alert.Status == models.PriceAlertStatusActive {
		s.activeAlerts[alert.Symbol] = append(s.activeAlerts[alert.Symbol], alert)
	}
}

// removeAlertFromCache 从缓存中移除预警
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

// validateAlert 验证预警参数
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

// GetAlertHistory 获取预警触发历史
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
