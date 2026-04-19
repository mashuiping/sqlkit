// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// INSERT 语句解析和生成
// ============================================================================

// parseInsert 解析 INSERT 语句为 AST
func (c *MySQLToPostgresConverter) parseInsert(stmt string) *InsertStatement {
	insert := &InsertStatement{
		BaseStatement: BaseStatement{RawSQL: stmt},
		Columns:       []string{},
		Values:        []string{},
	}

	// 检测 INSERT IGNORE
	insert.Ignore = regexp.MustCompile(`(?i)\bINSERT\s+IGNORE\s+`).MatchString(stmt)

	// 检测 INSERT INTO INTO 重复关键字
	insert.HasIntoInto = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+INTO\s+`).MatchString(stmt)

	// 提取表名
	// 匹配: INSERT [IGNORE] INTO table_name 或 INSERT INTO INTO table_name
	tableNameRe := regexp.MustCompile(`(?i)INSERT\s+(?:IGNORE\s+)?INTO\s+(?:INTO\s+)?` + "`?" + `([^\s` + "`" + `(]+)` + "`?")
	if matches := tableNameRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		insert.TableName = strings.Trim(matches[1], "`\"")
	}

	// 提取列名列表（如果有）
	// 匹配: (col1, col2, ...)
	columnsRe := regexp.MustCompile(`(?i)INTO\s+(?:INTO\s+)?[^\s(]+` + "`?" + `\s*\(([^)]+)\)`)
	if matches := columnsRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		columnsStr := matches[1]
		// 分割列名（考虑引号）
		insert.Columns = c.parseColumnList(columnsStr)
	}

	// 提取 VALUES 子句
	// 匹配: VALUES (val1, val2, ...), (val3, val4, ...)
	valuesRe := regexp.MustCompile(`(?i)VALUES\s+(.+?)(?:\s+ON\s+DUPLICATE|$)`)
	if matches := valuesRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		valuesStr := matches[1]
		// 分割多个 VALUES 行
		insert.Values = c.parseValuesList(valuesStr)
	}

	// 提取 SELECT 查询（如果有）
	selectRe := regexp.MustCompile(`(?i)INSERT\s+(?:IGNORE\s+)?INTO\s+(?:INTO\s+)?[^\s(]+(?:\s*\([^)]+\))?\s+(SELECT\s+.+?)(?:\s+ON\s+DUPLICATE|$)`)
	if matches := selectRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		insert.SelectQuery = matches[1]
	}

	// 提取 ON DUPLICATE KEY UPDATE 子句
	onDupRe := regexp.MustCompile(`(?i)ON\s+DUPLICATE\s+KEY\s+UPDATE\s+(.+)$`)
	if matches := onDupRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		insert.OnDuplicate = matches[1]
	}

	return insert
}

// generateInsert 生成 INSERT 语句
func (c *MySQLToPostgresConverter) generateInsert(stmt *InsertStatement) string {
	var parts []string

	// INSERT INTO
	insertPart := "INSERT INTO"
	if stmt.Ignore {
		// PostgreSQL 不支持 INSERT IGNORE，移除
	}
	insertPart += fmt.Sprintf(" \"%s\"", stmt.TableName)

	// 列名列表
	if len(stmt.Columns) > 0 {
		var quotedCols []string
		for _, col := range stmt.Columns {
			col = strings.TrimSpace(strings.Trim(col, "`\""))
			quotedCols = append(quotedCols, fmt.Sprintf("\"%s\"", col))
		}
		insertPart += fmt.Sprintf(" (%s)", strings.Join(quotedCols, ", "))
	}

	parts = append(parts, insertPart)

	// VALUES 或 SELECT
	if stmt.SelectQuery != "" {
		// 转换 SELECT 查询中的函数调用
		selectSQL := c.convertSelectFunctions(stmt.SelectQuery)
		parts = append(parts, selectSQL)
	} else if len(stmt.Values) > 0 {
		// 转换 VALUES 中的函数调用（如 CONVERT(... USING utf8mb4)）
		var convertedValues []string
		for _, val := range stmt.Values {
			convertedVal := c.convertInsertValues(val)
			convertedValues = append(convertedValues, convertedVal)
		}
		parts = append(parts, "VALUES "+strings.Join(convertedValues, ", "))
	}

	// ON DUPLICATE KEY UPDATE -> ON CONFLICT DO UPDATE
	if stmt.OnDuplicate != "" {
		parts = append(parts, fmt.Sprintf("ON CONFLICT DO UPDATE SET %s", c.convertOnDuplicateUpdate(stmt.OnDuplicate)))
	}

	result := strings.Join(parts, " ")
	result = c.basicConvert(result)
	return c.ensureSemicolon(result)
}

