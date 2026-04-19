package executor

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLExecutor MySQL 数据库执行器
type MySQLExecutor struct {
	BaseExecutor
}

// NewMySQLExecutor 创建 MySQL 执行器
func NewMySQLExecutor(dsn string) *MySQLExecutor {
	return &MySQLExecutor{
		BaseExecutor: BaseExecutor{
			DSN: dsn,
		},
	}
}

// Connect 连接到 MySQL 数据库
func (e *MySQLExecutor) Connect() error {
	db, err := sql.Open("mysql", e.DSN)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping MySQL: %w", err)
	}

	e.DB = db
	return nil
}

// Execute 执行 SQL（包装基类方法以满足接口）
func (e *MySQLExecutor) Execute(query string) (*ExecuteResult, error) {
	return e.BaseExecutor.Execute(query)
}

// ExecuteQuery 执行查询语句
func (e *MySQLExecutor) ExecuteQuery(query string) (*QueryResult, error) {
	return e.BaseExecutor.ExecuteQuery(query)
}

// ExecuteExec 执行非查询语句
func (e *MySQLExecutor) ExecuteExec(query string) (*ExecResult, error) {
	return e.BaseExecutor.ExecuteExec(query)
}

// ExecuteMultiple 执行多条 SQL 语句
func (e *MySQLExecutor) ExecuteMultiple(queries []string) ([]*ExecuteResult, error) {
	var results []*ExecuteResult

	for _, query := range queries {
		result, err := e.Execute(query)
		if err != nil {
			return results, fmt.Errorf("failed to execute query '%s': %w", truncateQuery(query), err)
		}
		results = append(results, result)
	}

	return results, nil
}
