package httpapi

import (
	"net/http"

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
	"satpam-go/internal/web"
)

func NewRouter(authHandler *auth.Handler, userHandler *users.Handler, roleHandler *roles.Handler, placeHandler *places.Handler, shiftHandler *shifts.Handler, userPlaceRoleHandler *userplaceroles.Handler, spotHandler *spots.Handler, spotAssignmentHandler *spotassignments.Handler, attendanceConfigHandler *attendanceconfig.Handler, attendanceHandler *attendances.Handler, leaveRequestHandler *leaverequests.Handler, patrolHandler *patrol.Handler, facilityHandler *facility.Handler, recentActivitiesHandler *recentactivities.Handler, reportHandler *reports.Handler, mediaHandler *media.Handler, tokenService *auth.TokenService, storageRoot string) http.Handler {
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
	mux.Handle("GET /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.List)))
	mux.Handle("POST /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.Create)))
	mux.Handle("PATCH /api/v1/leave-requests", auth.RequireAuth(tokenService, http.HandlerFunc(leaveRequestHandler.Patch)))
	mux.Handle("GET /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.ListRoutePoints)))
	mux.Handle("POST /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.CreateRoutePoint)))
	mux.Handle("DELETE /api/v1/patrol/route-points", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.DeleteRoutePoint)))
	mux.Handle("GET /api/v1/patrol/scans", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.ListScans)))
	mux.Handle("POST /api/v1/patrol/scans", auth.RequireAuth(tokenService, http.HandlerFunc(patrolHandler.CreateScan)))
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
	mux.Handle("GET /api/v1/reports/attendance", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListAttendance)))
	mux.Handle("GET /api/v1/reports/attendance/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadAttendance)))
	mux.Handle("GET /api/v1/reports/patrol-scans", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListPatrolScans)))
	mux.Handle("GET /api/v1/reports/patrol-scans/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadPatrolScans)))
	mux.Handle("GET /api/v1/reports/facility-scans", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.ListFacilityScans)))
	mux.Handle("GET /api/v1/reports/facility-scans/download", auth.RequireAuth(tokenService, http.HandlerFunc(reportHandler.DownloadFacilityScans)))
	mux.Handle("POST /api/v1/uploads/attendance", auth.RequireAuth(tokenService, http.HandlerFunc(mediaHandler.UploadAttendance)))
	mux.Handle("POST /api/v1/uploads/patrol", auth.RequireAuth(tokenService, http.HandlerFunc(mediaHandler.UploadPatrol)))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(storageRoot))))

	return recoverMiddleware(corsMiddleware(loggingMiddleware(mux)))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
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
