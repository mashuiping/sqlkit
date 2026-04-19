// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
package converter

import (
	"regexp"
	"sort"
	"strings"
)

// ============================================================================
// 辅助函数（复用现有逻辑）
// ============================================================================

// CommentInfo 存储从 CREATE TABLE 语句中提取的注释信息
type CommentInfo struct {
	TableComment   string
	ColumnComments map[string]string
}

// IndexInfo 存储索引信息
type IndexInfo struct {
	IsUnique  bool
	Name      string
	Columns   string
	TableName string
}

// extractComments 从 CREATE TABLE 语句中提取注释
func (c *MySQLToPostgresConverter) extractComments(stmt string, tableName string) CommentInfo {
	info := CommentInfo{
		ColumnComments: make(map[string]string),
	}

	lastParenIdx := strings.LastIndex(stmt, ")")
	if lastParenIdx != -1 {
		tableOptions := stmt[lastParenIdx:]
		tableCommentRe := regexp.MustCompile(`(?i)COMMENT\s*=?\s*'([^']*)'`)
		if matches := tableCommentRe.FindStringSubmatch(tableOptions); len(matches) >= 2 {
			info.TableComment = matches[1]
		}
	}

	lines := strings.Split(stmt, "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, ")") {
			continue
		}

		upperLine := strings.ToUpper(trimmedLine)
		if strings.HasPrefix(upperLine, "PRIMARY KEY") ||
			strings.HasPrefix(upperLine, "UNIQUE KEY") ||
			strings.HasPrefix(upperLine, "KEY ") ||
			strings.HasPrefix(upperLine, "INDEX ") ||
			strings.HasPrefix(upperLine, "CONSTRAINT ") ||
			strings.HasPrefix(upperLine, "FULLTEXT ") ||
			strings.HasPrefix(upperLine, "SPATIAL ") {
			continue
		}

		columnCommentRe := regexp.MustCompile("^\\s*`?(\\w+)`?\\s+\\w+.*?\\s+COMMENT\\s+'([^']*)'")
		if matches := columnCommentRe.FindStringSubmatch(trimmedLine); len(matches) >= 3 {
			columnName := matches[1]
			comment := matches[2]
			upperName := strings.ToUpper(columnName)
			if upperName != "DEFAULT" && upperName != "ENGINE" && upperName != "CHARSET" &&
				upperName != "INDEX" && upperName != "UNIQUE" && upperName != "KEY" &&
				upperName != "CREATE" && upperName != "TABLE" {
				info.ColumnComments[columnName] = comment
			}
		}
	}

	return info
}

