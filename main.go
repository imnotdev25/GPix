package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gpix/pkg/accounts"
	"gpix/pkg/albums"
	"gpix/pkg/apitoken"
	"gpix/pkg/bridge"
	"gpix/pkg/cachedb"
	"gpix/pkg/dav"
	"gpix/pkg/gpmc"
	"gpix/pkg/gwcreds"
	"gpix/pkg/immichimport"
	"gpix/pkg/library"
	"gpix/pkg/mediacrypt"
	"gpix/pkg/oidc"
	"gpix/pkg/s3"
	"gpix/pkg/share"
	"gpix/pkg/store/gpmcstore"
	"gpix/pkg/uploads"
	"gpix/pkg/users"
	"gpix/pkg/web"
)

func main() {
	mode := flag.String("mode", "all", "all | bot | web | cli | hashpw")
	cli := flag.Bool("cli", false, "shortcut for -mode cli")
	hashpw := flag.Bool("hashpw", false, "shortcut for -mode hashpw")

	envFlag := flag.String("env", ".env", "path to .env file (skipped if missing)")
	authFlag := flag.String("auth", "", "GP auth_data (defaults to $GP_AUTH_DATA)")
	profileFlag := flag.String("profile", "", "pixel-xl | pixel-5 (default: from web config or pixel-xl)")
	logLevel := flag.String("log", "info", "debug | info | warn | error")
	cfgFlag := flag.String("config", "gpix-web.conf", "deprecated: gpix-web.conf is no longer read (settings come from .env); its folder is still the default state folder")
	secretFlag := flag.String("secret", "", "path to secret.key (default: in GPIX_DATA_DIR, else the working directory)")

	qualityFlag := flag.String("quality", "original", "[cli] original | saver | quota")
	conc := flag.Int("concurrency", 1, "[cli] parallel uploads")
	recursive := flag.Bool("recursive", false, "[cli] descend into directories")
	force := flag.Bool("force", false, "[cli] skip dedup")
	deleteAfter := flag.Bool("delete-after", false, "[cli] delete local file after successful upload")

	flag.Parse()

	switch {
	case *cli:
		*mode = "cli"
	case *hashpw:
		*mode = "hashpw"
	}

	if err := loadDotEnv(*envFlag); err != nil {
		fmt.Fprintln(os.Stderr, "warn: load env:", err)
	}
	// Apply a TZ set in .env (photo capture dates without a zone are read in it).
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			time.Local = loc
		}
	}

	logFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "log" {
			logFlagSet = true
		}
	})
	log := newLogger(*logLevel, logFlagSet)

	switch *mode {
	case "hashpw":
		runHashpw()
		return
	case "cli":
		runCLI(*authFlag, *profileFlag, *qualityFlag, *conc, *recursive, *force, *deleteAfter)
		return
	}

	auth := *authFlag
	if auth == "" {
		auth = os.Getenv("GP_AUTH_DATA")
	}
	if auth == "" {
		die("missing GP_AUTH_DATA (in .env, env var, or -auth flag)")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch *mode {
	case "bot":
		if !telegramEnabled() {
			die("the Telegram bot is disabled; set TG_BOT_ENABLED=true to run -mode bot")
		}
		runBot(ctx, log, auth, *profileFlag)
	case "web":
		runWeb(ctx, log, auth, *cfgFlag, *secretFlag, *profileFlag)
	case "all":
		runAll(ctx, log, auth, *cfgFlag, *secretFlag, *profileFlag)
	default:
		die("unknown mode: " + *mode + " (want all|bot|web|cli|hashpw)")
	}
}

func runAll(ctx context.Context, log *slog.Logger, auth, cfgPath, secretPath, profileFlag string) {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	// The Telegram bot is off unless TG_BOT_ENABLED is set; otherwise `-mode all`
	// runs just the web UI and gateways.
	if telegramEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startBot(ctx, log.With("service", "bot"), auth, profileFlag); err != nil {
				errs <- fmt.Errorf("bot: %w", err)
			}
		}()
	} else {
		log.Info("telegram bot disabled (set TG_BOT_ENABLED=true to enable)")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := startWeb(ctx, log.With("service", "web"), auth, cfgPath, secretPath, profileFlag); err != nil {
			errs <- fmt.Errorf("web: %w", err)
		}
	}()

	wg.Wait()
	close(errs)
	var first error
	for e := range errs {
		log.Error("service failed", "err", e)
		if first == nil {
			first = e
		}
	}
	if first != nil {
		os.Exit(1)
	}
}

