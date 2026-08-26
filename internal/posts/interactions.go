package posts

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/connections"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entbookmark "github.com/kongken/ohome/internal/dao/ent/bookmark"
	entpost "github.com/kongken/ohome/internal/dao/ent/post"
	entpostlike "github.com/kongken/ohome/internal/dao/ent/postlike"
	entpostshare "github.com/kongken/ohome/internal/dao/ent/postshare"
	"github.com/kongken/ohome/internal/dao/ent/predicate"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
)

// RegisterInteractions wires like / share / bookmark routes onto `/api/v1`.
// Called from Register; kept separate to group the interaction endpoints.
func (h *Handler) RegisterInteractions(g *gin.RouterGroup) {
	g.POST("/posts/:id/like", auth.RequireAuth(h.issuer), h.likePost)
	g.DELETE("/posts/:id/like", auth.RequireAuth(h.issuer), h.unlikePost)
	g.GET("/posts/:id/likes", auth.OptionalAuth(h.issuer), h.listLikes)
	g.POST("/posts/:id/share", auth.RequireAuth(h.issuer), h.sharePost)
	g.POST("/posts/:id/bookmark", auth.RequireAuth(h.issuer), h.bookmarkPost)
	g.DELETE("/posts/:id/bookmark", auth.RequireAuth(h.issuer), h.unbookmarkPost)
}

// RegisterBookmarks wires `/api/v1/users` bookmark routes.
func (h *Handler) RegisterBookmarks(g *gin.RouterGroup) {
	g.GET("/me/bookmarks", auth.RequireAuth(h.issuer), h.listBookmarks)
}

func (h *Handler) likePost(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisiblePost(c, func(ctx context.Context, tx *ent.Tx, p *ent.Post) error {
		return likePostTx(ctx, tx, p.ID, uid)
	})
}

func (h *Handler) unlikePost(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisiblePost(c, func(ctx context.Context, tx *ent.Tx, p *ent.Post) error {
		return unlikePostTx(ctx, tx, p.ID, uid)
	})
}

func (h *Handler) sharePost(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisiblePost(c, func(ctx context.Context, tx *ent.Tx, p *ent.Post) error {
		return sharePostTx(ctx, tx, p.ID, uid)
	})
}

func (h *Handler) bookmarkPost(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisiblePost(c, func(ctx context.Context, tx *ent.Tx, p *ent.Post) error {
		return bookmarkPostTx(ctx, tx, p.ID, uid)
	})
}

func (h *Handler) unbookmarkPost(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisiblePost(c, func(ctx context.Context, tx *ent.Tx, p *ent.Post) error {
		return unbookmarkPostTx(ctx, tx, p.ID, uid)
	})
}

// mutateVisiblePost is the shared scaffold for post-state mutations:
// authenticate, load the :id post with visibility enforced, run mutate in a
// transaction, commit, and respond with the refreshed post response.
// Aborts with 401 when unauthenticated; never invokes mutate in that case.
func (h *Handler) mutateVisiblePost(c *gin.Context, mutate func(context.Context, *ent.Tx, *ent.Post) error) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}
	h.runPostMutation(c, ctx, p, viewerID, mutate)
}

