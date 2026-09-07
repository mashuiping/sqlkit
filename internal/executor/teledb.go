package executor

import (
	"database/sql"
	"fmt"

	_ "gitee.com/opengauss/openGauss-connector-go-pq"
)

// TeledbExecutor Teledb 数据库执行器
// Teledb 与 GaussDB 共享 opengauss 驱动协议，行为与 GaussDBExecutor 一致；
// 保留独立类型以便区分错误上下文与未来可能的方言差异。
type TeledbExecutor struct {
	BaseExecutor
}

// NewTeledbExecutor 创建 Teledb 执行器
func NewTeledbExecutor(dsn string) *TeledbExecutor {
	return &TeledbExecutor{
		BaseExecutor: BaseExecutor{
			DSN: dsn,
		},
	}
}

// Connect 连接到 Teledb 数据库
func (e *TeledbExecutor) Connect() error {
	db, err := sql.Open("opengauss", e.DSN)
	if err != nil {
		return fmt.Errorf("failed to open Teledb connection: %w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping Teledb: %w", err)
	}

	e.DB = db
	return nil
}

// Execute 执行 SQL（包装基类方法以满足接口）
func (e *TeledbExecutor) Execute(query string) (*ExecuteResult, error) {
	return e.BaseExecutor.Execute(query)
}

// ExecuteQuery 执行查询语句
func (e *TeledbExecutor) ExecuteQuery(query string) (*QueryResult, error) {
	return e.BaseExecutor.ExecuteQuery(query)
}

// ExecuteExec 执行非查询语句
func (e *TeledbExecutor) ExecuteExec(query string) (*ExecResult, error) {
	return e.BaseExecutor.ExecuteExec(query)
}

// ExecuteMultiple 执行多条 SQL 语句
func (e *TeledbExecutor) ExecuteMultiple(queries []string) ([]*ExecuteResult, error) {
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
