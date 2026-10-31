// Команда bot — Telegram-бот анонимных вопросов «Secure Ask».
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/config"
	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/ratelimit"
	"github.com/vitaly/secure-ask-tg-bot/internal/service"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
	"github.com/vitaly/secure-ask-tg-bot/internal/transport/api"
	tgtransport "github.com/vitaly/secure-ask-tg-bot/internal/transport/telegram"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("фатальная ошибка", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("конфиг: %w", err)
	}

	st, err := storage.New(ctx, cfg.DBURL)
	if err != nil {
		return fmt.Errorf("хранилище: %w", err)
	}
	defer st.Close()
	log.Info("миграции применены, БД доступна")

	codec, err := crypto.NewCodec(cfg.EncryptionKey, cfg.HMACKey)
	if err != nil {
		return fmt.Errorf("крипто: %w", err)
	}

	if err := st.EnsureOwnerSysadmin(ctx, cfg.OwnerTgID); err != nil {
		return fmt.Errorf("owner sysadmin: %w", err)
	}

	if err := st.EnsureDefaultAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Warn("ensure default admin", "err", err)
	}

	metrics := &tgtransport.Metrics{}
	var router *tgtransport.Router // заполняется ниже; хендлеру нужен до создания

	opts := []bot.Option{
		// Default-хендлер лишь кладёт update в шардированную очередь — обработка
		// не блокирует ни webhook-запрос Telegram, ни цикл getUpdates (docs/03).
		bot.WithDefaultHandler(func(hctx context.Context, b *bot.Bot, update *models.Update) {
			router.HandleUpdate(hctx, b, update)
		}),
		bot.WithNotAsyncHandlers(), // горутина на update не нужна: очередь и так неблокирующая по сути
	}
	if cfg.Mode == config.ModeWebhook {
		opts = append(opts, bot.WithWebhookSecretToken(cfg.WebhookSecret))
	}
	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		return fmt.Errorf("bot: %w", err)
	}
	me, err := b.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("getMe: %w", err)
	}
	log.Info("бот авторизован", "username", me.Username)

	store := storeAdapter{st}
	tgClient := tgtransport.NewClient(b)
	burst := ratelimit.New(cfg.RateBurstPerHour)

	users := service.NewUsers(store, cfg.OwnerTgID)
	links := service.NewLinks(store, me.Username)
	questions := service.NewQuestions(store, codec, tgClient, burst, cfg.RateDailyLimit, log)
	blocks := service.NewBlocks(store)
	admin := service.NewAdmin(store, tgClient, questions, cfg.OwnerTgID, log)

	router = tgtransport.NewRouter(tgtransport.Services{
		Users:     users,
		Links:     links,
		Questions: questions,
		Blocks:    blocks,
		Admin:     admin,
		Store:     store,
	}, tgClient, cfg.Workers, cfg.OwnerTgID, me.Username, metrics, log)

	setupCommands(ctx, b, cfg.OwnerTgID, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# TYPE bot_updates_total counter\nbot_updates_total %d\n", metrics.UpdatesTotal.Load())
		fmt.Fprintf(w, "# TYPE bot_errors_total counter\nbot_errors_total %d\n", metrics.ErrorsTotal.Load())
		fmt.Fprintf(w, "# TYPE bot_queue_len gauge\nbot_queue_len %d\n", metrics.QueueLen.Load())
	})

	apiServer := api.NewServer(st, codec, b, cfg.BotToken, cfg.HMACKey, log)
	apiServer.RegisterRoutes(mux)

	if cfg.WebDir != "" {
		mux.HandleFunc("/", api.StaticHandler(cfg.WebDir))
		log.Info("раздача веб-панели включена", "dir", cfg.WebDir)
	}

	routerDone := make(chan struct{})
	go func() {
		defer close(routerDone)
		router.Run(ctx)
	}()

	botDone := make(chan struct{})
	switch cfg.Mode {
	case config.ModeWebhook:
		wu, err := url.Parse(cfg.WebhookURL)
		if err != nil {
			return fmt.Errorf("WEBHOOK_URL: %w", err)
		}
		if _, err := b.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:         cfg.WebhookURL,
			SecretToken: cfg.WebhookSecret,
		}); err != nil {
			return fmt.Errorf("setWebhook: %w", err)
		}
		mux.HandleFunc(wu.Path, b.WebhookHandler())
		go func() {
			defer close(botDone)
			b.StartWebhook(ctx)
		}()
		log.Info("режим webhook", "path", wu.Path)
	case config.ModePolling:
		// Снять возможный webhook, иначе getUpdates вернёт конфликт.
		if _, err := b.DeleteWebhook(ctx, &bot.DeleteWebhookParams{}); err != nil {
			log.Warn("deleteWebhook предупреждение (можно игнорировать)", "err", err)
		}
		go func() {
			defer close(botDone)
			b.Start(ctx)
		}()
		log.Info("режим polling (локальная разработка)")
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	srvErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()
	log.Info("HTTP-сервер запущен", "addr", cfg.ListenAddr)

	select {
	case <-ctx.Done():
		log.Info("получен сигнал остановки")
	case err := <-srvErr:
		stop()
		log.Error("HTTP-сервер упал", "err", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("остановка HTTP", "err", err)
	}
	<-botDone

	// Дообрабатываем хвост очередей — принятые вопросы не теряются.
	router.Stop()
	select {
	case <-routerDone:
		log.Info("воркеры завершены")
	case <-shutdownCtx.Done():
		log.Warn("воркеры не успели завершиться за таймаут")
	}
	return nil
}

func setupCommands(ctx context.Context, b *bot.Bot, ownerTgID int64, log *slog.Logger) {
	if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: tgtransport.UserCommands(),
		Scope:    &models.BotCommandScopeAllPrivateChats{},
	}); err != nil {
		log.Warn("setMyCommands (default)", "err", err)
	}
	adminCmds := append(tgtransport.UserCommands(),
		models.BotCommand{Command: "admin", Description: "Панель администрирования"})
	if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: adminCmds,
		Scope:    &models.BotCommandScopeChat{ChatID: ownerTgID},
	}); err != nil {
		log.Warn("setMyCommands (owner)", "err", err)
	}
}

// storeAdapter адаптирует storage.Storage к service.Store:
// транзакционный колбэк принимает интерфейс, а не *gen.Queries.
type storeAdapter struct {
	*storage.Storage
}

func (a storeAdapter) WithTx(ctx context.Context, fn func(q service.Querier) error) error {
	return a.Storage.WithTx(ctx, func(q *gen.Queries) error { return fn(q) })
}