// telegramEnabled reports whether the Telegram bot feature is switched on. It is
// off by default, so bot credentials in the environment never start it alone.
func telegramEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TG_BOT_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func runBot(ctx context.Context, log *slog.Logger, auth, profileFlag string) {
	if err := startBot(ctx, log, auth, profileFlag); err != nil {
		log.Error("bot stopped", "err", err)
		os.Exit(1)
	}
}

func runWeb(ctx context.Context, log *slog.Logger, auth, cfgPath, secretPath, profileFlag string) {
	if err := startWeb(ctx, log, auth, cfgPath, secretPath, profileFlag); err != nil {
		log.Error("web stopped", "err", err)
		os.Exit(1)
	}
}

func startBot(ctx context.Context, log *slog.Logger, auth, profileFlag string) error {
	cfg, err := bridge.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	profile, err := parseProfile(coalesce(profileFlag, "pixel-xl"))
	if err != nil {
		return err
	}

	gp, err := gpmc.New(auth, gpmc.WithDeviceProfile(profile))
	if err != nil {
		return fmt.Errorf("gpmc.New: %w", err)
	}

	bot, err := bridge.New(cfg, gp, log)
	if err != nil {
		return fmt.Errorf("bridge.New: %w", err)
	}

	log.Info("gpix bot started",
		"owner", cfg.OwnerID,
		"temp_dir", cfg.TempDir,
		"max_concurrent", cfg.MaxConcurrent,
	)
	return bot.Start(ctx)
}

