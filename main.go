// MySQL to PostgreSQL-Compatible SQL Converter & GaussDB/Teledb Tool
// 功能：
// 1. MySQL SQL 转换为 PostgreSQL 兼容 SQL（适用于 GaussDB、Teledb）
// 2. 执行 GaussDB / Teledb SQL 语句
// 3. 执行 MySQL SQL 语句
// 4. 静态语法验证（MySQL 和 PostgreSQL-compatible SQL）
// 5. MySQL 数据迁移到 GaussDB / Teledb
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "gitee.com/opengauss/openGauss-connector-go-pq"
	_ "github.com/go-sql-driver/mysql"

	"github.com/mashuiping/sqlkit/internal/config"
	"github.com/mashuiping/sqlkit/internal/converter"
	"github.com/mashuiping/sqlkit/internal/executor"
	"github.com/mashuiping/sqlkit/internal/validator"
)

// 命令行参数
var (
	// 输入输出
	inputFile  = flag.String("f", "", "输入的 SQL 文件路径")
	outputFile = flag.String("o", "", "输出的 SQL 文件路径（不指定则输出到终端）")
	sqlQuery   = flag.String("sql", "", "直接指定要处理的 SQL 语句")

	// 操作模式
	modeConvert             = flag.Bool("convert", false, "转换 MySQL SQL 为 PostgreSQL 兼容 SQL（默认模式，适用于 GaussDB、Teledb）")
	modeExecMySQL           = flag.Bool("exec-mysql", false, "在 MySQL 数据库执行 SQL")
	modeExecGaussDB         = flag.Bool("exec-gaussdb", false, "在 GaussDB 数据库执行 SQL")
	modeExecTeledb          = flag.Bool("exec-teledb", false, "在 Teledb 数据库执行 SQL（使用 opengauss 驱动）")
	modeValidMySQL          = flag.Bool("validate-mysql", false, "验证 MySQL SQL 语法（静态检查，无需数据库连接）")
	modeValidPostgres       = flag.Bool("validate-postgres", false, "验证 PostgreSQL 兼容 SQL 语法（静态检查）")
	modeValidGaussDB        = flag.Bool("validate-gaussdb", false, "验证 PostgreSQL 兼容 SQL 语法（旧别名）")
	modeValidTeledb         = flag.Bool("validate-teledb", false, "验证 PostgreSQL 兼容 SQL 语法（适用于 Teledb，等价于 -validate-postgres）")
	modeValidPostgresOnline = flag.Bool("validate-postgres-online", false, "在线验证 PostgreSQL SQL（使用数据库事务，需要 PostgreSQL 连接）")
	modeValidGaussDBOnline  = flag.Bool("validate-gaussdb-online", false, "在线验证 GaussDB SQL（使用数据库事务，需要数据库连接）")
	modeValidTeledbOnline   = flag.Bool("validate-teledb-online", false, "在线验证 Teledb SQL（使用 opengauss 驱动，需要 Teledb 连接）")
	modeMigrateData         = flag.Bool("migrate-data", false, "将 MySQL 数据迁移到目标数据库（默认 GaussDB，可通过 -migrate-target 选择 teledb）")

	// 数据迁移配置
	migrateTable     = flag.String("table", "", "要迁移的表名（多个表用逗号分隔）")
	migrateBatchSize = flag.Int("batch-size", 1000, "批量插入的行数")
	migrateMaxRows   = flag.Int("max-rows", 0, "最大迁移行数（0 表示不限制）")
	migrateTruncate  = flag.Bool("truncate", true, "迁移前清空目标表")
	migrateTarget    = flag.String("migrate-target", "gaussdb", "数据迁移目标数据库：gaussdb 或 teledb（默认 gaussdb）")

	// 转换器配置
	useTimestampTZ = flag.Bool("timestamptz", true, "使用 TIMESTAMPTZ 而非 TIMESTAMP")
	useJSONB       = flag.Bool("jsonb", true, "使用 JSONB 而非 JSON")

	// 查询结果显示配置
	maxColumnWidth = flag.Int("max-column-width", 1000, "查询结果列的最大显示宽度（默认: 1000）")

	// MySQL 数据库配置
	mysqlDSN      = flag.String("mysql-dsn", "", "MySQL DSN 连接字符串（或设置 MYSQL_DSN 环境变量）")
	mysqlHost     = flag.String("mysql-host", "", "MySQL 主机地址（或设置 MYSQL_HOST 环境变量）")
	mysqlPort     = flag.Int("mysql-port", 0, "MySQL 端口（或设置 MYSQL_PORT 环境变量）")
	mysqlUser     = flag.String("mysql-user", "", "MySQL 用户名（或设置 MYSQL_USER 环境变量）")
	mysqlPassword = flag.String("mysql-password", "", "MySQL 密码（或设置 MYSQL_PASSWORD 环境变量）")
	mysqlDBName   = flag.String("mysql-dbname", "", "MySQL 数据库名（或设置 MYSQL_DBNAME 环境变量）")

	// GaussDB 数据库配置
	gaussdbDSN      = flag.String("gaussdb-dsn", "", "GaussDB DSN 连接字符串（或设置 GAUSSDB_DSN 环境变量）")
	gaussdbHost     = flag.String("gaussdb-host", "", "GaussDB 主机地址（或设置 GAUSSDB_HOST 环境变量）")
	gaussdbPort     = flag.Int("gaussdb-port", 0, "GaussDB 端口（或设置 GAUSSDB_PORT 环境变量）")
	gaussdbUser     = flag.String("gaussdb-user", "", "GaussDB 用户名（或设置 GAUSSDB_USER 环境变量）")
	gaussdbPassword = flag.String("gaussdb-password", "", "GaussDB 密码（或设置 GAUSSDB_PASSWORD 环境变量）")
	gaussdbDBName   = flag.String("gaussdb-dbname", "", "GaussDB 数据库名（或设置 GAUSSDB_DBNAME 环境变量）")
	gaussdbSSLMode  = flag.String("gaussdb-sslmode", "disable", "GaussDB SSL 模式（或设置 GAUSSDB_SSLMODE 环境变量）")

	// PostgreSQL 数据库配置（仅用于在线验证）
	postgresDSN      = flag.String("postgres-dsn", "", "PostgreSQL DSN 连接字符串（或设置 POSTGRES_DSN 环境变量）")
	postgresHost     = flag.String("postgres-host", "", "PostgreSQL 主机地址（或设置 POSTGRES_HOST 环境变量）")
	postgresPort     = flag.Int("postgres-port", 0, "PostgreSQL 端口（或设置 POSTGRES_PORT 环境变量）")
	postgresUser     = flag.String("postgres-user", "", "PostgreSQL 用户名（或设置 POSTGRES_USER 环境变量）")
	postgresPassword = flag.String("postgres-password", "", "PostgreSQL 密码（或设置 POSTGRES_PASSWORD 环境变量）")
	postgresDBName   = flag.String("postgres-dbname", "", "PostgreSQL 数据库名（或设置 POSTGRES_DBNAME 环境变量）")
	postgresSSLMode  = flag.String("postgres-sslmode", "disable", "PostgreSQL SSL 模式（或设置 POSTGRES_SSLMODE 环境变量）")

	// Teledb 数据库配置（仅用于在线验证）
	teledbDSN      = flag.String("teledb-dsn", "", "Teledb DSN 连接字符串（或设置 TELEDB_DSN 环境变量）")
	teledbHost     = flag.String("teledb-host", "", "Teledb 主机地址（或设置 TELEDB_HOST 环境变量）")
	teledbPort     = flag.Int("teledb-port", 0, "Teledb 端口（或设置 TELEDB_PORT 环境变量）")
	teledbUser     = flag.String("teledb-user", "", "Teledb 用户名（或设置 TELEDB_USER 环境变量）")
	teledbPassword = flag.String("teledb-password", "", "Teledb 密码（或设置 TELEDB_PASSWORD 环境变量）")
	teledbDBName   = flag.String("teledb-dbname", "", "Teledb 数据库名（或设置 TELEDB_DBNAME 环境变量）")
	teledbSSLMode  = flag.String("teledb-sslmode", "disable", "Teledb SSL 模式（或设置 TELEDB_SSLMODE 环境变量）")
)

