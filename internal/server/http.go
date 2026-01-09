// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/middleware"
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
func NewHTTPServer(c *conf.Server, logger log.Logger, bootstrap *conf.Bootstrap) *khttp.Server {
	rateLimiter := middleware.NewRateLimiter()
	statsService := service.NewUsageStatsService()

	var ipLocationService *service.IPLocationService
	if bootstrap != nil && bootstrap.ApiManagement != nil && bootstrap.ApiManagement.IpLocation != nil {
		dbPath := bootstrap.ApiManagement.IpLocation.GeoipDbPath
		if dbPath != "" {
			var err error
			ipLocationService, err = service.NewIPLocationService(dbPath)
			if err != nil {
				log.NewHelper(logger).Warnf("Failed to initialize IP location service: %v", err)
			} else {
				log.NewHelper(logger).Info("IP location service initialized")
			}
		}
	}

	var ipWhitelistChecker *middleware.IPWhitelistChecker
	if bootstrap != nil && bootstrap.ApiManagement != nil && bootstrap.ApiManagement.IpWhitelist != nil {
		whitelistIPs := bootstrap.ApiManagement.IpWhitelist.Ips
		if len(whitelistIPs) > 0 {
			ipWhitelistChecker = middleware.NewIPWhitelistChecker(whitelistIPs)
			log.NewHelper(logger).Infof("IP whitelist initialized with %d entries", len(whitelistIPs))
		}
	}

	ctx := context.Background()
	statsService.StartLogProcessor(ctx)

	archiveService := service.NewArchiveUsageService()
	scheduler := service.NewScheduler(statsService, archiveService)
	scheduler.Start(ctx)
	log.NewHelper(logger).Info("Background scheduler started")

	testMode := false
	if bootstrap != nil && bootstrap.ApiManagement != nil && bootstrap.ApiManagement.GetTestMode() {
		log.NewHelper(logger).Warn("WARNING: Test mode is enabled! All authentication and rate limiting restrictions are disabled.")
		testMode = bootstrap.ApiManagement.GetTestMode()
	}

	opts := []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
			middleware.SelectiveAuthAndRateLimitMiddleware(rateLimiter, statsService, ipLocationService, ipWhitelistChecker, testMode),
		),
	}

	enableCors := c.GetEnableCors()
	corsMiddleware := configureCORS(enableCors)

	if enableCors && corsMiddleware != nil {
		opts = append(opts, khttp.Filter(createCORSFilter(corsMiddleware)))
	}

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

	if httpSrv := srv.Server; httpSrv != nil {
		httpSrv.ReadTimeout = 30 * time.Second      
		httpSrv.WriteTimeout = 30 * time.Second     
		httpSrv.IdleTimeout = 120 * time.Second     
		httpSrv.ReadHeaderTimeout = 5 * time.Second 

		httpSrv.SetKeepAlivesEnabled(true)
		httpSrv.MaxHeaderBytes = 1 << 20 
	}

	explorerService := service.NewExplorerService()
	v1.RegisterExplorerHTTPServer(srv, explorerService)

	contractInfoService := service.NewContractInfoService()
	v1.RegisterContractInfoServiceHTTPServer(srv, contractInfoService)

	apiTokenService := service.NewAPITokenService()
	v1.RegisterAPITokenServiceHTTPServer(srv, apiTokenService)

	adminService := service.NewAdminService()
	v1.RegisterAdminServiceHTTPServer(srv, adminService)

	addressLabelService := service.NewAddressLabelService()
	v1.RegisterAddressLabelServiceHTTPServer(srv, addressLabelService)

	registerExportRoutes(srv, apiTokenService, logger)

	if c.EnableSwagger {
		registerSwaggerUI(srv, logger, corsMiddleware, enableCors)
		registerSwaggerRoutes(srv, logger)
	} else {
		log.NewHelper(logger).Warn("Swagger is disabled in configuration")
	}

	return srv
}

// configureCORS creates a CORS middleware if CORS is enabled.
// CORS handling notes:
// - Production: CORS is handled by Nginx and Envoy, no need to handle in code (default enable_cors=false)
// - Development: If direct access to application service is needed (e.g., Swagger UI), set enable_cors=true to enable CORS handling in code
// Note: Enabling CORS at multiple layers may cause configuration conflicts, it's recommended to handle CORS at only one layer
//
// Parameters:
//   - enableCors: Whether to enable CORS handling
//
// Returns:
//   - func(http.Handler) http.Handler: CORS middleware function, or nil if CORS is disabled
func configureCORS(enableCors bool) func(http.Handler) http.Handler {
	if !enableCors {
		return nil
	}

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
			"Accept",
			"Origin",
			// Note: Accept, Origin, and Access-Control-Request-* headers are simple headers
			// and don't need to be explicitly allowed
		}),
		handlers.MaxAge(86400), // Cache preflight requests for 24 hours
	)

	if allowCredentials {
		corsOpts = append(corsOpts, handlers.AllowCredentials())
	}

	// Create CORS middleware factory (reusable)
	// This creates a single CORS middleware function that can wrap any handler
	return handlers.CORS(corsOpts...)
}

