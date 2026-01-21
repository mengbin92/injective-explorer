# 地址标签管理功能技术方案

## 一、需求分析

### 1.1 功能概述
为区块链浏览器新增地址标签管理功能，允许用户为任意地址（账户地址、合约地址等）添加、编辑和删除标签，用于地址分类、标记和识别。

### 1.2 业务场景
- **地址标记**：用户可以为重要地址添加标签，如"交易所"、"项目方"、"个人钱包"等
- **地址分类**：通过标签对地址进行分类管理，便于快速查找和识别
- **风险标记**：标记可疑地址、诈骗地址等，提高安全性
- **地址识别**：为地址添加易于识别的标签名称，替代难以记忆的地址字符串

### 1.3 功能要求
- **添加地址标签**：为指定地址添加标签，支持标签类型、标签值、颜色、描述等信息
- **编辑地址标签**：修改已存在的地址标签信息
- **删除地址标签**：删除指定地址的标签
- **查询地址标签**：查询指定地址的所有标签
- **列表查询**：支持按标签类型、链ID等条件查询标签列表

### 1.4 与现有功能的关系
- **与合约标签的区别**：
  - 合约标签（ContractTag）：专门针对合约地址，与合约信息强关联
  - 地址标签（AddressLabel）：针对所有类型的地址（账户地址、合约地址等），更通用
- **设计原则**：参考现有合约标签的实现模式，保持架构一致性

## 二、数据库设计

### 2.1 地址标签表 (address_labels)

```sql
CREATE TABLE `address_labels` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `address` VARCHAR(128) NOT NULL COMMENT '地址（统一存储为以太坊格式0x...）',
  `chain_id` VARCHAR(50) NOT NULL COMMENT '链ID',
  `label_type` VARCHAR(50) NOT NULL COMMENT '标签类型: exchange, project, personal, scam, verified, custom等',
  `label_value` VARCHAR(255) COMMENT '标签值（标签的具体内容）',
  `label_color` VARCHAR(20) COMMENT '标签颜色（用于前端显示，如#FF5733）',
  `description` TEXT COMMENT '标签描述',
  `created_by` VARCHAR(128) COMMENT '创建者地址或用户名',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL COMMENT '软删除时间',
  
  -- 唯一索引：同一链上同一地址同一标签类型只能有一个
  UNIQUE KEY `uk_address_label` (`chain_id`, `address`, `label_type`),
  -- 索引：按地址查询
  INDEX `idx_address` (`chain_id`, `address`),
  -- 索引：按标签类型查询
  INDEX `idx_label_type` (`label_type`),
  -- 索引：按创建者查询
  INDEX `idx_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='地址标签表';
```

**设计说明：**
- 地址统一存储为以太坊格式（0x...），与现有系统保持一致
- 支持软删除，保留历史数据
- 唯一约束确保同一地址在同一链上同一标签类型只能有一个
- 支持多标签类型，便于扩展

### 2.2 标签类型枚举

预定义的标签类型：
- `exchange`: 交易所
- `project`: 项目方
- `personal`: 个人钱包
- `scam`: 诈骗地址
- `verified`: 已验证地址
- `custom`: 自定义标签
- `contract`: 合约地址（可选，与合约标签区分）

## 三、API设计

### 3.1 Proto定义

在 `api/explorer/v1/address_label.proto` 中定义新的服务：

```protobuf
syntax = "proto3";

package api.explorer.v1;

import "explorer/v1/common.proto";
import "google/api/annotations.proto";
import "google/protobuf/empty.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";
option java_multiple_files = true;
option java_package = "api.explorer.v1";