func main() {
	flag.Usage = printUsage
	flag.Parse()

	// 确定操作模式
	switch {
	case *modeMigrateData:
		migrateData()
	case *modeExecMySQL:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		execMySQL(sqlContent)
	case *modeExecGaussDB:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		execGaussDB(sqlContent)
	case *modeExecTeledb:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		execTeledb(sqlContent)
	case *modeValidMySQL:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validateMySQL(sqlContent)
	case *modeValidPostgres, *modeValidGaussDB:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validateGaussDB(sqlContent)
	case *modeValidTeledb:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validateTeledb(sqlContent)
	case *modeValidPostgresOnline:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validatePostgresOnline(sqlContent)
	case *modeValidGaussDBOnline:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validateGaussDBOnline(sqlContent)
	case *modeValidTeledbOnline:
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		validateTeledbOnline(sqlContent)
	default:
		// 默认转换模式
		sqlContent, err := getSQLContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		convertSQL(sqlContent)
	}
}

// getSQLContent 获取 SQL 内容
func getSQLContent() (string, error) {
	// 优先使用 -sql 参数
	if *sqlQuery != "" {
		return *sqlQuery, nil
	}

	// 从文件读取
	if *inputFile != "" {
		content, err := os.ReadFile(*inputFile)
		if err != nil {
			return "", fmt.Errorf("读取文件失败: %w", err)
		}
		return string(content), nil
	}

	// 从命令行参数读取
	if flag.NArg() > 0 {
		return strings.Join(flag.Args(), " "), nil
	}

	return "", fmt.Errorf("请指定 SQL 内容：使用 -f <文件> 或 -sql <SQL语句>")
}

// convertSQL 转换 SQL
func convertSQL(mysqlSQL string) {
	if strings.TrimSpace(mysqlSQL) == "" {
		fmt.Fprintf(os.Stderr, "错误: SQL 语句为空\n")
		os.Exit(1)
	}

	conv := &converter.MySQLToPostgresConverter{
		UseTimestampTZ: *useTimestampTZ,
		UseJSONB:       *useJSONB,
	}

	postgresSQL, err := conv.ConvertSQL(mysqlSQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 转换失败: %v\n", err)
		os.Exit(1)
	}

	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(postgresSQL), 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 错误: 写入文件失败 - %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ 转换成功，结果已保存到: %s\n", *outputFile)
	} else {
		if *inputFile != "" {
			fmt.Printf("MySQL SQL (from %s):\n", *inputFile)
			fmt.Println(strings.Repeat("=", 60))
			if len(mysqlSQL) > 200 {
				fmt.Println(mysqlSQL[:200] + "...")
			} else {
				fmt.Println(mysqlSQL)
			}
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println()
		}

		fmt.Println("PostgreSQL-compatible SQL (适用于 GaussDB、Teledb):")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println(postgresSQL)
		fmt.Println(strings.Repeat("=", 60))
	}
}

