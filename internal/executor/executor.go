// Package executor 提供数据库 SQL 执行功能
package executor

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Executor 定义数据库执行器接口
type Executor interface {
	// Connect 连接到数据库
	Connect() error
	// Close 关闭数据库连接
	Close() error
	// Execute 执行 SQL 语句（包含查询和非查询）
	Execute(query string) (*ExecuteResult, error)
	// ExecuteQuery 执行查询语句
	ExecuteQuery(query string) (*QueryResult, error)
	// ExecuteExec 执行非查询语句
	ExecuteExec(query string) (*ExecResult, error)
	// Ping 测试连接
	Ping() error
}

// ExecuteResult 执行结果
type ExecuteResult struct {
	IsQuery     bool
	QueryResult *QueryResult
	ExecResult  *ExecResult
	ExecutedSQL string
	Duration    time.Duration
}

// QueryResult 查询结果
type QueryResult struct {
	Columns  []string
	Rows     [][]string
	RowCount int
}

// ExecResult 非查询执行结果
type ExecResult struct {
	RowsAffected int64
	LastInsertId int64
}

// BaseExecutor 基础执行器，提供通用功能
type BaseExecutor struct {
	DB  *sql.DB
	DSN string
}

// Close 关闭数据库连接
func (e *BaseExecutor) Close() error {
	if e.DB != nil {
		return e.DB.Close()
	}
	return nil
}

// Ping 测试连接
func (e *BaseExecutor) Ping() error {
	if e.DB == nil {
		return fmt.Errorf("database connection not established")
	}
	return e.DB.Ping()
}

// Execute 执行 SQL（自动判断是查询还是非查询）
func (e *BaseExecutor) Execute(query string) (*ExecuteResult, error) {
	start := time.Now()
	trimmedQuery := strings.TrimSpace(query)
	upperQuery := strings.ToUpper(trimmedQuery)

	isQuery := strings.HasPrefix(upperQuery, "SELECT") ||
		strings.HasPrefix(upperQuery, "SHOW") ||
		strings.HasPrefix(upperQuery, "DESCRIBE") ||
		strings.HasPrefix(upperQuery, "DESC ") ||
		strings.HasPrefix(upperQuery, "EXPLAIN")

	result := &ExecuteResult{
		IsQuery:     isQuery,
		ExecutedSQL: query,
	}

	var err error
	if isQuery {
		result.QueryResult, err = e.ExecuteQuery(query)
	} else {
		result.ExecResult, err = e.ExecuteExec(query)
	}

	result.Duration = time.Since(start)
	return result, err
}

// ExecuteQuery 执行查询语句
func (e *BaseExecutor) ExecuteQuery(query string) (*QueryResult, error) {
	if e.DB == nil {
		return nil, fmt.Errorf("database connection not established")
	}

	rows, err := e.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to get column types: %w", err)
	}

	result := &QueryResult{
		Columns: columns,
		Rows:    make([][]string, 0),
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make([]string, len(values))
		for i, val := range values {
			row[i] = formatValue(val, columnTypes[i])
		}
		result.Rows = append(result.Rows, row)
		result.RowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return result, nil
}

// ExecuteExec 执行非查询语句
func (e *BaseExecutor) ExecuteExec(query string) (*ExecResult, error) {
	if e.DB == nil {
		return nil, fmt.Errorf("database connection not established")
	}

	sqlResult, err := e.DB.Exec(query)
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	result := &ExecResult{}

	if rowsAffected, err := sqlResult.RowsAffected(); err == nil {
		result.RowsAffected = rowsAffected
	}

	if lastInsertId, err := sqlResult.LastInsertId(); err == nil {
		result.LastInsertId = lastInsertId
	}

	return result, nil
}

// SplitStatements 将 SQL 文本拆分为独立语句
func SplitStatements(sqlText string) []string {
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

// truncateQuery 截断查询用于错误显示
func truncateQuery(query string, maxLen ...int) string {
	max := 100
	if len(maxLen) > 0 {
		max = maxLen[0]
	}
	if len(query) > max {
		return query[:max] + "..."
	}
	return query
}

// formatValue 格式化值为字符串
func formatValue(val interface{}, colType *sql.ColumnType) string {
	if val == nil {
		return "NULL"
	}

	switch v := val.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%.6f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// PrintQueryResult 打印查询结果
func PrintQueryResult(result *QueryResult, maxWidth ...int) {
	if result == nil {
		fmt.Println("No result")
		return
	}

	// 计算每列的最大宽度
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = len(col)
	}
	for _, row := range result.Rows {
		for i, val := range row {
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	// 限制最大宽度
	max := 1000
	if len(maxWidth) > 0 {
		max = maxWidth[0]
	}
	for i := range widths {
		if widths[i] > max {
			widths[i] = max
		}
	}

	// 打印分隔线
	printSeparator := func() {
		for i, w := range widths {
			if i > 0 {
				fmt.Print("+")
			}
			fmt.Print("+")
			fmt.Print(strings.Repeat("-", w+2))
		}
		fmt.Println("+")
	}

	// 打印行
	printRow := func(values []string) {
		for i, val := range values {
			if i > 0 {
				fmt.Print("|")
			}
			fmt.Print("| ")
			if len(val) > widths[i] {
				val = val[:widths[i]-3] + "..."
			}
			fmt.Printf("%-*s ", widths[i], val)
		}
		fmt.Println("|")
	}

	printSeparator()
	printRow(result.Columns)
	printSeparator()
	for _, row := range result.Rows {
		printRow(row)
	}
	printSeparator()

	fmt.Printf("Total: %d rows\n", result.RowCount)
}
