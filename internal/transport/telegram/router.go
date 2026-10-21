package telegram

import (
	"context"
	"hash/fnv"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/ratelimit"
	"github.com/vitaly/secure-ask-tg-bot/internal/service"
)

// Metrics — атомарные счётчики для /metrics (Prometheus text format в main).
type Metrics struct {
	UpdatesTotal atomic.Int64
	ErrorsTotal  atomic.Int64
	QueueLen     atomic.Int64
}

// Services — зависимости хендлеров (собираются в main).
type Services struct {
	Users     *service.Users
	Links     *service.Links
	Questions *service.Questions
	Blocks    *service.Blocks
	Admin     *service.Admin
	Store     service.Store
}

// Router принимает обновления от библиотеки бота и раскладывает их по
// шардированным очередям: обновления одного чата обрабатываются строго
// последовательно (диалоговые состояния не гоняются), разные чаты — параллельно.
type Router struct {
	svc       Services
	tg        *Client
	log       *slog.Logger
	metrics   *Metrics
	msgLimit  *ratelimit.Limiter // антифлуд на все входящие по tg_id
	ownerTgID int64
	botName   string // username бота без @ (для ссылок и /cmd@bot)

	shards []chan *models.Update
	wg     sync.WaitGroup
}

// NewRouter создаёт роутер. workers = 0 — значение по умолчанию GOMAXPROCS×4.
func NewRouter(svc Services, tg *Client, workers int, ownerTgID int64, botName string, metrics *Metrics, log *slog.Logger) *Router {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) * 4
	}
	r := &Router{
		svc:       svc,
		tg:        tg,
		log:       log,
		metrics:   metrics,
		msgLimit:  ratelimit.New(60), // 60 обновлений/час с burst — защита от флуда апдейтами
		ownerTgID: ownerTgID,
		botName:   botName,
		shards:    make([]chan *models.Update, workers),
	}
	for i := range r.shards {
		r.shards[i] = make(chan *models.Update, 64)
	}
	return r
}

// botUsername — username бота без @.
func (r *Router) botUsername() string { return r.botName }

// Run запускает воркеров и блокируется до их завершения.
// Останавливать через Stop() после завершения приёма обновлений.
func (r *Router) Run(ctx context.Context) {
	for i := range r.shards {
		r.wg.Add(1)
		go func(ch chan *models.Update) {
			defer r.wg.Done()
			for upd := range ch {
				r.metrics.QueueLen.Add(-1)
				r.process(ctx, upd)
			}
		}(r.shards[i])
	}
	r.wg.Wait()
}

// Stop закрывает очереди; Run вернётся, когда воркеры дообработают хвост.
// Ни один принятый вопрос не теряется при деплое (docs/03).
func (r *Router) Stop() {
	for _, ch := range r.shards {
		close(ch)
	}
}

// HandleUpdate — default-хендлер для библиотеки бота: только кладёт update
// в шард по chat_id и мгновенно возвращается (Telegram не ждёт бизнес-логику).
func (r *Router) HandleUpdate(_ context.Context, _ *bot.Bot, update *models.Update) {
	chatID := updateChatID(update)
	if chatID == 0 {
		return // не личное сообщение и не колбэк — игнорируем
	}
	h := fnv.New32a()
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(chatID >> (8 * i))
	}
	_, _ = h.Write(buf[:])
	r.metrics.UpdatesTotal.Add(1)
	r.metrics.QueueLen.Add(1)
	r.shards[h.Sum32()%uint32(len(r.shards))] <- update
}

// updateChatID возвращает chat_id для шардирования (0 — пропустить update).
func updateChatID(u *models.Update) int64 {
	switch {
	case u.Message != nil && u.Message.Chat.Type == models.ChatTypePrivate:
		return u.Message.Chat.ID
	case u.CallbackQuery != nil:
		return u.CallbackQuery.From.ID
	default:
		return 0
	}
}

// process обрабатывает один update: recovery, антифлуд, маршрутизация.
func (r *Router) process(ctx context.Context, update *models.Update) {
	defer func() {
		if p := recover(); p != nil {
			r.metrics.ErrorsTotal.Add(1)
			r.log.Error("паника в хендлере", "update_id", update.ID, "panic", p)
		}
	}()

	var err error
	switch {
	case update.Message != nil:
		msg := update.Message
		if msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}
		if !r.msgLimit.Allow(itoa(msg.From.ID)) {
			return // тихо игнорируем флуд
		}
		err = r.onMessage(ctx, msg)
	case update.CallbackQuery != nil:
		if !r.msgLimit.Allow(itoa(update.CallbackQuery.From.ID)) {
			return
		}
		err = r.onCallback(ctx, update.CallbackQuery)
	default:
		return
	}
	if err != nil {
		r.metrics.ErrorsTotal.Add(1)
		// В логах нет PII: только update_id (docs/05).
		r.log.Error("ошибка обработки update", "update_id", update.ID, "err", err)
	}
}
