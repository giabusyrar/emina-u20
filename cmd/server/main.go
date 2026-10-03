package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/config"
	goplatform "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/platform"

	// Customer assessments
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment"

	// Form Engine
	formCache "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/adapters/cache"
	formControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/adapters/controllers"
	formScoreEngine "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/adapters/scoreengine"
	formVisionClient "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/adapters/visionclient"
	formDomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
	formEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/entity"
	formRepo "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/repository"
	formService "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/service"

	// Score Engine
	scoreCache "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/adapters/cache"
	scoreControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/adapters/controllers"
	scoreVisionProvider "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/adapters/visionprovider"
	scoreDomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	scoreEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/entity"
	scoreRepo "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/repository"
	scoreService "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/service"

	// Match Engine
	matchCache "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/cache"
	matchControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/controllers"
	matchObjectStore "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/objectstore"
	matchTryon "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/adapters/tryonclient"
	matchEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
	matchRepo "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/repository"
	matchService "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/service"

	// Colour Engine (WCPA personal colour analysis + photo try-on)
	colourControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/adapters/controllers"
	colourWorker "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/adapters/workerclient"
	colourDomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
	colourEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/entity"
	colourRepo "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/repository"
	colourService "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/service"

	// Vision Engine
	visionControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/adapters/controllers"
	visionCatalog "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/catalog"
	visionInference "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/inference"
	visionOrchestrator "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/orchestrator"
	visionWorkers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/workers"

	// Face Architecture
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch"
	faceHead "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/headclient"
	faceMeasure "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
	faceProfile "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"

	// Conversation flows (read by apps/conversation-engine)
	flowControllers "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/adapters/controllers"
	flowDomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/domain"
	flowEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/entity"
	flowRepo "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/repository"
	flowService "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/service"
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
		&matchEntity.ConflictMatrixRule{},
		&matchEntity.ProductGroup{},
		&matchEntity.Shade{},
		&matchEntity.ShadeAsset{},
		&faceProfile.FaceArchitectureProfile{},
		&colourEntity.ShadeColorimetry{},
		&assessment.Record{},
		&flowEntity.Flow{},
	); err != nil {
		deps.Log.Error("automigrate failed", "err", err)
	}

	// Score Engine
	scoreC := scoreCache.NewScoreCache(rdb, cfg.ScoreCacheTTL)
	sRepo := scoreRepo.NewRulesetRepo(db, scoreC)
	// One brand resolver (ref_brands id or code) for score, form and match.
	brands := brand.NewResolver(db)
	sService := scoreService.NewScoreService(sRepo).WithBrands(brands)
	scoreHandler := scoreControllers.NewScoreHandler(sService)

	// Form Engine -- calls the Score Engine above in-process.
	formC := formCache.NewSurveyCache(rdb, cfg.FormCacheTTL)
	fRepo := formRepo.NewSurveyRepo(db)
	scoreEngine := formScoreEngine.New(sService, cfg.ScoreClientTimeout)
	paradevClient := formVisionClient.New(cfg.ParadevGatewayURL, cfg.ParadevGatewayAPIKey, cfg.ParadevTimeout)
	fService := formService.NewSurveyService(fRepo, formC, scoreEngine, paradevClient).WithBrands(brands)
	// Every successful survey evaluation is stored as a customer assessment.
	assessmentStore := assessment.NewGormStore(db)
	formHandler := formControllers.NewSurveyHandler(fService).WithRecorder(assessment.NewRecorder(assessmentStore))

	// Unified evaluate (Fase 3+4): Score Engine now owns the whole evaluate
	// pipeline, reaching into Form Engine's survey lookup and a vision
	// provider registry — currently just Paradev, wrapped around the same
	// visionclient.Client the pre-Fase-5 form-engine orchestration still uses.
	visionProviderRegistry := scoreDomain.NewVisionProviderRegistry(scoreVisionProvider.NewParadev(paradevClient))
	sService.WithEvaluateV2Deps(fService, visionProviderRegistry)
	evaluateV2Handler := scoreControllers.NewEvaluateV2Handler(sService)

	// Match Engine
	matchC := matchCache.NewMatchCache(rdb, cfg.MatchCacheTTL)
	mRepo := matchRepo.NewMatchRepo(db, matchC)
	mService := matchService.NewMatchService(mRepo, matchTryon.New(cfg.TryonEngineURL)).WithBrands(brands)
	presigner, err := matchObjectStore.New(matchObjectStore.Config{
		Endpoint:        cfg.S3Endpoint,
		Bucket:          cfg.S3Bucket,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		Region:          cfg.S3Region,
		TTL:             cfg.S3PresignTTL,
	})
	if err != nil {
		return fmt.Errorf("object storage: %w", err)
	}
	if presigner == nil {
		deps.Log.Warn("object storage not configured; shade asset links unavailable (503)")
	}
	matchHandler := matchControllers.NewMatchHandler(mService, presigner)

	// Vision Engine Orchestrator
	visionReferenceCatalog := visionCatalog.NewCachedCatalog(
		visionCatalog.NewReferenceClient(cfg.ReferenceServiceURL, cfg.ReferenceClientTimeout),
		cfg.VisionCatalogTTL,
	)
	// No capability worker unless VISION_CAPABILITY_URL names one: every
	// planned capability is then reported unscored (orchestrator warnings).
	var visionCapabilities visionInference.CapabilityClient
	if cfg.VisionCapabilityURL != "" {
		visionCapabilities = visionInference.NewHTTPCapabilityClient(cfg.VisionCapabilityURL, cfg.VisionDispatchTimeout)
	} else {
		deps.Log.Warn("VISION_CAPABILITY_URL not set; skin conditions are reported unscored (no capability worker)")
	}
	visionOrch := visionOrchestrator.NewVisionOrchestrator(
		visionWorkers.NewSegmenterWorkerWithURL(cfg.SkinWorkerURL, cfg.SegmenterTimeout),
		visionCatalog.NewResolver(visionReferenceCatalog),
		visionCapabilities,
		cfg.VisionDAGTimeout,
	)
	visionHandler := visionControllers.NewVisionController(visionOrch, visionReferenceCatalog)

	// Face Architecture (measurements from the face worker, per-brand profile)
	faceProfiles := faceProfile.NewGormStore(db)
	faceArchHandler := facearch.NewHandler(
		faceMeasure.New(cfg.FaceWorkerURL, cfg.FaceMeasureTimeout),
		faceProfiles,
	)
	faceHeadHandler := facearch.NewHeadHandler(
		faceHead.New(cfg.FaceWorkerURL, cfg.FaceHeadTimeout),
		faceProfiles,
		cfg.FaceHeadMaxUploadBytes,
	)
	// Conversation flows: which engines apps/conversation-engine runs after a
	// survey. Read by it over HTTP; nothing here executes a flow.
	flowHandler := flowControllers.NewHandler(flowService.NewService(
		flowRepo.NewGormStore(db),
		func(ctx context.Context, brandID, appID, code string) (flowDomain.QuestionTypes, error) {
			m, err := fService.GetSurvey(ctx, brandID, appID, code)
			if errors.Is(err, formDomain.ErrNotFound) {
				return nil, flowService.ErrSurveyNotFound
			}
			if err != nil {
				return nil, err
			}
			types := flowDomain.QuestionTypes{}
			for _, q := range m.GetAllQuestions() {
				types[q.Name] = q.Type
			}
			return types, nil
		},
		func(ctx context.Context, brandID, appID string) (bool, error) {
			_, err := faceProfiles.Active(ctx, brandID, appID)
			if errors.Is(err, faceProfile.ErrNoActiveProfile) {
				return false, nil
			}
			return err == nil, err
		},
		func(ctx context.Context, ref string) (string, error) {
			b, err := brands.Resolve(ctx, ref)
			if errors.Is(err, brand.ErrUnknown) || errors.Is(err, brand.ErrRequired) {
				return "", fmt.Errorf("%w: %v", flowService.ErrUnknownBrand, err)
			}
			return b.ID, err
		},
	))

	// Colour Engine. A missing or invalid WCPA config disables only these
	// routes (503 with the reason), not the whole service.
	var colourHandler *colourControllers.Handler
	colourCfg, colourCfgErr := colourDomain.LoadConfig(cfg.WCPAConfigPath)
	if colourCfgErr != nil {
		deps.Log.Warn("colour engine disabled", "err", colourCfgErr)
	} else {
		colourHandler = colourControllers.NewHandler(colourService.New(
			colourCfg,
			colourService.NewCachedStore(colourRepo.New(db), cfg.ColourCatalogTTL),
			colourWorker.New(cfg.ColourWorkerURL, cfg.ColourWorkerTimeout),
			cfg.ColourDebugEnabled,
		), cfg.ColourMaxImageBytes)
	}

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
		g.POST("/evaluate/:code", evaluateV2Handler.Evaluate)
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
		g.POST("/face-architecture/:brandId/:applicationId/head", faceHeadHandler.Head)
	}
	mountVisionRoutes(e.Group("/core/vision-engine"))
	mountVisionRoutes(e.Group("/api/v1/vision"))
	mountVisionRoutes(e.Group("/api/vision"))

	// --- COLOUR ENGINE (WCPA) ---
	mountColourRoutes := func(g *echo.Group) {
		if colourHandler == nil {
			disabled := colourControllers.Disabled(colourCfgErr)
			g.GET("/health", colourControllers.DisabledHealth(serviceName, colourCfgErr))
			g.POST("/analyze", disabled)
			g.POST("/tryon", disabled)
			g.GET("/catalog", disabled)
			g.GET("/quadrants", disabled)
			return
		}
		g.GET("/health", health)
		g.POST("/analyze", colourHandler.Analyze)
		g.POST("/tryon", colourHandler.TryOn)
		g.GET("/catalog", colourHandler.Catalog)
		g.GET("/quadrants", colourHandler.Quadrants)
	}
	mountColourRoutes(e.Group("/core/colour-engine"))
	mountColourRoutes(e.Group("/api/colour"))

	// Customer assessments (stored by the form evaluate routes above)
	assessmentHandler := assessment.NewHandler(assessmentStore, int(cfg.AssessmentListMaxLimit))
	assessmentHandler.Mount(e.Group("/core/assessments"))
	assessmentHandler.Mount(e.Group("/api/assessments"))

	// Conversation flows
	flowHandler.Mount(e.Group("/core/conversation-flows"))
	flowHandler.Mount(e.Group("/api/conversation-flows"))

	return goplatform.Run(ctx, e, ":"+cfg.Port, cfg.ShutdownTimeout)
}