// execMySQL 在 MySQL 执行 SQL
func execMySQL(sql string) {
	dsn := getMySQLDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 MySQL 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -mysql-dsn 或 -mysql-host/-mysql-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: MYSQL_DSN, MYSQL_HOST, MYSQL_USER 等\n")
		os.Exit(1)
	}

	exec := executor.NewMySQLExecutor(dsn)
	if err := exec.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 MySQL 失败: %v\n", err)
		os.Exit(1)
	}
	defer exec.Close()

	fmt.Println("✓ 成功连接到 MySQL")

	// 分割 SQL 语句
	statements := executor.SplitStatements(sql)
	if len(statements) == 0 {
		fmt.Fprintf(os.Stderr, "错误: 没有找到有效的 SQL 语句\n")
		os.Exit(1)
	}

	// 逐条执行 SQL 语句
	successCount := 0
	failCount := 0
	for i, stmt := range statements {
		// 跳过注释
		trimmed := strings.TrimSpace(stmt)
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if len(statements) > 1 {
			fmt.Printf("\n执行语句 %d/%d:\n", i+1, len(statements))
		}

		result, err := exec.Execute(stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 执行失败: %v\n", err)
			failCount++
			// 继续执行下一条语句，但最后会返回错误退出码
			continue
		}

		printExecuteResult(result)
		successCount++
	}

	// 如果有失败的语句，返回非零退出码
	if failCount > 0 {
		fmt.Fprintf(os.Stderr, "\n执行完成: 成功 %d 条, 失败 %d 条\n", successCount, failCount)
		os.Exit(1)
	}

	if len(statements) > 1 {
		fmt.Printf("\n✓ 所有语句执行成功 (共 %d 条)\n", successCount)
	}
}

// execGaussDB 在 GaussDB 执行 SQL
func execGaussDB(sql string) {
	dsn := getGaussDBDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 GaussDB 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -gaussdb-dsn 或 -gaussdb-host/-gaussdb-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: GAUSSDB_DSN, GAUSSDB_HOST, GAUSSDB_USER 等\n")
		os.Exit(1)
	}

	exec := executor.NewGaussDBExecutor(dsn)
	if err := exec.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 GaussDB 失败: %v\n", err)
		os.Exit(1)
	}
	defer exec.Close()

	fmt.Println("✓ 成功连接到 GaussDB")

	// 分割 SQL 语句
	statements := executor.SplitStatements(sql)
	if len(statements) == 0 {
		fmt.Fprintf(os.Stderr, "错误: 没有找到有效的 SQL 语句\n")
		os.Exit(1)
	}

	// 逐条执行 SQL 语句
	successCount := 0
	failCount := 0
	for i, stmt := range statements {
		// 跳过注释
		trimmed := strings.TrimSpace(stmt)
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if len(statements) > 1 {
			fmt.Printf("\n执行语句 %d/%d:\n", i+1, len(statements))
		}

		result, err := exec.Execute(stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 执行失败: %v\n", err)
			failCount++
			// 继续执行下一条语句，但最后会返回错误退出码
			continue
		}

		printExecuteResult(result)
		successCount++
	}

	// 如果有失败的语句，返回非零退出码
	if failCount > 0 {
		fmt.Fprintf(os.Stderr, "\n执行完成: 成功 %d 条, 失败 %d 条\n", successCount, failCount)
		os.Exit(1)
	}

	if len(statements) > 1 {
		fmt.Printf("\n✓ 所有语句执行成功 (共 %d 条)\n", successCount)
	}
}

// validateMySQL 验证 MySQL 语法
func validateMySQL(sql string) {
	v := validator.NewMySQLValidator()
	result := v.ValidateText(sql)

	validator.PrintResult(result, "")

	if !result.Valid {
		os.Exit(1)
	}
}

// execTeledb 在 Teledb 执行 SQL（使用 opengauss 驱动）。
func execTeledb(sql string) {
	dsn := getTeledbDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 Teledb 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -teledb-dsn 或 -teledb-host/-teledb-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: TELEDB_DSN, TELEDB_HOST, TELEDB_USER 等\n")
		os.Exit(1)
	}

	exec := executor.NewTeledbExecutor(dsn)
	if err := exec.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 Teledb 失败: %v\n", err)
		os.Exit(1)
	}
	defer exec.Close()

	fmt.Println("✓ 成功连接到 Teledb")

	statements := executor.SplitStatements(sql)
	if len(statements) == 0 {
		fmt.Fprintf(os.Stderr, "错误: 没有找到有效的 SQL 语句\n")
		os.Exit(1)
	}

	successCount := 0
	failCount := 0
	for i, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if len(statements) > 1 {
			fmt.Printf("\n执行语句 %d/%d:\n", i+1, len(statements))
		}

		result, err := exec.Execute(stmt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 执行失败: %v\n", err)
			failCount++
			continue
		}

		printExecuteResult(result)
		successCount++
	}

	if failCount > 0 {
		fmt.Fprintf(os.Stderr, "\n执行完成: 成功 %d 条, 失败 %d 条\n", successCount, failCount)
		os.Exit(1)
	}

	if len(statements) > 1 {
		fmt.Printf("\n✓ 所有语句执行成功 (共 %d 条)\n", successCount)
	}
}