// ============================================================================
// UPDATE 语句解析和生成
// ============================================================================

// parseUpdate 解析 UPDATE 语句为 AST
func (c *MySQLToPostgresConverter) parseUpdate(stmt string) *UpdateStatement {
	update := &UpdateStatement{
		BaseStatement: BaseStatement{RawSQL: stmt},
		SetClause:     []UpdateSetItem{},
	}

	upperStmt := strings.ToUpper(stmt)
	_ = upperStmt // 用于调试

	// 检测是否有 JOIN
	update.HasJoin = regexp.MustCompile(`(?i)\bJOIN\b`).MatchString(upperStmt)

	// 提取表名和别名
	// 匹配: UPDATE table_name [AS] alias [JOIN ...]
	// 需要处理两种情况：
	// 1. UPDATE table alias JOIN ... SET ...
	// 2. UPDATE table alias SET ... JOIN ...
	tableRe := regexp.MustCompile(`(?i)UPDATE\s+` + "`?" + `([^\s` + "`" + `]+)` + "`?" + `(?:\s+(?:AS\s+)?` + "`?" + `(\w+)` + "`?" + `)?`)
	if matches := tableRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		update.TableName = strings.Trim(matches[1], "`\"")
		if len(matches) >= 3 && matches[2] != "" {
			update.TableAlias = strings.Trim(matches[2], "`\"")
		}
	}

	// 提取 JOIN 子句（MySQL 语法）
	// UPDATE table SET ... JOIN table2 ON condition WHERE ...
	// 或 UPDATE table INNER JOIN table2 ON condition SET ... WHERE ...
	if update.HasJoin {
		// 尝试匹配 UPDATE table [alias] JOIN table2 ON condition SET ... 格式
		// 使用 [\s\S]*? 匹配任意字符（包括换行符）
		// 确保在遇到 SET 时停止匹配，使用 \bSET\b 确保是完整的单词
		// 使用更精确的正则，确保在遇到 SET 时停止匹配
		// 匹配: UPDATE table [alias] JOIN table2 ON condition SET ...
		// 使用非贪婪匹配，确保在遇到 SET 时停止
		joinBeforeSetRe := regexp.MustCompile(`(?s)(?i)UPDATE\s+[^\s]+(?:\s+\w+)?[\s\S]*?((?:INNER\s+)?JOIN\s+[^\s]+\s+\w+\s+ON\s+[^\s]+\s*=\s*[^\s]+)\s+SET\b`)
		if matches := joinBeforeSetRe.FindStringSubmatch(stmt); len(matches) >= 2 {
			// 保留 JOIN 关键字，这样 convertJoinToFrom 才能正确解析
			// 移除末尾可能存在的 SET 关键字和后续内容
			joinClause := strings.TrimSpace(matches[1])
			// 确保不包含 SET 关键字
			joinClause = regexp.MustCompile(`(?i)\s+SET\s+.*$`).ReplaceAllString(joinClause, "")
			joinClause = regexp.MustCompile(`(?i)\s+SET\b`).ReplaceAllString(joinClause, "")
			update.JoinClause = strings.TrimSpace(joinClause)
		} else {
			// 匹配 UPDATE table SET ... JOIN table2 ON condition WHERE ... 格式
			// 先找到 SET 子句的结束位置
			setEndRe := regexp.MustCompile(`(?i)SET\s+[^W]+`)
			if setMatch := setEndRe.FindString(stmt); setMatch != "" {
				setEndPos := len(setMatch)
				// 从 SET 子句后查找 JOIN
				restStmt := stmt[setEndPos:]
				joinRe := regexp.MustCompile(`(?i)(?:INNER\s+)?JOIN\s+.+?(?:\s+WHERE|\s+ORDER\s+BY|\s+LIMIT|$)`)
				if matches := joinRe.FindStringSubmatch(restStmt); len(matches) >= 1 {
					update.JoinClause = strings.TrimSpace(matches[0])
				}
			}
		}
	}

	// 提取 SET 子句
	// 匹配: SET col1 = val1, col2 = val2
	// 需要找到最后一个 SET（因为可能有 JOIN ... SET 的情况）
	// 使用更精确的正则，确保只匹配到 SET 子句的结束
	// 找到所有 SET 的位置，取最后一个
	setPositions := regexp.MustCompile(`(?i)\bSET\s+`).FindAllStringIndex(stmt, -1)
	if len(setPositions) > 0 {
		// 取最后一个 SET
		lastSetPos := setPositions[len(setPositions)-1][1]
		// 从 SET 后提取内容，直到遇到 WHERE、ORDER BY、LIMIT 或语句结束
		restAfterSet := stmt[lastSetPos:]
		setRe := regexp.MustCompile(`(?i)^(.+?)(?:\s+WHERE|\s+ORDER\s+BY|\s+LIMIT|;|$)`)
		if matches := setRe.FindStringSubmatch(restAfterSet); len(matches) >= 2 {
			setClause := strings.TrimSpace(matches[1])
			// 移除末尾的分号（如果有）
			setClause = strings.TrimSuffix(setClause, ";")
			setClause = strings.TrimSpace(setClause)
			if setClause != "" {
				update.SetClause = c.parseSetClause(setClause)
			}
		}
	}

	// 提取 WHERE 子句
	whereRe := regexp.MustCompile(`(?i)\bWHERE\s+(.+?)(?:\s+ORDER\s+BY|\s+LIMIT|$)`)
	if matches := whereRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		update.WhereClause = matches[1]
	}

	// 提取 ORDER BY 子句
	orderRe := regexp.MustCompile(`(?i)\bORDER\s+BY\s+(.+?)(?:\s+LIMIT|$)`)
	if matches := orderRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		update.OrderByClause = matches[1]
	}

	// 提取 LIMIT 子句
	limitRe := regexp.MustCompile(`(?i)\bLIMIT\s+(.+?)$`)
	if matches := limitRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		update.LimitClause = matches[1]
	}

	return update
}

