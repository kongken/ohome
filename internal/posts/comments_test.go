package posts

import (
	"strings"
	"testing"
	"time"

	"github.com/kongken/ohome/internal/dao/ent"
)

func TestValidateCommentContentRejectsEmpty(t *testing.T) {
	if err := validateCommentContent("   "); err == nil {
		t.Fatal("validateCommentContent returned nil error for blank content")
	}
}

func TestValidateCommentContentRejectsTooLong(t *testing.T) {
	long := strings.Repeat("a", maxContentLen+1)
	if err := validateCommentContent(long); err == nil {
		t.Fatal("validateCommentContent returned nil error for oversized content")
	}
}

func TestValidateCommentContentAcceptsNormal(t *testing.T) {
	if err := validateCommentContent(" nice post! "); err != nil {
		t.Fatalf("validateCommentContent returned error: %v", err)
	}
}

func TestThreadParentFlattensReplies(t *testing.T) {
	top := &ent.Comment{ID: "top"}
	if got := threadParent(top); got != "top" {
		t.Fatalf("threadParent(top-level) = %q, want %q", got, "top")
	}
	reply := &ent.Comment{ID: "reply", ParentID: "top"}
	if got := threadParent(reply); got != "top" {
		t.Fatalf("threadParent(reply) = %q, want %q (threads stay one level deep)", got, "top")
	}
}

func TestBuildCommentResponseMapsFields(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	cm := &ent.Comment{
		ID:         "c1",
		PostID:     "p1",
		ParentID:   "c0",
		AuthorID:   "u1",
		Content:    "hello",
		LikesCount: 3,
		CreatedAt:  now,
	}
	author := &ent.User{
		ID:          "u1",
		Username:    "alice",
		DisplayName: "Alice",
		AvatarURL:   "https://x/a.png",
		Title:       "Designer",
	}
	resp := buildCommentResponse(cm, author, true)

	if resp.ID != "c1" || resp.PostID != "p1" || resp.ParentID != "c0" {
		t.Fatalf("ids = %#v", resp)
	}
	if resp.Author.Username != "alice" || resp.Author.Name != "Alice" || resp.Author.Title != "Designer" {
		t.Fatalf("author = %#v", resp.Author)
	}
	if !resp.Liked || resp.Likes != 3 {
		t.Fatalf("likes/liked = %d/%v, want 3/true", resp.Likes, resp.Liked)
	}
	if !resp.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", resp.CreatedAt, now)
	}

	cm.ParentID = ""
	resp = buildCommentResponse(cm, author, false)
	if resp.ParentID != "" {
		t.Fatalf("parent_id should be omitted (empty) for top-level comments, got %q", resp.ParentID)
	}

	noName := author
	noName.DisplayName = ""
	resp = buildCommentResponse(cm, noName, false)
	if resp.Author.Name != "alice" {
		t.Fatalf("author name should fall back to username, got %q", resp.Author.Name)
	}
}

func TestBuildResponseAppliesViewerState(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	p := &ent.Post{
		ID:            "p1",
		AuthorID:      "u1",
		Content:       "body",
		LikesCount:    7,
		CommentsCount: 2,
		SharesCount:   1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	u := &ent.User{ID: "u1", Username: "bob"}

	resp := buildResponse(p, u, viewerState{})
	if resp.Viewer.Liked || resp.Viewer.Bookmarked || resp.Viewer.Shared {
		t.Fatalf("empty viewer state should stay all-false: %#v", resp.Viewer)
	}

	resp = buildResponse(p, u, viewerState{Liked: true, Bookmarked: true})
	if !resp.Viewer.Liked || !resp.Viewer.Bookmarked || resp.Viewer.Shared {
		t.Fatalf("viewer state = %#v, want liked+bookmarked only", resp.Viewer)
	}
	if resp.Stats.Likes != 7 || resp.Stats.Comments != 2 || resp.Stats.Shares != 1 {
		t.Fatalf("stats = %#v", resp.Stats)
	}
}