// validateGaussDB 验证 PostgreSQL 兼容 SQL 语法。
// -validate-gaussdb 是历史命令名；-validate-postgres 为推荐名称。
func validateGaussDB(sql string) {
	v := validator.NewGaussDBValidator()
	result := v.ValidateText(sql)

	validator.PrintResult(result, "")

	if !result.Valid {
		os.Exit(1)
	}
}

// validateTeledb 验证 PostgreSQL 兼容 SQL 语法（适用于 Teledb）。
// Teledb 与 GaussDB 同源（基于 openGauss/PostgreSQL 方言），
// 静态语法检查结果与 -validate-postgres / -validate-gaussdb 等价。
func validateTeledb(sql string) {
	v := validator.NewTeledbValidator()
	result := v.ValidateText(sql)

	validator.PrintResult(result, "")

	if !result.Valid {
		os.Exit(1)
	}
}

// validateGaussDBOnline 在线验证 GaussDB SQL（使用数据库事务）
func validateGaussDBOnline(sql string) {
	dsn := getGaussDBDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 GaussDB 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -gaussdb-dsn 或 -gaussdb-host/-gaussdb-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: GAUSSDB_DSN, GAUSSDB_HOST, GAUSSDB_USER 等\n")
		os.Exit(1)
	}

	v := validator.NewGaussDBOnlineValidator(dsn)
	if err := v.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 GaussDB 失败: %v\n", err)
		os.Exit(1)
	}
	defer v.Close()

	fmt.Println("✓ 成功连接到 GaussDB（在线验证模式）")

	result := v.ValidateText(sql)
	validator.PrintResult(result, "")

	if !result.Valid {
		os.Exit(1)
	}
}

// validatePostgresOnline 在线验证 PostgreSQL SQL。
func validatePostgresOnline(sql string) {
	dsn := getPostgresDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 PostgreSQL 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -postgres-dsn 或 -postgres-host/-postgres-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: POSTGRES_DSN, POSTGRES_HOST, POSTGRES_USER 等\n")
		os.Exit(1)
	}

	v := validator.NewPostgreSQLOnlineValidator(dsn)
	if err := v.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 PostgreSQL 失败: %v\n", err)
		os.Exit(1)
	}
	defer v.Close()

	fmt.Println("✓ 成功连接到 PostgreSQL（在线验证模式）")
	result := v.ValidateText(sql)
	validator.PrintResult(result, "")
	if !result.Valid {
		os.Exit(1)
	}
}

// validateTeledbOnline 在线验证 Teledb SQL。
func validateTeledbOnline(sql string) {
	dsn := getTeledbDSN()
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 Teledb 连接信息\n")
		fmt.Fprintf(os.Stderr, "使用 -teledb-dsn 或 -teledb-host/-teledb-user 等参数\n")
		fmt.Fprintf(os.Stderr, "或设置环境变量: TELEDB_DSN, TELEDB_HOST, TELEDB_USER 等\n")
		os.Exit(1)
	}

	v := validator.NewTeledbOnlineValidator(dsn)
	if err := v.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 Teledb 失败: %v\n", err)
		os.Exit(1)
	}
	defer v.Close()

	fmt.Println("✓ 成功连接到 Teledb（在线验证模式，opengauss 驱动）")
	result := v.ValidateText(sql)
	validator.PrintResult(result, "")
	if !result.Valid {
		os.Exit(1)
	}
}

// ============ 数据迁移功能 ============

// migrateData 执行数据迁移。
// 目标数据库通过 -migrate-target 选择（gaussdb 或 teledb），默认 gaussdb。
func migrateData() {
	if *migrateTable == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须指定 -table 参数\n")
		os.Exit(1)
	}

	// 解析并校验迁移目标。
	target := strings.ToLower(strings.TrimSpace(*migrateTarget))
	var targetLabel, targetEnvDoc string
	switch target {
	case "", "gaussdb":
		target = "gaussdb"
		targetLabel = "GaussDB"
		targetEnvDoc = "GAUSSDB_DSN, GAUSSDB_HOST, GAUSSDB_USER 等"
	case "teledb":
		targetLabel = "Teledb"
		targetEnvDoc = "TELEDB_DSN, TELEDB_HOST, TELEDB_USER 等"
	default:
		fmt.Fprintf(os.Stderr, "错误: 不支持的 -migrate-target=%q，仅支持 gaussdb 或 teledb\n", *migrateTarget)
		os.Exit(1)
	}

	mysqlDsnStr := getMySQLDSN()
	if mysqlDsnStr == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 MySQL 连接信息\n")
		os.Exit(1)
	}

	targetDsnStr, err := getMigrateTargetDSN(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	if targetDsnStr == "" {
		fmt.Fprintf(os.Stderr, "错误: 请提供 %s 连接信息\n", targetLabel)
		fmt.Fprintf(os.Stderr, "使用 -%s-dsn 或 -%s-host/-%s-user 等参数\n", target, target, target)
		fmt.Fprintf(os.Stderr, "或设置环境变量: %s\n", targetEnvDoc)
		os.Exit(1)
	}

	// 连接 MySQL
	mysqlDB, err := sql.Open("mysql", mysqlDsnStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 MySQL 失败: %v\n", err)
		os.Exit(1)
	}
	defer mysqlDB.Close()

	if err := mysqlDB.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ MySQL 连接测试失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 成功连接到 MySQL")

	// 连接目标数据库（GaussDB / Teledb 共用 opengauss 驱动协议）
	targetDB, err := sql.Open("opengauss", targetDsnStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接 %s 失败: %v\n", targetLabel, err)
		os.Exit(1)
	}
	defer targetDB.Close()

	if err := targetDB.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s 连接测试失败: %v\n", targetLabel, err)
		os.Exit(1)
	}
	fmt.Printf("✓ 成功连接到 %s（opengauss 驱动）\n", targetLabel)

	// 解析表名列表
	tableList := strings.Split(*migrateTable, ",")
	for i := range tableList {
		tableList[i] = strings.TrimSpace(tableList[i])
	}

	// 迁移每个表的数据
	successCount := 0
	failCount := 0
	for _, table := range tableList {
		if table == "" {
			continue
		}
		if err := migrateTableData(mysqlDB, targetDB, table); err != nil {
			fmt.Fprintf(os.Stderr, "❌ 迁移表 %s 失败: %v\n", table, err)
			failCount++
			continue
		}
		fmt.Printf("✓ 表 %s 迁移完成\n", table)
		successCount++
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("迁移完成: 成功 %d 个表, 失败 %d 个表 (目标: %s)\n", successCount, failCount, targetLabel)

	if failCount > 0 {
		os.Exit(1)
	}
}

