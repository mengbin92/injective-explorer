// Package service provides price data aggregation (OHLCV) functionality.
package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PriceAggregationService 价格聚合服务
type PriceAggregationService struct {
	db     *gorm.DB
	logger *log.Helper
}

// NewPriceAggregationService 创建价格聚合服务
func NewPriceAggregationService(logger *log.Helper) *PriceAggregationService {
	return &PriceAggregationService{
		db:     db.Get(),
		logger: logger,
	}
}

// AggregatePriceData 聚合价格数据生成K线
func (s *PriceAggregationService) AggregatePriceData(
	ctx context.Context,
	symbol string,
	interval string,
	startTime, endTime int64,
) (*models.AggregatedPriceData, error) {
	// 获取该时间段内的价格数据
	var prices []models.OnChainPrice
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND timestamp >= ? AND timestamp < ?", symbol, startTime, endTime).
		Order("timestamp ASC").
		Find(&prices).Error; err != nil {
		return nil, errors.Wrap(err, "failed to fetch price data")
	}

	if len(prices) == 0 {
		return nil, errors.New("no price data found for aggregation")
	}

	// 计算OHLCV
	agg := s.calculateOHLCV(prices, interval, startTime)

	return agg, nil
}

// calculateOHLCV 计算OHLCV数据
func (s *PriceAggregationService) calculateOHLCV(
	prices []models.OnChainPrice,
	interval string,
	timestamp int64,
) *models.AggregatedPriceData {
	if len(prices) == 0 {
		return nil
	}

	open := prices[0].PriceUSD
	close := prices[len(prices)-1].PriceUSD
	high := open
	low := open
	var totalVolume float64
	var totalPrice float64

	for _, p := range prices {
		if p.PriceUSD > high {
			high = p.PriceUSD
		}
		if p.PriceUSD < low {
			low = p.PriceUSD
		}
		totalPrice += p.PriceUSD
		// 简化处理，使用价格作为交易量指标
		// 实际应该使用真实的交易数据
		totalVolume += p.PriceUSD
	}

	avgPrice := totalPrice / float64(len(prices))

	return &models.AggregatedPriceData{
		Symbol:     prices[0].Symbol,
		Interval:   interval,
		Timestamp:  timestamp,
		Open:       open,
		High:       high,
		Low:        low,
		Close:      close,
		Volume:     totalVolume,
		TradeCount: len(prices),
		AvgPrice:   avgPrice,
		DataSource: "onchain",
	}
}

// SaveAggregatedData 保存聚合数据
func (s *PriceAggregationService) SaveAggregatedData(
	ctx context.Context,
	data *models.AggregatedPriceData,
) error {
	// 使用UPSERT避免重复
	result := s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "symbol"}, {Name: "interval"}, {Name: "timestamp"}},
			DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume", "trade_count", "avg_price"}),
		},
	).Create(data)

	if result.Error != nil {
		return errors.Wrap(result.Error, "failed to save aggregated data")
	}

	return nil
}

// GetAggregatedData 查询聚合数据（K线）
func (s *PriceAggregationService) GetAggregatedData(
	ctx context.Context,
	query *models.AggregatedPriceQuery,
) ([]*models.AggregatedPriceData, error) {
	dbQuery := s.db.WithContext(ctx).
		Where("symbol = ?", query.Symbol).
		Where("interval = ?", query.Interval)

	if query.StartTime > 0 {
		dbQuery = dbQuery.Where("timestamp >= ?", query.StartTime)
	}
	if query.EndTime > 0 {
		dbQuery = dbQuery.Where("timestamp <= ?", query.EndTime)
	}

	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var data []*models.AggregatedPriceData
	if err := dbQuery.Order("timestamp ASC").Limit(limit).Find(&data).Error; err != nil {
		return nil, errors.Wrap(err, "failed to get aggregated data")
	}

	return data, nil
}

// BatchAggregateHistoricalData 批量聚合历史数据
func (s *PriceAggregationService) BatchAggregateHistoricalData(
	ctx context.Context,
	symbol string,
	interval string,
	days int,
) error {
	endTime := time.Now().Unix()
	startTime := endTime - int64(days*24*3600)

	// 获取时间间隔（秒）
	intervalSeconds := s.getIntervalSeconds(interval)
	if intervalSeconds == 0 {
		return errors.Errorf("invalid interval: %s", interval)
	}

	s.logger.Infow("starting historical aggregation",
		"symbol", symbol,
		"interval", interval,
		"days", days,
	)

	// 按时间段分批聚合
	for ts := startTime; ts < endTime; ts += intervalSeconds {
		agg, err := s.AggregatePriceData(ctx, symbol, interval, ts, ts+intervalSeconds)
		if err != nil {
			// 没有数据是正常情况，继续下一个时间段
			if err.Error() == "no price data found for aggregation" {
				continue
			}
			s.logger.Warnw("aggregation failed", "timestamp", ts, "error", err)
			continue
		}

		if err := s.SaveAggregatedData(ctx, agg); err != nil {
			s.logger.Warnw("failed to save aggregated data", "error", err)
		}
	}

	s.logger.Infow("historical aggregation completed",
		"symbol", symbol,
		"interval", interval,
	)

	return nil
}

// getIntervalSeconds 获取间隔对应的秒数
func (s *PriceAggregationService) getIntervalSeconds(interval string) int64 {
	switch interval {
	case models.Interval1Minute:
		return 60
	case models.Interval5Minutes:
		return 5 * 60
	case models.Interval15Minutes:
		return 15 * 60
	case models.Interval1Hour:
		return 60 * 60
	case models.Interval4Hours:
		return 4 * 60 * 60
	case models.Interval1Day:
		return 24 * 60 * 60
	case models.Interval1Week:
		return 7 * 24 * 60 * 60
	default:
		return 0
	}
}

