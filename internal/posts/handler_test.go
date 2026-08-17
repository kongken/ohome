package posts

import (
	"strings"
	"testing"

	"github.com/kongken/ohome/internal/dao/ent/schema"
)

func TestNormalizeCreateDefaultsVisibility(t *testing.T) {
	req := createPostRequest{Content: "  hello world  "}
	if err := normalizeCreate(&req); err != nil {
		t.Fatalf("normalizeCreate returned error: %v", err)
	}
	if req.Content != "hello world" {
		t.Fatalf("content = %q, want %q", req.Content, "hello world")
	}
	if req.Visibility != visibilityPublic {
		t.Fatalf("visibility = %q, want %q", req.Visibility, visibilityPublic)
	}
}

func TestNormalizeCreateAllowsMediaOnlyPost(t *testing.T) {
	req := createPostRequest{
		Attachments: []attachment{{Type: "image", MediaID: "m_1"}},
	}
	if err := normalizeCreate(&req); err != nil {
		t.Fatalf("normalizeCreate returned error for media-only post: %v", err)
	}
}

func TestNormalizeCreateRejectsEmptyContentWithoutMedia(t *testing.T) {
	req := createPostRequest{}
	if err := normalizeCreate(&req); err == nil {
		t.Fatal("normalizeCreate returned nil error for empty post")
	}
}

func TestNormalizeCreateRejectsInvalidVisibility(t *testing.T) {
	req := createPostRequest{Content: "x", Visibility: "everyone"}
	if err := normalizeCreate(&req); err == nil {
		t.Fatal("normalizeCreate returned nil error for invalid visibility")
	}
}

func TestNormalizeCreateRejectsTooLongContent(t *testing.T) {
	req := createPostRequest{Content: strings.Repeat("a", maxContentLen+1)}
	if err := normalizeCreate(&req); err == nil {
		t.Fatal("normalizeCreate returned nil error for oversized content")
	}
}

func TestNormalizeCreateNormalizesHashtags(t *testing.T) {
	req := createPostRequest{
		Content:  "x",
		Hashtags: []string{"#Go", "go", "", "  #Rust  "},
	}
	if err := normalizeCreate(&req); err != nil {
		t.Fatalf("normalizeCreate returned error: %v", err)
	}
	if len(req.Hashtags) != 2 {
		t.Fatalf("hashtags len = %d, want 2: %#v", len(req.Hashtags), req.Hashtags)
	}
	if req.Hashtags[0] != "#Go" {
		t.Fatalf("hashtags[0] = %q, want %q (case preserved on first occurrence)", req.Hashtags[0], "#Go")
	}
	if req.Hashtags[1] != "#Rust" {
		t.Fatalf("hashtags[1] = %q, want %q", req.Hashtags[1], "#Rust")
	}
}

func TestNormalizeUpdateTrimsAndKeepsNilPointers(t *testing.T) {
	content := "  updated  "
	req := updatePostRequest{Content: &content}
	if err := normalizeUpdate(&req, false); err != nil {
		t.Fatalf("normalizeUpdate returned error: %v", err)
	}
	if *req.Content != "updated" {
		t.Fatalf("content = %q, want %q", *req.Content, "updated")
	}
	if req.Attachments != nil {
		t.Fatal("attachments should stay nil when not provided")
	}
}

func TestNormalizeUpdateAllowsEmptyContentWithMedia(t *testing.T) {
	content := ""
	req := updatePostRequest{Content: &content}
	if err := normalizeUpdate(&req, true); err != nil {
		t.Fatalf("normalizeUpdate returned error for media post with empty content: %v", err)
	}
}

func TestNormalizeUpdateRejectsEmptyContentWithoutMedia(t *testing.T) {
	content := ""
	req := updatePostRequest{Content: &content}
	if err := normalizeUpdate(&req, false); err == nil {
		t.Fatal("normalizeUpdate returned nil error for empty content without media")
	}
}

func TestNormalizeUpdateRejectsInvalidVisibility(t *testing.T) {
	vis := "nope"
	req := updatePostRequest{Visibility: &vis}
	if err := normalizeUpdate(&req, false); err == nil {
		t.Fatal("normalizeUpdate returned nil error for invalid visibility")
	}
}

