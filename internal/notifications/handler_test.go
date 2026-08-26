package notifications

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestValidTypeWhitelist(t *testing.T) {
	for _, typ := range []string{
		TypeLike, TypeComment, TypeMention, TypeFollow,
		TypeConnectionRequest, TypeCommunityInvite, TypePostShare, TypeSystem,
	} {
		if !ValidType(typ) {
			t.Fatalf("ValidType(%q) = false, want true", typ)
		}
	}
	for _, bad := range []string{"", "poke", "LIKE"} {
		if ValidType(bad) {
			t.Fatalf("ValidType(%q) = true, want false", bad)
		}
	}
}

func TestDefaultTextCoversAllTypes(t *testing.T) {
	for _, typ := range []string{
		TypeLike, TypeComment, TypeMention, TypeFollow,
		TypeConnectionRequest, TypeCommunityInvite, TypePostShare,
	} {
		if DefaultText(typ) == "" {
			t.Fatalf("DefaultText(%q) is empty", typ)
		}
	}
	if DefaultText("nope") != "" {
		t.Fatal("unknown type should map to empty text")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 8, 26, 12, 0, 0, 123456789, time.UTC)
	encoded := encodeCursor(cursor{ID: "abc-123", T: ts})
	if encoded == "" {
		t.Fatal("encodeCursor returned empty string")
	}

	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if decoded.ID != "abc-123" || !decoded.T.Equal(ts) {
		t.Fatalf("decoded = %#v, want id=abc-123 t=%v", decoded, ts)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	if cr, err := decodeCursor(""); err != nil || cr != nil {
		t.Fatalf("empty cursor should decode to (nil, nil), got (%#v, %v)", cr, err)
	}
	if _, err := decodeCursor("not-base64!!"); err == nil {
		t.Fatal("non-base64 cursor should fail")
	}
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("this-is-not-json"))
	if _, err := decodeCursor(notJSON); err == nil {
		t.Fatal("non-JSON payload should fail")
	}
	missingID := base64.RawURLEncoding.EncodeToString([]byte(`{"t":"2026-08-26T00:00:00Z"}`))
	if _, err := decodeCursor(missingID); err == nil {
		t.Fatal("cursor missing ID should fail")
	}
}
