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
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entcomment "github.com/kongken/ohome/internal/dao/ent/comment"
	entcommentlike "github.com/kongken/ohome/internal/dao/ent/commentlike"
	entpost "github.com/kongken/ohome/internal/dao/ent/post"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
	"github.com/kongken/ohome/internal/notifications"
)

// RegisterComments wires comment routes onto `/api/v1`.
func (h *Handler) RegisterComments(g *gin.RouterGroup) {
	g.GET("/posts/:id/comments", auth.OptionalAuth(h.issuer), h.listComments)
	g.POST("/posts/:id/comments", auth.RequireAuth(h.issuer), h.createComment)
	g.PATCH("/comments/:id", auth.RequireAuth(h.issuer), h.updateComment)
	g.DELETE("/comments/:id", auth.RequireAuth(h.issuer), h.deleteComment)
	g.POST("/comments/:id/like", auth.RequireAuth(h.issuer), h.likeComment)
	g.DELETE("/comments/:id/like", auth.RequireAuth(h.issuer), h.unlikeComment)
}

type createCommentRequest struct {
	Content  string `json:"content"`
	ParentID string `json:"parent_id"`
}

type updateCommentRequest struct {
	Content *string `json:"content"`
}

// commentResponse mirrors proto posts.v1.Comment field names.
type commentResponse struct {
	ID        string         `json:"id"`
	PostID    string         `json:"post_id"`
	ParentID  string         `json:"parent_id,omitempty"`
	Author    authorResponse `json:"author"`
	Content   string         `json:"content"`
	Likes     int            `json:"likes"`
	Liked     bool           `json:"liked"`
	CreatedAt time.Time      `json:"created_at"`
}

func (h *Handler) listComments(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}

	query := baseCommentQuery().Where(entcomment.PostIDEQ(p.ID))
	parentID := strings.TrimSpace(c.Query("parent_id"))
	if parentID == "" {
		query = query.Where(entcomment.ParentIDIsNil())
	} else {
		parent, err := baseCommentQuery().Where(entcomment.IDEQ(parentID)).Only(ctx)
		if err != nil || parent.PostID != p.ID {
			httpx.Abort(c, httpx.BadQuery("parent comment not found"))
			return
		}
		query = query.Where(entcomment.ParentIDEQ(parent.ID))
	}

	query = query.
		Order(entcomment.ByCreatedAt(sql.OrderAsc()), entcomment.ByID(sql.OrderAsc())).
		Limit(page.Limit + 1)
	comments, err := applyCommentCursor(ctx, query, page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}
	h.writeCommentList(c, ctx, comments, viewerID, page.Limit)
}