// generateUpdate 生成 UPDATE 语句
func (c *MySQLToPostgresConverter) generateUpdate(stmt *UpdateStatement) string {
	var parts []string

	// UPDATE table_name
	updatePart := fmt.Sprintf("UPDATE \"%s\"", stmt.TableName)
	parts = append(parts, updatePart)

	// SET 子句（移除表别名）
	var setItems []string
	for _, item := range stmt.SetClause {
		// 移除表别名（如 sj.column -> column）
		column := c.removeTableAlias(item.Column)
		// 移除列名中的反引号
		column = strings.Trim(column, "`\"")
		// 转换值：处理反引号和双重引号
		value := item.Value
		// 先应用 basicConvert 处理反引号
		value = c.basicConvert(value)
		// 然后移除双重引号
		value = regexp.MustCompile(`""([^"]*)""`).ReplaceAllString(value, "\"$1\"")
		setItems = append(setItems, fmt.Sprintf("\"%s\" = %s", column, value))
	}
	parts = append(parts, fmt.Sprintf("SET %s", strings.Join(setItems, ", ")))

	// FROM 子句（如果有 JOIN，转换为 FROM ... WHERE）
	var joinCondition string
	if stmt.HasJoin && stmt.JoinClause != "" {
		// 将 MySQL 的 JOIN 转换为 PostgreSQL 的 FROM ... WHERE
		// JOIN 子句现在包含 "JOIN" 关键字（在 parseUpdate 中已保留）
		fromClause, condition := c.convertJoinToFrom(stmt.JoinClause)
		joinCondition = condition
		parts = append(parts, "FROM "+fromClause)
	}

	// WHERE 子句（合并原有 WHERE 和 JOIN 条件）
	var whereParts []string
	if joinCondition != "" {
		// 确保 joinCondition 不包含 SET 关键字
		cleanCondition := regexp.MustCompile(`(?i)\s+SET\s+.*$`).ReplaceAllString(joinCondition, "")
		cleanCondition = regexp.MustCompile(`(?i)^.*\s+SET\s+`).ReplaceAllString(cleanCondition, "")
		cleanCondition = strings.TrimSpace(cleanCondition)
		if cleanCondition != "" {
			whereParts = append(whereParts, c.basicConvert(cleanCondition))
		}
	}
	if stmt.WhereClause != "" {
		whereParts = append(whereParts, c.basicConvert(stmt.WhereClause))
	}
	if len(whereParts) > 0 {
		parts = append(parts, "WHERE "+strings.Join(whereParts, " AND "))
	}

	// ORDER BY 和 LIMIT（PostgreSQL 支持）
	if stmt.OrderByClause != "" {
		parts = append(parts, "ORDER BY "+c.basicConvert(stmt.OrderByClause))
	}
	if stmt.LimitClause != "" {
		parts = append(parts, "LIMIT "+stmt.LimitClause)
	}

	result := strings.Join(parts, " ")
	// 不需要再次调用 basicConvert，因为已经处理了
	return c.ensureSemicolon(result)
}

