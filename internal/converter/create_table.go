// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// CREATE TABLE 解析和生成
// ============================================================================

// parseCreateTable 解析 CREATE TABLE 语句为 AST
func (c *MySQLToPostgresConverter) parseCreateTable(stmt string) *CreateTableStatement {
	createTable := &CreateTableStatement{
		BaseStatement: BaseStatement{RawSQL: stmt},
		Columns:       []*ColumnDef{},
		Constraints:   []*TableConstraint{},
		TableOptions:  &TableOptions{},
	}

	// 提取表名和 IF NOT EXISTS
	tableNameRe := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`?" + `([^\s` + "`" + `(]+)` + "`?")
	matches := tableNameRe.FindStringSubmatch(stmt)
	if len(matches) >= 2 {
		createTable.TableName = strings.Trim(matches[1], "`\"")
	}
	if strings.Contains(strings.ToUpper(stmt), "IF NOT EXISTS") {
		createTable.IfNotExists = true
	}

	// 提取列定义和约束（复用现有逻辑）
	comments := c.extractComments(stmt, createTable.TableName)
	stmtWithoutIndexes, indexes := c.extractIndexes(stmt, createTable.TableName)

	// 解析列定义
	createTable.Columns = c.parseColumnDefinitions(stmtWithoutIndexes, comments)

	// 解析约束
	createTable.Constraints = c.parseTableConstraints(stmtWithoutIndexes, indexes)

	// 解析表选项
	createTable.TableOptions = c.parseTableOptions(stmt)

	return createTable
}

