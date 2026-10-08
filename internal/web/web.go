// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/airencracken/comfylib/clientip"
	"github.com/airencracken/comfylib/token"
	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Config struct {
	SecureCookies  bool
	TrustedProxies []netip.Prefix
}
type App struct {
	store     *store.Store
	config    Config
	templates *template.Template
	handler   http.Handler
	dummyHash []byte
	mu        sync.Mutex
	attempts  map[string]attempt
}
type attempt struct {
	count   int
	expires time.Time
}
type requestState struct {
	User          *store.User
	CSRF, Session string
}
type stateKey struct{}
type page struct {
	View, Title, Error, CSRF, URL, Note, Username, Status string
	User                                                  *store.User
	Users                                                 []store.User
	Items                                                 []store.Recommendation
	Item                                                  store.Recommendation
	Comments                                              []store.Comment
	Recipient                                             int64
	Offset, Previous, Next                                int
	HasPrevious, HasNext                                  bool
	History                                               bool
}

func New(s *store.Store, cfg Config) (*App, error) {
	tmpl, err := template.New("pages").Funcs(template.FuncMap{
		"date":    func(unix int64) string { return time.Unix(unix, 0).UTC().Format("2 Jan 2006, 15:04 UTC") },
		"isoDate": func(unix int64) string { return time.Unix(unix, 0).UTC().Format(time.RFC3339) },
	}).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("a-dummy-password-never-used"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	a := &App{store: s, config: cfg, templates: tmpl, dummyHash: dummy, attempts: map[string]attempt{}}
	mux := http.NewServeMux()
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Healthy(r.Context()); err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.signedIn(a.logout))
	mux.HandleFunc("GET /{$}", a.signedIn(a.inbox))
	mux.HandleFunc("GET /inbox", a.signedIn(a.inbox))
	mux.HandleFunc("GET /history", a.signedIn(a.history))
	mux.HandleFunc("GET /recommendations/new", a.signedIn(a.newRecommendation))
	mux.HandleFunc("POST /recommendations/new", a.signedIn(a.recommend))
	mux.HandleFunc("GET /recommendations/{id}", a.signedIn(a.detail))
	mux.HandleFunc("POST /recommendations/{id}/reaction", a.signedIn(a.react))
	mux.HandleFunc("POST /recommendations/{id}/comments", a.signedIn(a.comment))
	mux.HandleFunc("GET /account/export", a.signedIn(a.export))
	a.handler = http.NewCrossOriginProtection().Handler(a.middleware(mux))
	return a, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.handler.ServeHTTP(w, r) }
func state(r *http.Request) requestState                        { return r.Context().Value(stateKey{}).(requestState) }
func (a *App) secure(r *http.Request) bool {
	return a.config.SecureCookies || (clientip.Resolver{Trusted: a.config.TrustedProxies}).ForwardedHTTPS(r)
}
func (a *App) cookieName(r *http.Request, name string) string {
	if a.secure(r) {
		return "__Host-songstead_" + name
	}
	return "songstead_" + name
}
func (a *App) cookie(w http.ResponseWriter, r *http.Request, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(r, name), Value: value, Path: "/", HttpOnly: true, Secure: a.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func secretShape(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' https://i.ytimg.com; frame-src https://www.youtube-nocookie.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		st := requestState{}
		if c, err := r.Cookie(a.cookieName(r, "session")); err == nil && secretShape(c.Value) {
			u, err := a.store.Session(r.Context(), c.Value)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				a.fail(w, r, err)
				return
			}
			if err == nil {
				st.User = &u
				st.Session = c.Value
			}
		}
		current := ""
		if c, err := r.Cookie(a.cookieName(r, "csrf")); err == nil && secretShape(c.Value) {
			current = c.Value
		}
		if st.Session != "" {
			st.CSRF = token.SessionCSRF(st.Session, "songstead-csrf-v1")
		} else {
			st.CSRF = current
		}
		if st.CSRF == "" {
			st.CSRF, _ = token.New()
		}
		if current != st.CSRF {
			a.cookie(w, r, "csrf", st.CSRF, 86400)
		}
		r = r.WithContext(context.WithValue(r.Context(), stateKey{}, st))
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Method != "TRACE" {
			r.Body = http.MaxBytesReader(w, r.Body, 16384)
			if err := r.ParseForm(); err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					http.Error(w, "form too large", 413)
				} else {
					http.Error(w, "invalid form", 400)
				}
				return
			}
			for _, values := range r.PostForm {
				if len(values) != 1 {
					http.Error(w, "duplicate form fields", 400)
					return
				}
			}
			provided := r.PostForm.Get("csrf")
			if header := r.Header.Get("X-CSRF-Token"); header != "" {
				provided = header
			}
			if provided == "" || !token.Equal(provided, st.CSRF) {
				w.Header().Set("HX-Refresh", "true")
				http.Error(w, "refresh the page before submitting", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) signedIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if state(r).User == nil {
			http.Redirect(w, r, "/login", 303)
			return
		}
		next(w, r)
	}
}

