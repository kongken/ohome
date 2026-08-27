package communities

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entcommunity "github.com/kongken/ohome/internal/dao/ent/community"
	entmembership "github.com/kongken/ohome/internal/dao/ent/membership"
	"github.com/kongken/ohome/internal/httpx"
	"github.com/kongken/ohome/internal/users"
)

// Handler bundles community HTTP handlers.
type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Register wires community routes onto the `/api/v1/communities` group.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", auth.OptionalAuth(h.issuer), h.list)
	g.GET("/:id", auth.OptionalAuth(h.issuer), h.get)
	g.POST("/:id/join", auth.RequireAuth(h.issuer), h.join)
	g.DELETE("/:id/join", auth.RequireAuth(h.issuer), h.leave)
	g.GET("/:id/members", auth.OptionalAuth(h.issuer), h.listMembers)
}

// RegisterOnUsers wires community routes under `/api/v1/users`.
func (h *Handler) RegisterOnUsers(g *gin.RouterGroup) {
	g.GET("/me/communities", auth.RequireAuth(h.issuer), h.listMine)
}

// Community mirrors the api.md §6 entity shape (proto communities.v1.Community).
type Community struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	CoverURL     string `json:"cover_url,omitempty"`
	MembersCount int    `json:"members_count"`
	Category     string `json:"category,omitempty"`
	IsMember     bool   `json:"is_member"`
}

