// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/service"

	"github.com/bytedance/sonic"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/handlers"
	httpSwagger "github.com/swaggo/http-swagger"
	"gopkg.in/yaml.v3"
)

var (
	openAPIFileCache     string
	openAPIFileCacheOnce sync.Once
	projectRootCache     string
	projectRootCacheOnce sync.Once
)

// NewHTTPServer creates and configures a new HTTP server instance.
// It sets up middleware, network configuration, address, and timeout from the provided configuration.
// The server registers the demo service HTTP handler.
//
// Parameters:
//   - c: Server configuration containing HTTP settings
//   - logger: Logger instance for server logging
//
// Returns:
//   - *khttp.Server: A configured HTTP server ready to accept connections
func NewHTTPServer(c *conf.Server, logger log.Logger) *khttp.Server {
	// Configure CORS with security best practices
	// Security: CORS configuration must follow browser security rules:
	// - If AllowedOrigins contains "*", AllowCredentials must be false
	// - If AllowCredentials is true, AllowedOrigins must specify exact origins (not "*")
	var corsOpts []handlers.CORSOption
	allowedOrigins := []string{"*"} // Default: allow all origins
	allowCredentials := false       // Default: disabled for security when using "*"

	// Security: If using "*" origin, credentials must be disabled
	// This prevents CORS security vulnerabilities
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		allowCredentials = false
	}

	corsOpts = append(corsOpts,
		handlers.AllowedOrigins(allowedOrigins),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{
			"Content-Type",
			"Authorization",
			"X-Requested-With",
			// Note: Accept, Origin, and Access-Control-Request-* headers are simple headers
			// and don't need to be explicitly allowed
		}),
		handlers.MaxAge(86400), // Cache preflight requests for 24 hours
	)

	if allowCredentials {
		corsOpts = append(corsOpts, handlers.AllowCredentials())
	}

	var opts = []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
		),
	}

	// Add Swagger filter first (will execute AFTER CORS filter)
	// Note: In Kratos, filters execute in reverse order (last added = first executed)
	// So we add Swagger filter first, then CORS filter
	// This ensures CORS headers are applied to Swagger requests
	if c.EnableSwagger {
		// Create swagger handler (will be used in filter)
		swaggerHandler := httpSwagger.Handler(
			httpSwagger.URL("/openapi.json"), // Point to our OpenAPI JSON file
			httpSwagger.DocExpansion("list"),
			httpSwagger.DomID("swagger-ui"),
		)
		opts = append(opts,
			// Add filter to handle swagger requests before routing
			khttp.Filter(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := r.URL.Path
					// Handle swagger requests
					if path == "/swagger" {
						http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
						return
					}
					if strings.HasPrefix(path, "/swagger/") {
						swaggerHandler.ServeHTTP(w, r)
						return
					}
					// Continue with normal routing
					next.ServeHTTP(w, r)
				})
			}),
		)
	}

	// Add CORS filter AFTER Swagger filter (will execute FIRST)
	// This ensures CORS headers are applied to all requests including Swagger
	// The CORS middleware will automatically handle OPTIONS preflight requests
	opts = append(opts, khttp.Filter(handlers.CORS(corsOpts...)))

	if c.Http.Network != "" {
		opts = append(opts, khttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, khttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, khttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := khttp.NewServer(opts...)

	demoService := service.NewDemoService()
	v1.RegisterDemoHTTPServer(srv, demoService)

	// Register Swagger UI routes
	if c.EnableSwagger {
		registerSwaggerRoutes(srv, logger)
	}

	return srv
}

// findProjectRoot tries to find the project root by looking for go.mod file.
// The result is cached to avoid repeated file system operations.
func findProjectRoot() string {
	projectRootCacheOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}

		// Start from current directory and walk up to find go.mod
		dir := cwd
		for {
			goModPath := filepath.Join(dir, "go.mod")
			if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
				projectRootCache = dir
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				// Reached root, stop
				break
			}
			dir = parent
		}

		// Fallback: try executable directory
		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			goModPath := filepath.Join(execDir, "go.mod")
			if _, err := os.Stat(goModPath); err == nil {
				projectRootCache = execDir
				return
			}
			// Try parent directories
			dir := execDir
			for i := 0; i < 5; i++ {
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				goModPath := filepath.Join(parent, "go.mod")
				if _, err := os.Stat(goModPath); err == nil {
					projectRootCache = parent
					return
				}
				dir = parent
			}
		}

		projectRootCache = cwd // Fallback to current working directory
	})

	return projectRootCache
}

// findOpenAPIFile tries to locate the openapi.yaml file in common locations
func findOpenAPIFile() (string, error) {
	// Use cached result if available
	openAPIFileCacheOnce.Do(func() {
		projectRoot := findProjectRoot()
		openAPIPath := filepath.Join(projectRoot, "openapi.yaml")
		if info, err := os.Stat(openAPIPath); err == nil && !info.IsDir() {
			openAPIFileCache = openAPIPath
			return
		}

		// Try other common locations
		cwd, _ := os.Getwd()
		paths := []string{
			filepath.Join(projectRoot, "openapi.yaml"),
			filepath.Join(cwd, "openapi.yaml"),
			filepath.Join(cwd, "..", "openapi.yaml"),
			filepath.Join(cwd, "..", "..", "openapi.yaml"),
			"openapi.yaml",
		}

		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			paths = append(paths,
				filepath.Join(execDir, "openapi.yaml"),
				filepath.Join(execDir, "..", "openapi.yaml"),
			)
		}

		for _, path := range paths {
			cleanPath := filepath.Clean(path)
			if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
				openAPIFileCache = cleanPath
				return
			}
		}
	})

	if openAPIFileCache != "" {
		return openAPIFileCache, nil
	}

	return "", os.ErrNotExist
}

