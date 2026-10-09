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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/airencracken/comfylib/clientip"
	"github.com/airencracken/comfylib/reference"
	"github.com/airencracken/comfylib/token"
	"github.com/airencracken/songstead/internal/annotations"
	"github.com/airencracken/songstead/internal/media"
	"github.com/airencracken/songstead/internal/store"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Config struct {
	WitmootURL, BaseURL, Version string
	SecureCookies                bool
	TrustedProxies               []netip.Prefix
}
type App struct {
	store        *store.Store
	config       Config
	templates    *template.Template
	handler      http.Handler
	dummyHash    []byte
	previewFetch func(context.Context, string) (media.Metadata, error)
	mu           sync.Mutex
	attempts     map[string]attempt
}
type attempt struct {
	count   int
	expires time.Time
}
type requestState struct {
	User          *store.User
	CSRF, Session string
	Settings      store.Settings
}
type stateKey struct{}
type recommendationCard struct {
	store.Recommendation
	ShowMusic bool
}
type bundle struct {
	Heading string
	Items   []recommendationCard
}
type page struct {
	AvailableGenres, AvailableTags                                                      []string
	Profile                                                                             store.Profile
	SocialImage                                                                         string
	Layout, Genre, Tags, Tag, Discovery, FeedbackNotice, AnnotationNotice, LabelsNotice string
	DiscoveryPreferences                                                                store.DiscoveryPreferences
	DiscoveryDraft                                                                      *store.DiscoveryPreferences
	Settings                                                                            store.Settings
	SettingsDraft                                                                       *store.Settings
	Invitations                                                                         []store.Invitation
	SecretURL, Notice, Secret, Version                                                  string
	Audience                                                                            string
	Recent                                                                              bool
	CommentDraft                                                                        string
	Bundles                                                                             []bundle
	Perspective, Kind, MusicTitle, Artist, AnnotationMode, Handoff, Members             string
	Person, GroupID                                                                     int64
	Groups                                                                              []store.Group
	Recordings                                                                          []store.Recording
	Discussions                                                                         []string
	Reveal                                                                              bool
	View, Title, Error, CSRF, URL, Note, Username, Status                               string
	User                                                                                *store.User
	Users                                                                               []store.User
	Items                                                                               []store.Recommendation
	Item                                                                                store.Recommendation
	Comments                                                                            []store.Comment
	RecentComments                                                                      []store.RecentComment
	CommentsBefore, CommentsNext                                                        int64
	Recipient                                                                           int64
	Offset, Previous, Next                                                              int
	HasPrevious, HasNext                                                                bool
	History                                                                             bool
}