func startWeb(ctx context.Context, log *slog.Logger, auth, cfgPath, secretPath, profileFlag string) error {
	// Settings come only from the environment (.env). An old gpix-web.conf is
	// no longer read; say what to move rather than ignoring it silently.
	if vars, found := web.LegacyConfVars(cfgPath); found {
		if len(vars) > 0 {
			log.Warn("gpix-web.conf is no longer read; put these in .env instead", "file", cfgPath, "set", strings.Join(vars, ", "))
		} else {
			log.Warn("gpix-web.conf is no longer read and can be deleted", "file", cfgPath)
		}
	}
	cfg := web.LoadConfig()

	if secretPath == "" {
		dir := cfg.DataDir
		if dir == "" {
			dir = filepath.Dir(cfgPath)
		}
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o700)
		}
		secretPath = filepath.Join(dir, "secret.key")
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(secretPath) // keep the thumbnail cache with the rest of the state
	}
	secret, err := web.LoadOrCreateSecret(secretPath)
	if err != nil {
		return fmt.Errorf("secret: %w", err)
	}
	cfg.SecretKey = secret

	// Gateway credentials (S3 keys + WebDAV app password) live next to secret.key
	// and are managed from the web UI. Seed them from any values in the config.
	gwPath := filepath.Join(filepath.Dir(secretPath), "gateways.json")
	gw, err := gwcreds.Load(gwPath)
	if err != nil {
		return fmt.Errorf("gateway credentials: %w", err)
	}
	if err := gw.Seed(cfg.S3AccessKey, cfg.S3SecretKey, ""); err != nil {
		return fmt.Errorf("seed gateway credentials: %w", err)
	}

	// Media-encryption manager: master key + "encrypt new uploads" toggle, also
	// stored next to secret.key.
	encKeyPath := filepath.Join(filepath.Dir(secretPath), "encryption.key")
	encStatePath := filepath.Join(filepath.Dir(secretPath), "encryption.json")
	crypt, err := mediacrypt.Load(encKeyPath, encStatePath)
	if err != nil {
		return fmt.Errorf("encryption: %w", err)
	}
	if _, statErr := os.Stat(encStatePath); os.IsNotExist(statErr) && cfg.EncryptUploads {
		if err := crypt.SetEnabled(true); err != nil {
			return fmt.Errorf("seed encryption toggle: %w", err)
		}
	}

	profileName := coalesce(profileFlag, cfg.DeviceProfile, "pixel-xl")
	profile, err := parseProfile(profileName)
	if err != nil {
		return err
	}

	// Linked Google accounts. GP_AUTH_DATA seeds the first ("Primary") account on
	// first run; more can be linked from the web UI, and the active one backs the
	// whole app.
	stateDir := filepath.Dir(secretPath)
	acctStore, err := accounts.Open(filepath.Join(stateDir, "accounts.db"))
	if err != nil {
		return fmt.Errorf("accounts db: %w", err)
	}
	defer acctStore.Close()
	if n, cerr := acctStore.Count(ctx); cerr == nil && n == 0 {
		if auth == "" {
			return fmt.Errorf("no accounts linked and GP_AUTH_DATA is empty — see the Auth section of the README")
		}
		if _, aerr := acctStore.Add(ctx, "Primary", auth); aerr != nil {
			return fmt.Errorf("seed account: %w", aerr)
		}
		log.Info("linked the GP_AUTH_DATA account as 'Primary'")
	}

	// Each account gets its own Google client, listing cache (with a SQLite
	// snapshot so listings are instant on startup) and object-store backend.
	// Built lazily on first activation and reused, so switching back is instant.
	var cacheStores []*cachedb.Store
	defer func() {
		for _, cs := range cacheStores {
			cs.Close()
		}
	}()

	// Log of items gpix itself uploads, per account. Google's listing can't tell
	// gpix uploads from the phone's, so "delete gpix uploads" relies on this.
	uploadLog, err := uploads.Open(filepath.Join(stateDir, "uploads.db"))
	if err != nil {
		return fmt.Errorf("uploads db: %w", err)
	}
	defer uploadLog.Close()

	buildAccount := func(bctx context.Context, id int64, label, authData string) (*accounts.Runtime, error) {
		var alib *library.Cache
		hook := func(it gpmc.MediaItem) {
			// List the upload right away instead of after the next full listing.
			if alib != nil {
				alib.Add(it)
			}
			if rerr := uploadLog.Record(context.Background(), id, it.MediaKey, it.Filename); rerr != nil {
				log.Warn("upload log: record", "account", label, "err", rerr)
			}
		}
		agp, gerr := gpmc.New(authData, gpmc.WithDeviceProfile(profile), gpmc.WithUploadHook(hook))
		if gerr != nil {
			return nil, fmt.Errorf("gpmc.New: %w", gerr)
		}
		cs, cerr := cachedb.Open(filepath.Join(stateDir, fmt.Sprintf("cache_%d.db", id)))
		if cerr != nil {
			return nil, fmt.Errorf("cache db: %w", cerr)
		}
		cacheStores = append(cacheStores, cs)
		alib = library.New(agp, 0, cs)
		alib.SetLogger(log.With("account", label))
		alib.Start(ctx, 0)
		// Bounded warm-up so the first S3/WebDAV listing serves from memory instead
		// of blocking a client on a cold full walk of Google. Instant when a
		// snapshot exists; a huge first-run library keeps warming in the background.
		if alib.WarmUp(bctx, 20*time.Second) {
			log.Info("library cache warm", "account", label)
		} else {
			log.Info("library cache warming in background; S3/WebDAV listings will speed up once it finishes", "account", label)
		}
		return &accounts.Runtime{
			ID:    id,
			Label: label,
			GP:    agp,
			Lib:   alib,
			BE:    gpmcstore.New(agp, gpmcstore.Options{TempDir: cfg.TempDir, Crypt: crypt, Library: alib}),
		}, nil
	}
	acctMgr, err := accounts.NewManager(ctx, acctStore, buildAccount)
	if err != nil {
		return fmt.Errorf("accounts: %w", err)
	}
	// Object store that always follows the active account, so the gateways and
	// the importer switch along with it.
	be := accounts.NewDynamicBackend(acctMgr)

	// Immich → gpix importer (background job; remembers imported asset IDs in
	// import.db so re-runs are idempotent).
	importer, err := immichimport.New(be, filepath.Join(filepath.Dir(secretPath), "import.db"), log.With("service", "import"))
	if err != nil {
		return fmt.Errorf("import db: %w", err)
	}
	defer importer.Close()

	// SQLite-backed share links, stored next to secret.key.
	sharePath := filepath.Join(filepath.Dir(secretPath), "shares.db")
	shareStore, err := share.Open(sharePath)
	if err != nil {
		return fmt.Errorf("share store: %w", err)
	}
	defer shareStore.Close()

	// API tokens for native clients (gpix app) hitting /api/v1.
	apiTokens, err := apitoken.Open(filepath.Join(filepath.Dir(secretPath), "api-tokens.db"))
	if err != nil {
		return fmt.Errorf("api-token store: %w", err)
	}
	defer apiTokens.Close()

	// Sign-in accounts: created by sign-up (until GPIX_MAX_USERS accounts
	// exist) or by single sign-on, both limited by GPIX_SIGNUP_ALLOWLIST.
	us, err := users.Open(filepath.Join(stateDir, "users.db"))
	if err != nil {
		return fmt.Errorf("users db: %w", err)
	}
	defer us.Close()
	if cfg.LegacyUsername != "" && cfg.LegacyPasswordHash != "" {
		created, ierr := us.ImportLogin(ctx, cfg.LegacyUsername, cfg.LegacyPasswordHash)
		if ierr != nil {
			return fmt.Errorf("import GPIX_USERNAME login: %w", ierr)
		}
		if created {
			log.Info("imported the GPIX_USERNAME / GPIX_PASSWORD_HASH login as an account; you can remove both from .env", "login", cfg.LegacyUsername)
		}
	}
	if n, cerr := us.Count(ctx); cerr == nil && n == 0 {
		log.Info("no accounts yet: open the web UI to create the first one", "max_users", cfg.MaxUsers)
	}

	// Optional Logto / OIDC single sign-on.
	var oc *oidc.Client
	if cfg.LogtoEnabled() {
		redirect := cfg.LogtoRedirect
		if redirect == "" {
			if cfg.ServerURL == "" {
				return fmt.Errorf("logto: set server_url (or logto_redirect) so the OAuth callback URL is known")
			}
			redirect = cfg.ServerURL + "/auth/logto/callback"
		}
		oc = oidc.New(oidc.Config{
			Issuer:       cfg.LogtoEndpoint,
			ClientID:     cfg.LogtoClientID,
			ClientSecret: cfg.LogtoClientSecret,
			RedirectURL:  redirect,
		})
		log.Info("logto login enabled", "redirect", redirect, "max_users", cfg.MaxUsers, "allowlist", len(cfg.SignupAllowlist))
	}

	// gpix's own albums and tags (not synced with Google Photos albums).
	albumStore, err := albums.Open(filepath.Join(stateDir, "albums.db"))
	if err != nil {
		return fmt.Errorf("albums db: %w", err)
	}
	defer albumStore.Close()

	srv, err := web.New(cfg, acctMgr, gw, crypt, shareStore, oc, us, importer, apiTokens, uploadLog, albumStore, log)
	if err != nil {
		return fmt.Errorf("web.New: %w", err)
	}

	log.Info("gpix web started",
		"version", web.AppVersion(),
		"listen", cfg.Listen,
		"profile", profileName,
		"data_dir", cfg.DataDir,
		"max_users", cfg.MaxUsers,
		"signup_allowlist", len(cfg.SignupAllowlist),
		"password_login", !cfg.DisablePasswordLogin,
		"thumb_gen", cfg.ThumbGen,
		"s3", cfg.S3Listen != "",
		"webdav", cfg.WebDAVListen != "",
	)
	if web.LeakyHEICDecoder {
		log.Warn(`built without -tags "nodynamic wasm2go": HEIC thumbnails leak about 400 MB per large photo; rebuild with those tags`)
	}

	runners := []func(context.Context) error{srv.Run}

	if cfg.S3Listen != "" {
		// Credentials resolve through the shared store, so keys generated in the
		// web UI take effect immediately without a restart.
		s3srv, err := s3.New(s3.Config{
			Listen:      cfg.S3Listen,
			Bucket:      cfg.S3Bucket,
			Region:      cfg.S3Region,
			Credentials: gw,
		}, be, log.With("service", "s3"))
		if err != nil {
			return fmt.Errorf("s3.New: %w", err)
		}
		log.Info("gpix s3 gateway started", "listen", cfg.S3Listen, "bucket", cfg.S3Bucket)
		runners = append(runners, s3srv.Run)
	}

	if cfg.WebDAVListen != "" {
		davsrv, err := dav.New(dav.Config{
			Listen:   cfg.WebDAVListen,
			BasePath: cfg.WebDAVBasePath,
			Realm:    "gpix",
		}, be, basicAuthChecker(us, gw), log.With("service", "webdav"))
		if err != nil {
			return fmt.Errorf("dav.New: %w", err)
		}
		log.Info("gpix webdav gateway started", "listen", cfg.WebDAVListen, "base", cfg.WebDAVBasePath)
		runners = append(runners, davsrv.Run)
	}

	return runConcurrently(ctx, runners)
}