// parseColumnDefinitions 解析列定义
func (c *MySQLToPostgresConverter) parseColumnDefinitions(stmt string, comments CommentInfo) []*ColumnDef {
	var columns []*ColumnDef

	// 找到 CREATE TABLE ... ( 之后的内容
	openParenIdx := strings.Index(stmt, "(")
	if openParenIdx == -1 {
		return columns
	}

	// 提取表定义部分（括号内的内容）
	closeParenIdx := c.findMatchingParen(stmt, openParenIdx)
	if closeParenIdx == -1 {
		return columns
	}
	tableDef := stmt[openParenIdx+1 : closeParenIdx]

	// 移除 ON UPDATE CURRENT_TIMESTAMP（PostgreSQL/GaussDB 不支持）
	tableDef = regexp.MustCompile(`(?i)\s+ON\s+UPDATE\s+CURRENT_TIMESTAMP`).ReplaceAllString(tableDef, "")

	// 按逗号分割列定义，但要考虑括号内的逗号（如函数调用、复合主键等）
	var colDefs []string
	var current strings.Builder
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for _, r := range tableDef {
		switch r {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
			current.WriteRune(r)
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
			current.WriteRune(r)
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
			current.WriteRune(r)
		case '(':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				depth++
			}
			current.WriteRune(r)
		case ')':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				depth--
			}
			current.WriteRune(r)
		case ',':
			if !inSingleQuote && !inDoubleQuote && !inBacktick && depth == 0 {
				// 这是一个列定义的分隔符
				colDef := strings.TrimSpace(current.String())
				if colDef != "" {
					colDefs = append(colDefs, colDef)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	// 添加最后一个列定义
	colDef := strings.TrimSpace(current.String())
	if colDef != "" {
		colDefs = append(colDefs, colDef)
	}

	// 解析每个列定义
	for _, colDefStr := range colDefs {
		colDefStr = strings.TrimSpace(colDefStr)
		if colDefStr == "" {
			continue
		}

		upperColDef := strings.ToUpper(colDefStr)
		// 跳过约束定义
		if strings.HasPrefix(upperColDef, "PRIMARY KEY") ||
			strings.HasPrefix(upperColDef, "UNIQUE KEY") ||
			strings.HasPrefix(upperColDef, "KEY ") ||
			strings.HasPrefix(upperColDef, "INDEX ") ||
			strings.HasPrefix(upperColDef, "CONSTRAINT ") ||
			strings.HasPrefix(upperColDef, "FULLTEXT ") ||
			strings.HasPrefix(upperColDef, "SPATIAL ") ||
			strings.HasPrefix(upperColDef, "FOREIGN KEY") {
			continue
		}

		// 解析列定义
		col := c.parseSingleColumn(colDefStr, comments)
		if col != nil {
			columns = append(columns, col)
		}
	}

	return columns
}

// parseSingleColumn 解析单个列定义
func (c *MySQLToPostgresConverter) parseSingleColumn(line string, comments CommentInfo) *ColumnDef {
	// 移除 ON UPDATE CURRENT_TIMESTAMP（PostgreSQL/GaussDB 不支持）
	line = regexp.MustCompile(`(?i)\s+ON\s+UPDATE\s+CURRENT_TIMESTAMP`).ReplaceAllString(line, "")

	// 提取列名
	colName := c.extractColumnName(line)
	if colName == "" {
		return nil
	}

	// 如果列名是关键字，跳过
	upperColName := strings.ToUpper(colName)
	if upperColName == "CREATE" || upperColName == "TABLE" || upperColName == "PRIMARY" || upperColName == "KEY" {
		return nil
	}

	col := &ColumnDef{
		Name: colName,
	}

	// 提取数据类型
	col.DataType = c.parseDataType(line)

	// 提取约束
	upperLine := strings.ToUpper(line)
	col.NotNull = strings.Contains(upperLine, "NOT NULL")
	col.AutoIncrement = strings.Contains(upperLine, "AUTO_INCREMENT")

	// 提取默认值
	col.Default = c.extractDefaultValue(line)

	// 提取注释
	col.Comment = c.extractColumnComment(line, colName, comments)

	// 提取位置（AFTER/FIRST）
	col.Position = c.extractColumnPosition(line, upperLine)

	return col
}

// extractColumnName 提取列名（支持反引号、双引号或普通标识符）
func (c *MySQLToPostgresConverter) extractColumnName(line string) string {
	// 尝试匹配带反引号的列名
	backtickRe := regexp.MustCompile("^\\s*`([^`]+)`\\s+")
	if matches := backtickRe.FindStringSubmatch(line); len(matches) >= 2 {
		return c.normalizeColumnName(matches[1])
	}

	// 尝试匹配带双引号的列名
	doubleQuoteRe := regexp.MustCompile(`^\s*"([^"]+)"\s+`)
	if matches := doubleQuoteRe.FindStringSubmatch(line); len(matches) >= 2 {
		return c.normalizeColumnName(matches[1])
	}

	// 尝试匹配不带引号的列名
	simpleRe := regexp.MustCompile(`^\s*([^\s]+)\s+`)
	if matches := simpleRe.FindStringSubmatch(line); len(matches) >= 2 {
		return c.normalizeColumnName(matches[1])
	}

	return ""
}

// normalizeColumnName 规范化列名（处理带点的列名）
func (c *MySQLToPostgresConverter) normalizeColumnName(name string) string {
	// 如果列名包含点，只取最后一部分（列名）
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		return parts[len(parts)-1]
	}
	return name
}

// extractDefaultValue 提取默认值（支持 CURRENT_TIMESTAMP, NOW(), 字符串, 数字等）
func (c *MySQLToPostgresConverter) extractDefaultValue(line string) string {
	// 尝试匹配带单引号的字符串
	singleQuoteRe := regexp.MustCompile(`(?i)\bDEFAULT\s+'([^']*)'`)
	if matches := singleQuoteRe.FindStringSubmatch(line); len(matches) >= 2 {
		return "'" + matches[1] + "'"
	}

	// 尝试匹配带双引号的字符串
	doubleQuoteRe := regexp.MustCompile(`(?i)\bDEFAULT\s+"([^"]*)"`)
	if matches := doubleQuoteRe.FindStringSubmatch(line); len(matches) >= 2 {
		return "'" + matches[1] + "'"
	}

	// 尝试匹配函数调用或数字
	defaultRe := regexp.MustCompile(`(?i)\bDEFAULT\s+([^\s,)]+(?:\s*\([^)]*\))?)`)
	if matches := defaultRe.FindStringSubmatch(line); len(matches) >= 2 {
		defaultVal := strings.TrimSpace(matches[1])
		upperDefault := strings.ToUpper(defaultVal)
		// 保留函数调用（如 CURRENT_TIMESTAMP, NOW()）
		if strings.Contains(upperDefault, "CURRENT_TIMESTAMP") || strings.Contains(upperDefault, "NOW()") {
			return defaultVal
		}
		// 数字或其他值，保持原样（移除可能的反引号）
		return strings.Trim(defaultVal, "`")
	}

	return ""
}

// extractColumnComment 提取列注释
func (c *MySQLToPostgresConverter) extractColumnComment(line, colName string, comments CommentInfo) string {
	commentRe := regexp.MustCompile(`(?i)COMMENT\s+['"]([^'"]*)['"]`)
	if matches := commentRe.FindStringSubmatch(line); len(matches) >= 2 {
		return matches[1]
	}
	// 如果注释在单独的注释映射中
	if comment, ok := comments.ColumnComments[colName]; ok {
		return comment
	}
	return ""
}

// extractColumnPosition 提取列位置（AFTER/FIRST）
func (c *MySQLToPostgresConverter) extractColumnPosition(line, upperLine string) string {
	if strings.Contains(upperLine, "AFTER") {
		afterRe := regexp.MustCompile(`(?i)\bAFTER\s+[` + "`" + `"]?(\w+)[` + "`" + `"]?`)
		if matches := afterRe.FindStringSubmatch(line); len(matches) >= 2 {
			return "AFTER " + matches[1]
		}
	} else if strings.Contains(upperLine, "FIRST") {
		return "FIRST"
	}
	return ""
}

// parseDataType 解析数据类型
func (c *MySQLToPostgresConverter) parseDataType(line string) *DataType {
	dt := &DataType{}

	// 匹配数据类型模式（按优先级排序，更具体的模式在前）
	typePatterns := []struct {
		pattern string
		pgType  string
	}{
		// BOOLEAN 类型（TINYINT(1) 或 BOOLEAN）
		{`(?i)\bBOOLEAN\b`, "BOOLEAN"},
		{`(?i)\bTINYINT\s*\(\s*1\s*\)`, "BOOLEAN"},
		{`(?i)\bBIT\s*\(\s*1\s*\)`, "BOOLEAN"},
		// DECIMAL 类型（支持无精度模式）
		{`(?i)\bDECIMAL\s*\((\d+),\s*(\d+)\)`, "DECIMAL"},
		{`(?i)\bDECIMAL\s*\((\d+)\)`, "DECIMAL"},
		{`(?i)\bDECIMAL\b`, "DECIMAL"}, // 无精度模式
		{`(?i)\bNUMERIC\s*\((\d+),\s*(\d+)\)`, "NUMERIC"},
		{`(?i)\bNUMERIC\s*\((\d+)\)`, "NUMERIC"},
		{`(?i)\bNUMERIC\b`, "NUMERIC"},
		// 整数类型
		{`(?i)\bTINYINT\s*\((\d+)\)`, "SMALLINT"},
		{`(?i)\bTINYINT\b`, "SMALLINT"},
		{`(?i)\bSMALLINT\s*\((\d+)\)`, "SMALLINT"},
		{`(?i)\bSMALLINT\b`, "SMALLINT"},
		{`(?i)\bMEDIUMINT\s*\((\d+)\)`, "INTEGER"},
		{`(?i)\bMEDIUMINT\b`, "INTEGER"},
		{`(?i)\bINT\s*\((\d+)\)`, "INTEGER"},
		{`(?i)\bINT\b`, "INTEGER"},
		{`(?i)\bINTEGER\s*\((\d+)\)`, "INTEGER"},
		{`(?i)\bINTEGER\b`, "INTEGER"},
		{`(?i)\bBIGINT\s*\((\d+)\)`, "BIGINT"},
		{`(?i)\bBIGINT\b`, "BIGINT"},
		// 浮点类型
		{`(?i)\bFLOAT\s*\((\d+),\s*(\d+)\)`, "REAL"},
		{`(?i)\bFLOAT\s*\((\d+)\)`, "REAL"},
		{`(?i)\bFLOAT\b`, "REAL"},
		{`(?i)\bDOUBLE\s*\((\d+),\s*(\d+)\)`, "DOUBLE PRECISION"},
		{`(?i)\bDOUBLE\b`, "DOUBLE PRECISION"},
		// 字符串类型
		{`(?i)\bVARCHAR\s*\((\d+)\)`, "VARCHAR"},
		{`(?i)\bCHAR\s*\((\d+)\)`, "CHAR"},
		// 时间类型
		{`(?i)\bDATE\b`, "DATE"},
		{`(?i)\bDATETIME\s*\((\d+)\)`, c.timestampType()},
		{`(?i)\bDATETIME\b`, c.timestampType()},
		{`(?i)\bTIMESTAMP\s*\((\d+)\)`, c.timestampType()},
		{`(?i)\bTIMESTAMP\b`, c.timestampType()},
		{`(?i)\bTIME\b`, "TIME"},
		{`(?i)\bTIME\s*\((\d+)\)`, "TIME"},
		{`(?i)\bYEAR\b`, "INTEGER"}, // MySQL YEAR 类型转换为 INTEGER
		// 文本类型
		{`(?i)\bTEXT\b`, "TEXT"},
		{`(?i)\bLONGTEXT\b`, "TEXT"},
		{`(?i)\bMEDIUMTEXT\b`, "TEXT"},
		{`(?i)\bTINYTEXT\b`, "TEXT"},
		// 二进制类型
		{`(?i)\bBLOB\b`, "BYTEA"},
		{`(?i)\bLONGBLOB\b`, "BYTEA"},
		{`(?i)\bMEDIUMBLOB\b`, "BYTEA"},
		{`(?i)\bTINYBLOB\b`, "BYTEA"},
		// JSON 类型（占位符，将在下面特殊处理）
		{`(?i)\bJSON\b`, ""},
	}

	for _, tp := range typePatterns {
		re := regexp.MustCompile(tp.pattern)
		if re.MatchString(line) {
			// 特殊处理 JSON 类型（需要在运行时检查 UseJSONB 标志）
			if tp.pattern == `(?i)\bJSON\b` {
				if c.UseJSONB {
					dt.Type = "JSONB"
				} else {
					dt.Type = "JSON"
				}
			} else {
				dt.Type = tp.pgType
			}
			// 提取精度
			if matches := re.FindStringSubmatch(line); len(matches) > 1 {
				if len(matches) > 2 {
					// DECIMAL(10,2) 这种情况
					fmt.Sscanf(matches[1], "%d", &dt.Precision)
					fmt.Sscanf(matches[2], "%d", &dt.Scale)
				} else if matches[1] != "" {
					// DECIMAL(10) 这种情况
					fmt.Sscanf(matches[1], "%d", &dt.Precision)
				}
				// 对于无精度模式（如 DECIMAL），Precision 和 Scale 保持为 0
			}
			break
		}
	}

	// 检查 UNSIGNED
	if strings.Contains(strings.ToUpper(line), "UNSIGNED") {
		dt.Unsigned = true
	}

	return dt
}

// parseTableConstraints 解析表级约束
func (c *MySQLToPostgresConverter) parseTableConstraints(stmt string, indexes []IndexInfo) []*TableConstraint {
	var constraints []*TableConstraint

	// 从提取的索引信息构建约束
	for _, idx := range indexes {
		constraint := &TableConstraint{
			Name:     idx.Name,
			Columns:  strings.Split(idx.Columns, ","),
			IsUnique: idx.IsUnique,
		}
		if idx.IsUnique {
			constraint.Type = ConstraintUniqueKey
		} else {
			constraint.Type = ConstraintIndex
		}
		constraints = append(constraints, constraint)
	}

	// 解析 PRIMARY KEY
	primaryKeyRe := regexp.MustCompile(`(?i)PRIMARY\s+KEY\s*\(([^)]+)\)`)
	if matches := primaryKeyRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		columns := strings.Split(matches[1], ",")
		for i := range columns {
			columns[i] = strings.TrimSpace(strings.Trim(columns[i], "`\""))
		}
		constraints = append(constraints, &TableConstraint{
			Type:    ConstraintPrimaryKey,
			Columns: columns,
		})
	}

	return constraints
}

