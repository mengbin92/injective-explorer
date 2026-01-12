# 安全扫描整改方案

## 架构说明

**部署架构**: Nginx (反向代理) → Go应用服务

```
用户请求 → Nginx (443/80) → Go应用 (内网端口)
```

由于使用了Nginx反向代理，大部分安全头应该在**Nginx层面**统一配置，而不是在应用层配置。这样有以下优势：

- 集中管理，便于维护
- 不影响应用代码
- 可统一调整策略
- 性能更好（Nginx处理更高效）

---

## 扫描概况

**扫描日期**: 2026年1月6日
**扫描对象**: 区块浏览器测试网
**扫描结果**: 发现13项安全风险

## 风险分析汇总

| 编号 | 风险名称 | 严重程度 | 整改位置 | 优先级 |
|------|----------|----------|----------|--------|
| 1 | Content Security Policy Header Not Set | 高 | Nginx | P0 |
| 2 | Missing Anti-clickjacking Header | 中 | Nginx | P0 |
| 3 | Cookie No HttpOnly Flag | 高 | 不适用 | - |
| 4 | Cookie Without Secure Flag | 高 | 不适用 | - |
| 5 | In Page Banner Information Leak (5) | 低 | 前端/应用 | P2 |
| 6 | Server Leaks via "X-Powered-By" | 低 | Nginx | P1 |
| 7 | Server Leaks Version via "Server" | 低 | Nginx | P1 |
| 8 | Strict-Transport-Security Header Not Set | 高 | Nginx | P0 |
| 9 | X-Content-Type-Options Header Missing | 中 | Nginx | P1 |
| 10 | 时间戳泄露 - Unix | 低 | 应用 | P2 |
| 11 | Content-Type Header Missing | 高 | Nginx | P1 |
| 12 | Information Disclosure - Suspicious Comments (10) | 低 | 应用 | P2 |
| 13 | Re-examine Cache-control Directives (2) | 中 | Nginx+应用 | P1 |

**说明**:
- 风险3和4（Cookie相关）不适用，因为项目使用Token认证而非Cookie认证
- 风险11（Content-Type）Nginx默认会处理，可能需要微调

---

## 整改方案

### 一、Nginx配置（主要整改）

#### Nginx安全头配置文件

创建或修改Nginx配置，添加安全头：

**文件**: `/etc/nginx/security-headers.conf`（或在server block中直接配置）

```nginx
# ============================================
# 安全头配置
# 用于修复安全扫描发现的漏洞
# ============================================

# 1. Content Security Policy (CSP) - 防止XSS攻击
# 限制内容加载来源，防止恶意脚本注入
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none';" always;

# 2. X-Frame-Options - 防止点击劫持
# 禁止页面被嵌入到iframe中
add_header X-Frame-Options "DENY" always;

# 3. Strict-Transport-Security (HSTS) - 强制HTTPS
# 31536000秒 = 1年，includeSubDomains 保护所有子域名
# 注意：确保HTTPS已正确配置后再启用
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;

# 4. X-Content-Type-Options - 防止MIME嗅探
# 禁止浏览器猜测文件类型
add_header X-Content-Type-Options "nosniff" always;

# 5. Referrer-Policy - 控制Referer信息泄露
add_header Referrer-Policy "strict-origin-when-cross-origin" always;

# 6. X-XSS-Protection - XSS过滤器（向后兼容旧浏览器）
add_header X-XSS-Protection "1; mode=block" always;

# 7. Permissions-Policy - 禁用不必要的浏览器功能
add_header Permissions-Policy "geolocation=(), microphone=(), camera=(), payment=(), usb=()" always;

# 8. 隐藏服务器版本信息
# 防止泄露Nginx版本号
server_tokens off;

# 9. 移除X-Powered-By头（如果有）
proxy_hide_header X-Powered-By;
```

#### 完整Nginx配置示例

**文件**: `/etc/nginx/sites-available/injective-explorer`

