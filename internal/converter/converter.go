// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
// 参考 sqlglot (https://github.com/tobymao/sqlglot) 的实现方式
// 核心思想：Parse(解析) -> Transform(转换) -> Generate(生成)
package converter

import (
	"regexp"
	"strings"
)

// MySQLToPostgresConverter 提供 MySQL SQL 到 PostgreSQL SQL 的转换功能
type MySQLToPostgresConverter struct {
	UseTimestampTZ bool // 是否使用 TIMESTAMPTZ 而非 TIMESTAMP
	UseJSONB       bool // 是否使用 JSONB 而非 JSON
}

// NewConverter 创建一个新的转换器，使用默认配置
func NewConverter() *MySQLToPostgresConverter {
	return &MySQLToPostgresConverter{
		UseTimestampTZ: true,
		UseJSONB:       true,
	}
}

// ============================================================================
// 主转换入口
// ============================================================================

// ConvertSQL 将 MySQL SQL 转换为 PostgreSQL SQL
// 支持多条语句（以分号分隔）
func (c *MySQLToPostgresConverter) ConvertSQL(mysqlSQL string) (string, error) {
	statements := c.splitSQLStatements(mysqlSQL)
	var results []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if c.shouldSkipStatement(stmt) {
			continue
		}

		converted := c.convertStatement(stmt)
		if converted != "" {
			results = append(results, converted)
		}
	}

	return strings.Join(results, "\n\n"), nil
}

// convertStatement 转换单条 SQL 语句
func (c *MySQLToPostgresConverter) convertStatement(stmt string) string {
	// 1. 提取和保留前缀注释行
	lines := strings.Split(stmt, "\n")
	var commentLines []string
	var sqlLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "/*!") || trimmed == "" {
			commentLines = append(commentLines, line)
		} else {
			sqlLines = append(sqlLines, line)
		}
	}

	sqlPart := strings.Join(sqlLines, " ")

	// 2. 解析：根据语句类型构造对应的 Statement 节点（AST）
	stmtNode := c.parseStatement(sqlPart)

	// 3. 生成：调用语句节点的 ToPostgres，生成目标 SQL
	converted := stmtNode.ToPostgres(c)

	// 4. 合并注释和转换后的 SQL
	if len(commentLines) > 0 {
		return strings.Join(commentLines, "\n") + "\n" + converted
	}
	return converted
}

// parseStatement 将单条 SQL 文本解析为对应的 Statement 节点（AST）
// 参考 sqlglot 的解析逻辑，这里实现简化版
func (c *MySQLToPostgresConverter) parseStatement(sqlPart string) Statement {
	trimmed := strings.TrimSpace(sqlPart)
	if trimmed == "" {
		return &UnknownStatement{BaseStatement{RawSQL: sqlPart}}
	}
	upperStmt := strings.ToUpper(trimmed)

	switch {
	case strings.HasPrefix(upperStmt, "CREATE TABLE"):
		return c.parseCreateTable(trimmed)
	case strings.HasPrefix(upperStmt, "ALTER TABLE"):
		return c.parseAlterTable(trimmed)
	case strings.HasPrefix(upperStmt, "INSERT"):
		return c.parseInsert(trimmed)
	case strings.HasPrefix(upperStmt, "UPDATE"):
		return c.parseUpdate(trimmed)
	case strings.HasPrefix(upperStmt, "SELECT"):
		return c.parseSelect(trimmed)
	case strings.HasPrefix(upperStmt, "DELETE"):
		return &DeleteStatement{BaseStatement{RawSQL: trimmed}}
	case strings.HasPrefix(upperStmt, "TRUNCATE"):
		return &UnknownStatement{BaseStatement{RawSQL: trimmed}}
	case strings.HasPrefix(upperStmt, "CREATE INDEX") || strings.HasPrefix(upperStmt, "CREATE UNIQUE INDEX"):
		return &CreateIndexStatement{BaseStatement{RawSQL: trimmed}}
	case strings.HasPrefix(upperStmt, "DROP TABLE"):
		return &DropTableStatement{BaseStatement{RawSQL: trimmed}}
	case strings.HasPrefix(upperStmt, "DROP INDEX"):
		return &DropIndexStatement{BaseStatement{RawSQL: trimmed}}
	default:
		return &UnknownStatement{BaseStatement{RawSQL: trimmed}}
	}
}

// ============================================================================
// 基础转换函数
// ============================================================================

// basicConvert 执行基本的语法转换
func (c *MySQLToPostgresConverter) basicConvert(stmt string) string {
	result := stmt
	result = strings.ReplaceAll(result, "`", "\"")
	// 移除 COLLATE utf8mb4_0900_ai_ci 等排序规则（PostgreSQL/GaussDB 不支持）
	result = regexp.MustCompile(`(?i)\s+COLLATE\s+\w+`).ReplaceAllString(result, "")
	return result
}

// ensureSemicolon 确保 SQL 语句以分号结尾
func (c *MySQLToPostgresConverter) ensureSemicolon(stmt string) string {
	result := strings.TrimSpace(stmt)
	if result != "" && !strings.HasSuffix(result, ";") {
		result += ";"
	}
	return result
}

// splitSQLStatements 将 SQL 文本拆分为独立语句
func (c *MySQLToPostgresConverter) splitSQLStatements(sqlDump string) []string {
	var stmts []string
	var builder strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false
	escaped := false

	for _, r := range sqlDump {
		if escaped {
			builder.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			builder.WriteRune(r)
			escaped = true
			continue
		}

		switch r {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
		case ';':
			if !inSingle && !inDouble && !inBacktick {
				stmts = append(stmts, builder.String())
				builder.Reset()
				continue
			}
		}
		builder.WriteRune(r)
	}

	if strings.TrimSpace(builder.String()) != "" {
		stmts = append(stmts, builder.String())
	}
	return stmts
}

// shouldSkipStatement 判断是否应该跳过该语句
func (c *MySQLToPostgresConverter) shouldSkipStatement(stmt string) bool {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return true
	}

	lines := strings.Split(trimmed, "\n")
	var nonCommentLines []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "--") && !strings.HasPrefix(trimmedLine, "/*") && !strings.HasPrefix(trimmedLine, "/*!") {
			nonCommentLines = append(nonCommentLines, trimmedLine)
		}
	}

	if len(nonCommentLines) == 0 {
		return true
	}

	if len(nonCommentLines) > 0 {
		firstLine := nonCommentLines[0]
		upperFirstLine := strings.ToUpper(firstLine)
		skipPrefixes := []string{
			"SET @",
			"SET @@",
			"SET NAMES",
			"SET CHARACTER",
			"SET FOREIGN_KEY_CHECKS",
			"SET SQL_MODE",
			"SET TIME_ZONE",
			"SET UNIQUE_CHECKS",
			"SET AUTOCOMMIT",
			"LOCK TABLES",
			"UNLOCK TABLES",
			"DELIMITER ",
			"USE ",
		}

		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(upperFirstLine, prefix) {
				return true
			}
		}
	}

	return false
}