// getMigrateTargetDSN 根据目标类型返回 DSN 字符串。
func getMigrateTargetDSN(target string) (string, error) {
	switch target {
	case "gaussdb":
		return getGaussDBDSN(), nil
	case "teledb":
		return getTeledbDSN(), nil
	default:
		return "", fmt.Errorf("不支持的迁移目标: %s", target)
	}
}

// migrateTableData 迁移单个表的数据
func migrateTableData(mysqlDB, targetDB *sql.DB, tableName string) error {
	fmt.Printf("开始迁移表: %s\n", tableName)
	startTime := time.Now()

	// 清空目标表的数据
	if *migrateTruncate {
		if err := truncateTable(targetDB, tableName); err != nil {
			return fmt.Errorf("清空表失败: %w", err)
		}
		fmt.Printf("  表 %s 已清空\n", tableName)
	}

	// 构建查询语句
	query := fmt.Sprintf("SELECT * FROM `%s`", tableName)
	if *migrateMaxRows > 0 {
		query += fmt.Sprintf(" LIMIT %d", *migrateMaxRows)
	}

	rows, err := mysqlDB.Query(query)
	if err != nil {
		return fmt.Errorf("查询 MySQL 数据失败: %w", err)
	}
	defer rows.Close()

	// 获取列名
	columnNames, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("获取列名失败: %w", err)
	}

	if len(columnNames) == 0 {
		return fmt.Errorf("表 %s 没有列", tableName)
	}

	// 准备批量插入
	batchSize := *migrateBatchSize
	batch := make([][]interface{}, 0, batchSize)
	totalRows := 0

	// 读取数据
	for rows.Next() {
		// 创建值切片
		values := make([]interface{}, len(columnNames))
		valuePtrs := make([]interface{}, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// 扫描行数据
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("扫描行数据失败: %w", err)
		}

		// 转换值类型
		convertedValues := make([]interface{}, len(values))
		for i, val := range values {
			if val == nil {
				convertedValues[i] = nil
			} else {
				switch v := val.(type) {
				case time.Time:
					convertedValues[i] = v
				case []byte:
					convertedValues[i] = string(v)
				default:
					convertedValues[i] = val
				}
			}
		}

		batch = append(batch, convertedValues)
		totalRows++

		// 达到批次大小时执行插入
		if len(batch) >= batchSize {
			if err := insertBatch(targetDB, tableName, columnNames, batch); err != nil {
				return fmt.Errorf("批量插入失败: %w", err)
			}
			fmt.Printf("  表 %s: 已插入 %d 行\n", tableName, totalRows)
			batch = batch[:0]
		}
	}

	// 插入剩余数据
	if len(batch) > 0 {
		if err := insertBatch(targetDB, tableName, columnNames, batch); err != nil {
			return fmt.Errorf("批量插入剩余数据失败: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取行数据时出错: %w", err)
	}

	// 更新自增序列
	if err := updateSequence(targetDB, tableName); err != nil {
		fmt.Printf("  警告: 更新表 %s 的序列失败: %v\n", tableName, err)
	}

	duration := time.Since(startTime)
	fmt.Printf("  表 %s 迁移完成，共 %d 行，耗时 %v\n", tableName, totalRows, duration)
	return nil
}

// truncateTable 清空目标表的数据
func truncateTable(db *sql.DB, tableName string) error {
	query := fmt.Sprintf(`TRUNCATE TABLE %s CASCADE`, tableName)
	_, err := db.Exec(query)
	return err
}

// insertBatch 批量插入数据到 GaussDB
func insertBatch(db *sql.DB, tableName string, columnNames []string, batch [][]interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 构建 INSERT 语句
	quotedColumns := make([]string, len(columnNames))
	for i, col := range columnNames {
		quotedColumns[i] = fmt.Sprintf(`"%s"`, col)
	}

	// 为每行构建 VALUES 子句
	valueStrings := make([]string, len(batch))
	args := make([]interface{}, 0, len(batch)*len(columnNames))

	for i, row := range batch {
		placeholders := make([]string, len(columnNames))
		for j := range columnNames {
			placeholders[j] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, row[j])
		}
		valueStrings[i] = "(" + strings.Join(placeholders, ",") + ")"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		tableName,
		strings.Join(quotedColumns, ","),
		strings.Join(valueStrings, ","),
	)

	// 执行插入
	_, err = tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("执行插入失败: %w", err)
	}

	// 提交事务
	return tx.Commit()
}