// runConcurrently runs every runner until ctx is cancelled or one of them
// returns an error, in which case the others are signalled to stop.
func runConcurrently(ctx context.Context, runners []func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(runners))
	var wg sync.WaitGroup
	for _, run := range runners {
		run := run
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := run(ctx); err != nil {
				errCh <- err
				cancel()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			return e
		}
	}
	return nil
}

// basicAuthChecker returns a dav.Authenticator for WebDAV Basic auth: the
// username is a gpix account's email (or imported login) and the password is
// that account's password or the revocable WebDAV app password. Successful
// pairs are remembered for a few minutes by a SHA-256 fingerprint, so bcrypt
// doesn't run on each of the many requests a WebDAV client makes.
func basicAuthChecker(us *users.Store, gw *gwcreds.Store) dav.Authenticator {
	var (
		mu   sync.Mutex
		good = map[[32]byte]time.Time{}
	)
	return func(user, pass string) bool {
		ctx := context.Background()
		fp := sha256.Sum256([]byte(user + "\x00" + pass))
		mu.Lock()
		t, ok := good[fp]
		mu.Unlock()
		if ok && time.Since(t) < 5*time.Minute {
			return true
		}
		_, uerr := us.GetByEmail(ctx, user)
		switch {
		case uerr != nil:
			_, _ = us.Authenticate(ctx, user, pass) // same cost as a wrong password
			return false
		case gw != nil && gw.CheckWebDAVPassword(pass):
		default:
			if _, err := us.Authenticate(ctx, user, pass); err != nil {
				return false
			}
		}
		mu.Lock()
		good[fp] = time.Now()
		mu.Unlock()
		return true
	}
}