// AddressLabelService provides address label management services
service AddressLabelService {
  // CreateAddressLabel creates a new address label
  rpc CreateAddressLabel(CreateAddressLabelRequest) returns (Reply) {
    option (google.api.http) = {
      post: "/api/v1/address/label"
      body: "*"
    };
  }

  // UpdateAddressLabel updates an existing address label
  rpc UpdateAddressLabel(UpdateAddressLabelRequest) returns (Reply) {
    option (google.api.http) = {
      put: "/api/v1/address/label"
      body: "*"
    };
  }

  // DeleteAddressLabel deletes an address label
  rpc DeleteAddressLabel(DeleteAddressLabelRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/api/v1/address/label"
    };
  }

  // GetAddressLabels retrieves all labels for an address
  rpc GetAddressLabels(GetAddressLabelsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/address/labels"
    };
  }

  // ListAddressLabels lists address labels with filter conditions
  rpc ListAddressLabels(ListAddressLabelsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/address/label/list"
    };
  }
}

// Request messages
message CreateAddressLabelRequest {
  string address = 1;        // Address (required, supports inj1... or 0x... format)
  string chain_id = 2;      // Chain ID (required)
  string label_type = 3;    // Label type (required)
  string label_value = 4;   // Label value (optional)
  string label_color = 5;   // Label color (optional, e.g., #FF5733)
  string description = 6;   // Description (optional)
  string created_by = 7;    // Creator (optional)
}

message UpdateAddressLabelRequest {
  string address = 1;        // Address (required)
  string chain_id = 2;      // Chain ID (required)
  string label_type = 3;    // Label type (required)
  string label_value = 4;   // Label value (optional)
  string label_color = 5;   // Label color (optional)
  string description = 6;   // Description (optional)
}

message DeleteAddressLabelRequest {
  string address = 1;        // Address (required)
  string chain_id = 2;      // Chain ID (required)
  string label_type = 3;    // Label type (required)
}

message GetAddressLabelsRequest {
  string address = 1;       // Address (required)
  string chain_id = 2;      // Chain ID (required)
}

message ListAddressLabelsRequest {
  string chain_id = 1;           // Chain ID (optional)
  string label_type = 2;         // Label type (optional)
  string address = 3;            // Address (optional, fuzzy match)
  string label_value = 4;         // Label value (optional, fuzzy match)
  PaginationRequest pagination = 5; // Pagination parameters
}

// Response data messages (used in Reply.data as Struct)
message AddressLabelDTO {
  uint64 id = 1;
  string address = 2;
  string chain_id = 3 [json_name = "chain_id"];
  string label_type = 4 [json_name = "label_type"];
  string label_value = 5 [json_name = "label_value"];
  string label_color = 6 [json_name = "label_color"];
  string description = 7;
  string created_by = 8 [json_name = "created_by"];
  string created_at = 9 [json_name = "created_at"];  // ISO 8601 format timestamp
  string updated_at = 10 [json_name = "updated_at"];  // ISO 8601 format timestamp
}
```

### 3.2 HTTP端点

| 方法   | 路径                         | 说明               |
| ------ | ---------------------------- | ------------------ |
| POST   | `/api/v1/address/label`      | 创建地址标签       |
| PUT    | `/api/v1/address/label`      | 更新地址标签       |
| DELETE | `/api/v1/address/label`      | 删除地址标签       |
| GET    | `/api/v1/address/labels`     | 获取地址的所有标签 |
| GET    | `/api/v1/address/label/list` | 列表查询地址标签   |

## 四、实现设计

### 4.1 数据模型

在 `internal/models/address_label.go` 中定义：

```go
package models

import (
	"gorm.io/gorm"
)

