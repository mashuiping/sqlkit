package validator

import (
	"strings"

	"github.com/mashuiping/sqlkit/internal/parser"
)

// TeledbValidator Teledb 语法验证器
// Teledb 与 GaussDB 同源（基于 openGauss/PostgreSQL 方言），
// 静态语法检查复用 PostgreSQL 解析器；语义/兼容性差异通过在线验证捕获。
type TeledbValidator struct {
	BaseValidator
}

// NewTeledbValidator 创建 Teledb 验证器
func NewTeledbValidator() *TeledbValidator {
	return &TeledbValidator{}
}

// Validate 验证单条 SQL 语句
func (v *TeledbValidator) Validate(sql string) *ValidationResult {
	result := NewValidationResult(true)

	trimmedSQL := strings.TrimSpace(sql)
	if trimmedSQL == "" {
		return result
	}

	// 使用 PostgreSQL 解析器构建 AST。
	_, err := parser.ParsePostgreSQL(trimmedSQL)
	if err != nil {
		result.AddError(0, 0, err.Error(), sql)
	}

	return result
}

// ValidateMultiple 验证多条 SQL 语句
func (v *TeledbValidator) ValidateMultiple(sqls []string) []*ValidationResult {
	results := make([]*ValidationResult, len(sqls))
	for i, sql := range sqls {
		results[i] = v.Validate(sql)
	}
	return results
}

// ValidateText 验证包含多条语句的 SQL 文本
func (v *TeledbValidator) ValidateText(sqlText string) *ValidationResult {
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
