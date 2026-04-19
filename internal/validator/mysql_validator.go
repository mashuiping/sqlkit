package validator

import (
	"github.com/xwb1989/sqlparser"
)

// MySQLValidator MySQL 语法验证器
// 使用 vitess sqlparser 进行语法解析
type MySQLValidator struct {
	BaseValidator
}

// NewMySQLValidator 创建 MySQL 验证器
func NewMySQLValidator() *MySQLValidator {
	return &MySQLValidator{}
}

// Validate 验证单条 SQL 语句
func (v *MySQLValidator) Validate(sql string) *ValidationResult {
	result := NewValidationResult(true)

	// 尝试解析 SQL
	_, err := sqlparser.Parse(sql)
	if err != nil {
		result.AddError(0, 0, err.Error(), sql)
	}

	return result
}

// ValidateMultiple 验证多条 SQL 语句
func (v *MySQLValidator) ValidateMultiple(sqls []string) []*ValidationResult {
	results := make([]*ValidationResult, len(sqls))
	for i, sql := range sqls {
		results[i] = v.Validate(sql)
	}
	return results
}

// ValidateText 验证包含多条语句的 SQL 文本
func (v *MySQLValidator) ValidateText(sqlText string) *ValidationResult {
	result := NewValidationResult(true)

	statements := splitStatements(sqlText)
	hasValidStmt := false
	for _, stmt := range statements {
		// 移除开头的注释和空行
		cleanedSQL := removeLeadingComments(stmt)
		if cleanedSQL == "" {
			continue
		}

		hasValidStmt = true
		stmtResult := v.Validate(cleanedSQL)
		if !stmtResult.Valid {
			result.Valid = false
			result.Errors = append(result.Errors, stmtResult.Errors...)
		}
		result.Warnings = append(result.Warnings, stmtResult.Warnings...)
	}

	// 如果没有找到任何有效的SQL语句,返回错误
	if !hasValidStmt {
		result.Valid = false
		result.AddError(0, 0, "SQL 文件为空或只包含注释", "")
	}

	return result
}
