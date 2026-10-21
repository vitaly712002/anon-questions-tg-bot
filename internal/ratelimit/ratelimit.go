// Package ratelimit — in-memory burst-защита от флуда по ключу отправителя.
// Суточные лимиты считаются отдельно, по БД (см. service/questions.go).
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter хранит по одному rate.Limiter на ключ (hex от sender_hash).
// Записи, к которым давно не обращались, вычищаются фоновой горутиной.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	limit   rate.Limit
	burst   int
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// New создаёт лимитер: perHour событий в час с burst'ом той же величины.
func New(perHour int) *Limiter {
	l := &Limiter{
		entries: make(map[string]*entry),
		limit:   rate.Limit(float64(perHour) / 3600.0),
		burst:   perHour,
	}
	go l.cleanupLoop()
	return l
}

// Allow сообщает, разрешено ли событие для ключа прямо сейчас.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = time.Now()
	return e.lim.Allow()
}

// cleanupLoop раз в 10 минут удаляет ключи, неактивные более 2 часов.
func (l *Limiter) cleanupLoop() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-2 * time.Hour)
		l.mu.Lock()
		for k, e := range l.entries {
			if e.lastSeen.Before(cutoff) {
				delete(l.entries, k)
			}
		}
		l.mu.Unlock()
	}
}