func (a *App) render(w http.ResponseWriter, r *http.Request, status int, p page) {
	st := state(r)
	p.User, p.CSRF = st.User, st.CSRF
	var buf bytes.Buffer
	if err := a.templates.ExecuteTemplate(&buf, "layout", p); err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
func (a *App) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrMissing) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, store.ErrInvalid) {
		http.Error(w, "invalid input", 422)
		return
	}
	slog.Error("request failed", "path", r.URL.Path, "error", err)
	http.Error(w, "Something went wrong. Please try again.", 500)
}

func (a *App) loginForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 200, page{View: "login", Title: "Welcome back"})
}
func (a *App) allowLogin(r *http.Request) bool {
	key := clientip.NetworkKey((clientip.Resolver{Trusted: a.config.TrustedProxies}).Client(r))
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if !v.expires.After(now) {
			delete(a.attempts, k)
		}
	}
	v, exists := a.attempts[key]
	if !exists {
		if len(a.attempts) >= 10000 {
			for k := range a.attempts {
				delete(a.attempts, k)
				break
			}
		}
		v.expires = now.Add(15 * time.Minute)
	}
	v.count++
	a.attempts[key] = v
	return v.count <= 20
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.allowLogin(r) {
		w.Header().Set("Retry-After", "900")
		http.Error(w, "too many sign-in attempts; try again in 15 minutes", 429)
		return
	}
	name, password := strings.TrimSpace(r.PostForm.Get("username")), r.PostForm.Get("password")
	u, hash, err := a.store.Credentials(r.Context(), name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.fail(w, r, err)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		hash = string(a.dummyHash)
	}
	check := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil || check != nil || len(password) > 72 {
		a.render(w, r, 422, page{View: "login", Title: "Welcome back", Username: name, Error: "That username and password did not match."})
		return
	}
	secret, err := a.store.NewSession(r.Context(), u.ID, hash)
	if errors.Is(err, store.ErrMissing) {
		a.render(w, r, 422, page{View: "login", Title: "Welcome back", Username: name, Error: "Your sign-in details changed. Please try again."})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.cookie(w, r, "session", secret, 7*86400)
	a.cookie(w, r, "csrf", token.SessionCSRF(secret, "songstead-csrf-v1"), 86400)
	http.Redirect(w, r, "/inbox", 303)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Logout(r.Context(), state(r).Session); err != nil {
		a.fail(w, r, err)
		return
	}
	a.cookie(w, r, "session", "", -1)
	a.cookie(w, r, "csrf", "", -1)
	http.Redirect(w, r, "/login", 303)
}
func (a *App) inbox(w http.ResponseWriter, r *http.Request)   { a.list(w, r, false) }
func (a *App) history(w http.ResponseWriter, r *http.Request) { a.list(w, r, true) }
func (a *App) list(w http.ResponseWriter, r *http.Request, history bool) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		var err error
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 || offset > 1000000 {
			http.Error(w, "invalid page", 400)
			return
		}
	}
	status := r.URL.Query().Get("status")
	items, err := a.store.List(r.Context(), state(r).User.ID, history, status, 51, offset)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	title := "Your inbox"
	if history {
		title = "Your history"
	}
	p := page{View: "list", Title: title, Items: items, History: history, Status: status, Offset: offset, HasNext: len(items) > 50, HasPrevious: offset > 0, Next: offset + 50, Previous: max(0, offset-50)}
	if p.HasNext {
		p.Items = items[:50]
	}
	a.render(w, r, 200, p)
}
func (a *App) newRecommendation(w http.ResponseWriter, r *http.Request) { a.compose(w, r, 200, page{}) }
func (a *App) compose(w http.ResponseWriter, r *http.Request, status int, p page) {
	users, err := a.store.Users(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Users = users
	p.View = "compose"
	p.Title = "Send some music"
	a.render(w, r, status, p)
}
func (a *App) recommend(w http.ResponseWriter, r *http.Request) {
	recipient, err := strconv.ParseInt(r.PostForm.Get("recipient"), 10, 64)
	p := page{URL: r.PostForm.Get("url"), Note: r.PostForm.Get("note"), Recipient: recipient}
	if err != nil || recipient <= 0 {
		p.Error = "Choose a recipient."
		a.compose(w, r, 422, p)
		return
	}
	users, err := a.store.Users(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	found := false
	for _, u := range users {
		if u.ID == recipient {
			found = true
		}
	}
	if !found {
		p.Error = "Choose a recipient from this instance."
		a.compose(w, r, 422, p)
		return
	}
	id, err := a.store.Recommend(r.Context(), state(r).User.ID, recipient, p.URL, p.Note)
	if err != nil {
		// Validate before storage; unknown database failures stay server errors.
		if errors.Is(err, store.ErrInvalid) {
			p.Error = "Keep your note within 2,000 characters."
		} else {
			if _, parseErr := media.Parse(p.URL); parseErr != nil {
				p.Error = parseErr.Error()
			} else {
				a.fail(w, r, err)
				return
			}
		}
		a.compose(w, r, 422, p)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10), 303)
}

func recommendationID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, store.ErrMissing
	}
	return id, nil
}
func (a *App) detail(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.showDetail(w, r, id, 200, "")
}
func (a *App) showDetail(w http.ResponseWriter, r *http.Request, id int64, status int, message string) {
	item, err := a.store.Recommendation(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	comments, err := a.store.Comments(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, status, page{View: "detail", Title: item.Title, Item: item, Comments: comments, Error: message})
}
func (a *App) react(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	rating, err := strconv.Atoi(r.PostForm.Get("rating"))
	if err != nil {
		a.showDetail(w, r, id, 422, "Choose a valid rating.")
		return
	}
	err = a.store.React(r.Context(), state(r).User.ID, id, store.Reaction{Listening: r.PostForm.Get("listening"), Rating: rating, Note: r.PostForm.Get("personal_note")})
	if errors.Is(err, store.ErrInvalid) {
		a.showDetail(w, r, id, 422, "Choose a listening status and rating; notes can contain up to 2,000 characters.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10), 303)
}
func (a *App) comment(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	err = a.store.AddComment(r.Context(), state(r).User.ID, id, r.PostForm.Get("body"))
	if errors.Is(err, store.ErrInvalid) {
		a.showDetail(w, r, id, 422, "Write a comment of 1-2,000 characters.")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10), 303)
}
func (a *App) export(w http.ResponseWriter, r *http.Request) {
	archive, err := a.store.Export(r.Context(), *state(r).User)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	data, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="songstead-export.json"`)
	_, _ = w.Write(data)
}