func (h *Handler) list(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	q := strings.TrimSpace(c.Query("q"))
	category := strings.TrimSpace(c.Query("category"))
	if err := validateSort(c.DefaultQuery("sort", "members")); err != nil {
		httpx.Abort(c, httpx.BadQuery(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	query := dao.Client().Community.Query()
	if q != "" {
		query = query.Where(entcommunity.Or(
			entcommunity.NameContainsFold(q),
			entcommunity.DescriptionContainsFold(q),
		))
	}
	if category != "" {
		query = query.Where(entcommunity.CategoryEQ(category))
	}

	communities, err := query.
		Order(entcommunity.ByMembersCount(sql.OrderDesc()), entcommunity.ByID(sql.OrderDesc())).
		Limit(page.Limit + 1).
		All(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("list communities: "+err.Error()))
		return
	}
	h.writeList(c, ctx, communities, viewerID, page.Limit)
}

func (h *Handler) get(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, err := dao.Client().Community.Get(ctx, c.Param("id"))
	if err != nil {
		if ent.IsNotFound(err) {
			httpx.Abort(c, httpx.NotFound("community not found"))
			return
		}
		httpx.Abort(c, httpx.Internal("load community: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, h.buildResponse(ctx, cm, viewerID))
}

func (h *Handler) join(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadCommunity(c, ctx, c.Param("id"))
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	exists, err := tx.Membership.Query().
		Where(entmembership.UserIDEQ(viewerID), entmembership.CommunityIDEQ(cm.ID)).
		Exist(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("check membership: "+err.Error()))
		return
	}
	inserted := false
	if !exists {
		inserted, err = dao.InsertOnce(func() error {
			_, err := tx.Membership.Create().
				SetUserID(viewerID).
				SetCommunityID(cm.ID).
				Save(ctx)
			return err
		})
		if err != nil {
			_ = tx.Rollback()
			httpx.Abort(c, httpx.Internal("join community: "+err.Error()))
			return
		}
	}
	// Only a genuinely new membership bumps the counter; concurrent
	// duplicates lose the race against the unique index and stay idempotent.
	if inserted {
		if err := tx.Community.UpdateOneID(cm.ID).AddMembersCount(1).Exec(ctx); err != nil {
			_ = tx.Rollback()
			httpx.Abort(c, httpx.Internal("update members count: "+err.Error()))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}

	fresh, err := dao.Client().Community.Get(ctx, cm.ID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load community: "+err.Error()))
		return
	}
	resp := buildCommunity(fresh, true)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) leave(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadCommunity(c, ctx, c.Param("id"))
	if !ok {
		return
	}

	tx, err := dao.Client().Tx(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("begin tx: "+err.Error()))
		return
	}
	n, err := tx.Membership.Delete().
		Where(entmembership.UserIDEQ(viewerID), entmembership.CommunityIDEQ(cm.ID)).
		Exec(ctx)
	if err != nil {
		_ = tx.Rollback()
		httpx.Abort(c, httpx.Internal("leave community: "+err.Error()))
		return
	}
	if n > 0 {
		if err := tx.Community.UpdateOneID(cm.ID).AddMembersCount(-1).Exec(ctx); err != nil {
			_ = tx.Rollback()
			httpx.Abort(c, httpx.Internal("update members count: "+err.Error()))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Abort(c, httpx.Internal("commit: "+err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listMembers(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cm, ok := loadCommunity(c, ctx, c.Param("id"))
	if !ok {
		return
	}

	rows, err := applyMembershipCursor(ctx,
		dao.Client().Membership.Query().
			Where(entmembership.CommunityIDEQ(cm.ID)).
			Order(entmembership.ByCreatedAt(sql.OrderDesc()), entmembership.ByID(sql.OrderDesc())).
			Limit(page.Limit+1),
		page.Cursor)
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
	summaries, err := users.SummariesByIDs(ctx, ids, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load members: "+err.Error()))
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

func (h *Handler) listMine(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := applyMembershipCursor(ctx,
		dao.Client().Membership.Query().
			Where(entmembership.UserIDEQ(viewerID)).
			Order(entmembership.ByCreatedAt(sql.OrderDesc()), entmembership.ByID(sql.OrderDesc())).
			Limit(page.Limit+1),
		page.Cursor)
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
		ids[i] = r.CommunityID
	}
	loaded, err := dao.Client().Community.Query().Where(entcommunity.IDIn(ids...)).All(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load communities: "+err.Error()))
		return
	}
	byID := make(map[string]*ent.Community, len(loaded))
	for _, cm := range loaded {
		byID[cm.ID] = cm
	}
	items := make([]Community, 0, len(ids))
	for _, id := range ids {
		if cm, ok := byID[id]; ok {
			// The viewer owns these memberships by construction.
			items = append(items, buildCommunity(cm, true))
		}
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// ---- shared helpers ----

func (h *Handler) writeList(c *gin.Context, ctx context.Context, communities []*ent.Community, viewerID string, limit int) {
	hasMore := len(communities) > limit
	if hasMore {
		communities = communities[:limit]
	}

	memberOf := map[string]bool{}
	if viewerID != "" && len(communities) > 0 {
		ids := make([]string, len(communities))
		for i, cm := range communities {
			ids[i] = cm.ID
		}
		cids, err := dao.Client().Membership.Query().
			Where(entmembership.UserIDEQ(viewerID), entmembership.CommunityIDIn(ids...)).
			IDs(ctx)
		if err != nil {
			httpx.Abort(c, httpx.Internal("check membership: "+err.Error()))
			return
		}
		for _, id := range cids {
			memberOf[id] = true
		}
	}

	items := make([]Community, len(communities))
	for i, cm := range communities {
		items[i] = buildCommunity(cm, memberOf[cm.ID])
	}

	var nextCursor string
	if hasMore && len(communities) > 0 {
		nextCursor = communities[len(communities)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// buildResponse is kept for parity with other modules; single-item paths use it.
func (h *Handler) buildResponse(ctx context.Context, cm *ent.Community, viewerID string) Community {
	isMember := false
	if viewerID != "" {
		isMember, _ = dao.Client().Membership.Query().
			Where(entmembership.UserIDEQ(viewerID), entmembership.CommunityIDEQ(cm.ID)).
			Exist(ctx)
	}
	return buildCommunity(cm, isMember)
}

func buildCommunity(cm *ent.Community, isMember bool) Community {
	return Community{
		ID:           cm.ID,
		Name:         cm.Name,
		Description:  cm.Description,
		IconURL:      cm.IconURL,
		CoverURL:     cm.CoverURL,
		MembersCount: cm.MembersCount,
		Category:     cm.Category,
		IsMember:     isMember,
	}
}

func loadCommunity(c *gin.Context, ctx context.Context, id string) (*ent.Community, bool) {
	cm, err := dao.Client().Community.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			httpx.Abort(c, httpx.NotFound("community not found"))
		} else {
			httpx.Abort(c, httpx.Internal("load community: "+err.Error()))
		}
		return nil, false
	}
	return cm, true
}

// validateSort whitelists the sort parameter; "trending" currently aliases
// the members ordering until per-community activity counters exist.
func validateSort(sort string) error {
	switch sort {
	case "", "members", "trending":
		return nil
	default:
		return fmt.Errorf("sort must be one of: members, trending")
	}
}

// applyMembershipCursor adds keyset pagination for (created_at DESC, ID DESC)
// ordering; the cursor is the last membership row ID from the previous page.
func applyMembershipCursor(ctx context.Context, query *ent.MembershipQuery, cursor string) ([]*ent.Membership, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	row, err := dao.Client().Membership.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor row not found: %w", err)
	}
	return query.Where(
		entmembership.Or(
			entmembership.CreatedAtLT(row.CreatedAt),
			entmembership.And(
				entmembership.CreatedAtEQ(row.CreatedAt),
				entmembership.IDLT(cursor),
			),
		),
	).All(ctx)
}
