// Package validator 提供 SQL 在线验证功能（使用数据库事务）
package validator

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "gitee.com/opengauss/openGauss-connector-go-pq"
)

// OnlineValidator 在线验证器接口
type OnlineValidator interface {
	// Validate 在线验证 SQL 语句（在事务中执行后回滚）
	Validate(sql string) *ValidationResult
	// ValidateText 验证包含多条语句的 SQL 文本
	ValidateText(sqlText string) *ValidationResult
	// Connect 连接到数据库
	Connect() error
	// Close 关闭数据库连接
	Close() error
}

// OnlineValidatorBase 在线验证器基础结构
type OnlineValidatorBase struct {
	DB  *sql.DB
	DSN string
}

// Close 关闭数据库连接
func (v *OnlineValidatorBase) Close() error {
	if v.DB != nil {
		return v.DB.Close()
	}
	return nil
}

// isQueryStatement 判断是否为查询语句
func isQueryStatement(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC ") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")
}

// validateInTransaction 在事务中验证单条 SQL
// 注意：某些 DDL 语句可能会自动提交事务
// 但这对验证目的来说是可以接受的，因为我们只关心 SQL 的有效性
func validateInTransaction(db *sql.DB, sqlStmt string) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 确保事务回滚
	defer func() {
		if err := tx.Rollback(); err != nil {
			// 如果回滚失败，可能是事务已经被自动提交（如某些 DDL 语句）
			// 这对验证目的来说是可以接受的
		}
	}()

	// 判断是否为查询语句
	if isQueryStatement(sqlStmt) {
		// 对于查询语句，执行查询并读取结果（但不返回）
		rows, err := tx.Query(sqlStmt)
		if err != nil {
			return fmt.Errorf("查询执行失败: %w", err)
		}
		defer rows.Close()

		// 读取所有行（但不处理数据，只是验证语法和执行计划）
		for rows.Next() {
			// 不扫描数据，只验证能否执行
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("读取查询结果失败: %w", err)
		}
	} else {
		// 对于非查询语句（INSERT/UPDATE/DELETE/CREATE等），执行但不提交
		_, err := tx.Exec(sqlStmt)
		if err != nil {
			return fmt.Errorf("执行失败: %w", err)
		}
		// 注意：某些 DDL 语句可能会自动提交事务，但这对验证来说是可以接受的
	}

	// 回滚事务（defer 中会自动执行）
	// 如果事务已经被自动提交，回滚会失败，但这不影响验证结果
	return nil
}

// StatementError 语句执行错误
type StatementError struct {
	Index   int    // 语句索引（从0开始）
	Line    int    // 语句行号（从1开始）
	SQL     string // SQL 语句
	Message string // 错误信息
}

// validateMultipleInTransaction 在单个事务中验证多条 SQL 语句
// 这样可以正确处理语句之间的依赖关系（如 CREATE TABLE 后 INSERT）
func validateMultipleInTransaction(db *sql.DB, statements []string) []*StatementError {
	var errors []*StatementError

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		// 如果无法开始事务，为所有语句添加错误
		for i, stmt := range statements {
			errors = append(errors, &StatementError{
				Index:   i,
				Line:    i + 1,
				SQL:     stmt,
				Message: fmt.Sprintf("开始事务失败: %v", err),
			})
		}
		return errors
	}

	// 确保事务回滚
	defer func() {
		if err := tx.Rollback(); err != nil {
			// 如果回滚失败，可能是事务已经被自动提交（如某些 DDL 语句）
			// 这对验证目的来说是可以接受的
		}
	}()

	// 逐条执行语句
	for i, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		// 跳过空语句和注释
		if trimmed == "" || strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// 判断是否为查询语句
		if isQueryStatement(trimmed) {
			// 对于查询语句，执行查询并读取结果
			rows, err := tx.Query(trimmed)
			if err != nil {
				errors = append(errors, &StatementError{
					Index:   i,
					Line:    i + 1,
					SQL:     stmt,
					Message: fmt.Sprintf("查询执行失败: %v", err),
				})
				// 继续执行后续语句，收集所有错误
				continue
			}

			// 读取所有行（但不处理数据，只是验证语法和执行计划）
			for rows.Next() {
				// 不扫描数据，只验证能否执行
			}
			readErr := rows.Err()
			rows.Close() // 立即关闭，避免资源泄漏

			if readErr != nil {
				errors = append(errors, &StatementError{
					Index:   i,
					Line:    i + 1,
					SQL:     stmt,
					Message: fmt.Sprintf("读取查询结果失败: %v", readErr),
				})
			}
		} else {
			// 对于非查询语句，执行但不提交
			_, err := tx.Exec(trimmed)
			if err != nil {
				errors = append(errors, &StatementError{
					Index:   i,
					Line:    i + 1,
					SQL:     stmt,
					Message: fmt.Sprintf("执行失败: %v", err),
				})
				// 继续执行后续语句，收集所有错误
				// 注意：某些 DDL 语句可能会自动提交事务，但这对验证来说是可以接受的
			}
		}
	}

	// 回滚事务（defer 中会自动执行）
	// 如果事务已经被自动提交，回滚会失败，但这不影响验证结果
	return errors
}

// GaussDBOnlineValidator GaussDB 在线验证器
type GaussDBOnlineValidator struct {
	OnlineValidatorBase
}

// NewGaussDBOnlineValidator 创建 GaussDB 在线验证器
func NewGaussDBOnlineValidator(dsn string) *GaussDBOnlineValidator {
	return &GaussDBOnlineValidator{
		OnlineValidatorBase: OnlineValidatorBase{
			DSN: dsn,
		},
	}
}

// Connect 连接到 GaussDB 数据库
func (v *GaussDBOnlineValidator) Connect() error {
	db, err := sql.Open("opengauss", v.DSN)
	if err != nil {
		return fmt.Errorf("打开 GaussDB 连接失败: %w", err)
	}

	// 设置连接参数
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("GaussDB 连接测试失败: %w", err)
	}

	v.DB = db
	return nil
}

// Validate 验证单条 SQL 语句
func (v *GaussDBOnlineValidator) Validate(sql string) *ValidationResult {
	result := NewValidationResult(true)

	if v.DB == nil {
		result.AddError(0, 0, "数据库连接未建立", sql)
		return result
	}

	trimmedSQL := strings.TrimSpace(sql)
	if trimmedSQL == "" {
		return result
	}

	// 跳过注释
	if strings.HasPrefix(trimmedSQL, "--") || strings.HasPrefix(trimmedSQL, "/*") {
		return result
	}

	// 在事务中验证
	err := validateInTransaction(v.DB, trimmedSQL)
	if err != nil {
		result.AddError(0, 0, err.Error(), sql)
	}

	return result
}

// ValidateText 验证包含多条语句的 SQL 文本
// 所有语句在同一个事务中执行，以正确处理语句之间的依赖关系
func (v *GaussDBOnlineValidator) ValidateText(sqlText string) *ValidationResult {
	result := NewValidationResult(true)

	if v.DB == nil {
		result.AddError(0, 0, "数据库连接未建立", sqlText)
		return result
	}

	statements := splitStatements(sqlText)
	if len(statements) == 0 {
		return result
	}

	// 在单个事务中执行所有语句
	stmtErrors := validateMultipleInTransaction(v.DB, statements)

	// 将错误转换为 ValidationError
	for _, stmtErr := range stmtErrors {
		result.Valid = false
		result.AddError(stmtErr.Line, 0, stmtErr.Message, stmtErr.SQL)
	}

	return result
}