// ============================================================================
// SELECT 语句解析和生成
// ============================================================================

// parseSelect 解析 SELECT 语句为 AST
func (c *MySQLToPostgresConverter) parseSelect(stmt string) *SelectStatement {
	selectStmt := &SelectStatement{
		BaseStatement: BaseStatement{RawSQL: stmt},
		Columns:       []string{},
	}

	upperStmt := strings.ToUpper(stmt)

	// 检测 DISTINCT
	selectStmt.Distinct = regexp.MustCompile(`(?i)\bSELECT\s+DISTINCT\b`).MatchString(upperStmt)

	// 提取列列表
	// 匹配: SELECT col1, col2, ... FROM (有 FROM 子句)
	columnsRe := regexp.MustCompile(`(?i)SELECT\s+(?:DISTINCT\s+)?(.+?)\s+FROM`)
	if matches := columnsRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		columnsStr := matches[1]
		selectStmt.Columns = c.parseColumnList(columnsStr)
	} else {
		// 没有 FROM 子句的情况（如 SELECT 1;）
		// 匹配: SELECT col1, col2, ... [WHERE/GROUP BY/HAVING/ORDER BY/LIMIT/;]
		noFromRe := regexp.MustCompile(`(?i)SELECT\s+(?:DISTINCT\s+)?(.+?)(?:\s+WHERE|\s+GROUP\s+BY|\s+HAVING|\s+ORDER\s+BY|\s+LIMIT|;|$)`)
		if matches := noFromRe.FindStringSubmatch(stmt); len(matches) >= 2 {
			columnsStr := matches[1]
			selectStmt.Columns = c.parseColumnList(columnsStr)
		}
	}

	// 提取 FROM 子句
	fromRe := regexp.MustCompile(`(?i)\bFROM\s+(.+?)(?:\s+WHERE|\s+GROUP\s+BY|\s+HAVING|\s+ORDER\s+BY|\s+LIMIT|$)`)
	if matches := fromRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		selectStmt.FromClause = matches[1]
	}

	// 提取 WHERE 子句
	whereRe := regexp.MustCompile(`(?i)\bWHERE\s+(.+?)(?:\s+GROUP\s+BY|\s+HAVING|\s+ORDER\s+BY|\s+LIMIT|$)`)
	if matches := whereRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		selectStmt.WhereClause = matches[1]
	}

	// 提取 GROUP BY 子句
	groupRe := regexp.MustCompile(`(?i)\bGROUP\s+BY\s+(.+?)(?:\s+HAVING|\s+ORDER\s+BY|\s+LIMIT|$)`)
	if matches := groupRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		selectStmt.GroupByClause = matches[1]
	}

	// 提取 HAVING 子句
	havingRe := regexp.MustCompile(`(?i)\bHAVING\s+(.+?)(?:\s+ORDER\s+BY|\s+LIMIT|$)`)
	if matches := havingRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		selectStmt.HavingClause = matches[1]
	}

	// 提取 ORDER BY 子句
	orderRe := regexp.MustCompile(`(?i)\bORDER\s+BY\s+(.+?)(?:\s+LIMIT|$)`)
	if matches := orderRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		selectStmt.OrderByClause = matches[1]
	}

	// 提取 LIMIT 子句（MySQL: LIMIT offset, count -> PostgreSQL: LIMIT count OFFSET offset）
	limitRe := regexp.MustCompile(`(?i)\bLIMIT\s+(.+?)$`)
	if matches := limitRe.FindStringSubmatch(stmt); len(matches) >= 2 {
		limitStr := matches[1]
		selectStmt.LimitClause, selectStmt.OffsetClause = c.parseLimitClause(limitStr)
	}

	return selectStmt
}

