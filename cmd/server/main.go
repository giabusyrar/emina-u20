package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/core-engine/pkg/config"

	// Form Engine
	formCache "github.com/experience-gateway/apps/core-engine/pkg/form/adapters/cache"
	formControllers "github.com/experience-gateway/apps/core-engine/pkg/form/adapters/controllers"
	formScoreClient "github.com/experience-gateway/apps/core-engine/pkg/form/adapters/scoreclient"
	formEntity "github.com/experience-gateway/apps/core-engine/pkg/form/entity"
	formRepo "github.com/experience-gateway/apps/core-engine/pkg/form/repository"
	formService "github.com/experience-gateway/apps/core-engine/pkg/form/service"

	// Score Engine
	scoreCache "github.com/experience-gateway/apps/core-engine/pkg/score/adapters/cache"
	scoreControllers "github.com/experience-gateway/apps/core-engine/pkg/score/adapters/controllers"
	scoreEntity "github.com/experience-gateway/apps/core-engine/pkg/score/entity"
	scoreRepo "github.com/experience-gateway/apps/core-engine/pkg/score/repository"
	scoreService "github.com/experience-gateway/apps/core-engine/pkg/score/service"

	// Match Engine
	matchCache "github.com/experience-gateway/apps/core-engine/pkg/match/adapters/cache"
	matchControllers "github.com/experience-gateway/apps/core-engine/pkg/match/adapters/controllers"
	matchEntity "github.com/experience-gateway/apps/core-engine/pkg/match/entity"
	matchRepo "github.com/experience-gateway/apps/core-engine/pkg/match/repository"
	matchService "github.com/experience-gateway/apps/core-engine/pkg/match/service"

	// Vision Engine
	visionControllers "github.com/experience-gateway/apps/core-engine/pkg/vision/adapters/controllers"
	visionCatalog "github.com/experience-gateway/apps/core-engine/pkg/vision/catalog"
	visionClient "github.com/experience-gateway/apps/core-engine/pkg/vision/client"
	visionInference "github.com/experience-gateway/apps/core-engine/pkg/vision/inference"
	visionOrchestrator "github.com/experience-gateway/apps/core-engine/pkg/vision/orchestrator"
	visionWorkers "github.com/experience-gateway/apps/core-engine/pkg/vision/workers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Core Engine: config warning: %v, using defaults", err)
	}

	// 1. Shared PostgreSQL Connection Pool
	var db *gorm.DB
	if cfg.DatabaseURL != "" {
		gormLogger := gormlogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			gormlogger.Config{
				SlowThreshold:             2 * time.Second,
				LogLevel:                  gormlogger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			},
		)

		openedDB, dbErr := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{Logger: gormLogger})
		if dbErr != nil {
			log.Printf("Core Engine: PostgreSQL connection notice (%v), running with in-memory fallback", dbErr)
		} else {
			db = openedDB
			log.Println("Core Engine: successfully connected to PostgreSQL via GORM")

			// Auto-migrate all 4 engine entities
			_ = db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Error)}).AutoMigrate(
				&formEntity.SurveyDefinition{},
				&scoreEntity.ScoreRuleset{},
				&matchEntity.ProductCatalogItem{},
				&matchEntity.ConflictMatrixRule{},
				&matchEntity.ProductGroup{},
				&matchEntity.Shade{},
				&matchEntity.ShadeAsset{},
			)
		}
	}

	// 2. Shared Redis Client Pool
	var rdb *redis.Client
	if cfg.RedisURL != "" {
		opt, parseErr := redis.ParseURL(cfg.RedisURL)
		if parseErr != nil {
			rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisURL})
		} else {
			rdb = redis.NewClient(opt)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
			log.Printf("Core Engine: Redis notice (%v), cache disabled", pingErr)
			rdb = nil
		} else {
			log.Println("Core Engine: successfully connected to Redis")
		}
	}

	// 3. Initialize Engine Services
	// Form Engine
	formC := formCache.NewSurveyCache(rdb, 15*time.Minute)
	fRepo := formRepo.NewSurveyRepo(db)
	// scoreclient appends /api/scoring/evaluate, so the base must be the host only.
	scoreClient := formScoreClient.New("http://127.0.0.1:" + cfg.Port)
	fService := formService.NewSurveyService(fRepo, formC, scoreClient)
	formHandler := formControllers.NewSurveyHandler(fService)

	// Score Engine
	scoreC := scoreCache.NewScoreCache(rdb, 15*time.Minute)
	sRepo := scoreRepo.NewRulesetRepo(db, scoreC)
	sService := scoreService.NewScoreService(sRepo)
	scoreHandler := scoreControllers.NewScoreHandler(sService)

	// Match Engine
	matchC := matchCache.NewMatchCache(rdb, 15*time.Minute)
	mRepo := matchRepo.NewMatchRepo(db, matchC)
	mService := matchService.NewMatchService(mRepo)
	matchHandler := matchControllers.NewMatchHandler(mService)

	// Vision Engine Orchestrator
	visionBridge := visionClient.NewEngineBridge(
		"http://127.0.0.1:"+cfg.Port+"/core/score-engine",
		"http://127.0.0.1:"+cfg.Port+"/core/match-engine",
	)
	visionReferenceCatalog := visionCatalog.NewCachedCatalog(
		visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL),
		time.Minute,
	)
	visionOrch := visionOrchestrator.NewVisionOrchestrator(
		visionWorkers.NewSegmenterWorkerWithURL(cfg.VisionAIWorkerURL),
		visionWorkers.NewOcclusionWorker(),
		visionCatalog.NewResolver(visionReferenceCatalog),
		visionInference.NewHTTPCapabilityClient(cfg.VisionAIWorkerURL),
	)
	visionHandler := visionControllers.NewVisionController(visionOrch, visionReferenceCatalog, visionBridge)

	// 4. Echo HTTP Server Setup
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"*"},
	}))

	// Health Checks
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":  "healthy",
			"service": "core-engine",
			"engines": map[string]string{
				"form-engine":   "active",
				"score-engine":  "active",
				"match-engine":  "active",
				"vision-engine": "active",
			},
		})
	})

	// 5. Mount Engine Route Namespaces

	// --- FORM ENGINE ---
	mountFormRoutes := func(g *echo.Group) {
		g.GET("/health", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "form-engine"})
		})
		g.GET("/survey", formHandler.List)
		g.GET("/survey/:code", formHandler.Get)
		g.POST("/survey", formHandler.Save)
		g.PUT("/survey/:code", formHandler.Save)
		g.POST("/evaluate", formHandler.Evaluate)
		g.POST("/survey/:code/evaluate", formHandler.Evaluate)
	}
	mountFormRoutes(e.Group("/core/form-engine"))
	mountFormRoutes(e.Group("/api/forms"))
	mountFormRoutes(e.Group("/api/form"))
	mountFormRoutes(e.Group("/v1"))

	// --- SCORE ENGINE ---
	mountScoreRoutes := func(g *echo.Group) {
		g.GET("/health", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "score-engine"})
		})
		g.POST("/evaluate", scoreHandler.Evaluate)
		g.POST("/simulate", scoreHandler.Simulate)
		g.GET("/rulesets", scoreHandler.ListRulesets)
		g.GET("/rulesets/:id", scoreHandler.GetRuleset)
		g.POST("/rulesets", scoreHandler.CreateRuleset)
		g.PUT("/rulesets/:id", scoreHandler.UpdateRuleset)
		g.DELETE("/rulesets/:id", scoreHandler.DeleteRuleset)
	}
	mountScoreRoutes(e.Group("/core/score-engine"))
	mountScoreRoutes(e.Group("/api/scoring"))
	mountScoreRoutes(e.Group("/api/score"))

	// --- MATCH ENGINE ---
	mountMatchRoutes := func(g *echo.Group) {
		g.GET("/health", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "match-engine"})
		})
		g.POST("/evaluate", matchHandler.Evaluate)
		g.GET("/products", matchHandler.ListProducts)
		g.GET("/conflicts", matchHandler.ListConflictRules)
		g.POST("/conflicts", matchHandler.CreateConflictRule)
		g.PUT("/conflicts", matchHandler.UpdateConflictRule)
		g.DELETE("/conflicts", matchHandler.DeleteConflictRule)
		g.GET("/product-groups", matchHandler.ListProductGroups)
		g.POST("/product-groups", matchHandler.CreateProductGroup)
		g.PUT("/product-groups", matchHandler.UpdateProductGroup)
		g.DELETE("/product-groups", matchHandler.DeleteProductGroup)
		g.GET("/shades", matchHandler.ListShades)
		g.POST("/shades", matchHandler.CreateShade)
		g.PUT("/shades", matchHandler.UpdateShade)
		g.DELETE("/shades", matchHandler.DeleteShade)
		g.GET("/shade-assets/:id", matchHandler.GetShadeAsset)
		g.POST("/shades/:id/extraction-callback", matchHandler.ExtractionCallback)
	}
	mountMatchRoutes(e.Group("/core/match-engine"))
	mountMatchRoutes(e.Group("/api/matching"))
	mountMatchRoutes(e.Group("/api/match"))

	// --- VISION ENGINE ORCHESTRATION ---
	mountVisionRoutes := func(g *echo.Group) {
		g.GET("/health", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "vision-engine"})
		})
		g.POST("/analyze-image", visionHandler.AnalyzeImage)
		g.POST("/evaluate", visionHandler.AnalyzeImage)
		g.GET("/config/:brandId/:applicationId", visionHandler.GetConfig)
		g.GET("/registry", visionHandler.GetRegistry)
	}
	mountVisionRoutes(e.Group("/core/vision-engine"))
	mountVisionRoutes(e.Group("/api/v1/vision"))
	mountVisionRoutes(e.Group("/api/vision"))

	log.Printf("Unified Core Engine Server starting on HTTP port :%s ...", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Core Engine failed to start: %v", err)
	}
}
