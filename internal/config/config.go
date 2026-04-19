// Package config 提供配置管理功能，支持命令行参数和环境变量
package config

import (
	"fmt"
	"os"
	"strconv"
)

// DatabaseType 数据库类型
type DatabaseType string

const (
	DatabaseTypeMySQL   DatabaseType = "mysql"
	DatabaseTypeGaussDB DatabaseType = "gaussdb"
)

// DatabaseConfig 数据库连接配置
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string // 主要用于 GaussDB/PostgreSQL
	Charset  string // 主要用于 MySQL
}

// Config 应用程序配置
type Config struct {
	// 输入输出配置
	InputFile  string
	OutputFile string
	SQLQuery   string

	// 转换器配置
	UseTimestampTZ bool
	UseJSONB       bool

	// 数据库执行配置
	ExecuteMySQL   bool
	ExecuteGaussDB bool
	MySQLConfig    *DatabaseConfig
	GaussDBConfig  *DatabaseConfig

	// 语法验证配置
	ValidateMySQL   bool
	ValidateGaussDB bool
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		UseTimestampTZ: true,
		UseJSONB:       true,
		MySQLConfig:    NewMySQLConfig(),
		GaussDBConfig:  NewGaussDBConfig(),
	}
}

// NewMySQLConfig 创建默认 MySQL 配置
func NewMySQLConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:    "localhost",
		Port:    3306,
		SSLMode: "false",
		Charset: "utf8mb4",
	}
}

// NewGaussDBConfig 创建默认 GaussDB 配置
func NewGaussDBConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:    "localhost",
		Port:    5432,
		SSLMode: "disable",
	}
}

// DSN 返回数据库连接字符串
func (c *DatabaseConfig) DSN(dbType DatabaseType) string {
	switch dbType {
	case DatabaseTypeMySQL:
		return c.MySQLDSN()
	case DatabaseTypeGaussDB:
		return c.GaussDBDSN()
	default:
		return ""
	}
}

// MySQLDSN 返回 MySQL 连接字符串
func (c *DatabaseConfig) MySQLDSN() string {
	// 格式: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.Charset)
}

// GaussDBDSN 返回 GaussDB 连接字符串
func (c *DatabaseConfig) GaussDBDSN() string {
	// 格式: host=xxx port=xxx user=xxx password=xxx dbname=xxx sslmode=xxx
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

// LoadFromEnv 从环境变量加载配置
func (c *DatabaseConfig) LoadFromEnv(prefix string) {
	if host := os.Getenv(prefix + "_HOST"); host != "" {
		c.Host = host
	}
	if port := os.Getenv(prefix + "_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.Port = p
		}
	}
	if user := os.Getenv(prefix + "_USER"); user != "" {
		c.User = user
	}
	if password := os.Getenv(prefix + "_PASSWORD"); password != "" {
		c.Password = password
	}
	if dbname := os.Getenv(prefix + "_DBNAME"); dbname != "" {
		c.DBName = dbname
	}
	if sslmode := os.Getenv(prefix + "_SSLMODE"); sslmode != "" {
		c.SSLMode = sslmode
	}
	if charset := os.Getenv(prefix + "_CHARSET"); charset != "" {
		c.Charset = charset
	}
}

// ParseDSN 从 DSN 字符串解析配置（简化实现）
func (c *DatabaseConfig) ParseDSN(dsn string) error {
	// 这里可以添加更完整的 DSN 解析逻辑
	// 目前简单处理，主要依赖用户提供完整参数
	return nil
}

// Validate 验证配置是否完整
func (c *DatabaseConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 {
		return fmt.Errorf("port must be positive")
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("dbname is required")
	}
	return nil
}

// LoadMySQLConfigFromEnv 加载 MySQL 配置
func LoadMySQLConfigFromEnv() *DatabaseConfig {
	cfg := NewMySQLConfig()
	cfg.LoadFromEnv("MYSQL")
	return cfg
}

// LoadGaussDBConfigFromEnv 加载 GaussDB 配置
func LoadGaussDBConfigFromEnv() *DatabaseConfig {
	cfg := NewGaussDBConfig()
	cfg.LoadFromEnv("GAUSSDB")
	return cfg
}