func New(s *store.Store, cfg Config) (*App, error) {
	for _, base := range []string{cfg.WitmootURL, cfg.BaseURL} {
		if base != "" {
			if _, err := reference.Handoff(base, reference.Draft{Source: "https://source.invalid"}); err != nil {
				return nil, err
			}
		}
	}
	tmpl, err := template.New("pages").Funcs(template.FuncMap{
		"supportsMetadata": media.SupportsMetadata,
		"browseURL":        browseURL,
		"joinLabels":       func(values []string) string { return strings.Join(values, ", ") },
		"labelLink": func(kind, value string) string {
			return "/recent?" + url.Values{kind: {value}, "discovery": {"all"}}.Encode()
		},
		"timestamp": annotations.Format,
		"timelineX": func(seconds, duration int) int {
			if duration <= 0 {
				return 20
			}
			return 20 + seconds*960/duration
		},
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
	client := media.Client()
	a.previewFetch = func(ctx context.Context, raw string) (media.Metadata, error) { return media.Fetch(ctx, client, raw) }
	a.adminRoutes(mux)
	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.signedIn(a.logout))
	mux.HandleFunc("GET /{$}", a.signedIn(a.shelf))
	mux.HandleFunc("GET /inbox", a.signedIn(a.shelf))
	mux.HandleFunc("GET /shelf", a.signedIn(a.shelf))
	mux.HandleFunc("GET /recent", a.signedIn(a.recent))
	mux.HandleFunc("GET /recent/comments", a.signedIn(a.recentComments))
	mux.HandleFunc("GET /history", a.signedIn(a.history))
	mux.HandleFunc("GET /recommendations/new", a.signedIn(a.newRecommendation))
	mux.HandleFunc("POST /recommendations/new", a.signedIn(a.recommend))
	mux.HandleFunc("POST /recommendations/preview", a.signedIn(a.musicPreview))
	mux.HandleFunc("GET /recommendations/{id}/preview", a.signedIn(a.savedPreview))
	mux.HandleFunc("POST /recommendations/{id}/preview", a.signedIn(a.retryPreview))
	mux.HandleFunc("GET /users/{id}/picture", a.signedIn(a.profilePicture))
	mux.HandleFunc("POST /account/picture", a.signedIn(a.saveProfilePicture))
	mux.HandleFunc("POST /account/animation", a.signedIn(a.saveAnimationPreference))
	mux.HandleFunc("GET /recommendations/{id}", a.signedIn(a.detail))
	mux.HandleFunc("GET /recommendations/{id}/thumbnail", a.signedIn(a.thumbnail))
	mux.HandleFunc("POST /recommendations/{id}/labels", a.signedIn(a.saveLabels))
	mux.HandleFunc("POST /recommendations/{id}/reaction", a.signedIn(a.react))
	mux.HandleFunc("POST /recommendations/{id}/comments", a.signedIn(a.comment))
	mux.HandleFunc("GET /groups", a.signedIn(a.groups))
	mux.HandleFunc("POST /groups", a.signedIn(a.saveGroup))
	mux.HandleFunc("POST /recommendations/{id}/recordings", a.signedIn(a.addRecording))
	mux.HandleFunc("POST /recommendations/{id}/position", a.signedIn(a.position))
	mux.HandleFunc("POST /recommendations/{id}/annotations", a.signedIn(a.annotationPreference))
	mux.HandleFunc("POST /recommendations/{id}/discussion", a.signedIn(a.discussion))
	mux.HandleFunc("POST /recommendations/{id}/share", a.signedIn(a.share))
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob:; frame-src https://www.youtube-nocookie.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		settings, err := a.store.Settings(r.Context(), store.DefaultSettings(a.config.BaseURL, a.config.WitmootURL))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		st := requestState{Settings: settings}
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
			limit := int64(16384)
			multipart := (r.URL.Path == "/admin/settings/images" || r.URL.Path == "/account/picture") && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
			if multipart {
				limit = 5 << 20
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			parseErr := r.ParseForm()
			if multipart {
				parseErr = r.ParseMultipartForm(5 << 20)
				if r.MultipartForm != nil {
					defer r.MultipartForm.RemoveAll()
					for _, files := range r.MultipartForm.File {
						if len(files) != 1 {
							http.Error(w, "duplicate upload fields", 400)
							return
						}
					}
				}
			}
			if err := parseErr; err != nil {
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
	p.Settings = st.Settings
	if p.SettingsDraft != nil {
		p.Settings = *p.SettingsDraft
	}
	p.Version = a.config.Version
	base := p.Settings.BaseURL
	if base == "" {
		scheme := "http"
		if a.secure(r) || r.TLS != nil {
			scheme = "https"
		}
		origin, err := url.Parse(scheme + "://" + r.Host)
		if err == nil && origin.Hostname() != "" && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == "" {
			base = origin.String()
		}
	}
	if base != "" {
		p.SocialImage = strings.TrimRight(base, "/") + "/static/jukebox.png"
	}
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
	if errors.Is(err, store.ErrForbidden) {
		http.Error(w, "permission denied", 403)
		return
	}
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
	if err != nil || check != nil || u.Suspended || len(password) > 72 {
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
	http.Redirect(w, r, "/shelf", 303)
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
func (a *App) shelf(w http.ResponseWriter, r *http.Request)   { a.list(w, r, false, false) }
func (a *App) history(w http.ResponseWriter, r *http.Request) { a.list(w, r, true, false) }
func (a *App) recent(w http.ResponseWriter, r *http.Request)  { a.list(w, r, false, true) }
func (a *App) list(w http.ResponseWriter, r *http.Request, history, recent bool) {
	layout := r.URL.Query().Get("layout")
	if layout == "" {
		layout = "chips"
	}
	if layout != "chips" && layout != "tiles" {
		http.Error(w, "invalid layout", 400)
		return
	}
	discovery := r.URL.Query().Get("discovery")
	if discovery == "" {
		discovery = "preferences"
	}
	if discovery != "preferences" && discovery != "all" {
		http.Error(w, "invalid discovery mode", 400)
		return
	}
	genre, tag := r.URL.Query().Get("genre"), r.URL.Query().Get("tag")
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
	person, _ := strconv.ParseInt(r.URL.Query().Get("person"), 10, 64)
	group, _ := strconv.ParseInt(r.URL.Query().Get("group"), 10, 64)
	for _, key := range []string{"person", "group"} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 0 {
				http.Error(w, "invalid filter", 400)
				return
			}
		}
	}
	kind := r.URL.Query().Get("kind")
	if kind != "" && kind != "track" && kind != "album" && kind != "artist" && kind != "link" {
		http.Error(w, "invalid music kind", 400)
		return
	}
	perspective := r.URL.Query().Get("view")
	if perspective == "" {
		perspective = "music"
		if recent {
			perspective = "recommendations"
		}
	}
	if perspective != "music" && perspective != "person" && perspective != "group" && perspective != "recommendations" {
		http.Error(w, "invalid view", 400)
		return
	}
	items, err := a.store.Browse(r.Context(), state(r).User.ID, history, store.Filter{Recent: recent, Status: status, Person: person, Group: group, Kind: kind, Genre: genre, Tag: tag, UsePreferences: recent && discovery == "preferences", GroupByMusic: perspective == "music"}, 51, offset)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	title := "Your shelf"
	if history {
		title = "Your history"
	}
	if recent {
		title = "Recent"
	}
	p := page{View: "list", Title: title, Items: items, History: history, Recent: recent, Status: status, Offset: offset, HasNext: len(items) > 50, HasPrevious: offset > 0, Next: offset + 50, Previous: max(0, offset-50)}
	if p.HasNext && perspective != "music" {
		p.Items = items[:50]
	}
	p.Person, p.GroupID, p.Kind, p.Perspective = person, group, kind, perspective
	p.Layout, p.Genre, p.Tag, p.Discovery = layout, genre, tag, discovery
	p.AvailableGenres, p.AvailableTags, err = a.store.LabelChoices(r.Context(), state(r).User.ID, recent)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Users, err = a.store.Users(r.Context(), 0)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Groups, err = a.store.Groups(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	seen := map[string]int{}
	for _, item := range p.Items {
		key := strconv.FormatInt(item.MediaID, 10)
		heading := item.Title
		if perspective == "person" {
			key = "person:" + strconv.FormatInt(item.SenderID, 10)
			heading = item.Sender
		}
		if perspective == "group" {
			key = "group:" + strconv.FormatInt(item.GroupID, 10)
			heading = item.Group
			if heading == "" {
				heading = "Between friends"
				if item.Visibility == "members" {
					heading = "Shared here"
				}
			}
		}
		if perspective == "recommendations" {
			key = strconv.FormatInt(item.ID, 10)
		}
		if i, ok := seen[key]; ok {
			p.Bundles[i].Items = append(p.Bundles[i].Items, recommendationCard{Recommendation: item})
		} else {
			seen[key] = len(p.Bundles)
			p.Bundles = append(p.Bundles, bundle{Heading: heading, Items: []recommendationCard{{Recommendation: item}}})
		}
	}
	for i := range p.Bundles {
		seenMusic := map[int64]bool{}
		for j := range p.Bundles[i].Items {
			mid := p.Bundles[i].Items[j].MediaID
			p.Bundles[i].Items[j].ShowMusic = !seenMusic[mid]
			seenMusic[mid] = true
		}
	}
	if perspective == "music" {
		p.HasNext = len(p.Bundles) > 50
		if p.HasNext {
			p.Bundles = p.Bundles[:50]
		}
	}
	a.render(w, r, 200, p)
}
func (a *App) newRecommendation(w http.ResponseWriter, r *http.Request) {
	a.compose(w, r, 200, page{Audience: "members"})
}
func (a *App) compose(w http.ResponseWriter, r *http.Request, status int, p page) {
	users, err := a.store.Users(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Groups, err = a.store.Groups(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.Users = users
	p.View = "compose"
	p.Title = "Share some music"
	a.render(w, r, status, p)
}
func (a *App) recommend(w http.ResponseWriter, r *http.Request) {
	p := page{Audience: r.PostForm.Get("audience"), Kind: r.PostForm.Get("kind"), MusicTitle: r.PostForm.Get("title"), Artist: r.PostForm.Get("artist"), URL: r.PostForm.Get("url"), Note: r.PostForm.Get("note"), Genre: r.PostForm.Get("genre"), Tags: r.PostForm.Get("tags")}
	_, explicit := r.PostForm["audience"]
	if explicit && (r.PostForm.Has("recipient") || r.PostForm.Has("group")) {
		p.Audience = ""
		p.Error = "Choose one audience for this recommendation."
		a.compose(w, r, 422, p)
		return
	}
	if !explicit {
		// Older forms and clients retain private delivery; omission never shares.
		group, err := parseOptionalID(r.PostForm.Get("group"))
		if err != nil {
			p.Error = "Choose a group from this instance."
			a.compose(w, r, 422, p)
			return
		}
		p.Audience = "person:" + r.PostForm.Get("recipient")
		if group > 0 {
			p.Audience = "group:" + strconv.FormatInt(group, 10)
		}
	}
	scope, rawID, _ := strings.Cut(p.Audience, ":")
	if p.Audience != "members" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 || scope != "person" && scope != "group" {
			p.Error = "Choose who can see this recommendation."
			a.compose(w, r, 422, p)
			return
		}
		if scope == "group" {
			p.GroupID = id
		} else {
			p.Recipient = id
		}
	}
	if p.Recipient > 0 {
		users, err := a.store.Users(r.Context(), state(r).User.ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		found := false
		for _, u := range users {
			found = found || u.ID == p.Recipient
		}
		if !found {
			p.Error = "Choose a friend from this instance."
			a.compose(w, r, 422, p)
			return
		}
	}
	var id int64
	var err error
	tags, err := store.ParseLabels(p.Tags, 40)
	if err != nil {
		p.Error = "Use up to 20 comma-separated tags, each at most 40 characters."
		a.compose(w, r, 422, p)
		return
	}
	labels := store.Labels{Genre: p.Genre, Tags: tags}
	if p.Audience == "members" {
		id, err = a.store.ShareLabeledMusic(r.Context(), state(r).User.ID, p.URL, p.Note, p.Kind, p.MusicTitle, p.Artist, labels)
	} else {
		id, err = a.store.RecommendLabeledMusic(r.Context(), state(r).User.ID, p.Recipient, p.GroupID, p.URL, p.Note, p.Kind, p.MusicTitle, p.Artist, labels)
	}
	if err != nil {
		// Validate before storage; unknown database failures stay server errors.
		if errors.Is(err, store.ErrInvalid) {
			p.Error = "Keep your note within 2,000 characters, title and artist within 160, genre within 80, and each of up to 20 tags within 40. Genre and tags use single-line names."
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
	tracks, err := a.store.Recordings(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	mode, err := a.store.AnnotationMode(r.Context(), state(r).User.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	reveal := r.URL.Query().Get("reveal") == "1"
	comments = store.VisibleComments(comments, tracks, mode, reveal)
	links, err := a.store.Discussions(r.Context(), state(r).User.ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p := page{View: "detail", Title: item.Title, Item: item, Comments: comments, Recordings: tracks, AnnotationMode: mode, Reveal: reveal, Discussions: links, Error: message, Handoff: state(r).Settings.WitmootURL, CommentDraft: r.PostForm.Get("body"), Genre: item.Genre, Tags: strings.Join(item.Tags, ", ")}
	if !state(r).Settings.WitmootDiscussions() {
		p.Handoff = ""
	}
	if status == 200 && r.Method == http.MethodGet {
		switch r.URL.Query().Get("saved") {
		case "feedback":
			p.FeedbackNotice = "Listening feedback saved."
		case "annotations":
			p.AnnotationNotice = "Spoiler preference saved for your account."
		case "labels":
			p.LabelsNotice = "Genre and tags saved."
		}
	}
	if status == 422 && strings.HasSuffix(r.URL.Path, "/reaction") {
		p.Item.Listening = r.PostForm.Get("listening")
		p.Item.PersonalNote = r.PostForm.Get("personal_note")
		if rating, err := strconv.Atoi(r.PostForm.Get("rating")); err == nil && rating >= -1 && rating <= 1 {
			p.Item.Rating = rating
		}
	}
	if status == 422 && strings.HasSuffix(r.URL.Path, "/labels") {
		p.Genre, p.Tags = r.PostForm.Get("genre"), r.PostForm.Get("tags")
	}
	a.render(w, r, status, p)
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
	http.Redirect(w, r, "/recommendations/"+strconv.FormatInt(id, 10)+"?saved=feedback#listening-notes", 303)
}
func (a *App) comment(w http.ResponseWriter, r *http.Request) {
	id, err := recommendationID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.store.Recommendation(r.Context(), state(r).User.ID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	if !state(r).Settings.LocalComments() {
		a.showDetail(w, r, id, http.StatusForbidden, "New comments are posted in Witmoot. A separate Witmoot account is required; ask your host for an invitation.")
		return
	}
	track, parseErr := parseOptionalID(r.PostForm.Get("recording"))
	if parseErr != nil {
		a.showDetail(w, r, id, 422, "Choose a recording.")
		return
	}
	err = a.store.Annotate(r.Context(), state(r).User.ID, id, track, r.PostForm.Get("body"))
	if errors.Is(err, store.ErrInvalid) {
		a.showDetail(w, r, id, 422, "Write a comment of 1-2,000 characters. For timestamps on an album, choose a track first.")
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