// generateSelect 生成 SELECT 语句
func (c *MySQLToPostgresConverter) generateSelect(stmt *SelectStatement) string {
	var parts []string

	// SELECT [DISTINCT] columns
	selectPart := "SELECT"
	if stmt.Distinct {
		selectPart += " DISTINCT"
	}

	// 转换列中的函数调用（如 CASE WHEN 中的正则表达式）
	var convertedCols []string
	for _, col := range stmt.Columns {
		convertedCol := c.convertSelectFunctions(col)
		convertedCols = append(convertedCols, convertedCol)
	}
	selectPart += " " + strings.Join(convertedCols, ", ")
	parts = append(parts, selectPart)

	// FROM
	if stmt.FromClause != "" {
		parts = append(parts, "FROM "+c.basicConvert(stmt.FromClause))
	}

	// WHERE
	if stmt.WhereClause != "" {
		parts = append(parts, "WHERE "+c.convertSelectFunctions(stmt.WhereClause))
	}

	// GROUP BY
	if stmt.GroupByClause != "" {
		parts = append(parts, "GROUP BY "+c.basicConvert(stmt.GroupByClause))
	}

	// HAVING
	if stmt.HavingClause != "" {
		parts = append(parts, "HAVING "+c.convertSelectFunctions(stmt.HavingClause))
	}

	// ORDER BY
	if stmt.OrderByClause != "" {
		parts = append(parts, "ORDER BY "+c.basicConvert(stmt.OrderByClause))
	}

	// LIMIT 和 OFFSET
	if stmt.LimitClause != "" {
		parts = append(parts, "LIMIT "+stmt.LimitClause)
	}
	if stmt.OffsetClause != "" {
		parts = append(parts, "OFFSET "+stmt.OffsetClause)
	}

	result := strings.Join(parts, " ")
	result = c.basicConvert(result)
	return c.ensureSemicolon(result)
}

// convertDelete 转换 DELETE 语句
func (c *MySQLToPostgresConverter) convertDelete(stmt string) string {
	result := c.basicConvert(stmt)
	return c.ensureSemicolon(result)
}

// convertCreateIndex 转换 CREATE INDEX 语句
func (c *MySQLToPostgresConverter) convertCreateIndex(stmt string) string {
	result := stmt
	// 提取索引名和表名
	// 匹配: CREATE [UNIQUE] INDEX index_name ON table_name (...)
	indexRe := regexp.MustCompile(`(?i)CREATE\s+(UNIQUE\s+)?INDEX\s+` + "`?" + `(\w+)` + "`?" + `\s+ON\s+` + "`?" + `(\w+)` + "`?")
	matches := indexRe.FindStringSubmatch(result)
	if len(matches) >= 4 {
		uniqueKeyword := matches[1] // 可能是 "UNIQUE " 或空字符串
		originalIndexName := matches[2]
		tableName := matches[3]
		// 应用索引命名规则
		newIndexName := c.makeIndexName(originalIndexName, tableName)
		// 替换整个 CREATE INDEX 部分
		replacement := fmt.Sprintf("CREATE %sINDEX \"%s\" ON \"%s\"", uniqueKeyword, newIndexName, tableName)
		result = indexRe.ReplaceAllString(result, replacement)
	}
	// 移除 USING 子句（USING BTREE, USING HASH 等）
	result = regexp.MustCompile(`(?i)\s+USING\s+\w+`).ReplaceAllString(result, "")
	// 移除索引列的长度说明（如 column_name(255)）
	result = regexp.MustCompile(`\((\w+)\s*\(\d+\)\)`).ReplaceAllString(result, "($1)")
	result = c.basicConvert(result)
	return c.ensureSemicolon(result)
}

