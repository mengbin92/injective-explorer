# 数据库初始化说明

## 概述

`init_database.sql` 是一个完整的数据库初始化脚本，包含了所有表结构和索引的定义。这个脚本可以替代 Go 代码中的 GORM AutoMigrate 功能，在 MySQL 启动时或手动执行来创建数据库结构。

## 文件说明

- **`init_database.sql`**: 完整的数据库初始化脚本
  - 包含所有表的创建语句
  - 包含所有基础索引
  - 包含所有性能优化索引

## 使用方法

### 方法 1: 手动执行（推荐用于首次初始化）

```bash
# 连接到 MySQL
mysql -u root -p

# 创建数据库（如果不存在）
CREATE DATABASE IF NOT EXISTS explorer CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

# 退出 MySQL
exit

# 执行初始化脚本
mysql -u root -p explorer < scripts/init_database.sql
```

### 方法 2: 在 MySQL 启动时自动执行

#### 2.1 使用 init-file 选项（MySQL 5.6+）

在 MySQL 配置文件 `my.cnf` 或 `my.ini` 中添加：

```ini
[mysqld]
init-file=/path/to/injective-explorer/scripts/init_database.sql
```

**注意**: 
- 确保 MySQL 用户有权限读取该文件
- 文件路径必须是绝对路径
- 脚本中的错误会导致 MySQL 启动失败，建议先手动测试

#### 2.2 使用 Docker 启动时执行

在 `docker-compose.yml` 中：

```yaml
services:
  mysql:
    image: mysql:8.0
    volumes:
      - ./scripts/init_database.sql:/docker-entrypoint-initdb.d/init.sql
    environment:
      MYSQL_ROOT_PASSWORD: yourpassword
      MYSQL_DATABASE: explorer
```

Docker MySQL 镜像会自动执行 `/docker-entrypoint-initdb.d/` 目录下的 `.sql` 文件。

#### 2.3 使用 systemd 服务

创建 systemd 服务文件 `/etc/systemd/system/mysql-init.service`:

```ini
[Unit]
Description=MySQL Database Initialization
After=mysql.service
Requires=mysql.service

[Service]
Type=oneshot
ExecStart=/usr/bin/mysql -u root -p'yourpassword' explorer < /path/to/injective-explorer/scripts/init_database.sql
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
```

### 方法 3: 在应用启动前执行

在应用启动脚本中添加：

```bash
#!/bin/bash

# 检查数据库是否已初始化
if ! mysql -u root -p'password' -e "USE explorer; SHOW TABLES;" | grep -q "transactions"; then
    echo "Initializing database..."
    mysql -u root -p'password' explorer < scripts/init_database.sql
fi

# 启动应用
./injective-explorer
```

## 注意事项

### 1. 索引重复创建

如果索引已存在，MySQL 会返回错误。这是正常的，可以安全忽略。错误信息类似：
```
ERROR 1061 (42000): Duplicate key name 'idx_transaction_to'
```

### 2. 表已存在

脚本使用 `DROP TABLE IF EXISTS`，**会删除现有表和数据**。如果数据库已有数据，请：

- **备份数据**：
  ```bash
  mysqldump -u root -p explorer > backup.sql
  ```

- **或者修改脚本**：将 `DROP TABLE IF EXISTS` 改为只创建不存在的表

### 3. MySQL 版本兼容性

- **MySQL 5.6+**: 支持所有功能
- **MySQL 5.7+**: 支持 JSON 索引和降序索引
- **MySQL 8.0+**: 完全支持所有功能

### 4. 执行时间

对于空数据库，执行时间通常 < 1 秒。
对于已有大量数据的数据库，创建索引可能需要较长时间（取决于数据量）。

### 5. 禁用 GORM AutoMigrate

如果使用此脚本初始化数据库，建议在 Go 代码中禁用 GORM 的 AutoMigrate：

```go
// 在 db.go 或类似文件中
func InitDB() {
    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
        // 禁用自动迁移
        DisableForeignKeyConstraintWhenMigrating: true,
    })
    
    // 注释掉或删除 AutoMigrate 调用
    // db.AutoMigrate(&models.Transaction{}, &models.Block{}, ...)
}
```

## 验证执行结果

执行脚本后，验证数据库结构：

```sql
-- 检查所有表
SHOW TABLES;

-- 检查 transactions 表的索引
SHOW INDEX FROM transactions;

-- 检查表结构
DESCRIBE transactions;

-- 检查索引统计信息
SELECT 
    TABLE_NAME,
    INDEX_NAME,
    GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ') AS COLUMNS
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = 'explorer'
AND TABLE_NAME = 'transactions'
GROUP BY TABLE_NAME, INDEX_NAME
ORDER BY INDEX_NAME;
```

## 故障排除

### 问题 1: 权限不足

```
ERROR 1045 (28000): Access denied for user 'root'@'localhost'
```

**解决**: 确保 MySQL 用户有足够的权限：
```sql
GRANT ALL PRIVILEGES ON explorer.* TO 'root'@'localhost';
FLUSH PRIVILEGES;
```

### 问题 2: 字符集错误

```
ERROR 1273 (HY000): Unknown collation: 'utf8mb4_0900_ai_ci'
```

**解决**: MySQL 5.6 不支持 `utf8mb4_0900_ai_ci`，修改脚本中的字符集：
```sql
-- 将 utf8mb4_0900_ai_ci 替换为 utf8mb4_general_ci
COLLATE=utf8mb4_general_ci
```

### 问题 3: JSON 类型不支持

```
ERROR 1064 (42000): You have an error in your SQL syntax near 'json'
```

**解决**: MySQL 5.6 不支持 JSON 类型，需要升级到 MySQL 5.7+ 或修改字段类型：
```sql
-- 将 JSON 类型改为 TEXT
metadata TEXT DEFAULT NULL
```

## 回滚

如果需要回滚到使用 GORM AutoMigrate：

1. 删除所有表：
   ```sql
   DROP DATABASE explorer;
   CREATE DATABASE explorer;
   ```

2. 在 Go 代码中重新启用 AutoMigrate

3. 重启应用，让 GORM 自动创建表结构

## 相关文件

- `scripts/migrations/003_add_transaction_timestamp_index.sql` - 时间戳索引迁移
- `scripts/migrations/004_add_comprehensive_query_indexes.sql` - 综合索引迁移
- `docs/database-index-optimization.md` - 索引优化文档

## 更新日志

- 2025-01-XX: 创建完整的数据库初始化脚本，合并表结构和所有索引