// extractIndexes 从 CREATE TABLE 语句中提取索引定义
func (c *MySQLToPostgresConverter) extractIndexes(stmt string, tableName string) (string, []IndexInfo) {
	var indexes []IndexInfo
	var rangesToRemove [][2]int

	uniqueKeyRe := regexp.MustCompile(`(?i),?\s*UNIQUE\s+(?:KEY|INDEX)\s+` + "`?" + `(\w+)` + "`?" + `\s*\(`)
	for _, matchIdx := range uniqueKeyRe.FindAllStringSubmatchIndex(stmt, -1) {
		if len(matchIdx) < 4 {
			continue
		}
		indexName := stmt[matchIdx[2]:matchIdx[3]]
		parenStart := matchIdx[1] - 1
		parenEnd := c.findMatchingParen(stmt, parenStart)
		if parenEnd == -1 {
			continue
		}

		columns := c.cleanColumnList(stmt[parenStart+1 : parenEnd])
		restStart := parenEnd + 1
		restEnd := c.findIndexSuffixEnd(stmt, restStart)

		indexes = append(indexes, IndexInfo{
			IsUnique:  true,
			Name:      indexName,
			Columns:   columns,
			TableName: tableName,
		})
		rangesToRemove = append(rangesToRemove, [2]int{matchIdx[0], restEnd})
	}

	keyRe := regexp.MustCompile(`(?i),?\s*(?:KEY|INDEX)\s+` + "`?" + `(\w+)` + "`?" + `\s*\(`)
	for _, matchIdx := range keyRe.FindAllStringSubmatchIndex(stmt, -1) {
		if len(matchIdx) < 4 {
			continue
		}

		alreadyMatched := false
		for _, r := range rangesToRemove {
			if matchIdx[0] >= r[0] && matchIdx[0] < r[1] {
				alreadyMatched = true
				break
			}
		}
		if alreadyMatched {
			continue
		}

		prefixStart := matchIdx[0] - 20
		if prefixStart < 0 {
			prefixStart = 0
		}
		prefix := strings.ToUpper(stmt[prefixStart:matchIdx[0]])
		if strings.Contains(prefix, "UNIQUE") || strings.Contains(prefix, "PRIMARY") {
			continue
		}

		indexName := stmt[matchIdx[2]:matchIdx[3]]
		parenStart := matchIdx[1] - 1
		parenEnd := c.findMatchingParen(stmt, parenStart)
		if parenEnd == -1 {
			continue
		}

		columns := c.cleanColumnList(stmt[parenStart+1 : parenEnd])
		restStart := parenEnd + 1
		restEnd := c.findIndexSuffixEnd(stmt, restStart)

		indexes = append(indexes, IndexInfo{
			IsUnique:  false,
			Name:      indexName,
			Columns:   columns,
			TableName: tableName,
		})
		rangesToRemove = append(rangesToRemove, [2]int{matchIdx[0], restEnd})
	}

	sort.Slice(rangesToRemove, func(i, j int) bool {
		return rangesToRemove[i][0] > rangesToRemove[j][0]
	})
	for _, r := range rangesToRemove {
		stmt = stmt[:r[0]] + stmt[r[1]:]
	}

	stmt = regexp.MustCompile(`(?s),\s*\)`).ReplaceAllString(stmt, ")")
	stmt = regexp.MustCompile(`(?s)\(\s*,`).ReplaceAllString(stmt, "(")
	stmt = regexp.MustCompile(`(?s),\s*,`).ReplaceAllString(stmt, ",")

	return stmt, indexes
}

// findIndexSuffixEnd 找到索引定义后缀的结束位置
func (c *MySQLToPostgresConverter) findIndexSuffixEnd(stmt string, start int) int {
	pos := start
	rest := stmt[start:]

	// 匹配 USING 子句（如 USING BTREE, USING HASH）
	usingRe := regexp.MustCompile(`(?i)^\s*USING\s+\w+`)
	if match := usingRe.FindString(rest); match != "" {
		pos += len(match)
		rest = stmt[pos:]
	}

	// 匹配 COMMENT 子句
	commentRe := regexp.MustCompile(`(?i)^\s*COMMENT\s+'[^']*'`)
	if match := commentRe.FindString(rest); match != "" {
		pos += len(match)
	}

	return pos
}

// cleanColumnList 清理索引列列表
func (c *MySQLToPostgresConverter) cleanColumnList(columns string) string {
	columns = strings.ReplaceAll(columns, "`", "")
	columns = strings.ReplaceAll(columns, "\"", "")
	// 移除列名后的长度说明，如 column_name(255)
	columns = regexp.MustCompile(`\(\d+\)`).ReplaceAllString(columns, "")
	// 移除 USING 子句（如果存在）
	columns = regexp.MustCompile(`(?i)\s+USING\s+\w+`).ReplaceAllString(columns, "")
	columns = regexp.MustCompile(`\s+`).ReplaceAllString(columns, " ")
	return strings.TrimSpace(columns)
}

// makeIndexName 生成带表名前缀的索引名
func (c *MySQLToPostgresConverter) makeIndexName(originalName, tableName string) string {
	if strings.Contains(strings.ToLower(originalName), strings.ToLower(tableName)) {
		return originalName
	}

	prefixRe := regexp.MustCompile(`(?i)^(idx_|index_|uniq_|unique_|uk_|key_|k_)`)
	if match := prefixRe.FindString(originalName); match != "" {
		suffix := originalName[len(match):]
		return match + tableName + "_" + suffix
	}

	return tableName + "_" + originalName
}

// timestampType 返回时间戳类型
func (c *MySQLToPostgresConverter) timestampType() string {
	if c.UseTimestampTZ {
		return "TIMESTAMPTZ"
	}
	return "TIMESTAMP"
}

// findMatchingParen 找到匹配的右括号位置
func (c *MySQLToPostgresConverter) findMatchingParen(s string, start int) int {
	if start >= len(s) || s[start] != '(' {
		return -1
	}
	depth := 0
	inSingle := false
	inDouble := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