```nginx
# HTTP 重定向到 HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name explorer.example.com;

    # 强制重定向到HTTPS
    return 301 https://$server_name$request_uri;
}

# HTTPS 主配置
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name explorer.example.com;

    # SSL证书配置
    ssl_certificate /etc/ssl/certs/explorer.example.com.crt;
    ssl_certificate_key /etc/ssl/private/explorer.example.com.key;

    # SSL安全配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # 引入安全头配置
    include /etc/nginx/security-headers.conf;

    # 隐藏后端服务器信息
    proxy_hide_header X-Powered-By;
    proxy_hide_header Server;

    # 日志配置
    access_log /var/log/nginx/explorer-access.log;
    error_log /var/log/nginx/explorer-error.log;

    # 代理到Go应用
    location / {
        proxy_pass http://127.0.0.1:8000;

        # 代理头设置
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时配置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;

        # WebSocket支持（如果使用）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # 静态文件缓存配置
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        proxy_pass http://127.0.0.1:8000;
        expires 7d;
        add_header Cache-Control "public, immutable";
        include /etc/nginx/security-headers.conf;
    }

    # API接口 - 禁用缓存
    location /api/ {
        proxy_pass http://127.0.0.1:8000;
        add_header Cache-Control "no-store, no-cache, must-revalidate, private" always;
        add_header Pragma "no-cache" always;
        add_header Expires "0" always;
        include /etc/nginx/security-headers.conf;
    }

    # Swagger UI（如果需要）
    location /swagger/ {
        proxy_pass http://127.0.0.1:8000;
        include /etc/nginx/security-headers.conf;
    }
}
```

#### 如果使用Envoy

如果你使用的是Envoy而不是Nginx，配置方式如下：

```yaml
# Envoy配置片段
http_filters:
  - name: envoy.filters.http.router
  - name: envoy.filters.http.set_metadata
  - name: envoy.filters.header_to_metadata
typed_config:
  "@type": type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config

# 在response_header_to_add中添加安全头
response_headers_to_add:
  - header:
      key: "Content-Security-Policy"
      value: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';"
  - header:
      key: "X-Frame-Options"
      value: "DENY"
  - header:
      key: "Strict-Transport-Security"
      value: "max-age=31536000; includeSubDomains"
  - header:
      key: "X-Content-Type-Options"
      value: "nosniff"
  - header:
      key: "Referrer-Policy"
      value: "strict-origin-when-cross-origin"
  - header:
      key: "X-XSS-Protection"
      value: "1; mode=block"
  - header:
      key: "Permissions-Policy"
      value: "geolocation=(), microphone=(), camera=()"

# 隐藏服务器信息
server_header_transformation: APPEND_IF_MISSING
```

---

### 二、应用层配置（补充整改）

虽然大部分安全头在Nginx配置，但以下内容需要在应用层处理：

#### 1. 缓存控制头 - 敏感API

**文件**: `internal/server/http.go`

在需要禁用缓存的API响应中添加：

```go
// 在敏感API响应中添加
func setNoCacheHeaders(w http.ResponseWriter) {
    w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
    w.Header().Set("Pragma", "no-cache")
    w.Header().Set("Expires", "0")
}
```

或者创建一个响应包装器中间件：

**新文件**: `internal/middleware/cache_control.go`

```go
package middleware

import (
    "net/http"
    "strings"
)

// NoCacheMiddleware 为指定路径禁用缓存
func NoCacheMiddleware(paths []string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 检查请求路径是否匹配
            for _, path := range paths {
                if strings.HasPrefix(r.URL.Path, path) {
                    w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
                    w.Header().Set("Pragma", "no-cache")
                    w.Header().Set("Expires", "0")
                    break
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

使用方式：

```go
// 在 internal/server/http.go 中
noCachePaths := []string{
    "/api/v1/admin",
    "/api/v1/tokens",
    "/api/v1/export",
}

opts := []khttp.ServerOption{
    khttp.Middleware(
        recovery.Recovery(),
        middleware.SelectiveAuthAndRateLimitMiddleware(...),
    ),
    khttp.Filter(middleware.NoCacheMiddleware(noCachePaths)),
}
```

#### 2. 清理敏感注释

**需要检查的文件**:
- `provider/chain/evm_client.go:292, 312`
- `provider/storage/storage_init.go:60, 63`
- `internal/service/admin_ip.go:571, 652, 684`

**操作**:
- 保留技术性TODO注释
- 移除包含安全细节的注释（如"未做权限校验"）

#### 3. API响应中的时间戳处理（可选）

如果需要处理时间戳泄露，可以创建响应过滤器：

**新文件**: `internal/middleware/response_sanitizer.go`

```go
package middleware