// registerSwaggerRoutes registers Swagger UI routes for OpenAPI documentation
func registerSwaggerRoutes(srv *khttp.Server, logger log.Logger) {
	r := srv.Route("/")

	// Serve OpenAPI YAML file
	r.GET("/openapi.yaml", func(ctx khttp.Context) error {
		openapiPath, err := findOpenAPIFile()
		if err != nil {
			log.NewHelper(logger).Warnf("openapi.yaml not found: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/x-yaml")
			ctx.Response().WriteHeader(http.StatusNotFound)
			ctx.Response().Write([]byte("# openapi.yaml not found"))
			return nil
		}

		// Security: Validate that the path is within project root to prevent path traversal
		projectRoot := findProjectRoot()
		cleanPath := filepath.Clean(openapiPath)
		cleanRoot := filepath.Clean(projectRoot)

		// Ensure the resolved path is within project root
		relPath, err := filepath.Rel(cleanRoot, cleanPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			log.NewHelper(logger).Warnf("Invalid openapi.yaml path: %s", cleanPath)
			ctx.Response().Header().Set("Content-Type", "application/x-yaml")
			ctx.Response().WriteHeader(http.StatusForbidden)
			ctx.Response().Write([]byte("# Invalid file path"))
			return nil
		}

		data, err := os.ReadFile(cleanPath)
		if err != nil {
			log.NewHelper(logger).Errorf("Failed to read openapi.yaml: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/x-yaml")
			ctx.Response().WriteHeader(http.StatusInternalServerError)
			ctx.Response().Write([]byte("# Failed to read openapi.yaml"))
			return nil
		}

		ctx.Response().Header().Set("Content-Type", "application/x-yaml")
		ctx.Response().Write(data)
		return nil
	})

	// Serve OpenAPI JSON file (converted from YAML)
	r.GET("/openapi.json", func(ctx khttp.Context) error {
		openapiPath, err := findOpenAPIFile()
		if err != nil {
			log.NewHelper(logger).Warnf("openapi.yaml not found: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusNotFound)
			errorMsg := map[string]string{
				"error":   "openapi.yaml not found",
				"message": "The openapi.yaml file could not be located. Please ensure it exists in the project root directory.",
			}
			if jsonData, err := sonic.Marshal(errorMsg); err == nil {
				ctx.Response().Write(jsonData)
			} else {
				ctx.Response().Write([]byte(`{"error":"openapi.yaml not found"}`))
			}
			return nil
		}

		// Security: Validate that the path is within project root to prevent path traversal
		projectRoot := findProjectRoot()
		cleanPath := filepath.Clean(openapiPath)
		cleanRoot := filepath.Clean(projectRoot)

		// Ensure the resolved path is within project root
		relPath, err := filepath.Rel(cleanRoot, cleanPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			log.NewHelper(logger).Warnf("Invalid openapi.yaml path: %s", cleanPath)
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusForbidden)
			ctx.Response().Write([]byte(`{"error":"invalid file path"}`))
			return nil
		}

		data, err := os.ReadFile(cleanPath)
		if err != nil {
			log.NewHelper(logger).Errorf("Failed to read openapi.yaml: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusInternalServerError)
			errorMsg := map[string]string{
				"error":   "failed to read openapi.yaml",
				"message": "Unable to read the openapi.yaml file",
			}
			if jsonData, err := sonic.Marshal(errorMsg); err == nil {
				ctx.Response().Write(jsonData)
			} else {
				ctx.Response().Write([]byte(`{"error":"failed to read openapi.yaml"}`))
			}
			return nil
		}

		// Parse YAML
		var yamlData interface{}
		if err := yaml.Unmarshal(data, &yamlData); err != nil {
			log.NewHelper(logger).Errorf("Failed to parse openapi.yaml: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusInternalServerError)
			errorMsg := map[string]string{
				"error":   "failed to parse openapi.yaml",
				"message": "Unable to parse the openapi.yaml file",
			}
			if jsonData, err := sonic.Marshal(errorMsg); err == nil {
				ctx.Response().Write(jsonData)
			} else {
				ctx.Response().Write([]byte(`{"error":"failed to parse openapi.yaml"}`))
			}
			return nil
		}

		// Convert to JSON
		jsonData, err := sonic.Marshal(yamlData)
		if err != nil {
			log.NewHelper(logger).Errorf("Failed to convert openapi.yaml to JSON: %v", err)
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusInternalServerError)
			errorMsg := map[string]string{
				"error":   "failed to convert to JSON",
				"message": "Unable to convert openapi.yaml to JSON format",
			}
			if jsonData, err := sonic.Marshal(errorMsg); err == nil {
				ctx.Response().Write(jsonData)
			} else {
				ctx.Response().Write([]byte(`{"error":"failed to convert to JSON"}`))
			}
			return nil
		}

		ctx.Response().Header().Set("Content-Type", "application/json")
		ctx.Response().Write(jsonData)
		return nil
	})
}
