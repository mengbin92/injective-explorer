// Package service provides tests for price alert functionality.
package service

import (
	"testing"
	"time"

	"injective-explorer/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestPriceAlert_IsCooldownActive(t *testing.T) {
	tests := []struct {
		name        string
		lastTriggered time.Time
		cooldown    int
		expected    bool
	}{
		{
			name:        "no last triggered",
			lastTriggered: time.Time{},
			cooldown:    60,
			expected:    false,
		},
		{
			name:        "cooldown active",
			lastTriggered: time.Now().Add(-5 * time.Minute),
			cooldown:    60,
			expected:    true,
		},
		{
			name:        "cooldown expired",
			lastTriggered: time.Now().Add(-2 * time.Hour),
			cooldown:    60,
			expected:    false,
		},
		{
			name:        "no cooldown",
			lastTriggered: time.Now(),
			cooldown:    0,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := &models.PriceAlert{
				LastTriggered:   tt.lastTriggered,
				CooldownMinutes: tt.cooldown,
			}
			assert.Equal(t, tt.expected, alert.IsCooldownActive())
		})
	}
}

func TestPriceAlert_CanTrigger(t *testing.T) {
	tests := []struct {
		name         string
		status       models.PriceAlertStatus
		cooldown     int
		lastTriggered time.Time
		maxTriggers  int
		triggerCount int
		expected     bool
	}{
		{
			name:     "active alert",
			status:   models.PriceAlertStatusActive,
			expected: true,
		},
		{
			name:     "disabled alert",
			status:   models.PriceAlertStatusDisabled,
			expected: false,
		},
		{
			name:     "triggered alert",
			status:   models.PriceAlertStatusTriggered,
			expected: false,
		},
		{
			name:         "max triggers reached",
			status:       models.PriceAlertStatusActive,
			maxTriggers:  5,
			triggerCount: 5,
			expected:     false,
		},
		{
			name:         "can trigger again",
			status:       models.PriceAlertStatusActive,
			maxTriggers:  5,
			triggerCount: 3,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := &models.PriceAlert{
				Status:        tt.status,
				CooldownMinutes: tt.cooldown,
				LastTriggered: tt.lastTriggered,
				MaxTriggers:   tt.maxTriggers,
				TriggerCount:  tt.triggerCount,
			}
			assert.Equal(t, tt.expected, alert.CanTrigger())
		})
	}
}

func TestPriceAlert_ShouldTrigger(t *testing.T) {
	tests := []struct {
		name           string
		alertType      models.PriceAlertType
		thresholdPrice float64
		thresholdPct   float64
		referencePrice float64
		currentPrice   float64
		expected       bool
	}{
		{
			name:           "above threshold - should trigger",
			alertType:      models.PriceAlertTypeAbove,
			thresholdPrice: 100,
			currentPrice:   110,
			expected:       true,
		},
		{
			name:           "above threshold - should not trigger",
			alertType:      models.PriceAlertTypeAbove,
			thresholdPrice: 100,
			currentPrice:   90,
			expected:       false,
		},
		{
			name:           "below threshold - should trigger",
			alertType:      models.PriceAlertTypeBelow,
			thresholdPrice: 100,
			currentPrice:   90,
			expected:       true,
		},
		{
			name:           "below threshold - should not trigger",
			alertType:      models.PriceAlertTypeBelow,
			thresholdPrice: 100,
			currentPrice:   110,
			expected:       false,
		},
		{
			name:           "change up - should trigger",
			alertType:      models.PriceAlertTypeChangeUp,
			thresholdPct:   5,
			referencePrice: 100,
			currentPrice:   106,
			expected:       true,
		},
		{
			name:           "change up - should not trigger",
			alertType:      models.PriceAlertTypeChangeUp,
			thresholdPct:   5,
			referencePrice: 100,
			currentPrice:   102,
			expected:       false,
		},
		{
			name:           "change down - should trigger",
			alertType:      models.PriceAlertTypeChangeDown,
			thresholdPct:   5,
			referencePrice: 100,
			currentPrice:   94,
			expected:       true,
		},
		{
			name:           "change down - should not trigger",
			alertType:      models.PriceAlertTypeChangeDown,
			thresholdPct:   5,
			referencePrice: 100,
			currentPrice:   98,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := &models.PriceAlert{
				Status:           models.PriceAlertStatusActive,
				AlertType:        tt.alertType,
				ThresholdPrice:   tt.thresholdPrice,
				ThresholdPercent: tt.thresholdPct,
				ReferencePrice:   tt.referencePrice,
			}
			assert.Equal(t, tt.expected, alert.ShouldTrigger(tt.currentPrice))
		})
	}
}