func runCLI(authFlag, profileFlag, qualityFlag string, conc int, recursive, force, deleteAfter bool) {
	auth := authFlag
	if auth == "" {
		auth = os.Getenv("GP_AUTH_DATA")
	}
	if auth == "" {
		die("missing GP auth: pass -auth or set GP_AUTH_DATA")
	}
	if flag.NArg() == 0 {
		die("missing positional path argument")
	}

	q, err := parseQuality(qualityFlag)
	if err != nil {
		die(err.Error())
	}
	profile, err := parseProfile(coalesce(profileFlag, "pixel-xl"))
	if err != nil {
		die(err.Error())
	}

	client, err := gpmc.New(auth, gpmc.WithDeviceProfile(profile))
	if err != nil {
		die(err.Error())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := gpmc.UploadOpts{
		Quality:     q,
		Force:       force,
		Concurrency: conc,
		Recursive:   recursive,
		DeleteAfter: deleteAfter,
	}

	results, err := client.UploadFiles(ctx, flag.Args(), opts, func(ev gpmc.UploadEvent) {
		if ev.Err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %s: %v\n", ev.Stage, ev.Path, ev.Err)
			return
		}
		fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.Stage, ev.Path)
	})
	if err != nil {
		die(err.Error())
	}

	var failures int
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", r.Path, r.Err)
			failures++
			continue
		}
		marker := "OK"
		if r.Skipped {
			marker = "SKIP"
		}
		fmt.Printf("%s\t%s\t%s\n", marker, r.Path, r.MediaKey)
	}
	if failures > 0 {
		os.Exit(2)
	}
}

func runHashpw() {
	fmt.Fprint(os.Stderr, "password: ")
	r := bufio.NewReader(os.Stdin)
	pw, _ := r.ReadString('\n')
	pw = strings.TrimRight(pw, "\r\n")
	if pw == "" {
		die("empty password")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	if err != nil {
		die(err.Error())
	}
	fmt.Println(string(h))
}

func parseQuality(s string) (gpmc.Quality, error) {
	switch s {
	case "original":
		return gpmc.QualityOriginal, nil
	case "saver":
		return gpmc.QualitySaver, nil
	case "quota":
		return gpmc.QualityUseQuota, nil
	}
	return 0, fmt.Errorf("unknown quality %q (want original|saver|quota)", s)
}

func parseProfile(s string) (gpmc.DeviceProfile, error) {
	switch s {
	case "", "pixel-xl":
		return gpmc.DefaultPixelXL(), nil
	case "pixel-5":
		return gpmc.DefaultPixel5(), nil
	}
	return gpmc.DeviceProfile{}, fmt.Errorf("unknown profile %q", s)
}

// newLogger builds the logger from the -log flag (when given) or GPIX_LOG_LEVEL,
// and GPIX_LOG_FORMAT (text, the default, or json for log collectors). It also
// becomes slog's default, so packages that fall back to it log the same way.
func newLogger(flagLevel string, flagSet bool) *slog.Logger {
	level := flagLevel
	if !flagSet {
		level = coalesce(os.Getenv("GPIX_LOG_LEVEL"), "info")
	}
	var lvl slog.Level
	badLevel := false
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "info", "":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl, badLevel = slog.LevelInfo, true
	}
	opts := &slog.HandlerOptions{Level: lvl}
	format := strings.ToLower(strings.TrimSpace(os.Getenv("GPIX_LOG_FORMAT")))
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	l := slog.New(h)
	if badLevel {
		l.Warn("unknown log level; using info", "level", level)
	}
	if format != "" && format != "json" && format != "text" {
		l.Warn("unknown GPIX_LOG_FORMAT; using text", "format", format)
	}
	slog.SetDefault(l)
	return l
}

func coalesce(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

var _ = errors.New
