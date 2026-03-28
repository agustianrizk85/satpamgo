package httpapi

import (
	"net/http"

	"satpam-go/internal/apierrorlogs"
	"satpam-go/internal/appversions"
	"satpam-go/internal/attendanceconfig"
	"satpam-go/internal/attendances"
	"satpam-go/internal/auth"
	"satpam-go/internal/facility"
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
	"satpam-go/internal/userplaceroles"
	"satpam-go/internal/users"
	"satpam-go/internal/visitors"
	"satpam-go/internal/web"
)

func NewRouter(authHandler *auth.Handler, userHandler *users.Handler, roleHandler *roles.Handler, placeHandler *places.Handler, shiftHandler *shifts.Handler, userPlaceRoleHandler *userplaceroles.Handler, spotHandler *spots.Handler, spotAssignmentHandler *spotassignments.Handler, attendanceConfigHandler *attendanceconfig.Handler, attendanceHandler *attendances.Handler, visitorHandler *visitors.Handler, leaveRequestHandler *leaverequests.Handler, patrolHandler *patrol.Handler, facilityHandler *facility.Handler, recentActivitiesHandler *recentactivities.Handler, reportHandler *reports.Handler, mediaHandler *media.Handler, apiErrorLogHandler *apierrorlogs.Handler, appVersionHandler *appversions.Handler, apiErrorLogRepo *apierrorlogs.Repository, tokenService *auth.TokenService, storageRoot string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		web.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "satpam-go",
		})
	})

	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.Handle("GET /api/v1/auth/me", auth.RequireAuth(tokenService, http.HandlerFunc(authHandler.Me)))
	mux.Handle("GET /api/v1/users", auth.RequireAuth(tokenService, http.HandlerFunc(userHandler.ListUsers)))
	mux.Handle("POST /api/v1/users", auth.RequireAuth(tokenService, http.HandlerFunc(userHandler.CreateUser)))
	mux.Handle("GET /api/v1/users/{userId}", auth.RequireAuth(tokenService, http.HandlerFunc(userHandler.GetUser)))
	mux.Handle("PATCH /api/v1/users/{userId}", auth.RequireAuth(tokenService, http.HandlerFunc(userHandler.PatchUser)))
	mux.Handle("DELETE /api/v1/users/{userId}", auth.RequireAuth(tokenService, http.HandlerFunc(userHandler.DeleteUser)))
	mux.Handle("GET /api/v1/roles", auth.RequireAuth(tokenService, http.HandlerFunc(roleHandler.List)))
	mux.Handle("POST /api/v1/roles", auth.RequireAuth(tokenService, http.HandlerFunc(roleHandler.Create)))
	mux.Handle("GET /api/v1/roles/{roleId}", auth.RequireAuth(tokenService, http.HandlerFunc(roleHandler.Get)))
	mux.Handle("PATCH /api/v1/roles/{roleId}", auth.RequireAuth(tokenService, http.HandlerFunc(roleHandler.Patch)))
	mux.Handle("DELETE /api/v1/roles/{roleId}", auth.RequireAuth(tokenService, http.HandlerFunc(roleHandler.Delete)))
	mux.Handle("GET /api/v1/places", auth.RequireAuth(tokenService, http.HandlerFunc(placeHandler.List)))
	mux.Handle("POST /api/v1/places", auth.RequireAuth(tokenService, http.HandlerFunc(placeHandler.Create)))
	mux.Handle("GET /api/v1/places/{placeId}", auth.RequireAuth(tokenService, http.HandlerFunc(placeHandler.Get)))
	mux.Handle("PATCH /api/v1/places/{placeId}", auth.RequireAuth(tokenService, http.HandlerFunc(placeHandler.Patch)))
	mux.Handle("DELETE /api/v1/places/{placeId}", auth.RequireAuth(tokenService, http.HandlerFunc(placeHandler.Delete)))
	mux.Handle("GET /api/v1/shifts", auth.RequireAuth(tokenService, http.HandlerFunc(shiftHandler.List)))
	mux.Handle("POST /api/v1/shifts", auth.RequireAuth(tokenService, http.HandlerFunc(shiftHandler.Create)))
	mux.Handle("GET /api/v1/shifts/{shiftId}", auth.RequireAuth(tokenService, http.HandlerFunc(shiftHandler.Get)))
	mux.Handle("PATCH /api/v1/shifts/{shiftId}", auth.RequireAuth(tokenService, http.HandlerFunc(shiftHandler.Patch)))
	mux.Handle("DELETE /api/v1/shifts/{shiftId}", auth.RequireAuth(tokenService, http.HandlerFunc(shiftHandler.Delete)))
	mux.Handle("GET /api/v1/user-place-roles", auth.RequireAuth(tokenService, http.HandlerFunc(userPlaceRoleHandler.List)))
	mux.Handle("POST /api/v1/user-place-roles", auth.RequireAuth(tokenService, http.HandlerFunc(userPlaceRoleHandler.Upsert)))
	mux.Handle("GET /api/v1/spots", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.List)))
	mux.Handle("POST /api/v1/spots", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.Create)))
	mux.Handle("GET /api/v1/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.Get)))
	mux.Handle("PATCH /api/v1/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.Patch)))
	mux.Handle("PUT /api/v1/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.Patch)))
	mux.Handle("DELETE /api/v1/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotHandler.Delete)))
	mux.Handle("GET /api/v1/spot-assignments", auth.RequireAuth(tokenService, http.HandlerFunc(spotAssignmentHandler.List)))
	mux.Handle("POST /api/v1/spot-assignments", auth.RequireAuth(tokenService, http.HandlerFunc(spotAssignmentHandler.Create)))
	mux.Handle("GET /api/v1/spot-assignments/{assignmentId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotAssignmentHandler.Get)))
	mux.Handle("PATCH /api/v1/spot-assignments/{assignmentId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotAssignmentHandler.Patch)))
	mux.Handle("DELETE /api/v1/spot-assignments/{assignmentId}", auth.RequireAuth(tokenService, http.HandlerFunc(spotAssignmentHandler.Delete)))
	mux.Handle("GET /api/v1/attendance-config", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceConfigHandler.Get)))
	mux.Handle("POST /api/v1/attendance-config", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceConfigHandler.Upsert)))
	mux.Handle("GET /api/v1/attendances", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceHandler.List)))
	mux.Handle("POST /api/v1/attendances", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceHandler.Create)))
	mux.Handle("PATCH /api/v1/attendances/{attendanceId}", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceHandler.Patch)))
	mux.Handle("DELETE /api/v1/attendances/{attendanceId}", auth.RequireAuth(tokenService, http.HandlerFunc(attendanceHandler.Delete)))
	mux.Handle("GET /api/v1/visitors", auth.RequireAuth(tokenService, http.HandlerFunc(visitorHandler.List)))
	mux.Handle("POST /api/v1/visitors", auth.RequireAuth(tokenService, http.HandlerFunc(visitorHandler.Create)))
	mux.Handle("GET /api/v1/visitors/{visitorId}", auth.RequireAuth(tokenService, http.HandlerFunc(visitorHandler.Get)))
	mux.Handle("PATCH /api/v1/visitors/{visitorId}", auth.RequireAuth(tokenService, http.HandlerFunc(visitorHandler.Patch)))
	mux.Handle("DELETE /api/v1/visitors/{visitorId}", auth.RequireAuth(tokenService, http.HandlerFunc(visitorHandler.Delete)))
	mux.Handle("GET /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.List)))
	mux.Handle("POST /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.Create)))
	mux.Handle("PATCH /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.Patch)))
	mux.Handle("GET /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.ListRoutePoints)))
	mux.Handle("POST /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.CreateRoutePoint)))
	mux.Handle("DELETE /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.DeleteRoutePoint)))
	mux.Handle("GET /api/v1/patrol/runs", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.ListRuns)))
	mux.Handle("POST /api/v1/patrol/runs", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.CreateRun)))
	mux.Handle("GET /api/v1/patrol/runs/{runId}", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.GetRun)))
	mux.Handle("PATCH /api/v1/patrol/runs/{runId}", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.PatchRun)))
	mux.Handle("DELETE /api/v1/patrol/runs/{runId}", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.DeleteRun)))
	mux.Handle("GET /api/v1/patrol/progress", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.GetProgress)))
	mux.Handle("GET /api/v1/patrol/scans", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.ListScans)))
	mux.Handle("POST /api/v1/patrol/scans", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.CreateScan)))
	mux.Handle("PATCH /api/v1/patrol/scans/{scanId}", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.PatchScan)))
	mux.Handle("DELETE /api/v1/patrol/scans/{scanId}", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.DeleteScan)))
	mux.Handle("GET /api/v1/facility/spots", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.ListSpots)))
	mux.Handle("POST /api/v1/facility/spots", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.CreateSpot)))
	mux.Handle("GET /api/v1/facility/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.GetSpot)))
	mux.Handle("PATCH /api/v1/facility/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.PatchSpot)))
	mux.Handle("DELETE /api/v1/facility/spots/{spotId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.DeleteSpot)))
	mux.Handle("GET /api/v1/facility/items", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.ListItems)))
	mux.Handle("POST /api/v1/facility/items", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.CreateItem)))
	mux.Handle("GET /api/v1/facility/items/{itemId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.GetItem)))
	mux.Handle("PATCH /api/v1/facility/items/{itemId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.PatchItem)))
	mux.Handle("DELETE /api/v1/facility/items/{itemId}", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.DeleteItem)))
	mux.Handle("GET /api/v1/facility/scans", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.ListScans)))
	mux.Handle("POST /api/v1/facility/scans", auth.RequireAuth(tokenService, http.HandlerFunc(facilityHandler.CreateScan)))
	mux.Handle("GET /api/v1/recent-activities", auth.RequireAuth(tokenService, http.HandlerFunc(recentActivitiesHandler.List)))
	mux.Handle("GET /api/v1/api-error-logs", auth.RequireAuth(tokenService, http.HandlerFunc(apiErrorLogHandler.List)))
	mux.Handle("GET /api/v1/app-versions", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.List)))
	mux.Handle("POST /api/v1/app-versions", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Create)))
	mux.Handle("GET /api/v1/app-versions/check", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Check)))
	mux.Handle("POST /api/v1/app-versions/upload", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Upload)))
	mux.Handle("GET /api/v1/app-version-masters", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.ListMasters)))
	mux.Handle("GET /api/v1/app-version-masters/check", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.CheckMaster)))
	mux.Handle("POST /api/v1/app-version-masters", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.CreateMaster)))
	mux.Handle("POST /api/v1/app-version-masters/upload", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.UploadMaster)))
	mux.Handle("GET /api/v1/app-version-masters/{masterId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.GetMaster)))
	mux.Handle("PATCH /api/v1/app-version-masters/{masterId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.PatchMaster)))
	mux.Handle("DELETE /api/v1/app-version-masters/{masterId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.DeleteMaster)))
	mux.Handle("GET /api/v1/app-versions/{versionId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Get)))
	mux.Handle("PATCH /api/v1/app-versions/{versionId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Patch)))
	mux.Handle("DELETE /api/v1/app-versions/{versionId}", auth.RequireAuth(tokenService, http.HandlerFunc(appVersionHandler.Delete)))
	mux.Handle("GET /api/v1/reports/attendance", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListAttendance)))
	mux.Handle("GET /api/v1/reports/attendance/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadAttendance)))
	mux.Handle("GET /api/v1/reports/visitors", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListVisitors)))
	mux.Handle("GET /api/v1/reports/visitors/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadVisitors)))
	mux.Handle("GET /api/v1/reports/patrol-scans", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListPatrolScans)))
	mux.Handle("GET /api/v1/reports/patrol-scans/dates", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.PatrolScanDates)))
	mux.Handle("GET /api/v1/reports/patrol-scans/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadPatrolScans)))
	mux.Handle("GET /api/v1/reports/facility-scans", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListFacilityScans)))
	mux.Handle("GET /api/v1/reports/facility-scans/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadFacilityScans)))
	mux.Handle("POST /api/v1/uploads/attendance", auth.RequireAuth(tokenService, http.HandlerFunc(mediaHandler.UploadAttendance)))
	mux.Handle("POST /api/v1/uploads/patrol", auth.RequireAuth(tokenService, http.HandlerFunc(mediaHandler.UploadPatrol)))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(storageRoot))))

	return loggingMiddleware(tokenService, apiErrorLogRepo, recoverMiddleware(corsMiddleware(mux)))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				web.WriteError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
