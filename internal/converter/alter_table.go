// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// ALTER TABLE 解析和生成
// ============================================================================

// parseAlterTable 解析 ALTER TABLE 语句为 AST
func (c *MySQLToPostgresConverter) parseAlterTable(stmt string) *AlterTableStatement {
	alterTable := &AlterTableStatement{
		BaseStatement: BaseStatement{RawSQL: stmt},
		Actions:       []AlterAction{},
	}

	// 提取表名
	tableNameRe := regexp.MustCompile(`(?i)ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?")
	matches := tableNameRe.FindStringSubmatch(stmt)
	if len(matches) >= 2 {
		alterTable.TableName = strings.Trim(matches[1], "`\"")
	}

	// 解析各种 ALTER 操作
	alterTable.Actions = c.parseAlterActions(stmt, alterTable.TableName)

	return alterTable
}

// parseAlterActions 解析 ALTER TABLE 的各种操作
func (c *MySQLToPostgresConverter) parseAlterActions(stmt string, tableName string) []AlterAction {
	var actions []AlterAction

	actions = append(actions, c.parseAddColumnActions(stmt)...)
	actions = append(actions, c.parseDropColumnActions(stmt)...)
	actions = append(actions, c.parseModifyColumnActions(stmt)...)
	actions = append(actions, c.parseChangeColumnActions(stmt)...)
	actions = append(actions, c.parseDropIndexActions(stmt)...)
	actions = append(actions, c.parseAddIndexActions(stmt)...)

	return actions
}

// parseAddColumnActions 解析 ADD COLUMN 操作
func (c *MySQLToPostgresConverter) parseAddColumnActions(stmt string) []AlterAction {
	var actions []AlterAction
	addColumnRe := regexp.MustCompile(`(?i)ADD\s+(?:COLUMN\s+)?` + "`?" + `(\w+)` + "`?" + `\s+(.+?)(?:,|$|;)`)
	matches := addColumnRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		// 跳过 ADD INDEX/KEY/UNIQUE
		upperMatch := strings.ToUpper(match[0])
		if c.isIndexOrKeyOperation(upperMatch) {
			continue
		}

		colName := match[1]
		colDefStr := match[2]
		col := c.parseSingleColumn(fmt.Sprintf("%s %s", colName, colDefStr), CommentInfo{})
		if col == nil {
			continue
		}

		action := &AddColumnAction{Column: col}
		c.setAddColumnPosition(colDefStr, &action.After, &action.First)
		actions = append(actions, action)
	}

	return actions
}

// parseDropColumnActions 解析 DROP COLUMN 操作
func (c *MySQLToPostgresConverter) parseDropColumnActions(stmt string) []AlterAction {
	var actions []AlterAction
	dropColumnRe := regexp.MustCompile(`(?i)DROP\s+(?:COLUMN\s+)?` + "`?" + `(\w+)` + "`?")
	matches := dropColumnRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		// 确保不是 DROP INDEX
		if strings.Contains(strings.ToUpper(match[0]), "DROP INDEX") {
			continue
		}
		actions = append(actions, &DropColumnAction{ColumnName: match[1]})
	}

	return actions
}

// parseModifyColumnActions 解析 MODIFY COLUMN 操作
func (c *MySQLToPostgresConverter) parseModifyColumnActions(stmt string) []AlterAction {
	var actions []AlterAction
	modifyRe := regexp.MustCompile(`(?i)MODIFY\s+(?:COLUMN\s+)?` + "`?" + `(\w+)` + "`?" + `\s+(.+)`)
	matches := modifyRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		colName := match[1]
		colDefStr := match[2]
		col := c.parseSingleColumn(fmt.Sprintf("%s %s", colName, colDefStr), CommentInfo{})
		if col != nil {
			actions = append(actions, &ModifyColumnAction{Column: col})
		}
	}

	return actions
}

// parseChangeColumnActions 解析 CHANGE COLUMN 操作
func (c *MySQLToPostgresConverter) parseChangeColumnActions(stmt string) []AlterAction {
	var actions []AlterAction
	changeRe := regexp.MustCompile(`(?i)CHANGE\s+(?:COLUMN\s+)?` + "`?" + `(\w+)` + "`?" + `\s+` + "`?" + `(\w+)` + "`?" + `\s+(.+)`)
	matches := changeRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		oldName := match[1]
		newName := match[2]
		colDefStr := match[3]
		col := c.parseSingleColumn(fmt.Sprintf("%s %s", newName, colDefStr), CommentInfo{})
		if col != nil {
			actions = append(actions, &ChangeColumnAction{
				OldName: oldName,
				NewName: newName,
				Column:  col,
			})
		}
	}

	return actions
}

// parseDropIndexActions 解析 DROP INDEX 操作
func (c *MySQLToPostgresConverter) parseDropIndexActions(stmt string) []AlterAction {
	var actions []AlterAction
	dropIndexRe := regexp.MustCompile(`(?i)DROP\s+INDEX\s+` + "`?" + `(\w+)` + "`?")
	matches := dropIndexRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			actions = append(actions, &DropIndexAction{IndexName: match[1]})
		}
	}

	return actions
}

