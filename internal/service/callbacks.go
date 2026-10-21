package service

import (
	"fmt"
	"strconv"
	"strings"
)

// Callback data inline-кнопок. Формат: "<префикс>:<аргументы>".
// Сервис формирует эти строки при построении клавиатур, транспорт разбирает.
// Telegram ограничивает callback data 64 байтами — все id укладываются.
const (
	CBAnswer      = "ans"  // ans:<question_id>    — получатель отвечает на вопрос
	CBBlock       = "blk"  // blk:<question_id>    — заблокировать отправителя вопроса
	CBReport      = "rep"  // rep:<question_id>    — пожаловаться на вопрос
	CBContinue    = "cont" // cont:<question_id>   — отправитель продолжает диалог
	CBAdminPrefix = "a"    // a:<действие>[:<арг>] — админ-панель
)

// CB собирает callback data из частей: CB("ans", 42) → "ans:42".
func CB(parts ...any) string {
	strs := make([]string, len(parts))
	for i, p := range parts {
		strs[i] = fmt.Sprint(p)
	}
	return strings.Join(strs, ":")
}

// ParseCB разбирает callback data на префикс и аргументы.
func ParseCB(data string) (prefix string, args []string) {
	parts := strings.Split(data, ":")
	return parts[0], parts[1:]
}

// CBInt64 разбирает числовой аргумент callback data.
func CBInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