func (h *Handler) writeCommentList(c *gin.Context, ctx context.Context, comments []*ent.Comment, viewerID string, limit int) {
	hasMore := len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}

	resp, err := h.toCommentResponses(ctx, comments, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load comments: "+err.Error()))
		return
	}

	var nextCursor string
	if hasMore && len(comments) > 0 {
		nextCursor = comments[len(comments)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       resp,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

func (h *Handler) createComment(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req createCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	content := strings.TrimSpace(req.Content)
	if err := validateCommentContent(content); err != nil {
		httpx.Abort(c, err)
		return
	}
	parentInput := strings.TrimSpace(req.ParentID)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, ok := h.loadVisiblePost(c, ctx, viewerID)
	if !ok {
		return
	}

	var parentID, parentAuthorID string
	if parentInput != "" {
		parent, err := baseCommentQuery().Where(entcomment.IDEQ(parentInput)).Only(ctx)
		if err != nil || parent.PostID != p.ID {
			httpx.Abort(c, httpx.BadBody("parent comment not found"))
			return
		}
		parentID = threadParent(parent)
		parentAuthorID = parent.AuthorID
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	create := tx.Comment.Create().
		SetID(uuid.NewString()).
		SetPostID(p.ID).
		SetAuthorID(viewerID).
		SetContent(content)
	if parentID != "" {
		create.SetParentID(parentID)
	}
	cm, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("create comment: "+err.Error()))
		return
	}
	if err := tx.Post.UpdateOneID(p.ID).AddCommentsCount(1).Exec(ctx); err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("update post counter: "+err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}

	// Best-effort notifications: post author plus the replied-to comment's
	// author; self-notifications are skipped by the service.
	for _, r := range []string{p.AuthorID, parentAuthorID} {
		if r == "" || r == viewerID {
			continue
		}
		notifications.TryNotify(ctx, notifications.Input{
			UserID: r, Type: notifications.TypeComment,
			ActorID: viewerID, PostID: p.ID,
		})
	}

	author, err := dao.Client().User.Query().Where(entuser.IDEQ(viewerID)).Only(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load author: "+err.Error()))
		return
	}
	c.JSON(http.StatusCreated, buildCommentResponse(cm, author, false))
}

func (h *Handler) updateComment(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req updateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	if req.Content == nil {
		httpx.Abort(c, httpx.BadBody("content is required"))
		return
	}
	content := strings.TrimSpace(*req.Content)
	if err := validateCommentContent(content); err != nil {
		httpx.Abort(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadEditableComment(c, ctx, viewerID, c.Param("id"))
	if !ok {
		return
	}

	cm, err := cm.Update().SetContent(content).Save(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("update comment: "+err.Error()))
		return
	}
	h.respondWithComment(c, ctx, cm, viewerID)
}

func (h *Handler) deleteComment(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadEditableComment(c, ctx, viewerID, c.Param("id"))
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	// Cascade-soft-delete direct replies so they don't become unreachable:
	// threads are one level deep, so replies have no children of their own.
	// Query through tx (not baseCommentQuery) so the snapshot is consistent
	// with the batch delete below and replies inserted concurrently are
	// either visible to both or neither.
	replyIDs, err := tx.Comment.Query().
		Where(entcomment.DeletedAtIsNil(), entcomment.ParentIDEQ(cm.ID)).
		IDs(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("load replies: "+err.Error()))
		return
	}
	ids := append([]string{cm.ID}, replyIDs...)
	deleted, err := tx.Comment.Update().
		Where(entcomment.IDIn(ids...)).
		SetDeletedAt(time.Now()).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("delete comment: "+err.Error()))
		return
	}
	if deleted > 0 {
		if err := tx.Post.UpdateOneID(cm.PostID).AddCommentsCount(-deleted).Exec(ctx); err != nil {
			_ = tx.Rollback()
			httpx.Abort(c, httpx.Internal("update post counter: "+err.Error()))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) likeComment(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisibleComment(c, func(ctx context.Context, tx *ent.Tx, cm *ent.Comment) error {
		return likeCommentTx(ctx, tx, cm.ID, uid)
	})
}

func (h *Handler) unlikeComment(c *gin.Context) {
	uid := auth.UserID(c)
	h.mutateVisibleComment(c, func(ctx context.Context, tx *ent.Tx, cm *ent.Comment) error {
		return unlikeCommentTx(ctx, tx, cm.ID, uid)
	})
}

// mutateVisibleComment mirrors mutateVisiblePost for comment-level mutations:
// authenticate, load with visibility enforced, mutate in a transaction,
// commit and respond with the refreshed comment.
func (h *Handler) mutateVisibleComment(c *gin.Context, mutate func(context.Context, *ent.Tx, *ent.Comment) error) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadVisibleComment(c, ctx, viewerID, c.Param("id"))
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	if err := mutate(ctx, tx, cm); err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("update comment state: "+err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}

	fresh, err := baseCommentQuery().Where(entcomment.IDEQ(cm.ID)).Only(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load comment: "+err.Error()))
		return
	}
	h.respondWithComment(c, ctx, fresh, viewerID)
}

func likeCommentTx(ctx context.Context, tx *ent.Tx, commentID, userID string) error {
	exists, err := tx.CommentLike.Query().
		Where(entcommentlike.CommentIDEQ(commentID), entcommentlike.UserIDEQ(userID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check like: %w", err)
	}
	if exists {
		return nil // idempotent re-like
	}
	inserted, err := insertOnce(func() error {
		_, err := tx.CommentLike.Create().
			SetID(uuid.NewString()).
			SetCommentID(commentID).
			SetUserID(userID).
			Save(ctx)
		return err
	})
	if err != nil || !inserted {
		return err // concurrent duplicate: already counted
	}
	return tx.Comment.UpdateOneID(commentID).AddLikesCount(1).Exec(ctx)
}

func unlikeCommentTx(ctx context.Context, tx *ent.Tx, commentID, userID string) error {
	n, err := tx.CommentLike.Delete().
		Where(entcommentlike.CommentIDEQ(commentID), entcommentlike.UserIDEQ(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	if n == 0 {
		return nil // idempotent re-unlike
	}
	return tx.Comment.UpdateOneID(commentID).AddLikesCount(-1).Exec(ctx)
}

func validateCommentContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return httpx.BadBody("content is required")
	}
	if len(content) > maxContentLen {
		return httpx.BadBody(fmt.Sprintf("content must be at most %d characters", maxContentLen))
	}
	return nil
}

// threadParent returns the parent ID a reply should attach to. Threads stay
// one level deep: replies to replies attach to the top-level parent.
func threadParent(parent *ent.Comment) string {
	if parent.ParentID != "" {
		return parent.ParentID
	}
	return parent.ID
}

func baseCommentQuery() *ent.CommentQuery {
	return dao.Client().Comment.Query().Where(entcomment.DeletedAtIsNil())
}

// loadVisibleComment fetches a non-deleted comment whose post the viewer can see.
func loadVisibleComment(c *gin.Context, ctx context.Context, viewerID, commentID string) (*ent.Comment, bool) {
	cm, err := baseCommentQuery().Where(entcomment.IDEQ(commentID)).Only(ctx)
	if err != nil {
		abortQuery(c, err, "comment not found")
		return nil, false
	}
	p, err := basePostQuery().Where(entpost.IDEQ(cm.PostID)).Only(ctx)
	if err != nil {
		abortQuery(c, err, "post not found")
		return nil, false
	}
	allowed, err := canView(ctx, p, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("check visibility: "+err.Error()))
		return nil, false
	}
	if !allowed {
		httpx.Abort(c, httpx.NotFound("comment not found"))
		return nil, false
	}
	return cm, true
}

