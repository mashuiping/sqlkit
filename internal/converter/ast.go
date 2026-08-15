// Package converter 提供 MySQL SQL 到 PostgreSQL/GaussDB SQL 的转换功能
// 参考 sqlglot (https://github.com/tobymao/sqlglot) 的实现方式
// 核心思想：Parse(解析) -> Transform(转换) -> Generate(生成)
package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// AST 节点定义（参考 sqlglot 的设计）
// ============================================================================

// Statement 表示一条已解析的语句节点（AST 根节点）
type Statement interface {
	ToPostgres(c *MySQLToPostgresConverter) string
}

// BaseStatement 提供所有语句的公共字段
type BaseStatement struct {
	RawSQL string // 原始 SQL 文本（用于兜底或调试）
}

// ============================================================================
// CREATE TABLE AST
// ============================================================================

// CreateTableStatement 对应 CREATE TABLE 语句
type CreateTableStatement struct {
	BaseStatement
	IfNotExists  bool
	TableName    string
	Columns      []*ColumnDef
	Constraints  []*TableConstraint
	TableOptions *TableOptions
}

// ColumnDef 列定义（参考 sqlglot.expressions.ColumnDef）
type ColumnDef struct {
	Name          string
	DataType      *DataType
	NotNull       bool
	Default       string // 默认值表达式
	AutoIncrement bool
	Comment       string
	Position      string // AFTER column_name 或 FIRST
	Check         string // 列级 CHECK 表达式
}

// DataType 数据类型（参考 sqlglot.expressions.DataType）
type DataType struct {
	Type       string // INTEGER, VARCHAR, DATETIME 等
	Precision  int    // 精度（如 VARCHAR(255) 的 255）
	Scale      int    // 小数位数（如 DECIMAL(10,2) 的 2）
	Unsigned   bool   // UNSIGNED 修饰符
	EnumValues string // ENUM 值列表（不含括号）
}

// TableConstraint 表级约束（PRIMARY KEY, UNIQUE KEY, INDEX 等）
type TableConstraint struct {
	Type       ConstraintType // PRIMARY_KEY, UNIQUE_KEY, INDEX, FOREIGN_KEY
	Name       string
	Columns    []string
	IsUnique   bool // 用于 UNIQUE KEY/INDEX
	Definition string
}

// ConstraintType 约束类型
type ConstraintType int

const (
	ConstraintPrimaryKey ConstraintType = iota
	ConstraintUniqueKey
	ConstraintIndex
	ConstraintForeignKey
	ConstraintCheck
)

// TableOptions 表选项（ENGINE, CHARSET, COMMENT 等）
type TableOptions struct {
	Engine        string
	Charset       string
	Collate       string
	Comment       string
	AutoIncrement int
}

// ============================================================================
// ALTER TABLE AST
// ============================================================================

// AlterTableStatement 对应 ALTER TABLE 语句
type AlterTableStatement struct {
	BaseStatement
	TableName string
	Actions   []AlterAction
}

// AlterAction 表示 ALTER TABLE 的一个操作
type AlterAction interface {
	ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string
}

// AddColumnAction ADD COLUMN 操作
type AddColumnAction struct {
	Column *ColumnDef
	After  string // AFTER column_name
	First  bool   // FIRST
}

// DropColumnAction DROP COLUMN 操作
type DropColumnAction struct {
	ColumnName string
}

// ModifyColumnAction MODIFY COLUMN 操作
type ModifyColumnAction struct {
	Column *ColumnDef
}

// ChangeColumnAction CHANGE COLUMN 操作
type ChangeColumnAction struct {
	OldName string
	NewName string
	Column  *ColumnDef
}

// DropIndexAction DROP INDEX 操作
type DropIndexAction struct {
	IndexName string
}

// AddIndexAction ADD INDEX 操作
type AddIndexAction struct {
	IndexName string
	Columns   []string
	IsUnique  bool
}

// ============================================================================
// INSERT 语句 AST
// ============================================================================

// InsertStatement 对应 INSERT 语句
type InsertStatement struct {
	BaseStatement
	Ignore      bool     // INSERT IGNORE
	TableName   string   // 表名
	Columns     []string // 列名列表
	Values      []string // VALUES 子句的值列表（字符串形式，用于转换）
	SelectQuery string   // SELECT 查询（如果有）
	OnDuplicate string   // ON DUPLICATE KEY UPDATE 子句
	HasIntoInto bool     // 检测 INSERT INTO INTO 重复关键字
}

// ============================================================================
// UPDATE 语句 AST
// ============================================================================

// UpdateStatement 对应 UPDATE 语句
type UpdateStatement struct {
	BaseStatement
	TableName     string          // 主表名
	TableAlias    string          // 表别名
	SetClause     []UpdateSetItem // SET 子句
	JoinClause    string          // JOIN 子句（MySQL 语法）
	FromClause    string          // FROM 子句（PostgreSQL 语法）
	WhereClause   string          // WHERE 子句
	OrderByClause string          // ORDER BY 子句
	LimitClause   string          // LIMIT 子句
	HasJoin       bool            // 是否有 JOIN
}

// UpdateSetItem 表示 UPDATE SET 的一个赋值项
type UpdateSetItem struct {
	Column string // 列名（可能带表别名）
	Value  string // 值表达式
}

// ============================================================================
// SELECT 语句 AST
// ============================================================================

