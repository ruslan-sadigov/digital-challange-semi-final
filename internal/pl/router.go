// Package pl is the presentation layer: routing, controllers, views and the WebSocket hub.
package pl

import (
	"html/template"
	"io/fs"
	"net/http"

	"smartcity/internal/domain"
	"smartcity/internal/pl/controller"
	mw "smartcity/internal/pl/middleware"
	"smartcity/internal/pl/view"
)

// NewRouter wires every route. deviceKey protects the bus GPS endpoint.
func NewRouter(s *controller.Services, deviceKey string) (http.Handler, error) {
	tpl, err := template.ParseFS(view.Templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(view.Static, "static")
	if err != nil {
		return nil, err
	}

	auth := &controller.AuthController{S: s}
	pass := &controller.PassengerController{S: s}
	op := &controller.OperatorController{S: s}
	pages := &controller.PageController{S: s, Templates: tpl}

	anyone := mw.RequireRole()
	operator := mw.RequireRole(domain.RoleOperator)
	device := mw.DeviceKey(deviceKey)
	h := func(f http.HandlerFunc, m ...func(http.Handler) http.Handler) http.Handler {
		return mw.Chain(f, m...)
	}

	mux := http.NewServeMux()

	// Pages (MVC views)
	mux.HandleFunc("GET /", pages.Passenger)
	mux.HandleFunc("GET /login", pages.Login)
	mux.HandleFunc("GET /register", pages.Register)
	mux.HandleFunc("GET /operator/login", pages.OperatorLogin)
	mux.HandleFunc("GET /operator", pages.Operator)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// Auth
	mux.HandleFunc("POST /api/auth/register", auth.Register)
	mux.HandleFunc("POST /api/auth/login", auth.Login)
	mux.HandleFunc("POST /api/auth/operator/login", auth.OperatorLogin)
	mux.HandleFunc("POST /api/auth/guest", auth.Guest)
	mux.HandleFunc("POST /api/auth/logout", auth.Logout)
	mux.HandleFunc("GET /api/auth/me", auth.Me)

	// Passenger (public reads)
	mux.HandleFunc("GET /api/config", pass.Config)
	mux.HandleFunc("GET /api/buses", pass.SearchBuses)
	mux.HandleFunc("GET /api/buses/{id}", pass.Bus)
	mux.HandleFunc("GET /api/buses/{id}/eta", pass.ETA)
	mux.HandleFunc("GET /api/routes", pass.Routes)
	mux.HandleFunc("GET /api/routes/density", pass.RouteDensities)
	mux.HandleFunc("GET /api/routes/{id}", pass.Route)
	mux.HandleFunc("GET /api/stations", pass.Stations)
	mux.HandleFunc("GET /api/stations/near", pass.NearbyStations)

	// Passenger (session required: guest, passenger or operator)
	mux.Handle("POST /api/buses/{id}/density", h(pass.ReportDensity, anyone))
	mux.Handle("POST /api/locations", h(pass.RecordLocation, anyone))
	mux.Handle("GET /ws", h(pass.WebSocket, anyone))

	// Bus GPS devices
	mux.Handle("POST /api/device/buses/{id}/location", h(pass.DeviceLocation, device))

	// Operator
	mux.Handle("GET /api/operator/heatmap", h(op.Heatmap, operator))
	mux.Handle("POST /api/operator/buses", h(op.CreateBus, operator))
	mux.Handle("PUT /api/operator/buses/{id}", h(op.UpdateBus, operator))
	mux.Handle("DELETE /api/operator/buses/{id}", h(op.DeleteBus, operator))
	mux.Handle("POST /api/operator/routes", h(op.CreateRoute, operator))
	mux.Handle("PUT /api/operator/routes/{id}", h(op.UpdateRoute, operator))
	mux.Handle("DELETE /api/operator/routes/{id}", h(op.DeleteRoute, operator))
	mux.Handle("POST /api/operator/stations", h(op.CreateStation, operator))
	mux.Handle("PUT /api/operator/stations/{id}", h(op.UpdateStation, operator))
	mux.Handle("DELETE /api/operator/stations/{id}", h(op.DeleteStation, operator))
	mux.Handle("GET /api/operator/anomalies", h(op.Anomalies, operator))
	mux.Handle("GET /api/operator/overrides", h(op.ActiveOverrides, operator))
	mux.Handle("POST /api/operator/overrides/{id}/approve", h(op.Approve(), operator))
	mux.Handle("POST /api/operator/overrides/{id}/reject", h(op.Reject(), operator))
	mux.Handle("POST /api/operator/overrides/{id}/end", h(op.End(), operator))
	mux.Handle("POST /api/operator/operators", h(op.CreateOperator, operator))

	return mw.Chain(mux, mw.Recover, mw.Logger, mw.LoadSession(s.Auth)), nil
}
