package posts

import (
	"context"
	"fmt"
	"net/http"
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
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}
	h.mutatePostLikeState(c, func(ctx context.Context, tx *ent.Tx, postID string) error {
		return likePostTx(ctx, tx, postID, viewerID)
	})
}

func (h *Handler) unlikePost(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}
	h.mutatePostLikeState(c, func(ctx context.Context, tx *ent.Tx, postID string) error {
		return unlikePostTx(ctx, tx, postID, viewerID)
	})
}

// mutatePostLikeState loads the post (with visibility check), runs the given
// mutation in a transaction, and responds with the refreshed post response.
func (h *Handler) mutatePostLikeState(c *gin.Context, mutate func(context.Context, *ent.Tx, string) error) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	if err := mutate(ctx, tx, p.ID); err != nil {
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
	if _, err := tx.PostLike.Create().
		SetID(uuid.NewString()).
		SetPostID(postID).
		SetUserID(userID).
		Save(ctx); err != nil {
		return fmt.Errorf("create like: %w", err)
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

func (h *Handler) sharePost(c *gin.Context) {
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

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	exists, err := tx.PostShare.Query().
		Where(entpostshare.PostIDEQ(p.ID), entpostshare.UserIDEQ(viewerID)).
		Exist(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("check share: "+err.Error()))
		return
	}
	if !exists {
		if _, err := tx.PostShare.Create().
			SetID(uuid.NewString()).
			SetPostID(p.ID).
			SetUserID(viewerID).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			httpx.Abort(c, httpx.Internal("create share: "+err.Error()))
			return
		}
	}
	// Every share event bumps the counter; the PostShare row only tracks
	// whether this viewer has shared before (viewer.shared flag).
	if err := tx.Post.UpdateOneID(p.ID).AddSharesCount(1).Exec(ctx); err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("update share count: "+err.Error()))
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

func (h *Handler) bookmarkPost(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}
	h.mutateBookmark(c, func(ctx context.Context, tx *ent.Tx, postID string) error {
		exists, err := tx.Bookmark.Query().
			Where(entbookmark.UserIDEQ(viewerID), entbookmark.PostIDEQ(postID)).
			Exist(ctx)
		if err != nil {
			return fmt.Errorf("check bookmark: %w", err)
		}
		if exists {
			return nil // idempotent re-bookmark
		}
		_, err = tx.Bookmark.Create().
			SetID(uuid.NewString()).
			SetUserID(viewerID).
			SetPostID(postID).
			Save(ctx)
		return err
	})
}

func (h *Handler) unbookmarkPost(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}
	h.mutateBookmark(c, func(ctx context.Context, tx *ent.Tx, postID string) error {
		_, err := tx.Bookmark.Delete().
			Where(entbookmark.UserIDEQ(viewerID), entbookmark.PostIDEQ(postID)).
			Exec(ctx)
		return err
	})
}

// mutateBookmark loads the post (visibility checked), applies a bookmark
// mutation in a transaction and answers with the refreshed post response.
func (h *Handler) mutateBookmark(c *gin.Context, mutate func(context.Context, *ent.Tx, string) error) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	if err := mutate(ctx, tx, p.ID); err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("update bookmark: "+err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}

	resp, err := h.toResponse(ctx, p, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
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

func (h *Handler) listBookmarks(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	query := dao.Client().Bookmark.Query().
		Where(entbookmark.UserIDEQ(viewerID)).
		Order(entbookmark.ByCreatedAt(sql.OrderDesc()), entbookmark.ByID(sql.OrderDesc())).
		Limit(page.Limit + 1)
	rows, err := applyBookmarkCursor(ctx, query, page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}

	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}

	// Keep bookmark order (newest saved first) while filtering out posts the
	// viewer is no longer allowed to see (e.g. since turned private).
	postIDs := make([]string, len(rows))
	for i, r := range rows {
		postIDs[i] = r.PostID
	}
	loaded, err := basePostQuery().Where(entpost.IDIn(postIDs...)).All(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load posts: "+err.Error()))
		return
	}
	byID := make(map[string]*ent.Post, len(loaded))
	for _, p := range loaded {
		byID[p.ID] = p
	}
	visible := make([]*ent.Post, 0, len(postIDs))
	for _, id := range postIDs {
		p, ok := byID[id]
		if !ok {
			continue
		}
		allowed, err := canView(ctx, p, viewerID)
		if err != nil {
			httpx.Abort(c, httpx.Internal("check visibility: "+err.Error()))
			return
		}
		if allowed {
			visible = append(visible, p)
		}
	}

	resp, err := h.toResponses(ctx, visible, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load posts: "+err.Error()))
		return
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID
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
	return query.Where(
		entpostlike.Or(
			entpostlike.CreatedAtLT(row.CreatedAt),
			entpostlike.And(
				entpostlike.CreatedAtEQ(row.CreatedAt),
				entpostlike.IDLT(cursor),
			),
		),
	).All(ctx)
}

func applyBookmarkCursor(ctx context.Context, query *ent.BookmarkQuery, cursor string) ([]*ent.Bookmark, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	row, err := dao.Client().Bookmark.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor row not found: %w", err)
	}
	return query.Where(
		entbookmark.Or(
			entbookmark.CreatedAtLT(row.CreatedAt),
			entbookmark.And(
				entbookmark.CreatedAtEQ(row.CreatedAt),
				entbookmark.IDLT(cursor),
			),
		),
	).All(ctx)
}

// userSummariesByIDs builds UserSummary values in the given ID order,
// batch-loading users and follow status in two queries.
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
			continue // user deleted since liking
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