// parseAddIndexActions 解析 ADD INDEX 操作
func (c *MySQLToPostgresConverter) parseAddIndexActions(stmt string) []AlterAction {
	var actions []AlterAction
	addIndexRe := regexp.MustCompile(`(?i)ADD\s+(?:UNIQUE\s+)?(?:INDEX|KEY)\s+` + "`?" + `(\w+)` + "`?" + `\s*\(([^)]+)\)(?:\s+USING\s+\w+)?`)
	matches := addIndexRe.FindAllStringSubmatch(stmt, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		indexName := match[1]
		columnsStr := match[2]
		// 移除 USING 子句（如果存在）
		columnsStr = regexp.MustCompile(`(?i)\s+USING\s+\w+`).ReplaceAllString(columnsStr, "")
		columns := c.parseIndexColumns(columnsStr)
		isUnique := strings.Contains(strings.ToUpper(match[0]), "UNIQUE")
		actions = append(actions, &AddIndexAction{
			IndexName: indexName,
			Columns:   columns,
			IsUnique:  isUnique,
		})
	}

	return actions
}

// isIndexOrKeyOperation 判断是否为索引或键操作
func (c *MySQLToPostgresConverter) isIndexOrKeyOperation(upperMatch string) bool {
	return strings.Contains(upperMatch, "ADD INDEX") ||
		strings.Contains(upperMatch, "ADD KEY") ||
		strings.Contains(upperMatch, "ADD UNIQUE") ||
		strings.Contains(upperMatch, "ADD PRIMARY")
}

// setAddColumnPosition 设置 ADD COLUMN 的位置（AFTER/FIRST）
func (c *MySQLToPostgresConverter) setAddColumnPosition(colDefStr string, after *string, first *bool) {
	upperColDef := strings.ToUpper(colDefStr)
	if strings.Contains(upperColDef, "AFTER") {
		afterRe := regexp.MustCompile(`(?i)\bAFTER\s+` + "`?" + `(\w+)` + "`?")
		if matches := afterRe.FindStringSubmatch(colDefStr); len(matches) >= 2 {
			*after = matches[1]
		}
	} else if strings.Contains(upperColDef, "FIRST") {
		*first = true
	}
}

// parseIndexColumns 解析索引列列表
func (c *MySQLToPostgresConverter) parseIndexColumns(columnsStr string) []string {
	columns := strings.Split(columnsStr, ",")
	for i := range columns {
		columns[i] = strings.TrimSpace(strings.Trim(columns[i], "`\""))
	}
	return columns
}

// generateAlterTable 生成 ALTER TABLE 语句
func (c *MySQLToPostgresConverter) generateAlterTable(stmt *AlterTableStatement) string {
	alterActions, indexStmts, dropIndexStmts := c.separateAlterActions(stmt)

	var finalParts []string

	// 生成 ALTER TABLE 语句（如果有列操作）
	if len(alterActions) > 0 {
		allAlterParts, separateStatements := c.splitMultiStatementActions(alterActions)
		if len(allAlterParts) > 0 {
			finalParts = append(finalParts, c.buildAlterTableStatement(stmt.TableName, allAlterParts))
		}
		if len(separateStatements) > 0 {
			finalParts = append(finalParts, strings.Join(separateStatements, "\n"))
		}
	}

	// 添加 DROP INDEX 语句
	if len(dropIndexStmts) > 0 {
		finalParts = append(finalParts, strings.Join(dropIndexStmts, ";\n")+";")
	}

	// 添加 CREATE INDEX 语句
	if len(indexStmts) > 0 {
		finalParts = append(finalParts, strings.Join(indexStmts, ";\n")+";")
	}

	if len(finalParts) == 0 {
		return c.basicConvert(stmt.RawSQL)
	}

	return strings.Join(finalParts, "\n\n")
}

// separateAlterActions 分离不同类型的 ALTER 操作
func (c *MySQLToPostgresConverter) separateAlterActions(stmt *AlterTableStatement) (alterActions, indexStmts, dropIndexStmts []string) {
	for _, action := range stmt.Actions {
		actionSQL := action.ToPostgresSQL(c, stmt.TableName)

		switch action.(type) {
		case *AddIndexAction:
			indexStmts = append(indexStmts, actionSQL)
		case *DropIndexAction:
			dropIndexStmts = append(dropIndexStmts, actionSQL)
		default:
			alterActions = append(alterActions, actionSQL)
		}
	}
	return alterActions, indexStmts, dropIndexStmts
}

// splitMultiStatementActions 分割可能包含多个语句的 actionSQL（如 CHANGE COLUMN）
func (c *MySQLToPostgresConverter) splitMultiStatementActions(alterActions []string) (allAlterParts, separateStatements []string) {
	for _, actionSQL := range alterActions {
		if !strings.Contains(actionSQL, ";") {
			allAlterParts = append(allAlterParts, actionSQL)
			continue
		}

		// 包含分号，说明是多个语句
		parts := strings.Split(actionSQL, ";")
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			upperPart := strings.ToUpper(part)
			if strings.HasPrefix(upperPart, "ALTER TABLE") {
				separateStatements = append(separateStatements, part+";")
			} else if i == 0 {
				// 第一个部分是 ALTER COLUMN，属于当前 ALTER TABLE
				allAlterParts = append(allAlterParts, part)
			} else {
				allAlterParts = append(allAlterParts, part)
			}
		}
	}
	return allAlterParts, separateStatements
}

// buildAlterTableStatement 构建 ALTER TABLE 语句
func (c *MySQLToPostgresConverter) buildAlterTableStatement(tableName string, alterParts []string) string {
	if len(alterParts) == 1 {
		return fmt.Sprintf("ALTER TABLE \"%s\" %s;", tableName, alterParts[0])
	}
	actionsStr := strings.Join(alterParts, ",\n    ")
	return fmt.Sprintf("ALTER TABLE \"%s\"\n    %s;", tableName, actionsStr)
}

