// Package controller holds the HTTP controllers (MVC "C"): they parse requests, call BL services and render JSON or views.
package controller

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agilli/internal/bl"
	"agilli/internal/domain"
	"agilli/internal/pl/middleware"
	"agilli/internal/pl/ws"
)

// Services are the BL dependencies of the controllers.
type Services struct {
	Auth     *bl.AuthService
	Fleet    *bl.FleetService
	Location *bl.LocationService
	Density  *bl.DensityService
	ETA      *bl.ETAService
	Heatmap  *bl.HeatmapService
	Anomaly  *bl.AnomalyService
	Hub      *ws.Hub
	Settings bl.Settings
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps domain errors to HTTP status codes.
func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	status := http.StatusInternalServerError
	msg := "internal error"
	switch {
	case errors.As(err, &ve):
		status, msg = http.StatusBadRequest, ve.Msg
	case errors.Is(err, domain.ErrNotFound):
		status, msg = http.StatusNotFound, "not found"
	case errors.Is(err, domain.ErrConflict):
		status, msg = http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		status, msg = http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, domain.ErrForbidden):
		status, msg = http.StatusForbidden, strings.TrimPrefix(strings.TrimPrefix(err.Error(), domain.ErrForbidden.Error()), ": ")
		if msg == "" {
			msg = "forbidden"
		}
	case errors.Is(err, domain.ErrRateLimited):
		status, msg = http.StatusTooManyRequests, "you already reported this bus recently, try again in a few minutes"
	default:
		log.Printf("error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return domain.ErrInvalidInput("invalid JSON body")
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidInput("invalid " + name)
	}
	return id, nil
}

func queryFloat(r *http.Request, name string) (float64, error) {
	v, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	if err != nil {
		return 0, domain.ErrInvalidInput("invalid " + name)
	}
	return v, nil
}

func session(r *http.Request) bl.Session {
	s, _ := middleware.SessionFrom(r.Context())
	return s
}

func setCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: middleware.CookieName, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// ---------- auth ----------

type AuthController struct{ S *Services }

type credentials struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

func (c *AuthController) login(w http.ResponseWriter, u *domain.User) {
	token, sess := c.S.Auth.IssueToken(u)
	setCookie(w, token, int((24 * time.Hour).Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"session": sess, "token": token})
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	u, err := c.S.Auth.RegisterWithConfirmation(r.Context(), in.Email, in.Password, in.ConfirmPassword)
	if err != nil {
		writeError(w, err)
		return
	}
	c.login(w, u)
}

// Login is the passenger sign-in; OperatorLogin is the separate operator sign-in.
func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	c.loginAs(w, r, domain.RolePassenger)
}

func (c *AuthController) OperatorLogin(w http.ResponseWriter, r *http.Request) {
	c.loginAs(w, r, domain.RoleOperator)
}

func (c *AuthController) loginAs(w http.ResponseWriter, r *http.Request, portal domain.Role) {
	var in credentials
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	u, err := c.S.Auth.Login(r.Context(), in.Email, in.Password, portal)
	if err != nil {
		writeError(w, err)
		return
	}
	c.login(w, u)
}

func (c *AuthController) Guest(w http.ResponseWriter, r *http.Request) {
	u, err := c.S.Auth.Guest(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	c.login(w, u)
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	setCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *AuthController) Me(w http.ResponseWriter, r *http.Request) {
	s, ok := middleware.SessionFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no session"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// ---------- passenger API ----------

type PassengerController struct{ S *Services }

func (c *PassengerController) Config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"userSampleIntervalMs": c.S.Settings.UserSampleInterval.Milliseconds(),
		"anomalyThresholdS":    c.S.Settings.AnomalyThreshold.Seconds(),
	})
}

func (c *PassengerController) SearchBuses(w http.ResponseWriter, r *http.Request) {
	buses, err := c.S.Fleet.Buses(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, buses)
}