// parseTableOptions 解析表选项
func (c *MySQLToPostgresConverter) parseTableOptions(stmt string) *TableOptions {
	options := &TableOptions{}

	// 提取 COMMENT（只从表选项部分提取，即在右括号之后）
	lastParenIdx := strings.LastIndex(stmt, ")")
	if lastParenIdx != -1 {
		tableOptions := stmt[lastParenIdx:]
		commentRe := regexp.MustCompile(`(?i)COMMENT\s*=?\s*'([^']*)'`)
		if matches := commentRe.FindStringSubmatch(tableOptions); len(matches) >= 2 {
			options.Comment = matches[1]
		}
	}

	// 提取 ENGINE
	engineRe := regexp.MustCompile(`(?i)ENGINE\s*=\s*(\w+)`)
	if matches := engineRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		options.Engine = matches[1]
	}

	// 提取 CHARSET
	charsetRe := regexp.MustCompile(`(?i)(?:DEFAULT\s+)?CHARSET\s*=\s*(\w+)`)
	if matches := charsetRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		options.Charset = matches[1]
	}

	return options
}

// generateCreateTable 生成 CREATE TABLE 语句
func (c *MySQLToPostgresConverter) generateCreateTable(stmt *CreateTableStatement) string {
	var parts []string

	// CREATE TABLE 头部
	createPart := "CREATE TABLE"
	if stmt.IfNotExists {
		createPart += " IF NOT EXISTS"
	}
	createPart += fmt.Sprintf(" \"%s\" (", stmt.TableName)
	parts = append(parts, createPart)

	// 列定义
	var colDefs []string
	for _, col := range stmt.Columns {
		colDefs = append(colDefs, "    "+c.generateColumnDef(col))
	}

	// 主键约束（如果有）
	for _, constraint := range stmt.Constraints {
		if constraint.Type == ConstraintPrimaryKey {
			cols := strings.Join(constraint.Columns, ", ")
			colDefs = append(colDefs, fmt.Sprintf("    PRIMARY KEY (%s)", cols))
		}
	}

	parts = append(parts, strings.Join(colDefs, ",\n"))

	parts = append(parts, ");")

	// 索引语句（单独生成）
	var indexStmts []string
	for _, constraint := range stmt.Constraints {
		if constraint.Type == ConstraintIndex || constraint.Type == ConstraintUniqueKey {
			indexName := c.makeIndexName(constraint.Name, stmt.TableName)
			columns := strings.Join(constraint.Columns, ", ")
			uniqueKeyword := ""
			if constraint.IsUnique {
				uniqueKeyword = "UNIQUE "
			}
			indexStmts = append(indexStmts, fmt.Sprintf("CREATE %sINDEX %s ON \"%s\" (%s);",
				uniqueKeyword, indexName, stmt.TableName, columns))
		}
	}

	// 注释语句
	var commentStmts []string
	for _, col := range stmt.Columns {
		if col.Comment != "" {
			escaped := strings.ReplaceAll(col.Comment, "'", "''")
			commentStmts = append(commentStmts, fmt.Sprintf("COMMENT ON COLUMN \"%s\".\"%s\" IS '%s';",
				stmt.TableName, col.Name, escaped))
		}
	}
	if stmt.TableOptions.Comment != "" {
		escaped := strings.ReplaceAll(stmt.TableOptions.Comment, "'", "''")
		commentStmts = append(commentStmts, fmt.Sprintf("COMMENT ON TABLE \"%s\" IS '%s';",
			stmt.TableName, escaped))
	}

	// 合并所有部分
	var result []string
	result = append(result, strings.Join(parts, "\n"))
	if len(indexStmts) > 0 {
		result = append(result, strings.Join(indexStmts, "\n"))
	}
	if len(commentStmts) > 0 {
		result = append(result, strings.Join(commentStmts, "\n"))
	}

	return strings.Join(result, "\n\n")
}