func TestPriceAlert_RecordTrigger(t *testing.T) {
	alert := &models.PriceAlert{
		TriggerCount: 0,
	}

	alert.RecordTrigger(100.5)
	assert.Equal(t, 1, alert.TriggerCount)
	assert.Equal(t, 100.5, alert.LastPrice)
	assert.False(t, alert.LastTriggered.IsZero())

	alert.RecordTrigger(110.0)
	assert.Equal(t, 2, alert.TriggerCount)
	assert.Equal(t, 110.0, alert.LastPrice)
}

func TestAggregatedPriceData_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		data     *models.AggregatedPriceData
		expected bool
	}{
		{
			name: "complete data",
			data: &models.AggregatedPriceData{
				High: 110,
				Low:  90,
			},
			expected: true,
		},
		{
			name: "invalid high low",
			data: &models.AggregatedPriceData{
				High: 90,
				Low:  110,
			},
			expected: false,
		},
		{
			name: "zero high",
			data: &models.AggregatedPriceData{
				High: 0,
				Low:  90,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.data.IsComplete())
		})
	}
}

func TestAggregatedPriceData_GetPriceChange(t *testing.T) {
	tests := []struct {
		name     string
		open     float64
		close    float64
		expected float64
	}{
		{
			name:     "price up",
			open:     100,
			close:    110,
			expected: 10.0,
		},
		{
			name:     "price down",
			open:     100,
			close:    90,
			expected: -10.0,
		},
		{
			name:     "no change",
			open:     100,
			close:    100,
			expected: 0.0,
		},
		{
			name:     "zero open",
			open:     0,
			close:    100,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &models.AggregatedPriceData{
				Open:  tt.open,
				Close: tt.close,
			}
			assert.InDelta(t, tt.expected, data.GetPriceChange(), 0.0001)
		})
	}
}

func TestPriceAlertService_validateAlert(t *testing.T) {
	// Create service without database for validation testing
	service := &PriceAlertService{}

	tests := []struct {
		name      string
		alert     *models.PriceAlert
		wantError bool
	}{
		{
			name: "valid above alert",
			alert: &models.PriceAlert{
				Symbol:    "INJ",
				AlertType: models.PriceAlertTypeAbove,
				ThresholdPrice: 100,
			},
			wantError: false,
		},
		{
			name: "missing symbol",
			alert: &models.PriceAlert{
				AlertType: models.PriceAlertTypeAbove,
				ThresholdPrice: 100,
			},
			wantError: true,
		},
		{
			name: "missing alert type",
			alert: &models.PriceAlert{
				Symbol: "INJ",
				ThresholdPrice: 100,
			},
			wantError: true,
		},
		{
			name: "missing threshold for above alert",
			alert: &models.PriceAlert{
				Symbol:    "INJ",
				AlertType: models.PriceAlertTypeAbove,
			},
			wantError: true,
		},
		{
			name: "valid change alert",
			alert: &models.PriceAlert{
				Symbol:           "INJ",
				AlertType:        models.PriceAlertTypeChangeUp,
				ThresholdPercent: 5,
				ReferencePrice:   100,
			},
			wantError: false,
		},
		{
			name: "missing reference price for change alert",
			alert: &models.PriceAlert{
				Symbol:           "INJ",
				AlertType:        models.PriceAlertTypeChangeUp,
				ThresholdPercent: 5,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateAlert(tt.alert)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHealthChecker_calculateOverallStatus(t *testing.T) {
	checker := &HealthChecker{}

	tests := []struct {
		name       string
		components []*ComponentHealth
		expected   HealthStatus
	}{
		{
			name: "all healthy",
			components: []*ComponentHealth{
				{Name: "a", Status: HealthStatusHealthy},
				{Name: "b", Status: HealthStatusHealthy},
			},
			expected: HealthStatusHealthy,
		},
		{
			name: "one degraded",
			components: []*ComponentHealth{
				{Name: "a", Status: HealthStatusHealthy},
				{Name: "b", Status: HealthStatusDegraded},
			},
			expected: HealthStatusDegraded,
		},
		{
			name: "one unhealthy",
			components: []*ComponentHealth{
				{Name: "a", Status: HealthStatusHealthy},
				{Name: "b", Status: HealthStatusUnhealthy},
			},
			expected: HealthStatusUnhealthy,
		},
		{
			name: "degraded and unhealthy",
			components: []*ComponentHealth{
				{Name: "a", Status: HealthStatusDegraded},
				{Name: "b", Status: HealthStatusUnhealthy},
			},
			expected: HealthStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.calculateOverallStatus(tt.components)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHealthChecker_LivenessCheck(t *testing.T) {
	checker := &HealthChecker{}
	assert.True(t, checker.LivenessCheck())
}

// Benchmark alert checking
func BenchmarkPriceAlert_ShouldTrigger(b *testing.B) {
	alert := &models.PriceAlert{
		Status:         models.PriceAlertStatusActive,
		AlertType:      models.PriceAlertTypeAbove,
		ThresholdPrice: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alert.ShouldTrigger(110)
	}
}
