# SQLKit

<div align="center">

**MySQL 到 GaussDB SQL 转换与数据迁移工具**

<span style="display: inline-block; margin: 0 4px; padding: 2px 8px; background: #00ADD8; color: white; border-radius: 3px; font-size: 12px; font-weight: bold;">Go 1.21+</span>
<span style="display: inline-block; margin: 0 4px; padding: 2px 8px; background: #28a745; color: white; border-radius: 3px; font-size: 12px; font-weight: bold;">MIT License</span>

一个功能强大的命令行工具，用于将 MySQL SQL 转换为 GaussDB/PostgreSQL SQL，并提供数据迁移、语法验证等功能。

</div>

---

## ✨ 功能特性

- 🔄 **SQL 转换** - 将 MySQL SQL 自动转换为 GaussDB/PostgreSQL SQL
- ✅ **语法验证** - 支持 MySQL 和 GaussDB 的静态语法验证
- 🌐 **在线验证** - 使用数据库事务进行在线 SQL 验证
- 🚀 **SQL 执行** - 支持在 MySQL 和 GaussDB 数据库中执行 SQL
- 📦 **数据迁移** - 批量将 MySQL 数据迁移到 GaussDB
- 🎯 **智能转换** - 自动处理数据类型、索引、注释等转换
- 🔧 **灵活配置** - 支持命令行参数和环境变量配置

## 📋 目录

