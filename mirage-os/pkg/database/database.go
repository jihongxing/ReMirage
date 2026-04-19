// Package database - 数据库连接管理
package database

import (
	"fmt"
	"log"
	"time"

	"mirage-os/pkg/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config 数据库配置
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	TimeZone string
}

// DB 全局数据库实例
var DB *gorm.DB

// Connect 连接数据库
func Connect(cfg *Config) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode, cfg.TimeZone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库实例失败: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db
	log.Println("✅ 数据库连接成功")
	return nil
}

// Migrate 执行数据库迁移
func Migrate() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}

	log.Println("🔄 开始数据库迁移...")
	if err := models.AutoMigrate(DB); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	log.Println("✅ 数据库迁移完成")
	return nil
}

// Close 关闭数据库连接
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return DB
}

// InitDefaultCells 初始化默认蜂窝
func InitDefaultCells() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}

	defaultCells := []models.Cell{
		{
			CellID:         "cell-us-west-01",
			CellName:       "US West Standard",
			RegionCode:     "US-West",
			Country:        "US",
			City:           "Los Angeles",
			CellLevel:      1,
			CostMultiplier: 1.0,
			Status:         "active",
		},
		{
			CellID:         "cell-is-standard-01",
			CellName:       "Iceland Standard",
			RegionCode:     "EU-North",
			Country:        "IS",
			City:           "Reykjavik",
			Latitude:       64.1466,
			Longitude:      -21.9426,
			CellLevel:      1,
			CostMultiplier: 1.0,
			Status:         "active",
		},
		{
			CellID:         "cell-ch-platinum-01",
			CellName:       "Switzerland Platinum",
			RegionCode:     "EU-Central",
			Country:        "CH",
			City:           "Zurich",
			Latitude:       47.3769,
			Longitude:      8.5417,
			CellLevel:      2,
			CostMultiplier: 1.5,
			Status:         "active",
		},
		{
			CellID:         "cell-hk-01",
			CellName:       "Hong Kong Platinum",
			RegionCode:     "HK-01",
			Country:        "HK",
			City:           "Hong Kong",
			CellLevel:      2,
			CostMultiplier: 1.5,
			Status:         "active",
		},
		{
			CellID:         "cell-sg-diamond-01",
			CellName:       "Singapore Diamond",
			RegionCode:     "SG-01",
			Country:        "SG",
			City:           "Singapore",
			Latitude:       1.3521,
			Longitude:      103.8198,
			CellLevel:      3,
			CostMultiplier: 2.0,
			Status:         "active",
		},
	}

	for _, cell := range defaultCells {
		var existing models.Cell
		result := DB.Where("cell_id = ?", cell.CellID).First(&existing)
		if result.Error == gorm.ErrRecordNotFound {
			if err := DB.Create(&cell).Error; err != nil {
				log.Printf("⚠️  创建蜂窝 %s 失败: %v", cell.CellID, err)
			} else {
				log.Printf("✅ 创建蜂窝: %s", cell.CellName)
			}
		}
	}

	return nil
}