// convertTruncate 转换 TRUNCATE 语句
func (c *MySQLToPostgresConverter) convertTruncate(stmt string) string {
	// TRUNCATE TABLE `name` -> TRUNCATE "name"
	result := stmt
	result = regexp.MustCompile(`(?i)TRUNCATE\s+(?:TABLE\s+)?`).ReplaceAllString(result, "TRUNCATE ")
	// 移除反引号并替换为双引号
	result = strings.ReplaceAll(result, "`", "\"")
	return c.ensureSemicolon(result)
}

// convertDropTable 转换 DROP TABLE 语句
func (c *MySQLToPostgresConverter) convertDropTable(stmt string) string {
	result := c.basicConvert(stmt)
	return c.ensureSemicolon(result)
}

// convertDropIndex 转换 DROP INDEX 语句
func (c *MySQLToPostgresConverter) convertDropIndex(stmt string) string {
	result := stmt
	// 提取索引名和表名
	// 匹配: DROP INDEX index_name ON table_name
	// 注意：索引名和表名可能包含下划线，所以使用更宽松的匹配模式
	// 匹配模式：DROP INDEX [反引号或双引号]索引名[反引号或双引号] ON [反引号或双引号]表名[反引号或双引号]
	dropIndexRe := regexp.MustCompile(`(?i)DROP\s+INDEX\s+[` + "`" + `"]?([^` + "`" + `"\s]+)[` + "`" + `"]?\s+ON\s+[` + "`" + `"]?([^` + "`" + `"\s]+)[` + "`" + `"]?`)
	matches := dropIndexRe.FindStringSubmatch(result)
	if len(matches) >= 3 {
		originalIndexName := strings.Trim(matches[1], "`\"")
		tableName := strings.Trim(matches[2], "`\"")
		// 应用索引命名规则
		newIndexName := c.makeIndexName(originalIndexName, tableName)
		// 替换为 DROP INDEX IF EXISTS "index_name"（完全移除 ON table_name 部分）
		result = dropIndexRe.ReplaceAllString(result, fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", newIndexName))
	} else {
		// 如果没有匹配到 ON table_name 格式，尝试移除任何残留的 ON table_name 部分
		// 这处理了可能已经部分转换的情况
		result = regexp.MustCompile(`(?i)\s+ON\s+[`+"`"+`"]?[^`+"`"+`"\s]+[`+"`"+`"]?`).ReplaceAllString(result, "")
	}
	result = c.basicConvert(result)
	return c.ensureSemicolon(result)
}

// ============================================================================
// DML 辅助函数
// ============================================================================

// parseColumnList 解析列名列表
func (c *MySQLToPostgresConverter) parseColumnList(columnsStr string) []string {
	var columns []string
	var current strings.Builder
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false

	for _, r := range columnsStr {
		switch r {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
			}
			current.WriteRune(r)
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
			current.WriteRune(r)
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
			current.WriteRune(r)
		case '(':
			if !inSingle && !inDouble && !inBacktick {
				depth++
			}
			current.WriteRune(r)
		case ')':
			if !inSingle && !inDouble && !inBacktick {
				depth--
			}
			current.WriteRune(r)
		case ',':
			if !inSingle && !inDouble && !inBacktick && depth == 0 {
				col := strings.TrimSpace(current.String())
				if col != "" {
					columns = append(columns, col)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	col := strings.TrimSpace(current.String())
	if col != "" {
		columns = append(columns, col)
	}

	return columns
}

// parseValuesList 解析 VALUES 列表
func (c *MySQLToPostgresConverter) parseValuesList(valuesStr string) []string {
	var values []string
	var current strings.Builder
	depth := 0
	inSingle := false
	inDouble := false

	// 使用 rune slice 以便更好地处理索引
	runes := []rune(valuesStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			current.WriteRune(r)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			current.WriteRune(r)
		case '(':
			if !inSingle && !inDouble {
				if depth == 0 {
					// 开始新的 VALUES 行，重置 current
					current.Reset()
				}
				depth++
			}
			current.WriteRune(r)
		case ')':
			if !inSingle && !inDouble {
				depth--
				current.WriteRune(r)
				if depth == 0 {
					// 完成一个 VALUES 行
					val := strings.TrimSpace(current.String())
					if val != "" {
						values = append(values, val)
					}
					current.Reset()
					// 跳过后续的逗号和空格
					for i+1 < len(runes) {
						next := runes[i+1]
						if next == ',' || next == ' ' || next == '\t' || next == '\n' || next == '\r' {
							i++
							continue
						}
						break
					}
					continue
				}
			} else {
				current.WriteRune(r)
			}
		case ',':
			// 如果 depth == 0，说明是 VALUES 行之间的分隔符，跳过
			if !inSingle && !inDouble && depth == 0 {
				// 跳过逗号和后续空格
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}

	// 处理最后一个值（如果没有以括号结束）
	if current.Len() > 0 {
		val := strings.TrimSpace(current.String())
		if val != "" {
			values = append(values, val)
		}
	}

	return values
}

// parseSetClause 解析 SET 子句
func (c *MySQLToPostgresConverter) parseSetClause(setClause string) []UpdateSetItem {
	var items []UpdateSetItem
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false
	equalFound := false
	var column strings.Builder
	var value strings.Builder

	for _, r := range setClause {
		switch r {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
			}
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		case '(':
			if !inSingle && !inDouble && !inBacktick {
				depth++
			}
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		case ')':
			if !inSingle && !inDouble && !inBacktick {
				depth--
			}
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		case '=':
			if !inSingle && !inDouble && !inBacktick && depth == 0 {
				equalFound = true
			} else {
				if equalFound {
					value.WriteRune(r)
				} else {
					column.WriteRune(r)
				}
			}
		case ',':
			if !inSingle && !inDouble && !inBacktick && depth == 0 && equalFound {
				// 完成一个 SET 项
				col := strings.TrimSpace(column.String())
				val := strings.TrimSpace(value.String())
				if col != "" && val != "" {
					items = append(items, UpdateSetItem{Column: col, Value: val})
				}
				column.Reset()
				value.Reset()
				equalFound = false
			} else {
				if equalFound {
					value.WriteRune(r)
				} else {
					column.WriteRune(r)
				}
			}
		default:
			if equalFound {
				value.WriteRune(r)
			} else {
				column.WriteRune(r)
			}
		}
	}

	// 添加最后一个项
	if equalFound {
		col := strings.TrimSpace(column.String())
		val := strings.TrimSpace(value.String())
		if col != "" && val != "" {
			items = append(items, UpdateSetItem{Column: col, Value: val})
		}
	}

	return items
}

// parseLimitClause 解析 LIMIT 子句（MySQL: LIMIT offset, count -> PostgreSQL: LIMIT count OFFSET offset）
func (c *MySQLToPostgresConverter) parseLimitClause(limitStr string) (limit, offset string) {
	limitStr = strings.TrimSpace(limitStr)
	// 检查是否有逗号分隔（offset, count 格式）
	if strings.Contains(limitStr, ",") {
		parts := strings.Split(limitStr, ",")
		if len(parts) == 2 {
			offset = strings.TrimSpace(parts[0])
			limit = strings.TrimSpace(parts[1])
		}
	} else {
		// 只有 count
		limit = limitStr
	}
	return limit, offset
}

// convertInsertValues 转换 INSERT VALUES 中的函数调用
func (c *MySQLToPostgresConverter) convertInsertValues(value string) string {
	// 转换 CONVERT(... USING utf8mb4) -> CAST(... AS text)
	value = regexp.MustCompile(`(?i)CONVERT\s*\(([^,]+)\s*USING\s+utf8mb4\)`).
		ReplaceAllString(value, "CAST($1 AS text)")
	// 转换其他 CONVERT 调用
	value = regexp.MustCompile(`(?i)CONVERT\s*\(([^,]+)\s*,\s*([^)]+)\)`).
		ReplaceAllString(value, "CAST($1 AS $2)")
	return value
}

// convertSelectFunctions 转换 SELECT 中的函数调用
func (c *MySQLToPostgresConverter) convertSelectFunctions(expr string) string {
	// 转换 CONVERT(... USING utf8mb4)
	expr = regexp.MustCompile(`(?i)CONVERT\s*\(([^,]+)\s*USING\s+utf8mb4\)`).
		ReplaceAllString(expr, "CAST($1 AS text)")
	// 转换其他 CONVERT 调用
	expr = regexp.MustCompile(`(?i)CONVERT\s*\(([^,]+)\s*,\s*([^)]+)\)`).
		ReplaceAllString(expr, "CAST($1 AS $2)")
	// 转换 CASE WHEN 中的正则表达式（MySQL REGEXP -> PostgreSQL ~）
	expr = regexp.MustCompile(`(?i)(\w+)\s+REGEXP\s+([^\s]+)`).
		ReplaceAllString(expr, "$1 ~ $2")
	expr = regexp.MustCompile(`(?i)(\w+)\s+NOT\s+REGEXP\s+([^\s]+)`).
		ReplaceAllString(expr, "$1 !~ $2")
	// 修复正则表达式中的字符类（如 [^/]* -> SIMILAR TO）
	// PostgreSQL 中 ~ 不支持字符类，需要使用 SIMILAR TO
	// 例如：'^[^/]*/' 在 PostgreSQL 中应写为 '^./' 或使用 SIMILAR TO
	expr = regexp.MustCompile(`(~)\s*('[^']*\\[\\^\\][^']*')`).ReplaceAllString(expr, "$1 SIMILAR TO $2")
	return expr
}

// convertOnDuplicateUpdate 转换 ON DUPLICATE KEY UPDATE 子句
func (c *MySQLToPostgresConverter) convertOnDuplicateUpdate(onDup string) string {
	// 基本转换：移除表别名等
	result := c.basicConvert(onDup)
	return result
}

// convertJoinToFrom 将 MySQL 的 JOIN 转换为 PostgreSQL 的 FROM ... WHERE
// 返回表名和 JOIN 条件（条件需要添加到 WHERE 子句）
func (c *MySQLToPostgresConverter) convertJoinToFrom(joinClause string) (tableName, joinCondition string) {
	// 匹配: JOIN table [alias] ON condition [SET ...]
	joinRe := regexp.MustCompile(`(?i)JOIN\s+` + "`?" + `([^\s` + "`" + `]+)` + "`?" + `(?:\s+(?:AS\s+)?` + "`?" + `(\w+)` + "`?" + `)?\s+ON\s+(.+?)(?:\s+SET\b|$)`)
	matches := joinRe.FindStringSubmatch(joinClause)
	if len(matches) < 4 {
		return c.basicConvert(joinClause), ""
	}

	tableName = strings.Trim(matches[1], "`\"")
	alias := matches[2]
	joinCondition = c.cleanJoinCondition(matches[3])

	// 格式化表名（带别名）
	if alias != "" {
		tableName = fmt.Sprintf("\"%s\" \"%s\"", tableName, alias)
	} else {
		tableName = fmt.Sprintf("\"%s\"", tableName)
	}

	return tableName, joinCondition
}

// cleanJoinCondition 清理 JOIN 条件，移除 SET 关键字
func (c *MySQLToPostgresConverter) cleanJoinCondition(condition string) string {
	condition = strings.TrimSpace(condition)
	// 移除开头的 SET 关键字
	condition = regexp.MustCompile(`(?i)^\s*SET\s+`).ReplaceAllString(condition, "")
	// 移除末尾的 SET 关键字和后续内容
	condition = regexp.MustCompile(`(?i)\s+SET\s+.*$`).ReplaceAllString(condition, "")
	// 移除末尾的 SET 关键字
	condition = regexp.MustCompile(`(?i)\s+SET\s*$`).ReplaceAllString(condition, "")
	return strings.TrimSpace(condition)
}

// removeTableAlias 移除列名中的表别名
func (c *MySQLToPostgresConverter) removeTableAlias(column string) string {
	// 匹配: alias.column 或 `alias`.`column`
	column = strings.TrimSpace(column)
	re := regexp.MustCompile("^[`\"]?\\w+[`\"]?\\.[`\"]?([^`\"]+)[`\"]?$")
	if matches := re.FindStringSubmatch(column); len(matches) >= 2 {
		return matches[1]
	}
	return column
}