import (
    "encoding/json"
    "net/http"
    "strings"
    "time"
)

// ResponseSanitizer 中间件用于清理敏感信息
func ResponseSanitizer(shouldSanitize bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 仅在生产环境且需要时执行
            if !shouldSanitize {
                next.ServeHTTP(w, r)
                return
            }

            // 使用response wrapper来修改响应
            wrapper := &sanitizingResponseWriter{ResponseWriter: w}
            next.ServeHTTP(wrapper, r)
        })
    }
}

type sanitizingResponseWriter struct {
    http.ResponseWriter
    sanitized bool
}

func (w *sanitizingResponseWriter) Write(b []byte) (int, error) {
    // 这里可以添加响应清理逻辑
    // 例如：将时间戳转换为日期字符串
    return w.ResponseWriter.Write(b)
}
```

---

## 实施步骤

### 阶段1: Nginx配置（核心）

| 步骤 | 任务 | 文件 | 预计时间 |
|------|------|------|----------|
| 1.1 | 创建安全头配置文件 | `/etc/nginx/security-headers.conf` | 10分钟 |
| 1.2 | 更新站点配置 | `/etc/nginx/sites-available/injective-explorer` | 15分钟 |
| 1.3 | 测试Nginx配置 | `nginx -t` | 5分钟 |
| 1.4 | 重载Nginx | `nginx -s reload` | 2分钟 |

### 阶段2: 验证测试

| 步骤 | 任务 | 预计时间 |
|------|------|----------|
| 2.1 | 本地验证响应头 | 10分钟 |
| 2.2 | 测试环境验证 | 20分钟 |

### 阶段3: 应用层补充（可选）

| 步骤 | 任务 | 预计时间 |
|------|------|----------|
| 3.1 | 添加缓存控制中间件 | `internal/middleware/cache_control.go` | 20分钟 |
| 3.2 | 清理敏感注释 | 多个文件 | 15分钟 |
| 3.3 | 前端信息泄露检查 | 前端文件 | 20分钟 |

**总预计时间**: 约2小时

---

## 验证方法

### 1. 验证Nginx配置语法

```bash
# 测试Nginx配置
sudo nginx -t

# 如果没有错误，重载配置
sudo nginx -s reload
```

### 2. 检查响应头

```bash
# 检查所有安全头
curl -I https://explorer.example.com

# 使用详细模式查看所有头
curl -v https://explorer.example.com 2>&1 | grep -i "< "

# 检查特定安全头
curl -I https://explorer.example.com | grep -E "Content-Security-Policy|X-Frame-Options|Strict-Transport-Security"
```

**期望输出**:

```http
HTTP/2 200
content-type: application/json
content-security-policy: default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; ...
x-frame-options: DENY
strict-transport-security: max-age=31536000; includeSubDomains; preload
x-content-type-options: nosniff
referrer-policy: strict-origin-when-cross-origin
x-xss-protection: 1; mode=block
permissions-policy: geolocation=(), microphone=(), camera=(), payment=(), usb=()
```

### 3. 在线安全评分

```bash
# 使用 securityheaders.com
curl https://securityheaders.com/?q=https://explorer.example.com&followRedirects=on

# 或直接访问网站评分
# https://securityheaders.com/
```

期望获得 **A** 或 **A+** 评级。

### 4. 使用工具扫描

```bash
# 安装 wafcli
go install github.com/commjoen/wafcli/cmd/wafcli@latest

# 扫描安全头
wafcli headers https://explorer.example.com

# 使用 nmap 扫描
nmap --script http-headers https://explorer.example.com
```

---

## CSP策略调优

### 如果使用CDN

如果前端资源通过CDN加载，需要在CSP中添加CDN域名：

```nginx
# 在 security-headers.conf 中修改
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.example.com; style-src 'self' 'unsafe-inline' https://cdn.example.com; img-src 'self' data: https: https://cdn.example.com; font-src 'self' data: https://cdn.example.com; connect-src 'self'; frame-ancestors 'none';" always;
```

### 环境区分

开发环境和生产环境可以使用不同的CSP策略：

```nginx
# 开发环境 - 更宽松
map $environment $csp_policy {
    default "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; ...";
    development "default-src 'self' 'unsafe-inline' 'unsafe-eval'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; ...";
}

