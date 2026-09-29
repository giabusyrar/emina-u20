package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/packages/goplatform"

	// Form Engine
	formCache "github.com/experience-gateway/apps/core-engine/internal/form/adapters/cache"
	formControllers "github.com/experience-gateway/apps/core-engine/internal/form/adapters/controllers"
	formScoreEngine "github.com/experience-gateway/apps/core-engine/internal/form/adapters/scoreengine"
	formVisionClient "github.com/experience-gateway/apps/core-engine/internal/form/adapters/visionclient"
	formEntity "github.com/experience-gateway/apps/core-engine/internal/form/entity"
	formRepo "github.com/experience-gateway/apps/core-engine/internal/form/repository"
	formService "github.com/experience-gateway/apps/core-engine/internal/form/service"

	// Score Engine
	scoreCache "github.com/experience-gateway/apps/core-engine/internal/score/adapters/cache"
	scoreControllers "github.com/experience-gateway/apps/core-engine/internal/score/adapters/controllers"
	scoreEntity "github.com/experience-gateway/apps/core-engine/internal/score/entity"
	scoreRepo "github.com/experience-gateway/apps/core-engine/internal/score/repository"
	scoreService "github.com/experience-gateway/apps/core-engine/internal/score/service"

	// Match Engine
	matchCache "github.com/experience-gateway/apps/core-engine/internal/match/adapters/cache"
	matchControllers "github.com/experience-gateway/apps/core-engine/internal/match/adapters/controllers"
	matchEntity "github.com/experience-gateway/apps/core-engine/internal/match/entity"
	matchRepo "github.com/experience-gateway/apps/core-engine/internal/match/repository"
	matchService "github.com/experience-gateway/apps/core-engine/internal/match/service"

	// Vision Engine
	visionControllers "github.com/experience-gateway/apps/core-engine/internal/vision/adapters/controllers"
	visionCatalog "github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	visionInference "github.com/experience-gateway/apps/core-engine/internal/vision/inference"
	visionOrchestrator "github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	visionWorkers "github.com/experience-gateway/apps/core-engine/internal/vision/workers"

	// Face Architecture
	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	faceMeasure "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	faceProfile "github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

const serviceName = "core-engine"

