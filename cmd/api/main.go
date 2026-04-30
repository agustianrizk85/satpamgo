package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"satpam-go/internal/apierrorlogs"
	"satpam-go/internal/appversions"
	"satpam-go/internal/attendanceconfig"
	"satpam-go/internal/attendances"
	"satpam-go/internal/auth"
	"satpam-go/internal/communication"
	"satpam-go/internal/config"
	"satpam-go/internal/database"
	"satpam-go/internal/facility"
	"satpam-go/internal/httpapi"
	"satpam-go/internal/leaverequests"
	"satpam-go/internal/media"
	"satpam-go/internal/patrol"
	"satpam-go/internal/places"
	"satpam-go/internal/recentactivities"
	"satpam-go/internal/reports"
	"satpam-go/internal/roles"
	"satpam-go/internal/shifts"
	"satpam-go/internal/spotassignments"
	"satpam-go/internal/spots"
	"satpam-go/internal/tokenconfig"
	"satpam-go/internal/userplaceroles"
	"satpam-go/internal/users"
	"satpam-go/internal/visitors"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbPool, err := database.NewPostgresPool(context.Background(), cfg)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer dbPool.Close()

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer)
	mediaService := media.NewService(cfg.StorageRoot)
	authRepo := auth.NewRepository(dbPool)
	userRepo := users.NewRepository(dbPool)
	roleRepo := roles.NewRepository(dbPool)
	placeRepo := places.NewRepository(dbPool)
	shiftRepo := shifts.NewRepository(dbPool)
	userPlaceRoleRepo := userplaceroles.NewRepository(dbPool)
	spotRepo := spots.NewRepository(dbPool)
	spotAssignmentRepo := spotassignments.NewRepository(dbPool)
	attendanceConfigRepo := attendanceconfig.NewRepository(dbPool)
	tokenConfigRepo := tokenconfig.NewRepository(dbPool)
	attendanceRepo := attendances.NewRepository(dbPool)
	visitorRepo := visitors.NewRepository(dbPool)
	leaveRequestRepo := leaverequests.NewRepository(dbPool)
	patrolRepo := patrol.NewRepository(dbPool)
	facilityRepo := facility.NewRepository(dbPool)
	recentActivitiesRepo := recentactivities.NewRepository(dbPool)
	reportRepo := reports.NewRepository(dbPool)
	apiErrorLogRepo := apierrorlogs.NewRepository(dbPool)
	appVersionRepo := appversions.NewRepository(dbPool)
	appVersionStorage := appversions.NewStorage(cfg.StorageRoot)

	resolveTokenTTL := func(ctx context.Context) (time.Duration, time.Duration, error) {
		cfg, err := tokenConfigRepo.Get(ctx)
		if err != nil {
			if errors.Is(err, tokenconfig.ErrNotFound) {
				return 8 * time.Hour, 30 * 24 * time.Hour, nil
			}
			return 0, 0, err
		}
		return time.Duration(cfg.AccessTTLSeconds) * time.Second, time.Duration(cfg.RefreshTTLSeconds) * time.Second, nil
	}

	authHandler := auth.NewHandler(authRepo, tokenService, resolveTokenTTL)
	userHandler := users.NewHandler(userRepo, authRepo)
	roleHandler := roles.NewHandler(roleRepo, authRepo)
	placeHandler := places.NewHandler(placeRepo, authRepo)
	shiftHandler := shifts.NewHandler(shiftRepo, authRepo)
	userPlaceRoleHandler := userplaceroles.NewHandler(userPlaceRoleRepo, authRepo)
	spotHandler := spots.NewHandler(spotRepo, authRepo)
	spotAssignmentHandler := spotassignments.NewHandler(spotAssignmentRepo, authRepo)
	attendanceConfigHandler := attendanceconfig.NewHandler(attendanceConfigRepo, authRepo)
	tokenConfigHandler := tokenconfig.NewHandler(tokenConfigRepo, authRepo)
	attendanceHandler := attendances.NewHandler(attendanceRepo)
	visitorHandler := visitors.NewHandler(visitorRepo, authRepo)
	leaveRequestHandler := leaverequests.NewHandler(leaveRequestRepo)
	patrolHandler := patrol.NewHandler(patrolRepo, authRepo)
	facilityHandler := facility.NewHandler(facilityRepo, authRepo)
	mediaHandler := media.NewHandler(mediaService, cfg.UploadMaxBytes)
	recentActivitiesHandler := recentactivities.NewHandler(recentActivitiesRepo, authRepo)
	reportHandler := reports.NewHandler(reportRepo, authRepo, cfg.StorageRoot)
	communicationHandler := communication.NewHandler(communication.NewHub())
	apiErrorLogHandler := apierrorlogs.NewHandler(apiErrorLogRepo, authRepo)
	appVersionHandler := appversions.NewHandler(appVersionRepo, authRepo, appVersionStorage, cfg.AppVersionUploadMaxBytes)

	if err := mediaService.CleanupAttendanceExpired(time.Now()); err != nil {
		log.Printf("cleanup attendance photos: %v", err)
	}
	go runAttendanceCleanup(mediaService)
	configureAutoCheckoutDefaults(cfg, mediaService)
	go runAttendanceAutoCheckout(attendanceRepo, cfg)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.NewRouter(authHandler, userHandler, roleHandler, placeHandler, shiftHandler, userPlaceRoleHandler, spotHandler, spotAssignmentHandler, attendanceConfigHandler, tokenConfigHandler, attendanceHandler, visitorHandler, leaveRequestHandler, patrolHandler, facilityHandler, recentActivitiesHandler, reportHandler, mediaHandler, communicationHandler, apiErrorLogHandler, appVersionHandler, apiErrorLogRepo, tokenService, mediaService.Root()),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("satpam-go listening on http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	waitForShutdown(server, cfg.ShutdownTimeout)
}

func waitForShutdown(server *http.Server, timeout time.Duration) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func runAttendanceCleanup(mediaService *media.Service) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := mediaService.CleanupAttendanceExpired(time.Now()); err != nil {
			log.Printf("cleanup attendance photos: %v", err)
		}
	}
}

func configureAutoCheckoutDefaults(cfg config.Config, mediaService *media.Service) {
	if !cfg.AutoCheckoutEnabled {
		return
	}
	if cfg.AutoCheckoutSystemPhotoURL != "/uploads/system/attendance/check-out-by-system.svg" {
		return
	}

	if _, err := mediaService.EnsureDefaultSystemCheckoutAsset(); err != nil {
		log.Printf("ensure auto checkout asset: %v", err)
	}
}

func runAttendanceAutoCheckout(attendanceRepo *attendances.Repository, cfg config.Config) {
	if !cfg.AutoCheckoutEnabled {
		return
	}

	execute := func() {
		updated, err := attendanceRepo.AutoCheckoutDue(
			context.Background(),
			time.Now(),
			cfg.AutoCheckoutGraceMinutes,
			cfg.AutoCheckoutSystemPhotoURL,
			cfg.AutoCheckoutSystemNote,
		)
		if err != nil {
			log.Printf("auto checkout attendances: %v", err)
			return
		}
		if updated > 0 {
			log.Printf("auto checkout attendances: updated=%d", updated)
		}
	}

	execute()

	ticker := time.NewTicker(time.Duration(cfg.AutoCheckoutPollSeconds) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		execute()
	}
}
