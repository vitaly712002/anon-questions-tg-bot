package crypto

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func newTestCodec(t *testing.T) *Codec {
	t.Helper()
	encKey := bytes.Repeat([]byte{0x11}, 32)
	hmacKey := bytes.Repeat([]byte{0x22}, 32)
	c, err := NewCodec(encKey, hmacKey)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

func TestNewCodecRejectsBadKeys(t *testing.T) {
	if _, err := NewCodec(make([]byte, 16), make([]byte, 32)); err == nil {
		t.Error("короткий encryption key должен отклоняться")
	}
	if _, err := NewCodec(make([]byte, 32), make([]byte, 31)); err == nil {
		t.Error("короткий hmac key должен отклоняться")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	c := newTestCodec(t)
	const sender, question = int64(987654321), int64(42)

	enc, err := c.EncryptSender(sender, question)
	if err != nil {
		t.Fatalf("EncryptSender: %v", err)
	}
	got, err := c.DecryptSender(enc, question)
	if err != nil {
		t.Fatalf("DecryptSender: %v", err)
	}
	if got != sender {
		t.Errorf("got %d, want %d", got, sender)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	c := newTestCodec(t)
	a, _ := c.EncryptSender(1, 1)
	b, _ := c.EncryptSender(1, 1)
	if bytes.Equal(a, b) {
		t.Error("одинаковый отправитель должен давать разный шифротекст (случайный nonce)")
	}
}

func TestDecryptRejectsWrongAAD(t *testing.T) {
	c := newTestCodec(t)
	enc, _ := c.EncryptSender(987654321, 42)
	// «Пересадка» шифротекста в другую строку (другой question_id) должна провалиться.
	if _, err := c.DecryptSender(enc, 43); err == nil {
		t.Error("расшифровка с чужим AAD должна возвращать ошибку")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCodec(t)
	enc, _ := c.EncryptSender(987654321, 42)
	enc[len(enc)-1] ^= 0xFF
	if _, err := c.DecryptSender(enc, 42); err == nil {
		t.Error("подменённый шифротекст должен отклоняться (GCM-целостность)")
	}
}

func TestDecryptRejectsTooShort(t *testing.T) {
	c := newTestCodec(t)
	if _, err := c.DecryptSender([]byte{1, 2, 3}, 1); err == nil {
		t.Error("короткий вход должен отклоняться")
	}
}

func TestSenderHashDeterministic(t *testing.T) {
	c := newTestCodec(t)
	h1 := c.SenderHash(987654321)
	h2 := c.SenderHash(987654321)
	if !bytes.Equal(h1, h2) {
		t.Error("HMAC должен быть детерминированным")
	}
	if len(h1) != 32 {
		t.Errorf("длина HMAC-SHA256 = %d, ожидается 32", len(h1))
	}
	if bytes.Equal(h1, c.SenderHash(987654322)) {
		t.Error("разные отправители должны давать разные хэши")
	}
}

func TestSenderHashDependsOnKey(t *testing.T) {
	c1 := newTestCodec(t)
	c2, _ := NewCodec(bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x33}, 32))
	if bytes.Equal(c1.SenderHash(1), c2.SenderHash(1)) {
		t.Error("хэш должен зависеть от HMAC-ключа")
	}
}

func TestNewLinkToken(t *testing.T) {
	re := regexp.MustCompile(`^q_[A-Za-z0-9_-]{22}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := NewLinkToken()
		if err != nil {
			t.Fatalf("NewLinkToken: %v", err)
		}
		if !re.MatchString(tok) {
			t.Fatalf("токен %q не соответствует формату q_<22 base64url>", tok)
		}
		if !strings.HasPrefix(tok, TokenPrefix) {
			t.Fatalf("токен %q без префикса", tok)
		}
		if seen[tok] {
			t.Fatalf("дубликат токена %q", tok)
		}
		seen[tok] = true
	}
}
