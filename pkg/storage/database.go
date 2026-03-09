package storage

import (
	"fmt"
	"log"
	"strings"
	"time"

	"stock-options/pkg/forecast"
	"stock-options/pkg/model"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Database struct {
	DB *gorm.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	return NewDatabaseWithConfig("sqlite", "", dbPath)
}

func NewDatabaseWithConfig(driver string, dsn string, dbPath string) (*Database, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		driver = "sqlite"
	}

	var (
		db  *gorm.DB
		err error
	)
	switch driver {
	case "postgres", "postgresql":
		if strings.TrimSpace(dsn) == "" {
			return nil, fmt.Errorf("DB_DSN is required when DB_DRIVER is postgres")
		}
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		if strings.TrimSpace(dbPath) == "" {
			dbPath = "stock_data.db"
		}
		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	}
	if err != nil {
		return nil, err
	}

	// Migrate the schema
	err = db.AutoMigrate(&model.StockData{}, &model.ForecastResult{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	return &Database{DB: db}, nil
}

func (d *Database) SaveStockData(stockData []model.StockData) error {
	for _, data := range stockData {
		if err := d.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "ticker"}, {Name: "trading_date"}},
			DoUpdates: clause.AssignmentColumns([]string{"open_price", "high_price", "low_price", "close_price", "adjusted_close", "volume", "updated_at"}),
		}).Create(&data).Error; err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) GetStockData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, error) {
	var stockData []model.StockData
	result := d.DB.Where("ticker = ? AND trading_date BETWEEN ? AND ?", ticker, startDate, endDate).
		Order("trading_date asc").
		Find(&stockData)
	if result.Error != nil {
		return nil, result.Error
	}
	return stockData, nil
}

func (d *Database) SaveForecastResult(forecast model.ForecastResult) error {
	update := d.DB.Model(&model.ForecastResult{}).
		Where("ticker = ?", forecast.Ticker).
		Updates(map[string]interface{}{
			"current_year":                    forecast.CurrentYear,
			"remaining_months":                forecast.RemainingMonths,
			"current_year_remaining_forecast": forecast.CurrentYearRemainingForecast,
			"next_year":                       forecast.NextYear,
			"next_year_forecast":              forecast.NextYearForecast,
			"year_after_next":                 forecast.YearAfterNext,
			"year_after_next_forecast":        forecast.YearAfterNextForecast,
			"regression_slope":                forecast.RegressionSlope,
			"regression_intercept":            forecast.RegressionIntercept,
			"generated_at":                    forecast.GeneratedAt,
		})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected > 0 {
		return nil
	}
	return d.DB.Create(&forecast).Error
}

func (d *Database) GetForecastResult(ticker string) (*model.ForecastResult, error) {
	var forecast model.ForecastResult
	result := d.DB.Where("ticker = ?", ticker).First(&forecast)
	if result.Error != nil {
		return nil, result.Error
	}
	return &forecast, nil
}

func (d *Database) IsDataUpToDate(ticker string, maxDate time.Time) bool {
	var count int64
	d.DB.Model(&model.StockData{}).
		Where("ticker = ? AND trading_date > ?", ticker, maxDate).
		Count(&count)
	return count == 0
}

func (d *Database) HasStockData(ticker string) (bool, error) {
	var count int64
	if err := d.DB.Model(&model.StockData{}).
		Where("ticker = ?", ticker).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *Database) GetLatestTradingDate(ticker string) (time.Time, bool, error) {
	var stocks []model.StockData
	err := d.DB.Where("ticker = ?", ticker).Order("trading_date desc").Limit(1).Find(&stocks).Error
	if err != nil {
		return time.Time{}, false, err
	}
	if len(stocks) == 0 {
		return time.Time{}, false, nil
	}
	return stocks[0].TradingDate, true, nil
}

func (d *Database) GenerateForecast(ticker string) (*model.ForecastResult, error) {
	var series []model.StockData
	if err := d.DB.Where("ticker = ?", ticker).Order("trading_date asc").Find(&series).Error; err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, fmt.Errorf("no historical data for ticker %s", ticker)
	}

	engine := forecast.LinearRegressionForecaster{}
	forecastResult, err := engine.Forecast(ticker, series, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if err := d.SaveForecastResult(*forecastResult); err != nil {
		return nil, err
	}
	return forecastResult, nil
}

func (d *Database) GenerateAdvancedAnalysis(ticker string) (*model.AdvancedAnalysis, error) {
	var series []model.StockData
	if err := d.DB.Where("ticker = ?", ticker).Order("trading_date asc").Find(&series).Error; err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, fmt.Errorf("no historical data for ticker %s", ticker)
	}
	return forecast.AnalyzeAdvanced(ticker, series, time.Now().UTC())
}

func (d *Database) Ping() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