- [快速开始](#快速开始)
- [安装](#安装)
- [使用方法](#使用方法)
- [功能详解](#功能详解)
- [配置说明](#配置说明)
- [示例](#示例)
- [构建](#构建)
- [贡献](#贡献)

## 🚀 快速开始

### 转换 SQL 文件

```bash
# 转换 SQL 文件并输出到终端
sqlkit -f input.sql

# 转换并保存到文件
sqlkit -f input.sql -o output.sql

# 直接转换 SQL 语句
sqlkit -sql "SELECT * FROM users LIMIT 10, 20"
```

### 验证 SQL 语法

```bash
# 验证 MySQL 语法（无需数据库连接）
sqlkit -validate-mysql -f input.sql

# 验证 GaussDB 语法（静态检查）
sqlkit -validate-gaussdb -f converted.sql

# 在线验证 GaussDB SQL（使用数据库事务）
sqlkit -validate-gaussdb-online \
       -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb" \
       -f converted.sql
```

### 执行 SQL

```bash
# 在 MySQL 中执行 SQL
sqlkit -exec-mysql \
       -mysql-dsn "root:pass@tcp(localhost:3306)/mydb" \
       -sql "SELECT * FROM users"

# 在 GaussDB 中执行 SQL
sqlkit -exec-gaussdb \
       -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb" \
       -sql "SELECT * FROM users"
```

### 数据迁移

```bash
# 迁移单个表
sqlkit -migrate-data -table users \
       -mysql-dsn "root:pass@tcp(localhost:3306)/mydb" \
       -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb"

# 迁移多个表
sqlkit -migrate-data -table "users,orders,products" \
       -mysql-dsn "root:pass@tcp(localhost:3306)/mydb" \
       -gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb"
```

## 📦 安装

### 从源码构建

```bash
# 克隆仓库
git clone <repository-url>
cd sqlkit

# 构建（需要 Go 1.21 或更高版本）
make all

# 或直接使用 go build
go build -o sqlkit .
```

### 使用 go install 安装

```bash
# 安装最新版本
go install github.com/mashuiping/sqlkit@latest
```

**注意：** 如果您的本地 Go 版本满足要求（>= 1.21），但 Go 仍然尝试自动下载新版本，可以通过设置环境变量来控制行为：

```bash
# 只使用本地已安装的 Go 版本，不自动下载
export GOTOOLCHAIN=local

# 然后再执行安装
go install github.com/mashuiping/sqlkit@latest
```

**GOTOOLCHAIN 环境变量选项：**
- `auto`（默认）：如果本地版本不满足要求，自动下载所需版本
- `local`：只使用本地已安装的版本，不自动下载（推荐用于生产环境）
- `path`：使用 PATH 中的 go 命令
- 具体版本：如 `go1.21.0`，使用指定版本

**业界最佳实践：**
1. 在 `go.mod` 中设置合理的版本要求（通常使用当前稳定版本或稍低版本）
2. 在 CI/CD 环境中明确指定 Go 版本
3. 对于需要严格控制版本的环境，使用 `GOTOOLCHAIN=local`
4. 在项目文档中说明版本要求和工具链配置选项

### 使用预编译二进制

预编译的二进制文件位于 `build/bin/` 目录：

- `sqlkit-darwin-arm64` - macOS (Apple Silicon)
- `sqlkit-darwin-amd64` - macOS (Intel)
- `sqlkit-linux-amd64` - Linux
- `sqlkit-windows-amd64.exe` - Windows

将对应平台的二进制文件复制到系统 PATH 目录即可使用。

## 📖 使用方法

### 操作模式

SQLKit 支持以下操作模式：

| 模式 | 参数 | 说明 |
|------|------|------|
| SQL 转换 | `-convert` | 将 MySQL SQL 转换为 GaussDB SQL（默认模式） |
| MySQL 执行 | `-exec-mysql` | 在 MySQL 数据库中执行 SQL |
| GaussDB 执行 | `-exec-gaussdb` | 在 GaussDB 数据库中执行 SQL |
| MySQL 验证 | `-validate-mysql` | 静态验证 MySQL SQL 语法 |
| GaussDB 验证 | `-validate-gaussdb` | 静态验证 GaussDB SQL 语法 |
| 在线验证 | `-validate-gaussdb-online` | 使用数据库事务在线验证 |
| 数据迁移 | `-migrate-data` | 将 MySQL 数据迁移到 GaussDB |

### 输入/输出选项

```bash
-f <文件>          # 输入的 SQL 文件路径
-o <文件>          # 输出的 SQL 文件路径（不指定则输出到终端）
-sql <SQL语句>     # 直接指定要处理的 SQL 语句
```

### 转换选项

```bash
-timestamptz       # 使用 TIMESTAMPTZ 而非 TIMESTAMP（默认: true）
-jsonb             # 使用 JSONB 而非 JSON（默认: true）
```

### 数据迁移选项

```bash
-table <表名>      # 要迁移的表名（必需，多个表用逗号分隔）
-batch-size <n>    # 批量插入的行数（默认: 1000）
-max-rows <n>      # 最大迁移行数（默认: 0 表示不限制）
-truncate          # 迁移前清空目标表（默认: true）
```

## 🔧 配置说明

### MySQL 数据库连接

**方式一：使用 DSN（推荐）**

```bash
-mysql-dsn "user:password@tcp(host:port)/dbname?parseTime=True"
```

**方式二：分别指定连接参数**

```bash
-mysql-host localhost    # 主机地址（默认: localhost）
-mysql-port 3306         # 端口（默认: 3306）
-mysql-user root         # 用户名
-mysql-password secret   # 密码
-mysql-dbname mydb       # 数据库名
```

**环境变量**

```bash
export MYSQL_DSN="user:password@tcp(host:port)/dbname?parseTime=True"
# 或
export MYSQL_HOST=localhost
export MYSQL_PORT=3306
export MYSQL_USER=root
export MYSQL_PASSWORD=secret
export MYSQL_DBNAME=mydb
```

### GaussDB 数据库连接

**方式一：使用 DSN（推荐）**

```bash
-gaussdb-dsn "host=localhost port=5432 user=admin password=xxx dbname=mydb sslmode=disable"
```

**方式二：分别指定连接参数**

```bash
-gaussdb-host localhost    # 主机地址（默认: localhost）
-gaussdb-port 5432         # 端口（默认: 5432）
-gaussdb-user admin        # 用户名
-gaussdb-password secret   # 密码
-gaussdb-dbname mydb       # 数据库名
-gaussdb-sslmode disable   # SSL 模式（默认: disable）
```

**环境变量**

```bash
export GAUSSDB_DSN="host=localhost port=5432 user=admin password=xxx dbname=mydb"
# 或
export GAUSSDB_HOST=localhost
export GAUSSDB_PORT=5432
export GAUSSDB_USER=admin
export GAUSSDB_PASSWORD=secret
export GAUSSDB_DBNAME=mydb
export GAUSSDB_SSLMODE=disable
```

> **注意**：命令行参数的优先级高于环境变量。

## 💡 功能详解

### SQL 转换

SQLKit 能够智能转换以下 MySQL 特性：

- **数据类型转换**
  - `TINYINT(1)` → `BOOLEAN`
  - `DATETIME` → `TIMESTAMPTZ`
  - `TEXT` / `MEDIUMTEXT` / `LONGTEXT` → `TEXT`
  - `BLOB` → `BYTEA`
  - `ENUM` / `SET` → `VARCHAR(255)`
  - `AUTO_INCREMENT` → `SERIAL` / `BIGSERIAL`

- **语法转换**
  - `LIMIT offset, count` → `LIMIT count OFFSET offset`
  - `IFNULL()` → `COALESCE()`
  - `IF()` → `CASE WHEN ... THEN ... ELSE ... END`
  - `GROUP_CONCAT()` → `STRING_AGG()`
  - `UNIX_TIMESTAMP()` → `EXTRACT(EPOCH FROM ...)`

- **索引和约束**
  - 自动提取并转换索引定义
  - 处理唯一索引和普通索引
  - 保留索引名称或生成新名称

- **注释处理**
  - 提取表注释和列注释
  - 转换为 PostgreSQL 的 `COMMENT ON` 语句

### 语法验证

- **静态验证**：无需数据库连接，快速检查 SQL 语法
- **在线验证**：使用数据库事务进行真实验证，确保 SQL 可以执行

### 数据迁移

数据迁移功能支持：

- ✅ 批量数据迁移（可配置批次大小）
- ✅ 自动处理数据类型转换
- ✅ 自动更新序列值（AUTO_INCREMENT）
- ✅ 支持迁移前清空目标表
- ✅ 支持限制迁移行数
- ✅ 事务保证数据一致性

## 📝 示例

### 示例 1：转换 CREATE TABLE 语句

**输入（MySQL）：**

```sql
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL COMMENT '用户名',
    email VARCHAR(100) UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TINYINT(1) DEFAULT 1 COMMENT '状态'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';
```

**输出（GaussDB）：**

```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(100) UNIQUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    status BOOLEAN DEFAULT 1
);

COMMENT ON TABLE users IS '用户表';
COMMENT ON COLUMN users.username IS '用户名';
COMMENT ON COLUMN users.status IS '状态';
```

### 示例 2：转换 SELECT 语句

**输入（MySQL）：**

```sql
SELECT * FROM users 
WHERE status = 1 
ORDER BY created_at DESC 
LIMIT 10, 20;
```

**输出（GaussDB）：**

```sql
SELECT * FROM users 
WHERE status = 1 
ORDER BY created_at DESC 
LIMIT 20 OFFSET 10;
```

### 示例 3：使用环境变量

```bash
# 设置环境变量
export MYSQL_DSN="root:pass@tcp(localhost:3306)/mydb?parseTime=True"
export GAUSSDB_DSN="host=localhost port=5432 user=admin password=xxx dbname=mydb"

# 直接使用，无需指定连接参数
sqlkit -migrate-data -table users
```

## 🔨 构建

### 构建所有平台

```bash
make all
```

### 构建特定平台

```bash
make mac-arm      # macOS (Apple Silicon)
make mac-intel    # macOS (Intel)
make windows      # Windows
make linux        # Linux
```

### 清理构建产物

```bash
make clean
```

### 查看帮助

```bash
make help
```

## 🤝 贡献

欢迎贡献代码！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📄 许可证

本项目采用 MIT 许可证。详情请参阅 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- [openGauss Connector](https://gitee.com/opengauss/openGauss-connector-go-pq) - GaussDB Go 驱动
- [MySQL Driver](https://github.com/go-sql-driver/mysql) - MySQL Go 驱动
- [PostgreSQL Parser](https://github.com/auxten/postgresql-parser) - PostgreSQL SQL 解析器

---

<div align="center">

**Made with ❤️ for database migration**

如有问题或建议，欢迎提交 [Issue](../../issues) 或 [Pull Request](../../pulls)

</div>