// generateColumnDef 生成列定义 SQL
func (c *MySQLToPostgresConverter) generateColumnDef(col *ColumnDef) string {
	var parts []string

	// 列名
	parts = append(parts, fmt.Sprintf("\"%s\"", col.Name))

	// 数据类型（处理 AUTO_INCREMENT -> SERIAL 转换）
	dataType := c.generateDataType(col.DataType)
	if col.AutoIncrement && col.DataType != nil {
		// 根据原始类型转换为对应的 SERIAL 类型
		originalType := strings.ToUpper(col.DataType.Type)
		switch originalType {
		case "BIGINT":
			dataType = "BIGSERIAL"
		case "INTEGER", "INT":
			dataType = "SERIAL"
		case "SMALLINT":
			dataType = "SMALLSERIAL"
		}
	}
	parts = append(parts, dataType)

	// NOT NULL
	if col.NotNull {
		parts = append(parts, "NOT NULL")
	}

	// DEFAULT（AUTO_INCREMENT 列不需要 DEFAULT）
	if col.Default != "" && !col.AutoIncrement {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", col.Default))
	}

	return strings.Join(parts, " ")
}

// generateDataType 生成数据类型 SQL
func (c *MySQLToPostgresConverter) generateDataType(dt *DataType) string {
	if dt == nil {
		return "TEXT"
	}

	result := dt.Type

	// 添加精度（如果需要）
	if dt.Precision > 0 {
		if dt.Scale > 0 {
			// DECIMAL(10,2)
			result = fmt.Sprintf("%s(%d,%d)", result, dt.Precision, dt.Scale)
		} else if dt.Type == "VARCHAR" || dt.Type == "CHAR" {
			// VARCHAR(255)
			result = fmt.Sprintf("%s(%d)", result, dt.Precision)
		}
	}

	return result
}