func TestNormalizeHashtagsLimits(t *testing.T) {
	in := make([]string, maxHashtags+1)
	for i := range in {
		in[i] = "tag"
	}
	if _, err := normalizeHashtags(in); err == nil {
		t.Fatal("normalizeHashtags returned nil error for too many items")
	}
}

func TestNormalizeHashtagsRejectsTooLongTag(t *testing.T) {
	long := strings.Repeat("a", maxHashtagLen+1)
	if _, err := normalizeHashtags([]string{long}); err == nil {
		t.Fatal("normalizeHashtags returned nil error for oversized tag")
	}
}

func TestValidateAttachmentsRequiresType(t *testing.T) {
	err := validateAttachments([]attachment{{MediaID: "m_1"}})
	if err == nil {
		t.Fatal("validateAttachments returned nil error for missing type")
	}
}

func TestValidateAttachmentsRequiresMediaIDOrURL(t *testing.T) {
	err := validateAttachments([]attachment{{Type: "image"}})
	if err == nil {
		t.Fatal("validateAttachments returned nil error for missing media_id/url")
	}
}

func TestValidateAttachmentsRejectsNegativeDimensions(t *testing.T) {
	err := validateAttachments([]attachment{{Type: "image", URL: "u", Width: -1}})
	if err == nil {
		t.Fatal("validateAttachments returned nil error for negative width")
	}
}

func TestValidateAttachmentsLimits(t *testing.T) {
	in := make([]attachment, maxAttachments+1)
	for i := range in {
		in[i] = attachment{Type: "image", URL: "u"}
	}
	if err := validateAttachments(in); err == nil {
		t.Fatal("validateAttachments returned nil error for too many items")
	}
}

func TestToFromEntAttachmentsRoundTrip(t *testing.T) {
	in := []attachment{
		{Type: "image", MediaID: "m_1", URL: "https://x/u.png", Width: 100, Height: 50},
		{Type: "video", MediaID: "m_2", URL: "https://x/v.mp4"},
	}
	ent := toEntAttachments(in)
	if len(ent) != len(in) {
		t.Fatalf("toEntAttachments len = %d, want %d", len(ent), len(in))
	}
	if ent[0].MediaID != "m_1" || ent[0].Width != 100 {
		t.Fatalf("ent[0] = %#v", ent[0])
	}
	out := fromEntAttachments(ent)
	for i := range out {
		if out[i] != in[i] {
			t.Fatalf("out[%d] = %#v, want %#v", i, out[i], in[i])
		}
	}
}

func TestToEntAttachmentsTrimsFields(t *testing.T) {
	in := []attachment{{Type: " image ", MediaID: " m_1 ", URL: "  url  "}}
	out := toEntAttachments(in)
	if out[0].Type != "image" || out[0].MediaID != "m_1" || out[0].URL != "url" {
		t.Fatalf("out[0] = %#v", out[0])
	}
}

func TestOptionalStringEmptyReturnsNil(t *testing.T) {
	if s := optionalString("  "); s != nil {
		t.Fatalf("optionalString = %v, want nil", *s)
	}
	if s := optionalString("abc"); s == nil || *s != "abc" {
		t.Fatalf("optionalString = %v, want %q", s, "abc")
	}
}

func TestApplyOptionalValue(t *testing.T) {
	var setVal string
	var cleared bool
	set := func(s string) { setVal = s }
	clear := func() { cleared = true }

	applyOptionalValue(set, clear, nil)
	if cleared || setVal != "" {
		t.Fatalf("nil value should be a no-op, cleared=%v set=%q", cleared, setVal)
	}

	empty := ""
	applyOptionalValue(set, clear, &empty)
	if !cleared {
		t.Fatal("empty string should clear")
	}

	cleared = false
	val := "x"
	applyOptionalValue(set, clear, &val)
	if cleared || setVal != "x" {
		t.Fatalf("non-empty should set, cleared=%v set=%q", cleared, setVal)
	}
}

func TestSchemaAttachmentShape(t *testing.T) {
	// Guard the JSON column type used by the ent schema.
	var _ []schema.PostAttachment
}