func (h *Handler) runPostMutation(c *gin.Context, ctx context.Context, p *ent.Post, viewerID string, mutate func(context.Context, *ent.Tx, *ent.Post) error) {
	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	if err := mutate(ctx, tx, p); err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("update post state: "+err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}

	fresh, err := basePostQuery().Where(entpost.IDEQ(p.ID)).Only(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	resp, err := h.toResponse(ctx, fresh, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

const (
	keysetColCreatedAt = "created_at"
	keysetColID        = "id"
)

// keysetPredicate builds the shared (created_at, id) comparison predicate:
// rows strictly older than the cursor position for desc order, strictly
// newer for asc. Single-table queries only (columns are unqualified).
func keysetPredicate(asc bool, createdAt time.Time, id string) *sql.Predicate {
	before := sql.LT(keysetColCreatedAt, createdAt)
	idTie := sql.LT(keysetColID, id)
	if asc {
		before = sql.GT(keysetColCreatedAt, createdAt)
		idTie = sql.GT(keysetColID, id)
	}
	return sql.Or(before, sql.And(sql.EQ(keysetColCreatedAt, createdAt), idTie))
}

// Per-entity adapters bridging the shared predicate to typed queries.

func postKeyset(asc bool, row *ent.Post) predicate.Post {
	return func(s *sql.Selector) { s.Where(keysetPredicate(asc, row.CreatedAt, row.ID)) }
}

func postLikeKeyset(asc bool, row *ent.PostLike) predicate.PostLike {
	return func(s *sql.Selector) { s.Where(keysetPredicate(asc, row.CreatedAt, row.ID)) }
}

func bookmarkKeyset(asc bool, row *ent.Bookmark) predicate.Bookmark {
	return func(s *sql.Selector) { s.Where(keysetPredicate(asc, row.CreatedAt, row.ID)) }
}

func commentKeyset(asc bool, row *ent.Comment) predicate.Comment {
	return func(s *sql.Selector) { s.Where(keysetPredicate(asc, row.CreatedAt, row.ID)) }
}

// insertOnce runs create, treating a unique-constraint failure as success
// (false): a concurrent duplicate request raced past the existence check.
// Returns whether this call newly inserted the row.
func insertOnce(create func() error) (bool, error) {
	err := create()
	switch {
	case err == nil:
		return true, nil
	case isUniqueViolation(err):
		return false, nil
	default:
		return false, err
	}
}

// isUniqueViolation covers ent's typed wrapper plus raw driver strings so it
// also works for errors that skipped the ent wrapping layer.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if ent.IsConstraintError(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "violates unique constraint") || // Postgres
		strings.Contains(msg, "UNIQUE constraint failed") || // SQLite
		strings.Contains(msg, "Error 1062") // MySQL
}

func likePostTx(ctx context.Context, tx *ent.Tx, postID, userID string) error {
	exists, err := tx.PostLike.Query().
		Where(entpostlike.PostIDEQ(postID), entpostlike.UserIDEQ(userID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check like: %w", err)
	}
	if exists {
		return nil // idempotent re-like
	}
	inserted, err := insertOnce(func() error {
		_, err := tx.PostLike.Create().
			SetID(uuid.NewString()).
			SetPostID(postID).
			SetUserID(userID).
			Save(ctx)
		return err
	})
	if err != nil || !inserted {
		return err // concurrent duplicate: already counted
	}
	return tx.Post.UpdateOneID(postID).AddLikesCount(1).Exec(ctx)
}

func unlikePostTx(ctx context.Context, tx *ent.Tx, postID, userID string) error {
	n, err := tx.PostLike.Delete().
		Where(entpostlike.PostIDEQ(postID), entpostlike.UserIDEQ(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	if n == 0 {
		return nil // idempotent re-unlike
	}
	return tx.Post.UpdateOneID(postID).AddLikesCount(-1).Exec(ctx)
}

// sharePostTx records that this user shared the post and bumps the share
// counter for every share event; the row only backs the viewer.shared flag.
func sharePostTx(ctx context.Context, tx *ent.Tx, postID, userID string) error {
	exists, err := tx.PostShare.Query().
		Where(entpostshare.PostIDEQ(postID), entpostshare.UserIDEQ(userID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check share: %w", err)
	}
	if !exists {
		if _, err := tx.PostShare.Create().
			SetID(uuid.NewString()).
			SetPostID(postID).
			SetUserID(userID).
			Save(ctx); err != nil && !isUniqueViolation(err) {
			return fmt.Errorf("create share: %w", err)
		}
	}
	return tx.Post.UpdateOneID(postID).AddSharesCount(1).Exec(ctx)
}

func bookmarkPostTx(ctx context.Context, tx *ent.Tx, postID, userID string) error {
	exists, err := tx.Bookmark.Query().
		Where(entbookmark.UserIDEQ(userID), entbookmark.PostIDEQ(postID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check bookmark: %w", err)
	}
	if exists {
		return nil // idempotent re-bookmark
	}
	_, err = insertOnce(func() error {
		_, err := tx.Bookmark.Create().
			SetID(uuid.NewString()).
			SetUserID(userID).
			SetPostID(postID).
			Save(ctx)
		return err
	})
	return err
}

func unbookmarkPostTx(ctx context.Context, tx *ent.Tx, postID, userID string) error {
	_, err := tx.Bookmark.Delete().
		Where(entbookmark.UserIDEQ(userID), entbookmark.PostIDEQ(postID)).
		Exec(ctx)
	return err
}

func (h *Handler) listLikes(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}

	query := dao.Client().PostLike.Query().
		Where(entpostlike.PostIDEQ(p.ID)).
		Order(entpostlike.ByCreatedAt(sql.OrderDesc()), entpostlike.ByID(sql.OrderDesc())).
		Limit(page.Limit + 1)
	rows, err := applyRelationCursor(ctx, query, page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}

	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}

	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	summaries, err := userSummariesByIDs(ctx, ids, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load users: "+err.Error()))
		return
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       summaries,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// listBookmarks pages over the viewer's bookmark rows and filters out posts
// they may no longer see BEFORE page boundaries are fixed, so clients never
// receive an empty page while has_more=true (cursor always advances).
func (h *Handler) listBookmarks(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var visible []*ent.Post
	probe := httpx.ParsePage(c)
	limit := probe.Limit
	cursor := probe.Cursor

	for {
		query := dao.Client().Bookmark.Query().
			Where(entbookmark.UserIDEQ(viewerID)).
			Order(entbookmark.ByCreatedAt(sql.OrderDesc()), entbookmark.ByID(sql.OrderDesc())).
			Limit(limit + 1)
		rows, err := applyBookmarkCursor(ctx, query, cursor)
		if err != nil {
			httpx.Abort(c, httpx.BadQuery("invalid cursor"))
			return
		}
		if len(rows) == 0 {
			break
		}

		overflow := false
		for _, r := range rows {
			cursor = r.ID
			p, ok, err := visiblePostByID(ctx, viewerID, r.PostID)
			if err != nil {
				httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
				return
			}
			if !ok {
				continue
			}
			visible = append(visible, p)
			if len(visible) > limit {
				overflow = true
				break
			}
		}
		if overflow || len(rows) < limit+1 {
			break
		}
	}
	hasMore := len(visible) > limit
	if hasMore {
		visible = visible[:limit]
	}

	resp, err := h.toResponses(ctx, visible, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load posts: "+err.Error()))
		return
	}

	var nextCursor string
	if hasMore {
		nextCursor = cursor
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       resp,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

func applyRelationCursor(ctx context.Context, query *ent.PostLikeQuery, cursor string) ([]*ent.PostLike, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	row, err := dao.Client().PostLike.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor row not found: %w", err)
	}
	return query.Where(postLikeKeyset(false, row)).All(ctx)
}

func applyBookmarkCursor(ctx context.Context, query *ent.BookmarkQuery, cursor string) ([]*ent.Bookmark, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	row, err := dao.Client().Bookmark.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor row not found: %w", err)
	}
	return query.Where(bookmarkKeyset(false, row)).All(ctx)
}

// userSummariesByIDs builds UserSummary values in the given ID order,
// batch-loading users and follow status in two queries. Users that vanished
// since liking are silently skipped (consistent with comment lists).
func userSummariesByIDs(ctx context.Context, ids []string, viewerID string) ([]connections.UserSummary, error) {
	out := make([]connections.UserSummary, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	users, err := dao.Client().User.Query().Where(entuser.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load users: %w", err)
	}
	byID := make(map[string]*ent.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	following := map[string]bool{}
	if viewerID != "" {
		fids, err := dao.Client().User.Query().
			Where(entuser.IDEQ(viewerID)).
			QueryFollowing().
			Where(entuser.IDIn(ids...)).
			IDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch is_following check: %w", err)
		}
		for _, id := range fids {
			following[id] = true
		}
	}

	for _, id := range ids {
		u, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, connections.UserSummary{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			IsFollowing: following[u.ID],
		})
	}
	return out, nil
}