// updateSequence 更新 GaussDB 表的自增序列值
func updateSequence(db *sql.DB, tableName string) error {
	const columnName = "id"

	// 查找对应的序列名
	sequenceName, err := findSequenceName(db, tableName, columnName)
	if err != nil {
		return fmt.Errorf("查找序列名失败: %w", err)
	}

	if sequenceName == "" {
		return nil // 没有序列，跳过
	}

	// 查询表中最大的 ID 值
	var maxID interface{}
	query := fmt.Sprintf(`SELECT MAX("%s") FROM %s`, columnName, tableName)
	err = db.QueryRow(query).Scan(&maxID)
	if err != nil {
		return fmt.Errorf("查询最大 ID 失败: %w", err)
	}

	// 如果表为空或最大值为 NULL，设置为 0
	var maxValue int64 = 0
	if maxID != nil {
		switch v := maxID.(type) {
		case int64:
			maxValue = v
		case int32:
			maxValue = int64(v)
		case int:
			maxValue = int64(v)
		case float64:
			maxValue = int64(v)
		}
	}

	// 设置序列的当前值
	setValQuery := fmt.Sprintf(`SELECT setval('%s', $1, true)`, sequenceName)
	_, err = db.Exec(setValQuery, maxValue)
	if err != nil {
		return fmt.Errorf("设置序列值失败: %w", err)
	}

	fmt.Printf("  表 %s: 序列 %s 已更新为 %d\n", tableName, sequenceName, maxValue)
	return nil
}

