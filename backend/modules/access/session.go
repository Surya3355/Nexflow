package access

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionUserKey = "user_id"
	// sessionPasswordKey holds the user's password stamp at sign-in. When the
	// password changes, the stamp no longer matches and the session ends.
	sessionPasswordKey = "pw_stamp"
	sessionLife        = 12 * time.Hour
	sessionCookie      = "nexflow_session"
)

type ctxKey struct{}

// NewSessionManager returns server-side sessions stored in PostgreSQL. The
// browser holds only a random cookie. secure must be true in production.
func NewSessionManager(pool *pgxpool.Pool, secure bool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = sessionLife
	sm.Cookie.Name = sessionCookie
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = secure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	return sm
}

// Handler serves the login, logout and current-user endpoints.
type Handler struct {
	svc        *Service
	sm         *scs.SessionManager
	addrs      *AddressLimiter
	gate       *HashGate
	trustProxy bool
}

// NewHandler wires the access service to the session manager, with the
// default login limits.
func NewHandler(svc *Service, sm *scs.SessionManager) *Handler {
	return &Handler{
		svc: svc, sm: sm,
		addrs: NewAddressLimiter(loginsPerAddressPerMinute, time.Now),
		gate:  NewHashGate(maxConcurrentHashes, hashWait),
	}
}

// TrustProxy makes the login limit use the X-Real-IP header set by nginx.
// Only for when the API is reachable through the proxy alone.
func (h *Handler) TrustProxy(on bool) { h.trustProxy = on }

// Routes returns the access routes. Session loading and saving wrap all of them.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(h.sm.LoadAndSave)
	h.Mount(r)
	return r
}

// Mount registers the login, logout and account routes on r. The caller must
// wrap r with the session manager's LoadAndSave.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/login", h.login)
	r.Post("/api/logout", h.logout)
	r.Group(func(r chi.Router) {
		r.Use(h.RequireUser)
		r.Get("/api/me", h.me)
		r.Post("/api/me/password", h.changePassword)
	})
}

// SessionManager returns the session manager used by this handler.
func (h *Handler) SessionManager() *scs.SessionManager { return h.sm }

type userJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

func toJSON(u User) userJSON {
	return userJSON{ID: u.ID, Username: u.Username, FullName: u.FullName, Role: string(u.Role)}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !h.addrs.Allow(ClientAddr(r, h.trustProxy)) {
		writeJSONError(w, http.StatusTooManyRequests, "Too many attempts. Wait a minute.")
		return
	}
	var u User
	var err error
	if gateErr := h.gate.Do(r.Context(), func() {
		u, err = h.svc.Authenticate(r.Context(), in.Username, in.Password)
	}); gateErr != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Busy, try again.")
		return
	}
	var locked *LockedError
	if errors.As(err, &locked) {
		writeJSONError(w, http.StatusTooManyRequests, locked.Message(h.svc.clock()))
		return
	}
	if errors.Is(err, ErrInvalidCredentials) {
		writeJSONError(w, http.StatusUnauthorized, ErrInvalidCredentials.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// A new ID on login stops session fixation.
	if err := h.sm.RenewToken(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), sessionUserKey, u.ID)
	h.sm.Put(r.Context(), sessionPasswordKey, passwordStamp(u))
	writeJSON(w, http.StatusOK, toJSON(u))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sm.Destroy(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	err := h.svc.ChangePassword(r.Context(), u, in.Current, in.New)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		writeJSONError(w, http.StatusUnauthorized, "current password is wrong")
	case errors.Is(err, ErrWeakPassword):
		writeJSONError(w, http.StatusBadRequest, ErrWeakPassword.Error())
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	default:
		// Keep this session: take the new password stamp. Other sessions end.
		if fresh, err := h.svc.loadActive(r.Context(), u.ID); err == nil {
			h.sm.Put(r.Context(), sessionPasswordKey, passwordStamp(fresh))
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	writeJSON(w, http.StatusOK, toJSON(u))
}

// RequireUser refuses requests without a valid session for an active user.
// It loads the user from the database on every request (see loadActive).
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := h.sm.GetInt64(r.Context(), sessionUserKey)
		if id == 0 {
			writeJSONError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		u, err := h.svc.loadActive(r.Context(), id)
		if err == nil && h.sm.GetInt64(r.Context(), sessionPasswordKey) != passwordStamp(u) {
			err = ErrInvalidCredentials // password changed since this session signed in
		}
		if err != nil {
			_ = h.sm.Destroy(r.Context())
			writeJSONError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// WithUser stores the signed-in user in ctx.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFromContext returns the signed-in user set by RequireUser.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// passwordStamp identifies the user's current password: when it was last
// changed, in microseconds, or 0 if never.
func passwordStamp(u User) int64 {
	if u.PasswordChangedAt == nil {
		return 0
	}
	return u.PasswordChangedAt.UnixMicro()
}