// AddressLabel represents an address label in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
type AddressLabel struct {
	gorm.Model

	Address    string `json:"address" gorm:"column:address;size:128;not null"`
	ChainID    string `json:"chain_id" gorm:"column:chain_id;size:50;not null"`
	LabelType  string `json:"label_type" gorm:"column:label_type;size:50;not null"`
	LabelValue string `json:"label_value" gorm:"column:label_value;size:255"`
	LabelColor string `json:"label_color" gorm:"column:label_color;size:20"`
	Description string `json:"description" gorm:"column:description;type:text"`
	CreatedBy  string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for AddressLabel.
func (AddressLabel) TableName() string {
	return "address_labels"
}
```

### 4.2 DTO转换

在 `internal/models/address_label.go` 中添加转换方法：

```go
// ToAddressLabelDTO converts AddressLabel to AddressLabelDTO
func (l *AddressLabel) ToAddressLabelDTO() *pb.AddressLabelDTO {
	return &pb.AddressLabelDTO{
		Id:          uint64(l.ID),
		Address:     l.Address,
		ChainId:     l.ChainID,
		LabelType:   l.LabelType,
		LabelValue:  l.LabelValue,
		LabelColor:  l.LabelColor,
		Description: l.Description,
		CreatedBy:   l.CreatedBy,
		CreatedAt:   l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   l.UpdatedAt.Format(time.RFC3339),
	}
}

// ToAddressLabelDTOs converts []AddressLabel to []AddressLabelDTO
func ToAddressLabelDTOs(labels []AddressLabel) []*pb.AddressLabelDTO {
	dtos := make([]*pb.AddressLabelDTO, 0, len(labels))
	for _, label := range labels {
		dtos = append(dtos, label.ToAddressLabelDTO())
	}
	return dtos
}
```

### 4.3 服务实现

在 `internal/service/address_label.go` 中实现服务：

```go
package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// AddressLabelService implements the address label management service
type AddressLabelService struct {
	pb.UnimplementedAddressLabelServiceServer
}

// NewAddressLabelService creates a new instance of AddressLabelService
func NewAddressLabelService() *AddressLabelService {
	return &AddressLabelService{}
}

// Cache key constants
const (
	CacheKeyAddressLabels = "address_label:labels:%s:%s" // chain_id:address
)

// Cache expiration constants
const (
	CacheExpireAddressLabels = 1 * time.Hour
)

// CreateAddressLabel creates a new address label
func (s *AddressLabelService) CreateAddressLabel(ctx context.Context, req *pb.CreateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Create label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	label := &models.AddressLabel{
		Address:     address,
		ChainID:     req.ChainId,
		LabelType:   req.LabelType,
		LabelValue:  req.LabelValue,
		LabelColor:  req.LabelColor,
		Description: req.Description,
		CreatedBy:   req.CreatedBy,
	}

	// Use FirstOrCreate to avoid duplicates
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		FirstOrCreate(label).Error

	if err != nil {
		global.Logger.Errorf("failed to create address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := label.ToAddressLabelDTO()
	return MarshalAndReply(labelDTO, "marshal label error")
}

// UpdateAddressLabel updates an existing address label
func (s *AddressLabelService) UpdateAddressLabel(ctx context.Context, req *pb.UpdateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Update label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	updates := make(map[string]interface{})
	if req.LabelValue != "" {
		updates["label_value"] = req.LabelValue
	}
	if req.LabelColor != "" {
		updates["label_color"] = req.LabelColor
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if len(updates) == 0 {
		return ErrorReply(http.StatusBadRequest, "at least one field must be provided for update", errors.New("no fields to update")), errors.New("no fields to update")
	}

	var label models.AddressLabel
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		First(&label).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "address label not found", err), err
		}
		global.Logger.Errorf("failed to find address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to find address label", err), err
	}

	err = db.Get().WithContext(queryCtx).
		Model(&models.AddressLabel{}).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		Updates(updates).Error

	if err != nil {
		global.Logger.Errorf("failed to update address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update address label", err), err
	}

	// Reload to get updated data
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		First(&label).Error

	if err != nil {
		global.Logger.Errorf("failed to reload address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := label.ToAddressLabelDTO()
	return MarshalAndReply(labelDTO, "marshal label error")
}

// DeleteAddressLabel deletes an address label
func (s *AddressLabelService) DeleteAddressLabel(ctx context.Context, req *pb.DeleteAddressLabelRequest) (*emptypb.Empty, error) {
	// Input validation
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	if req.Address == "" {
		return nil, errors.New("address is required")
	}

	if req.ChainId == "" {
		return nil, errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return nil, errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Delete label (soft delete)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		Delete(&models.AddressLabel{}).Error

	if err != nil {
		global.Logger.Errorf("failed to delete address label: %v", err)
		return nil, err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	return &emptypb.Empty{}, nil
}

// GetAddressLabels retrieves all labels for an address
func (s *AddressLabelService) GetAddressLabels(ctx context.Context, req *pb.GetAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Try to get from cache
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	cachedLabels, err := s.getLabelsFromCache(queryCtx, req.ChainId, address)
	if err == nil && cachedLabels != nil {
		global.Logger.Debugf("address labels found in cache: address=%s", address)
		labelDTOs := models.ToAddressLabelDTOs(cachedLabels)
		return MarshalAndReply(labelDTOs, "marshal labels error")
	}

	// Query from database
	var labels []models.AddressLabel
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		Find(&labels).Error

	if err != nil {
		global.Logger.Errorf("failed to get address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get address labels", err), err
	}

	// Cache the labels
	go s.cacheLabels(context.Background(), req.ChainId, address, labels)

	// Convert to response
	labelDTOs := models.ToAddressLabelDTOs(labels)
	return MarshalAndReply(labelDTOs, "marshal labels error")
}

// ListAddressLabels lists address labels with filter conditions
func (s *AddressLabelService) ListAddressLabels(ctx context.Context, req *pb.ListAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.LabelType != "" {
		query = query.Where("label_type = ?", req.LabelType)
	}

	if req.Address != "" {
		address := models.ConvertAddressToEthForStorage(req.Address)
		query = query.Where("address LIKE ?", "%"+address+"%")
	}

	if req.LabelValue != "" {
		query = query.Where("label_value LIKE ?", "%"+req.LabelValue+"%")
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.AddressLabel{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for address labels: %v, using fallback value", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	var labels []models.AddressLabel
	if useCursor {
		query = query.Where("id < ?", cursorValue).
			Order("id DESC").
			Limit(int(pageSize))
	} else {
		query = query.Order("id DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	err := query.Find(&labels).Error
	if err != nil {
		global.Logger.Errorf("failed to list address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to list address labels", err), err
	}

	// Build response
	nextCursor := calculateNextCursor(useCursor, cursorValue, labels, func(l models.AddressLabel) uint64 { return uint64(l.ID) })
	labelDTOs := models.ToAddressLabelDTOs(labels)
	labelsResp := buildPaginatedResponse(labelDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(labels))

	return MarshalAndReply(labelsResp, "marshal labels error")
}

// Cache helper methods

// getLabelsFromCache retrieves address labels from Redis cache
func (s *AddressLabelService) getLabelsFromCache(ctx context.Context, chainID, address string) ([]models.AddressLabel, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	data, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var labels []models.AddressLabel
	if err := sonic.Unmarshal([]byte(data), &labels); err != nil {
		return nil, err
	}

	return labels, nil
}

// cacheLabels caches address labels to Redis
func (s *AddressLabelService) cacheLabels(ctx context.Context, chainID, address string, labels []models.AddressLabel) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	data, err := sonic.Marshal(labels)
	if err != nil {
		global.Logger.Warnf("failed to marshal address labels for cache: %v", err)
		return
	}

	if err := redisClient.Set(ctx, key, data, CacheExpireAddressLabels).Err(); err != nil {
		global.Logger.Warnf("failed to cache address labels: %v", err)
	}
}

// clearAddressLabelCache clears the cache for address labels
func (s *AddressLabelService) clearAddressLabelCache(ctx context.Context, chainID, address string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	if err := redisClient.Del(ctx, key).Err(); err != nil {
		global.Logger.Warnf("failed to clear address label cache: %v", err)
	}
}
```

### 4.4 服务注册

在 `internal/server/grpc.go` 中注册服务：

```go
addressLabelService := service.NewAddressLabelService()
v1.RegisterAddressLabelServiceServer(srv, addressLabelService)
```

在 `internal/server/http.go` 中注册HTTP路由（Kratos会自动处理）。

### 4.5 数据库迁移

在 `internal/global/global.go` 中添加模型迁移：

```go
// AutoMigrate database models
err = db.AutoMigrate(
	// ... existing models ...
	&models.AddressLabel{},
)
```

## 五、缓存策略

### 5.1 缓存键设计
- 键格式：`address_label:labels:{chain_id}:{address}`
- 示例：`address_label:labels:injective-1:0x1234...`

### 5.2 缓存策略
- **读取**：优先从Redis缓存读取，缓存未命中则查询数据库并写入缓存
- **写入**：创建/更新/删除标签后清除相关缓存
- **过期时间**：1小时（可配置）

### 5.3 缓存更新
- **创建标签**：清除该地址的标签缓存
- **更新标签**：清除该地址的标签缓存
- **删除标签**：清除该地址的标签缓存

## 六、错误处理

### 6.1 错误码定义

| HTTP状态码 | 错误码         | 说明                 |
| ---------- | -------------- | -------------------- |
| 400        | BAD_REQUEST    | 请求参数错误         |
| 404        | NOT_FOUND      | 标签不存在           |
| 409        | CONFLICT       | 标签已存在（创建时） |
| 500        | INTERNAL_ERROR | 服务器内部错误       |

### 6.2 错误响应格式

```json
{
  "code": 400,
  "message": "address is required",
  "data": null
}
```

## 七、性能优化

### 7.1 数据库优化
- **索引优化**：在 `chain_id`、`address`、`label_type` 上建立复合索引
- **查询优化**：使用索引覆盖查询，避免全表扫描
- **分页优化**：支持游标分页，提高大数据量查询性能

### 7.2 缓存优化
- **缓存预热**：高频访问的地址标签可以预热到缓存
- **缓存穿透**：对不存在的标签也进行短时间缓存，避免缓存穿透

### 7.3 批量操作优化
- **批量查询**：支持一次查询多个地址的标签（可选扩展）
- **批量更新**：支持批量更新标签（可选扩展）

## 八、安全性考虑

### 8.1 输入验证
- **地址格式验证**：验证地址格式是否正确（inj1... 或 0x...）
- **标签类型验证**：限制标签类型为预定义的类型或自定义类型
- **长度限制**：对标签值、描述等字段进行长度限制

### 8.2 权限控制（可选）
- **创建者验证**：记录标签创建者，便于审计
- **权限验证**：未来可扩展权限控制，限制某些用户只能查看或编辑特定标签

### 8.3 数据保护
- **软删除**：使用软删除，保留历史数据
- **审计日志**：记录标签的创建、更新、删除操作（可选）

## 九、测试计划

### 9.1 单元测试
- 模型转换测试
- 服务方法测试
- 缓存操作测试

### 9.2 集成测试
- API端点测试
- 数据库操作测试
- 缓存集成测试

### 9.3 性能测试
- 并发创建标签测试
- 大量标签查询测试
- 缓存命中率测试

## 十、实施计划

### 10.1 第一阶段：基础功能
1. 创建数据库表
2. 定义Proto接口
3. 实现数据模型和DTO转换
4. 实现基础CRUD服务
5. 注册服务到gRPC和HTTP服务器

### 10.2 第二阶段：缓存优化
1. 实现Redis缓存
2. 优化缓存策略
3. 性能测试和调优

### 10.3 第三阶段：扩展功能（可选）
1. 批量操作支持
2. 标签导入导出
3. 标签统计功能
4. 权限控制

## 十一、注意事项

1. **地址格式统一**：所有地址统一转换为以太坊格式（0x...）存储，与现有系统保持一致
2. **与合约标签区分**：地址标签和合约标签是独立的功能，互不干扰
3. **向后兼容**：新增功能不影响现有功能
4. **扩展性**：设计时考虑未来可能的扩展需求，如标签分类、标签层级等

## 十二、参考文档

- [合约标签实现](./contract-query-service-design.md)
- [合约信息导入导出设计](./contract-info-import-export-design.md)
- [技术架构文档](./technical-architecture.md)
