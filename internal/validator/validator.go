// Package validator 提供 SQL 语法静态验证功能
package validator

import (
	"fmt"
	"strings"
)

// ValidationError 验证错误
type ValidationError struct {
	Line    int    // 错误所在行号
	Column  int    // 错误所在列号
	Message string // 错误信息
	SQL     string // 相关 SQL 片段
}

func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Message)
	}
	return e.Message
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid    bool               // 是否有效
	Errors   []*ValidationError // 错误列表
	Warnings []string           // 警告列表
}

// Validator SQL 验证器接口
type Validator interface {
	// Validate 验证 SQL 语句
	Validate(sql string) *ValidationResult
	// ValidateMultiple 验证多条 SQL 语句
	ValidateMultiple(sqls []string) []*ValidationResult
}

// BaseValidator 基础验证器
type BaseValidator struct{}

// splitStatements 将 SQL 文本拆分为独立语句
func splitStatements(sqlText string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	escaped := false

	for _, r := range sqlText {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}

		switch r {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
		case ';':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
				continue
			}
		}
		current.WriteRune(r)
	}

	// 处理最后一条语句（可能没有分号）
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// removeLeadingComments 移除语句开头的注释
func removeLeadingComments(sql string) string {
	lines := strings.Split(sql, "\n")
	var resultLines []string
	foundNonComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !foundNonComment {
			// 跳过开头的注释行和空行
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			// 跳过多行注释的开始
			if strings.HasPrefix(trimmed, "/*") {
				// 简单处理: 跳过整行
				continue
			}
			foundNonComment = true
		}
		resultLines = append(resultLines, line)
	}

	if len(resultLines) == 0 {
		return ""
	}

	// 移除行尾的多行注释结束标记
	result := strings.Join(resultLines, "\n")
	if idx := strings.Index(result, "*/"); idx != -1 {
		result = strings.TrimSpace(result[idx+2:])
	}

	return strings.TrimSpace(result)
}

// NewValidationResult 创建验证结果
func NewValidationResult(valid bool) *ValidationResult {
	return &ValidationResult{
		Valid:    valid,
		Errors:   make([]*ValidationError, 0),
		Warnings: make([]string, 0),
	}
}

// AddError 添加错误
func (r *ValidationResult) AddError(line, column int, message, sql string) {
	r.Valid = false
	r.Errors = append(r.Errors, &ValidationError{
		Line:    line,
		Column:  column,
		Message: message,
		SQL:     sql,
	})
}

// AddWarning 添加警告
func (r *ValidationResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// PrintResult 打印验证结果
func PrintResult(result *ValidationResult, prefix string) {
	if result.Valid {
		fmt.Printf("%s✓ SQL 语法验证通过\n", prefix)
	} else {
		fmt.Printf("%s✗ SQL 语法验证失败:\n", prefix)
		for i, err := range result.Errors {
			fmt.Printf("%s  [%d] %s\n", prefix, i+1, err.Error())
			if err.SQL != "" {
				sqlPreview := err.SQL
				if len(sqlPreview) > 100 {
					sqlPreview = sqlPreview[:100] + "..."
				}
				fmt.Printf("%s      SQL: %s\n", prefix, sqlPreview)
			}
		}
	}
	if len(result.Warnings) > 0 {
		fmt.Printf("%s警告:\n", prefix)
		for _, w := range result.Warnings {
			fmt.Printf("%s  - %s\n", prefix, w)
		}
	}
}