func (c *PassengerController) Bus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	d, err := c.S.Fleet.BusDetail(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (c *PassengerController) ETA(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	sid, err := strconv.ParseInt(r.URL.Query().Get("station_id"), 10, 64)
	if err != nil {
		writeError(w, domain.ErrInvalidInput("station_id is required"))
		return
	}
	res, err := c.S.ETA.ETA(r.Context(), id, sid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (c *PassengerController) Routes(w http.ResponseWriter, r *http.Request) {
	routes, err := c.S.Fleet.Routes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (c *PassengerController) Route(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	rt, err := c.S.Fleet.Route(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (c *PassengerController) RouteDensities(w http.ResponseWriter, r *http.Request) {
	st, err := c.S.Anomaly.RouteStates(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (c *PassengerController) Stations(w http.ResponseWriter, r *http.Request) {
	st, err := c.S.Fleet.Stations(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (c *PassengerController) NearbyStations(w http.ResponseWriter, r *http.Request) {
	lat, err1 := queryFloat(r, "lat")
	lng, err2 := queryFloat(r, "lng")
	if err1 != nil || err2 != nil {
		writeError(w, domain.ErrInvalidInput("lat and lng are required"))
		return
	}
	radius := 500.0
	if v, err := queryFloat(r, "radius"); err == nil && v > 0 && v <= 5000 {
		radius = v
	}
	st, err := c.S.Fleet.NearbyStations(r.Context(), lat, lng, radius)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (c *PassengerController) ReportDensity(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Density string `json:"density"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Density.Report(r.Context(), session(r), id, strings.ToLower(in.Density)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

type latLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func (c *PassengerController) RecordLocation(w http.ResponseWriter, r *http.Request) {
	var in latLng
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Location.RecordUser(r.Context(), session(r), in.Lat, in.Lng); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// DeviceLocation is the bus GPS ingestion endpoint.
func (c *PassengerController) DeviceLocation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in latLng
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Location.RecordBus(r.Context(), id, in.Lat, in.Lng); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (c *PassengerController) WebSocket(w http.ResponseWriter, r *http.Request) {
	c.S.Hub.Serve(w, r, session(r).Role)
}

// ---------- operator API ----------

type OperatorController struct{ S *Services }

func (c *OperatorController) Heatmap(w http.ResponseWriter, r *http.Request) {
	pts, err := c.S.Heatmap.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pts)
}

type busInput struct {
	Name     string `json:"name"`
	RouteID  *int64 `json:"routeId"`
	Capacity int    `json:"passengerCapacity"`
}

func (c *OperatorController) CreateBus(w http.ResponseWriter, r *http.Request) {
	var in busInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	b := &domain.Bus{Name: in.Name, RouteID: in.RouteID, Capacity: in.Capacity, Density: domain.DensityUnknown}
	if err := c.S.Fleet.CreateBus(r.Context(), b); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (c *OperatorController) UpdateBus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in busInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	b := &domain.Bus{ID: id, Name: in.Name, RouteID: in.RouteID, Capacity: in.Capacity}
	if err := c.S.Fleet.UpdateBus(r.Context(), b); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (c *OperatorController) DeleteBus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Fleet.DeleteBus(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type routeInput struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	StationIDs []int64 `json:"stationIds"`
}

func (c *OperatorController) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var in routeInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	rt := &domain.Route{Code: in.Code, Name: in.Name}
	if err := c.S.Fleet.CreateRoute(r.Context(), rt, in.StationIDs); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rt)
}

func (c *OperatorController) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in routeInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	rt := &domain.Route{ID: id, Code: in.Code, Name: in.Name}
	if err := c.S.Fleet.UpdateRoute(r.Context(), rt, in.StationIDs); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (c *OperatorController) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Fleet.DeleteRoute(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type stationInput struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

func (c *OperatorController) CreateStation(w http.ResponseWriter, r *http.Request) {
	var in stationInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	st := &domain.Station{Name: in.Name, Lat: in.Lat, Lng: in.Lng}
	if err := c.S.Fleet.CreateStation(r.Context(), st); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (c *OperatorController) UpdateStation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in stationInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	st := &domain.Station{ID: id, Name: in.Name, Lat: in.Lat, Lng: in.Lng}
	if err := c.S.Fleet.UpdateStation(r.Context(), st); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (c *OperatorController) DeleteStation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := c.S.Fleet.DeleteStation(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *OperatorController) Anomalies(w http.ResponseWriter, r *http.Request) {
	as, err := c.S.Anomaly.ListAnomalies(r.Context(), 50)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, as)
}

func (c *OperatorController) ActiveOverrides(w http.ResponseWriter, r *http.Request) {
	ovs, err := c.S.Anomaly.ListActiveOverrides(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ovs)
}

func (c *OperatorController) override(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r, "id")
		if err != nil {
			writeError(w, err)
			return
		}
		op := session(r).UserID
		var o *domain.RouteOverride
		switch action {
		case "approve":
			o, err = c.S.Anomaly.Approve(r.Context(), id, op)
		case "reject":
			o, err = c.S.Anomaly.Reject(r.Context(), id, op)
		default:
			o, err = c.S.Anomaly.End(r.Context(), id)
		}
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, o)
	}
}

func (c *OperatorController) Approve() http.HandlerFunc { return c.override("approve") }
func (c *OperatorController) Reject() http.HandlerFunc  { return c.override("reject") }
func (c *OperatorController) End() http.HandlerFunc     { return c.override("end") }

func (c *OperatorController) CreateOperator(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	u, err := c.S.Auth.CreateOperator(r.Context(), in.Email, in.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// ---------- pages (MVC views) ----------

type PageController struct {
	S         *Services
	Templates *template.Template
}

func (c *PageController) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// Passenger page; issues a guest session on first visit.
func (c *PageController) Passenger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	sess, ok := middleware.SessionFrom(r.Context())
	if !ok {
		u, err := c.S.Auth.Guest(r.Context())
		if err != nil {
			http.Error(w, "could not start a session", http.StatusInternalServerError)
			return
		}
		var token string
		token, sess = c.S.Auth.IssueToken(u)
		setCookie(w, token, int((24 * time.Hour).Seconds()))
	}
	c.render(w, "passenger.html", map[string]any{"Session": sess})
}

// Login, Register and OperatorLogin render the auth page on its passenger, register or operator tab.
func (c *PageController) Login(w http.ResponseWriter, r *http.Request) {
	c.render(w, "login.html", map[string]any{"Tab": "passenger"})
}

func (c *PageController) Register(w http.ResponseWriter, r *http.Request) {
	c.render(w, "login.html", map[string]any{"Tab": "register"})
}

func (c *PageController) OperatorLogin(w http.ResponseWriter, r *http.Request) {
	c.render(w, "login.html", map[string]any{"Tab": "operator"})
}

func (c *PageController) Operator(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.SessionFrom(r.Context())
	if !ok || sess.Role != domain.RoleOperator {
		http.Redirect(w, r, "/operator/login", http.StatusSeeOther)
		return
	}
	c.render(w, "operator.html", map[string]any{"Session": sess})
}
