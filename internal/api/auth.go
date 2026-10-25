package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/SiddheshNan/moonshed/internal/setup"
	"github.com/SiddheshNan/moonshed/internal/store"
)

const (
	sessionCookie = "session"
	sessionTTL    = 30 * 24 * time.Hour
	tokenPrefix   = "msh_"
)

type actorKey struct{}

// actor is who made a request: a logged-in user, or a user's API token.
type actor struct {
	user  *store.User
	token string
}

func currentActor(r *http.Request) actor {
	a, _ := r.Context().Value(actorKey{}).(actor)
	return a
}

func currentUser(r *http.Request) *store.User { return currentActor(r).user }

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) authenticate(r *http.Request) (actor, error) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		token, ok := strings.CutPrefix(auth, "Bearer ")
		if ok && strings.HasPrefix(token, tokenPrefix) {
			if u, name, err := s.Store.APITokenUser(hashToken(token)); err == nil {
				return actor{user: u, token: name}, nil
			}
		}
		return actor{}, errorf(http.StatusUnauthorized, "invalid API token")
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return actor{}, errorf(http.StatusUnauthorized, "not logged in")
	}
	u, err := s.Store.SessionUser(hashToken(c.Value))
	if err != nil {
		return actor{}, errorf(http.StatusUnauthorized, "not logged in")
	}
	return actor{user: u}, nil
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) error {
	n, err := s.Store.UserCount()
	if err != nil {
		return err
	}
	resp := map[string]any{"setup_required": n == 0, "version": s.Version, "build": s.Build}
	if n == 0 {
		resp["setup_command"] = s.SetupCommand
	}
	if a, err := s.authenticate(r); err == nil {
		resp["user"] = a.user
	}
	return ok(w, resp)
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c *credentials) validate() error {
	c.Username = strings.TrimSpace(c.Username)
	if c.Username == "" || len(c.Username) > 64 {
		return badRequest("username is required")
	}
	if len(c.Password) < 8 {
		return badRequest("password must be at least 8 characters")
	}
	return nil
}

type setupRequest struct {
	credentials
	Token string `json:"token"`
}

// setup creates the first admin. It needs the token from the setup link and only
// works while there are no users.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	var in setupRequest
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	n, err := s.Store.UserCount()
	if err != nil {
		return err
	}
	if n > 0 {
		return errorf(http.StatusConflict, "setup is already done")
	}
	if !setup.Valid(s.Paths, in.Token) {
		return errorf(http.StatusForbidden, "invalid setup link; run %s on the server for a new one", s.SetupCommand)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := &store.User{Username: in.Username, PasswordHash: string(hash)}
	created, err := s.Store.CreateFirstUser(u)
	if err != nil {
		return err
	}
	if !created {
		return errorf(http.StatusConflict, "setup is already done")
	}
	if err := setup.Done(s.Paths); err != nil {
		slog.Error("remove setup token", "err", err)
	}
	s.auditAs(r, store.ActorUser, u.Username, "user.setup", "user", u.ID, u.Username, "")
	return s.startSession(w, r, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		return errorf(http.StatusTooManyRequests, "too many failed attempts, try again later")
	}
	var in credentials
	if err := decode(r, &in); err != nil {
		return err
	}
	u, err := s.Store.UserByName(strings.TrimSpace(in.Username))
	if err == nil {
		err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password))
	}
	if err != nil {
		s.limiter.fail(ip)
		s.auditAs(r, store.ActorUser, strings.TrimSpace(in.Username), "auth.login_failed", "", "", "", "")
		return errorf(http.StatusUnauthorized, "invalid username or password")
	}
	s.limiter.reset(ip)
	s.auditAs(r, store.ActorUser, u.Username, "auth.login", "user", u.ID, u.Username, "")
	return s.startSession(w, r, u)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token := randomToken()
	expires := time.Now().Add(sessionTTL)
	if err := s.Store.CreateSession(hashToken(token), u.ID, expires); err != nil {
		return err
	}
	s.Store.DeleteExpiredSessions()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return ok(w, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Store.DeleteSession(hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true})
	s.audit(r, "auth.logout", "", "", "", "")
	return noContent(w)
}

func (s *Server) newUser(in credentials) (*store.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &store.User{Username: in.Username, PasswordHash: string(hash)}
	if err := s.Store.CreateUser(u); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errorf(http.StatusConflict, "username is taken")
		}
		return nil, err
	}
	return u, nil
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) error {
	users, err := s.Store.Users()
	if err != nil {
		return err
	}
	return ok(w, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) error {
	var in credentials
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	u, err := s.newUser(in)
	if err != nil {
		return err
	}
	s.audit(r, "user.create", "user", u.ID, u.Username, "")
	return writeJSON(w, http.StatusCreated, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.Password) < 8 {
		return badRequest("password must be at least 8 characters")
	}
	u, err := s.Store.User(id)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.Store.SetPassword(id, string(hash)); err != nil {
		return err
	}
	s.audit(r, "user.password", "user", u.ID, u.Username, "")
	return noContent(w)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if id == currentUser(r).ID {
		return badRequest("you can't delete yourself")
	}
	u, err := s.Store.User(id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteUser(id); err != nil {
		return err
	}
	s.audit(r, "user.delete", "user", u.ID, u.Username, "")
	return noContent(w)
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) error {
	tokens, err := s.Store.APITokens()
	if err != nil {
		return err
	}
	return ok(w, tokens)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return badRequest("name is required")
	}
	token := tokenPrefix + randomToken()
	t := &store.APIToken{UserID: currentUser(r).ID, Name: in.Name}
	if err := s.Store.CreateAPIToken(t, hashToken(token)); err != nil {
		return err
	}
	s.audit(r, "token.create", "token", t.ID, t.Name, "")
	return writeJSON(w, http.StatusCreated, map[string]any{"token": token, "api_token": t})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.Store.DeleteAPIToken(id); err != nil {
		return err
	}
	s.audit(r, "token.delete", "token", id, "", "")
	return noContent(w)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter blocks an IP for a while after repeated failed logins.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

const (
	maxFailures   = 10
	failureWindow = 15 * time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string][]time.Time{}}
}

func (l *loginLimiter) recent(ip string) []time.Time {
	cutoff := time.Now().Add(-failureWindow)
	times := l.failures[ip]
	for len(times) > 0 && times[0].Before(cutoff) {
		times = times[1:]
	}
	l.failures[ip] = times
	return times
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) < maxFailures
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recent(ip), time.Now())
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}