// SelectStatement 对应 SELECT 语句
type SelectStatement struct {
	BaseStatement
	Distinct      bool     // SELECT DISTINCT
	Columns       []string // SELECT 列列表
	FromClause    string   // FROM 子句
	WhereClause   string   // WHERE 子句
	GroupByClause string   // GROUP BY 子句
	HavingClause  string   // HAVING 子句
	OrderByClause string   // ORDER BY 子句
	LimitClause   string   // LIMIT 子句
	OffsetClause  string   // OFFSET 子句
}

// ============================================================================
// DELETE 语句 AST
// ============================================================================

// DeleteStatement 对应 DELETE 语句
type DeleteStatement struct {
	BaseStatement
}

// CreateIndexStatement 对应 CREATE [UNIQUE] INDEX 语句
type CreateIndexStatement struct {
	BaseStatement
}

// DropTableStatement 对应 DROP TABLE 语句
type DropTableStatement struct {
	BaseStatement
}

// DropIndexStatement 对应 DROP INDEX 语句
type DropIndexStatement struct {
	BaseStatement
}

// UnknownStatement 兜底语句：未知类型，做基本转换
type UnknownStatement struct {
	BaseStatement
}

// ============================================================================
// Statement.ToPostgres 实现
// ============================================================================

func (s *CreateTableStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.generateCreateTable(s)
}

func (s *AlterTableStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.generateAlterTable(s)
}

func (s *InsertStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.generateInsert(s)
}

func (s *UpdateStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.generateUpdate(s)
}

func (s *SelectStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.generateSelect(s)
}

func (s *DeleteStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.convertDelete(s.RawSQL)
}

func (s *CreateIndexStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.convertCreateIndex(s.RawSQL)
}

func (s *UnknownStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	// 特殊处理 TRUNCATE 语句
	trimmed := strings.TrimSpace(s.RawSQL)
	if strings.HasPrefix(strings.ToUpper(trimmed), "TRUNCATE") {
		return c.convertTruncate(s.RawSQL)
	}
	return c.basicConvert(s.RawSQL)
}

func (s *DropTableStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.convertDropTable(s.RawSQL)
}

func (s *DropIndexStatement) ToPostgres(c *MySQLToPostgresConverter) string {
	return c.convertDropIndex(s.RawSQL)
}

// ============================================================================
// AlterAction.ToPostgresSQL 实现
// ============================================================================

func (a *AddColumnAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	colDef := c.generateColumnDef(a.Column)
	// PostgreSQL 不支持 AFTER/FIRST，忽略
	return fmt.Sprintf("ADD COLUMN %s", colDef)
}

func (a *DropColumnAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	return fmt.Sprintf("DROP COLUMN \"%s\"", a.ColumnName)
}

func (a *ModifyColumnAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	colType := c.generateDataType(a.Column.DataType)
	var parts []string
	parts = append(parts, fmt.Sprintf("ALTER COLUMN \"%s\" TYPE %s", a.Column.Name, colType))

	// 添加 NOT NULL 约束（如果需要）
	if a.Column.NotNull {
		parts = append(parts, fmt.Sprintf("ALTER COLUMN \"%s\" SET NOT NULL", a.Column.Name))
	} else {
		parts = append(parts, fmt.Sprintf("ALTER COLUMN \"%s\" DROP NOT NULL", a.Column.Name))
	}

	// 添加 DEFAULT 值（如果有）
	if a.Column.Default != "" {
		parts = append(parts, fmt.Sprintf("ALTER COLUMN \"%s\" SET DEFAULT %s", a.Column.Name, a.Column.Default))
	} else {
		parts = append(parts, fmt.Sprintf("ALTER COLUMN \"%s\" DROP DEFAULT", a.Column.Name))
	}

	return strings.Join(parts, ", ")
}

func (a *ChangeColumnAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	colType := c.generateDataType(a.Column.DataType)
	if strings.EqualFold(a.OldName, a.NewName) {
		return fmt.Sprintf("ALTER COLUMN \"%s\" TYPE %s", a.OldName, colType)
	}
	// 需要拆分为两个语句
	return fmt.Sprintf("ALTER COLUMN \"%s\" TYPE %s; ALTER TABLE \"%s\" RENAME COLUMN \"%s\" TO \"%s\"",
		a.OldName, colType, tableName, a.OldName, a.NewName)
}

func (a *DropIndexAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	// PostgreSQL/GaussDB 中 DROP INDEX 是独立语句，不是 ALTER TABLE 的一部分
	// 索引名可能需要包含表名前缀
	indexName := a.IndexName
	if !strings.Contains(strings.ToLower(indexName), strings.ToLower(tableName)) {
		indexName = c.makeIndexName(indexName, tableName)
	}
	return fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", indexName)
}

func (a *AddIndexAction) ToPostgresSQL(c *MySQLToPostgresConverter, tableName string) string {
	indexName := c.makeIndexName(a.IndexName, tableName)
	// 清理列名，移除反引号和引号，确保格式正确
	var cleanColumns []string
	for _, col := range a.Columns {
		col = strings.TrimSpace(col)
		col = strings.Trim(col, "`\"")
		// 移除列名后的长度说明，如 column_name(255)
		col = regexp.MustCompile(`\(\d+\)`).ReplaceAllString(col, "")
		cleanColumns = append(cleanColumns, fmt.Sprintf("\"%s\"", col))
	}
	columns := strings.Join(cleanColumns, ", ")
	uniqueKeyword := ""
	if a.IsUnique {
		uniqueKeyword = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX \"%s\" ON \"%s\" (%s)", uniqueKeyword, indexName, tableName, columns)
}