// loadEditableComment additionally enforces author-only edits/deletes.
func loadEditableComment(c *gin.Context, ctx context.Context, viewerID, commentID string) (*ent.Comment, bool) {
	cm, ok := loadVisibleComment(c, ctx, viewerID, commentID)
	if !ok {
		return nil, false
	}
	if cm.AuthorID != viewerID {
		httpx.Abort(c, httpx.Forbidden("only the author can modify this comment"))
		return nil, false
	}
	return cm, true
}

func (h *Handler) toCommentResponses(ctx context.Context, comments []*ent.Comment, viewerID string) ([]commentResponse, error) {
	if len(comments) == 0 {
		return []commentResponse{}, nil
	}

	authorIDs := make([]string, 0, len(comments))
	commentIDs := make([]string, 0, len(comments))
	seenAuthors := map[string]bool{}
	for _, cm := range comments {
		commentIDs = append(commentIDs, cm.ID)
		if !seenAuthors[cm.AuthorID] {
			seenAuthors[cm.AuthorID] = true
			authorIDs = append(authorIDs, cm.AuthorID)
		}
	}

	users, err := dao.Client().User.Query().Where(entuser.IDIn(authorIDs...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load authors: %w", err)
	}
	authors := make(map[string]*ent.User, len(users))
	for _, u := range users {
		authors[u.ID] = u
	}

	liked := map[string]bool{}
	if viewerID != "" {
		ids, err := dao.Client().CommentLike.Query().
			Where(entcommentlike.UserIDEQ(viewerID), entcommentlike.CommentIDIn(commentIDs...)).
			IDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("load liked comments: %w", err)
		}
		for _, id := range ids {
			liked[id] = true
		}
	}

	out := make([]commentResponse, 0, len(comments))
	for _, cm := range comments {
		author, ok := authors[cm.AuthorID]
		if !ok {
			continue // author vanished since posting; skip silently (same as likes lists)
		}
		out = append(out, buildCommentResponse(cm, author, liked[cm.ID]))
	}
	return out, nil
}

func (h *Handler) respondWithComment(c *gin.Context, ctx context.Context, cm *ent.Comment, viewerID string) {
	author, err := dao.Client().User.Query().Where(entuser.IDEQ(cm.AuthorID)).Only(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load author: "+err.Error()))
		return
	}
	liked := false
	if viewerID != "" {
		liked, err = dao.Client().CommentLike.Query().
			Where(entcommentlike.CommentIDEQ(cm.ID), entcommentlike.UserIDEQ(viewerID)).
			Exist(ctx)
		if err != nil {
			httpx.Abort(c, httpx.Internal("check like: "+err.Error()))
			return
		}
	}
	c.JSON(http.StatusOK, buildCommentResponse(cm, author, liked))
}

func buildCommentResponse(cm *ent.Comment, u *ent.User, liked bool) commentResponse {
	name := u.DisplayName
	if name == "" {
		name = u.Username
	}
	return commentResponse{
		ID:       cm.ID,
		PostID:   cm.PostID,
		ParentID: cm.ParentID,
		Author: authorResponse{
			ID:        u.ID,
			Name:      name,
			Username:  u.Username,
			AvatarURL: u.AvatarURL,
			Title:     u.Title,
		},
		Content:   cm.Content,
		Likes:     cm.LikesCount,
		Liked:     liked,
		CreatedAt: cm.CreatedAt,
	}
}

// applyCommentCursor adds keyset pagination for ascending (created_at, id)
// ordering used by comment lists; the cursor is the last comment ID.
func applyCommentCursor(ctx context.Context, query *ent.CommentQuery, cursor string) ([]*ent.Comment, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	cm, err := dao.Client().Comment.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor comment not found: %w", err)
	}
	return query.Where(commentKeyset(true, cm)).All(ctx)
}