// createCORSFilter creates a CORS filter that explicitly handles OPTIONS preflight requests.
// This ensures CORS headers are set even when routes don't match.
//
// Parameters:
//   - corsMiddleware: The CORS middleware function created by configureCORS
//
// Returns:
//   - func(http.Handler) http.Handler: A filter function that handles CORS
func createCORSFilter(corsMiddleware func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OPTIONS preflight requests explicitly
			// This is critical because browsers send OPTIONS requests before the actual request
			// and these must return proper CORS headers even if the route doesn't exist
			if r.Method == "OPTIONS" {
				origin := r.Header.Get("Origin")
				if origin != "" {
					// Set CORS headers for preflight request
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
					w.Header().Set("Access-Control-Max-Age", "86400")
					w.Header().Set("Access-Control-Allow-Credentials", "false")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// For non-OPTIONS requests, apply CORS middleware and continue to next handler
			// This ensures CORS headers are added to all responses
			corsMiddleware(next).ServeHTTP(w, r)
		})
	}
}

// registerSwaggerUI registers Swagger UI routes using the Route system.
// This is more reliable than using Filters and avoids duplicate route registration.
//
// Parameters:
//   - srv: The HTTP server instance
//   - logger: Logger instance for logging
//   - corsMiddleware: CORS middleware function (can be nil if CORS is disabled)
//   - enableCors: Whether CORS is enabled
func registerSwaggerUI(srv *khttp.Server, logger log.Logger, corsMiddleware func(http.Handler) http.Handler, enableCors bool) {
	log.NewHelper(logger).Info("Registering Swagger UI routes...")

	// Create swagger handler
	swaggerHandler := httpSwagger.Handler(
		httpSwagger.URL("/openapi.json"), // Point to our OpenAPI JSON file
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	)

	// Wrap swagger handler with CORS middleware if enabled
	var swaggerHandlerToUse http.Handler = swaggerHandler
	if enableCors && corsMiddleware != nil {
		swaggerHandlerToUse = corsMiddleware(swaggerHandler)
	}

	r := srv.Route("/")

	// Handle /swagger redirect to /swagger/
	r.GET("/swagger", func(ctx khttp.Context) error {
		log.NewHelper(logger).Info("Route /swagger called, redirecting to /swagger/")
		http.Redirect(ctx.Response(), ctx.Request(), "/swagger/", http.StatusMovedPermanently)
		return nil
	})

	// Register /swagger/ route - use http.Handler adapter
	// Since Kratos Route doesn't support wildcards, we need to handle all swagger paths
	// The swagger handler will handle sub-paths internally via its own routing
	r.GET("/swagger/", func(ctx khttp.Context) error {
		log.NewHelper(logger).Infof("Route /swagger/ called, path=%s", ctx.Request().URL.Path)
		swaggerHandlerToUse.ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	})

	// Register common swagger static file paths
	// These are typically requested by the swagger UI
	swaggerPaths := []string{
		"/swagger/index.html",
		"/swagger/swagger-ui.css",
		"/swagger/swagger-ui-bundle.js",
		"/swagger/swagger-ui-standalone-preset.js",
		"/swagger/favicon-32x32.png",
		"/swagger/favicon-16x16.png",
	}

	for _, swaggerPath := range swaggerPaths {
		r.GET(swaggerPath, func(ctx khttp.Context) error {
			log.NewHelper(logger).Infof("Route %s called", ctx.Request().URL.Path)
			swaggerHandlerToUse.ServeHTTP(ctx.Response(), ctx.Request())
			return nil
		})
	}

	log.NewHelper(logger).Info("Swagger UI routes registered successfully")
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

// registerExportRoutes registers custom HTTP routes for file exports.
// Note: Export usage logs endpoint is now handled by proto-generated route.
// The endpoint /api/v1/export/logs returns CSV data in JSON response format
// (same as ExportIPList), not Excel file download.
func registerExportRoutes(srv *khttp.Server, apiTokenService *service.APITokenService, logger log.Logger) {
	// Export routes are now handled by proto-generated routes
	_ = srv // Suppress unused parameter warning
	_ = apiTokenService
	_ = logger
}