// AlignTimestamp 对齐时间戳到区间边界
func (s *PriceAggregationService) AlignTimestamp(timestamp int64, interval string) int64 {
	intervalSeconds := s.getIntervalSeconds(interval)
	if intervalSeconds == 0 {
		return timestamp
	}

	return (timestamp / intervalSeconds) * intervalSeconds
}

// CalculateIndicators 计算技术指标
func (s *PriceAggregationService) CalculateIndicators(
	ctx context.Context,
	symbol string,
	interval string,
	period int, // 计算周期
) (*TechnicalIndicators, error) {
	// 获取最近的数据
	endTime := time.Now().Unix()
	startTime := endTime - int64(period*24*3600)

	query := &models.AggregatedPriceQuery{
		Symbol:    symbol,
		Interval:  interval,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     period * 2, // 获取更多数据以确保计算准确
	}

	data, err := s.GetAggregatedData(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(data) < period {
		return nil, errors.Errorf("insufficient data for indicator calculation: got %d, need %d", len(data), period)
	}

	indicators := &TechnicalIndicators{}

	// 计算SMA
	indicators.SMA = s.calculateSMA(data, period)

	// 计算EMA
	indicators.EMA = s.calculateEMA(data, period)

	// 计算RSI
	indicators.RSI = s.calculateRSI(data, period)

	// 计算波动率
	indicators.Volatility = s.calculateVolatility(data, period)

	return indicators, nil
}

// TechnicalIndicators 技术指标
type TechnicalIndicators struct {
	SMA        float64 `json:"sma"`
	EMA        float64 `json:"ema"`
	RSI        float64 `json:"rsi"`
	Volatility float64 `json:"volatility"`
}

// calculateSMA 计算简单移动平均线
func (s *PriceAggregationService) calculateSMA(data []*models.AggregatedPriceData, period int) float64 {
	if len(data) < period {
		return 0
	}

	var sum float64
	for i := len(data) - period; i < len(data); i++ {
		sum += data[i].Close
	}

	return sum / float64(period)
}

// calculateEMA 计算指数移动平均线
func (s *PriceAggregationService) calculateEMA(data []*models.AggregatedPriceData, period int) float64 {
	if len(data) < period {
		return 0
	}

	// EMA = Price(t) * k + EMA(y) * (1 – k)
	// k = 2 / (N + 1)
	k := 2.0 / float64(period+1)

	// 使用SMA作为初始EMA值
	ema := s.calculateSMA(data[:period], period)

	for i := period; i < len(data); i++ {
		ema = data[i].Close*k + ema*(1-k)
	}

	return ema
}

// calculateRSI 计算相对强弱指数
func (s *PriceAggregationService) calculateRSI(data []*models.AggregatedPriceData, period int) float64 {
	if len(data) < period+1 {
		return 0
	}

	var gains, losses float64
	count := 0

	// 计算平均涨跌
	for i := len(data) - period; i < len(data); i++ {
		change := data[i].Close - data[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
		count++
	}

	if count == 0 {
		return 0
	}

	avgGain := gains / float64(count)
	avgLoss := losses / float64(count)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateVolatility 计算波动率（标准差）
func (s *PriceAggregationService) calculateVolatility(data []*models.AggregatedPriceData, period int) float64 {
	if len(data) < period {
		return 0
	}

	// 计算收益率
	returns := make([]float64, 0, len(data)-1)
	for i := 1; i < len(data); i++ {
		if data[i-1].Close != 0 {
			ret := (data[i].Close - data[i-1].Close) / data[i-1].Close
			returns = append(returns, ret)
		}
	}

	if len(returns) == 0 {
		return 0
	}

	// 计算平均收益率
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// 计算方差
	var variance float64
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns))

	// 返回标准差（日波动率）
	return math.Sqrt(variance)
}

// GetSupportedIntervals 获取支持的K线间隔
func (s *PriceAggregationService) GetSupportedIntervals() []string {
	return []string{
		models.Interval1Minute,
		models.Interval5Minutes,
		models.Interval15Minutes,
		models.Interval1Hour,
		models.Interval4Hours,
		models.Interval1Day,
		models.Interval1Week,
	}
}

// CleanupOldData 清理过期的聚合数据
func (s *PriceAggregationService) CleanupOldData(ctx context.Context, retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays).Unix()

	result := s.db.WithContext(ctx).
		Where("timestamp < ?", cutoffTime).
		Delete(&models.AggregatedPriceData{})

	if result.Error != nil {
		return errors.Wrap(result.Error, "failed to cleanup old data")
	}

	s.logger.Infow("cleaned up old aggregated data",
		"retention_days", retentionDays,
		"deleted_count", result.RowsAffected,
	)

	return nil
}

// FormatKlineResponse 格式化K线数据响应
func (s *PriceAggregationService) FormatKlineResponse(data []*models.AggregatedPriceData) [][]interface{} {
	result := make([][]interface{}, len(data))

	for i, d := range data {
		result[i] = []interface{}{
			d.Timestamp * 1000, // 转换为毫秒时间戳
			fmt.Sprintf("%.8f", d.Open),
			fmt.Sprintf("%.8f", d.High),
			fmt.Sprintf("%.8f", d.Low),
			fmt.Sprintf("%.8f", d.Close),
			fmt.Sprintf("%.4f", d.Volume),
		}
	}

	return result
}