server {
    # 使用环境变量
    add_header Content-Security-Policy $csp_policy always;
}
```

### CSP报告模式

上线前可以先使用报告模式，观察是否有违规：

```nginx
# 报告模式 - 不拦截，只报告
add_header Content-Security-Policy-Report-Only "default-src 'self'; script-src 'self' ...; report-uri /csp-report" always;

# 确认无问题后切换到正式模式
add_header Content-Security-Policy "default-src 'self'; script-src 'self' ..." always;
```

---

## HSTS注意事项

### 前置条件检查

启用HSTS前，确保：

1. ✅ HTTPS已正确配置
2. ✅ SSL证书有效
3. ✅ HTTP全部重定向到HTTPS
4. ✅ 没有混合内容问题

### 逐步启用

```nginx
# 第一阶段：短时间测试（5分钟）
add_header Strict-Transport-Security "max-age=300" always;

# 第二阶段：延长到1周
add_header Strict-Transport-Security "max-age=604800" always;

# 第三阶段：延长到1年
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

# 第四阶段：申请加入HSTS Preload列表
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
```

### 移除HSTS警告

一旦启用HSTS且浏览器缓存后，在max-age过期前无法移除HTTPS强制。建议：
- 先从小值开始测试
- 确认无误后再设置大值
- preload一旦加入很难移除

---

## 不适用项说明

### Cookie相关风险 (风险3、4)

项目使用API Token认证，不使用Cookie session，因此以下风险不适用：

- Cookie No HttpOnly Flag
- Cookie Without Secure Flag

**当前认证方式**:
- 请求头 `X-API-Token` 传递认证令牌
- 令牌存储在Redis中
- 不依赖浏览器Cookie机制

---

## 风险评估矩阵

```
        高影响
          │
          │  P0 (必须修复 - Nginx配置)
          │  • CSP未设置
          │  • X-Frame-Options缺失
          │  • HSTS缺失
          │
          │
          │  P1 (强烈建议)
          │  • X-Powered-By泄露
          │  • Server版本泄露
          │  • X-Content-Type-Options
          │  • 缓存控制
          │
──────────────────────────►
          低影响

          │  P2 (建议修复 - 应用层)
          │  • 信息泄露注释
          │  • 时间戳泄露
          │  • Banner信息泄露
          │
          ▼
        低影响
```

---

## 常见问题

### Q1: Nginx和Go应用都设置了安全头会怎样？

A: Nginx的 `always` 参数会确保安全头始终添加。如果Go应用也设置了，会产生重复头。建议只在Nginx层设置。

### Q2: 如何调试CSP策略？

A: 使用浏览器的开发者工具：

1. 打开 Console 面板
2. 查看CSP违规报告
3. 或使用 `Content-Security-Policy-Report-Only` 先只报告不拦截

### Q3: 某些页面需要iframe嵌入怎么办？

A: 使用 `SAMEORIGIN` 或 `ALLOW-FROM uri` 替代 `DENY`：

```nginx
# 允许同源嵌入
add_header X-Frame-Options "SAMEORIGIN" always;

# 或在CSP中使用 frame-ancestors
add_header Content-Security-Policy "...; frame-ancestors 'self' https://trusted.domain.com;" always;
```

---

## 后续建议

1. **定期检查**: 每季度运行一次安全扫描
2. **依赖更新**: 定期更新Nginx版本和Go依赖
3. **监控**: 配置CSP违规报告收集
4. **HSTS Preload**: 稳定后申请加入 https://hstspreload.org/

---

## 参考资料

- [MDN HTTP Headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers)
- [OWASP Secure Headers](https://owasp.org/www-project-secure-headers/)
- [Nginx Security Headers Guide](https://nginx.org/en/docs/http/ngx_http_headers_module.html)
- [CSP Evaluator](https://csp-evaluator.withgoogle.com/)
- [Security Headers Best Practices](https://web.dev/security-headers/)