// findSequenceName 查找列对应的序列名
func findSequenceName(db *sql.DB, tableName, columnName string) (string, error) {
	// 查询列的默认值
	query := `
		SELECT pg_get_expr(d.adbin, d.adrelid) as default_value
		FROM pg_attrdef d
		JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = $1
		AND a.attname = $2
	`

	var defaultValue string
	err := db.QueryRow(query, tableName, columnName).Scan(&defaultValue)
	if err != nil {
		// 尝试使用标准命名规则
		sequenceName := fmt.Sprintf("%s_%s_seq", tableName, columnName)
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1 AND relkind = 'S')`
		err = db.QueryRow(checkQuery, sequenceName).Scan(&exists)
		if err != nil || !exists {
			return "", nil
		}
		return sequenceName, nil
	}

	// 从默认值中提取序列名
	if strings.Contains(defaultValue, "nextval") {
		start := strings.Index(defaultValue, "'")
		if start != -1 {
			end := strings.Index(defaultValue[start+1:], "'")
			if end != -1 {
				return defaultValue[start+1 : start+1+end], nil
			}
		}
	}

	return "", nil
}

// ============ DSN 获取函数 ============

// getMySQLDSN 获取 MySQL DSN
func getMySQLDSN() string {
	// 优先使用命令行参数
	if *mysqlDSN != "" {
		return *mysqlDSN
	}

	// 尝试从环境变量获取完整 DSN
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		return dsn
	}

	// 从各个参数/环境变量组装 DSN
	cfg := config.NewMySQLConfig()
	cfg.LoadFromEnv("MYSQL")

	// 命令行参数覆盖环境变量
	if *mysqlHost != "" {
		cfg.Host = *mysqlHost
	}
	if *mysqlPort > 0 {
		cfg.Port = *mysqlPort
	}
	if *mysqlUser != "" {
		cfg.User = *mysqlUser
	}
	if *mysqlPassword != "" {
		cfg.Password = *mysqlPassword
	}
	if *mysqlDBName != "" {
		cfg.DBName = *mysqlDBName
	}

	// 验证必要参数
	if err := cfg.Validate(); err != nil {
		return ""
	}

	return cfg.MySQLDSN()
}

// getGaussDBDSN 获取 GaussDB DSN
func getGaussDBDSN() string {
	// 优先使用命令行参数
	if *gaussdbDSN != "" {
		return *gaussdbDSN
	}

	// 尝试从环境变量获取完整 DSN
	if dsn := os.Getenv("GAUSSDB_DSN"); dsn != "" {
		return dsn
	}

	// 从各个参数/环境变量组装 DSN
	cfg := config.NewGaussDBConfig()
	cfg.LoadFromEnv("GAUSSDB")

	// 命令行参数覆盖环境变量
	if *gaussdbHost != "" {
		cfg.Host = *gaussdbHost
	}
	if *gaussdbPort > 0 {
		cfg.Port = *gaussdbPort
	}
	if *gaussdbUser != "" {
		cfg.User = *gaussdbUser
	}
	if *gaussdbPassword != "" {
		cfg.Password = *gaussdbPassword
	}
	if *gaussdbDBName != "" {
		cfg.DBName = *gaussdbDBName
	}
	if *gaussdbSSLMode != "" {
		cfg.SSLMode = *gaussdbSSLMode
	}

	// 验证必要参数
	if err := cfg.Validate(); err != nil {
		return ""
	}

	return cfg.GaussDBDSN()
}

// getPostgresDSN 获取 PostgreSQL DSN。
func getPostgresDSN() string {
	if *postgresDSN != "" {
		return *postgresDSN
	}
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}

	cfg := config.NewPostgresConfig()
	cfg.LoadFromEnv("POSTGRES")
	if *postgresHost != "" {
		cfg.Host = *postgresHost
	}
	if *postgresPort > 0 {
		cfg.Port = *postgresPort
	}
	if *postgresUser != "" {
		cfg.User = *postgresUser
	}
	if *postgresPassword != "" {
		cfg.Password = *postgresPassword
	}
	if *postgresDBName != "" {
		cfg.DBName = *postgresDBName
	}
	if *postgresSSLMode != "" {
		cfg.SSLMode = *postgresSSLMode
	}
	if cfg.User == "" || cfg.DBName == "" {
		return ""
	}

	return cfg.PostgresDSN()
}

// getTeledbDSN 获取 Teledb DSN。
func getTeledbDSN() string {
	if *teledbDSN != "" {
		return *teledbDSN
	}
	if dsn := os.Getenv("TELEDB_DSN"); dsn != "" {
		return dsn
	}

	cfg := config.NewTeledbConfig()
	cfg.LoadFromEnv("TELEDB")
	if *teledbHost != "" {
		cfg.Host = *teledbHost
	}
	if *teledbPort > 0 {
		cfg.Port = *teledbPort
	}
	if *teledbUser != "" {
		cfg.User = *teledbUser
	}
	if *teledbPassword != "" {
		cfg.Password = *teledbPassword
	}
	if *teledbDBName != "" {
		cfg.DBName = *teledbDBName
	}
	if *teledbSSLMode != "" {
		cfg.SSLMode = *teledbSSLMode
	}
	if cfg.User == "" || cfg.DBName == "" {
		return ""
	}

	return cfg.TeledbDSN()
}

// printExecuteResult 打印执行结果
func printExecuteResult(result *executor.ExecuteResult) {
	fmt.Printf("执行耗时: %v\n", result.Duration)

	if result.IsQuery {
		executor.PrintQueryResult(result.QueryResult, *maxColumnWidth)
	} else {
		fmt.Printf("✓ 执行成功，受影响行数: %d\n", result.ExecResult.RowsAffected)
	}
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Print(`sqlkit - MySQL to PostgreSQL-Compatible SQL Converter & GaussDB/Teledb Tool

用法:
  sqlkit [操作模式] [选项]

操作模式:
  -convert                 转换 MySQL SQL 为 PostgreSQL 兼容 SQL（默认模式，适用于 GaussDB、Teledb）
  -exec-mysql              在 MySQL 数据库执行 SQL
  -exec-gaussdb            在 GaussDB 数据库执行 SQL
  -exec-teledb             在 Teledb 数据库执行 SQL（使用 opengauss 驱动）
  -validate-mysql          验证 MySQL SQL 语法（静态检查，无需数据库连接）
  -validate-postgres       验证 PostgreSQL 兼容 SQL 语法（静态检查，推荐）
  -validate-gaussdb        验证 PostgreSQL 兼容 SQL 语法（旧别名）
  -validate-teledb         验证 PostgreSQL 兼容 SQL 语法（适用于 Teledb，等价于 -validate-postgres）
  -validate-postgres-online 在线验证 PostgreSQL SQL（使用 PostgreSQL 连接）
  -validate-gaussdb-online 在线验证 GaussDB SQL（使用数据库事务，需要数据库连接）
  -validate-teledb-online   在线验证 Teledb SQL（使用 opengauss 驱动，需要 Teledb 连接）
  -migrate-data            将 MySQL 数据迁移到目标数据库（通过 -migrate-target 选择 gaussdb 或 teledb，默认 gaussdb）

输入/输出选项:
  -f <文件>          输入的 SQL 文件路径
  -o <文件>          输出的 SQL 文件路径（不指定则输出到终端）
  -sql <SQL语句>     直接指定要处理的 SQL 语句

转换选项:
  -timestamptz       使用 TIMESTAMPTZ 而非 TIMESTAMP（默认: true）
  -jsonb             使用 JSONB 而非 JSON（默认: true）

查询结果显示选项（-exec-mysql / -exec-gaussdb / -exec-teledb 模式）:
  -max-column-width <n>  查询结果列的最大显示宽度（默认: 1000）

数据迁移选项（-migrate-data 模式）:
  -table <表名>         要迁移的表名（必需，多个表用逗号分隔）
  -batch-size <n>       批量插入的行数（默认: 1000）
  -max-rows <n>         最大迁移行数（默认: 0 表示不限制）
  -truncate             迁移前清空目标表（默认: true）
  -migrate-target <db>  目标数据库：gaussdb（默认）或 teledb

MySQL 数据库连接:
  方式一: 使用 DSN（推荐，指定后无需其他连接参数）
    -mysql-dsn <dsn>   完整的 DSN 连接字符串
                       格式: user:password@tcp(host:port)/dbname?parseTime=True

  方式二: 分别指定连接参数
    -mysql-host        主机地址（默认: localhost）
    -mysql-port        端口（默认: 3306）
    -mysql-user        用户名
    -mysql-password    密码
    -mysql-dbname      数据库名

  环境变量（优先级低于命令行参数）:
    MYSQL_DSN, MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DBNAME

GaussDB 数据库连接:
  方式一: 使用 DSN（推荐，指定后无需其他连接参数）
    -gaussdb-dsn <dsn> 完整的 DSN 连接字符串
                       格式: host=xxx port=xxx user=xxx password=xxx dbname=xxx sslmode=xxx

  方式二: 分别指定连接参数
    -gaussdb-host      主机地址（默认: localhost）
    -gaussdb-port      端口（默认: 5432）
    -gaussdb-user      用户名
    -gaussdb-password  密码
    -gaussdb-dbname    数据库名
    -gaussdb-sslmode   SSL 模式（默认: disable）

  环境变量（优先级低于命令行参数）:
    GAUSSDB_DSN, GAUSSDB_HOST, GAUSSDB_PORT, GAUSSDB_USER, GAUSSDB_PASSWORD, 
    GAUSSDB_DBNAME, GAUSSDB_SSLMODE

PostgreSQL 数据库连接（仅用于 -validate-postgres-online）:
  方式一: 使用 DSN（推荐，指定后无需其他连接参数）
    -postgres-dsn <dsn> 完整的 DSN 连接字符串
                       格式: host=xxx port=xxx user=xxx password=xxx dbname=xxx sslmode=xxx

  方式二: 分别指定连接参数
    -postgres-host      主机地址（默认: localhost）
    -postgres-port      端口（默认: 5432）
    -postgres-user      用户名
    -postgres-password  密码
    -postgres-dbname    数据库名
    -postgres-sslmode   SSL 模式（默认: disable）

  环境变量（优先级低于命令行参数）:
    POSTGRES_DSN, POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD,
    POSTGRES_DBNAME, POSTGRES_SSLMODE

Teledb 数据库连接（用于 -validate-teledb-online / -exec-teledb / -migrate-data -migrate-target=teledb）:
  Teledb 使用 opengauss 驱动，配置与 GaussDB 独立。
  方式一: 使用 DSN（推荐，指定后无需其他连接参数）
    -teledb-dsn <dsn>    完整的 DSN 连接字符串
                         格式: host=xxx port=xxx user=xxx password=xxx dbname=xxx sslmode=xxx

  方式二: 分别指定连接参数
    -teledb-host         主机地址（默认: localhost）
    -teledb-port         端口（默认: 5432）
    -teledb-user         用户名
    -teledb-password     密码
    -teledb-dbname       数据库名
    -teledb-sslmode      SSL 模式（默认: disable）

  环境变量（优先级低于命令行参数）:
    TELEDB_DSN, TELEDB_HOST, TELEDB_PORT, TELEDB_USER, TELEDB_PASSWORD,
    TELEDB_DBNAME, TELEDB_SSLMODE

示例:
  # 转换 SQL 文件
  sqlkit -f input.sql -o output.sql

  # 转换并输出到终端
  sqlkit -f input.sql

  # 直接转换 SQL 语句
  sqlkit -sql "SELECT * FROM users LIMIT 10, 20"

  # 验证 MySQL 语法
  sqlkit -validate-mysql -f input.sql

  # 验证 PostgreSQL 兼容 SQL 语法（适用于 GaussDB、Teledb）
  sqlkit -validate-postgres -f converted.sql

  # 在线验证 GaussDB SQL（使用数据库事务）
  sqlkit -validate-gaussdb-online \
         -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb" \
         -f converted.sql

  # 在线验证 PostgreSQL SQL（使用 PostgreSQL 连接）
  sqlkit -validate-postgres-online \
         -postgres-dsn "host=localhost port=5432 user=postgres password=xxx dbname=mydb sslmode=disable" \
         -f converted.sql

  # 在线验证 Teledb SQL（使用 opengauss 驱动）
  sqlkit -validate-teledb-online \
         -teledb-dsn "host=localhost port=5432 user=teledb password=xxx dbname=mydb sslmode=disable" \
         -f converted.sql

  # 使用 DSN 执行 GaussDB SQL
  sqlkit -exec-gaussdb \
         -gaussdb-dsn "host=192.168.1.100 user=myuser password=mypass dbname=mydb" \
         -sql "SELECT * FROM users"

  # 使用分离参数执行 GaussDB SQL
  sqlkit -exec-gaussdb \
         -gaussdb-host 192.168.1.100 -gaussdb-user myuser \
         -gaussdb-password mypass -gaussdb-dbname mydb \
         -sql "SELECT * FROM users"

  # 使用 DSN 执行 Teledb SQL（opengauss 驱动）
  sqlkit -exec-teledb \
         -teledb-dsn "host=192.168.1.100 user=teledb password=xxx dbname=mydb sslmode=disable" \
         -sql "SELECT * FROM users"

  # 验证 Teledb 兼容 SQL 语法（静态）
  sqlkit -validate-teledb -f converted.sql

  # 使用 DSN 迁移数据到 GaussDB（默认）
  sqlkit -migrate-data -table "users,orders,products" \
         -mysql-dsn "root:pass@tcp(localhost:3306)/mydb?parseTime=True" \
         -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb"

  # 使用 DSN 迁移数据到 Teledb
  sqlkit -migrate-data -migrate-target teledb -table "users,orders,products" \
         -mysql-dsn "root:pass@tcp(localhost:3306)/mydb?parseTime=True" \
         -teledb-dsn "host=localhost port=5432 user=teledb password=xxx dbname=mydb sslmode=disable"

  # 使用分离参数迁移数据
  sqlkit -migrate-data -table users \
         -mysql-host mysql.example.com -mysql-user root \
         -mysql-password secret -mysql-dbname mydb \
         -gaussdb-host gaussdb.example.com -gaussdb-user admin \
         -gaussdb-password secret -gaussdb-dbname mydb

  # 使用环境变量迁移数据
  export MYSQL_DSN="root:pass@tcp(localhost:3306)/mydb?parseTime=True"
  export GAUSSDB_DSN="host=localhost user=admin password=xxx dbname=mydb"
  sqlkit -migrate-data -table "users,orders"
`)
}