func main() {
	if err := run(); err != nil {
		slog.Error(serviceName+" exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := goplatform.Bootstrap(ctx, serviceName, cfg.CommonConfig)
	if err != nil {
		return err
	}
	defer deps.Close()
	db, rdb := deps.DB, deps.Redis

	// AutoMigrate at error-level logging, as before. Its error used to be
	// discarded; it is now logged. It stays non-fatal to preserve the previous
	// startup behaviour; making it fatal is a separate decision, because the
	// schema is shared with Seagull-gateway (docs/SHARED-DATABASE.md).
	if err := db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Error)}).AutoMigrate(
		&formEntity.SurveyDefinition{},
		&scoreEntity.ScoreRuleset{},
		&matchEntity.ProductCatalogItem{},
		&matchEntity.ConflictMatrixRule{},
		&matchEntity.ProductGroup{},
		&matchEntity.Shade{},
		&matchEntity.ShadeAsset{},
		&faceProfile.FaceArchitectureProfile{},
	); err != nil {
		deps.Log.Error("automigrate failed", "err", err)
	}

	// Score Engine
	scoreC := scoreCache.NewScoreCache(rdb, cfg.ScoreCacheTTL)
	sRepo := scoreRepo.NewRulesetRepo(db, scoreC)
	sService := scoreService.NewScoreService(sRepo)
	scoreHandler := scoreControllers.NewScoreHandler(sService)

	// Form Engine -- calls the Score Engine above in-process.
	formC := formCache.NewSurveyCache(rdb, cfg.FormCacheTTL)
	fRepo := formRepo.NewSurveyRepo(db)
	scoreEngine := formScoreEngine.New(sService, cfg.ScoreClientTimeout)
	paradevClient := formVisionClient.New(cfg.ParadevGatewayURL, cfg.ParadevGatewayAPIKey, cfg.ParadevTimeout)
	fService := formService.NewSurveyService(fRepo, formC, scoreEngine, paradevClient)
	formHandler := formControllers.NewSurveyHandler(fService)

	// Match Engine
	matchC := matchCache.NewMatchCache(rdb, cfg.MatchCacheTTL)
	mRepo := matchRepo.NewMatchRepo(db, matchC)
	mService := matchService.NewMatchService(mRepo, cfg.TryonEngineURL)
	matchHandler := matchControllers.NewMatchHandler(mService)

	// Vision Engine Orchestrator
	visionReferenceCatalog := visionCatalog.NewCachedCatalog(
		visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL, cfg.ReferenceClientTimeout),
		cfg.VisionCatalogTTL,
	)
	visionOrch := visionOrchestrator.NewVisionOrchestrator(
		visionWorkers.NewSegmenterWorkerWithURL(cfg.VisionAIWorkerURL, cfg.SegmenterTimeout),
		visionWorkers.NewOcclusionWorker(),
		visionCatalog.NewResolver(visionReferenceCatalog),
		visionInference.NewHTTPCapabilityClient(cfg.VisionAIWorkerURL, cfg.VisionDispatchTimeout),
		cfg.VisionDAGTimeout,
	)
	visionHandler := visionControllers.NewVisionController(visionOrch, visionReferenceCatalog)

	// Face Architecture (measurements from the ai-worker, per-brand profile)
	faceArchHandler := facearch.NewHandler(
		faceMeasure.New(cfg.VisionAIWorkerURL, cfg.FaceMeasureTimeout),
		faceProfile.NewGormStore(db),
	)

	e := goplatform.NewEcho(goplatform.ServerOptions{
		Logger:           deps.Log,
		CORSAllowOrigins: cfg.CORSAllowOrigins,
		CORSAllowHeaders: cfg.CORSAllowHeaders,
	})

	// One health handler for the service and every engine prefix. It reports
	// real dependency state instead of a static "active" map.
	health := deps.HealthHandler()
	e.GET("/health", health)

	// --- FORM ENGINE ---
	mountFormRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.GET("/survey", formHandler.List)
		g.GET("/survey/:code", formHandler.Get)
		g.POST("/survey", formHandler.Save)
		g.PUT("/survey/:code", formHandler.Save)
		g.POST("/evaluate", formHandler.Evaluate)
		g.POST("/survey/:code/evaluate", formHandler.Evaluate)
		g.POST("/survey/:code/evaluate-with-photos", formHandler.EvaluateWithPhotos)
	}
	mountFormRoutes(e.Group("/core/form-engine"))
	mountFormRoutes(e.Group("/api/forms"))
	mountFormRoutes(e.Group("/api/form"))
	mountFormRoutes(e.Group("/v1"))

	// --- SCORE ENGINE ---
	mountScoreRoutes := func(g *echo.Group) {
		g.GET("/health", health)
		g.POST("/evaluate", scoreHandler.Evaluate)
		g.POST("/simulate", scoreHandler.Simulate)
		g.GET("/rulesets", scoreHandler.ListRulesets)
		g.GET("/rulesets/active", scoreHandler.GetActiveRuleset)
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
		g.GET("/health", health)
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
		g.GET("/health", health)
		g.POST("/analyze-image", visionHandler.AnalyzeImage)
		g.POST("/evaluate", visionHandler.AnalyzeImage)
		g.GET("/config/:brandId/:applicationId", visionHandler.GetConfig)
		g.GET("/registry", visionHandler.GetRegistry)
		g.POST("/face-architecture/:brandId/:applicationId", faceArchHandler.Analyze)
	}
	mountVisionRoutes(e.Group("/core/vision-engine"))
	mountVisionRoutes(e.Group("/api/v1/vision"))
	mountVisionRoutes(e.Group("/api/vision"))

	return goplatform.Run(ctx, e, ":"+cfg.Port, cfg.ShutdownTimeout)
}
