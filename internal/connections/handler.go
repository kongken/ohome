package connections

import (
	"context"
	"net/http"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
)

// Handler bundles follow / connection HTTP handlers.
type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// RegisterOnUsers wires follow-related routes under the /users router group
// (e.g. /api/v1/users/:username/follow, .../followers, .../following).
func (h *Handler) RegisterOnUsers(g *gin.RouterGroup) {
	g.POST("/:username/follow", auth.RequireAuth(h.issuer), h.follow)
	g.DELETE("/:username/follow", auth.RequireAuth(h.issuer), h.unfollow)
	g.GET("/:username/followers", auth.OptionalAuth(h.issuer), h.listFollowers)
	g.GET("/:username/following", auth.OptionalAuth(h.issuer), h.listFollowing)
}

// UserSummary is the compact user representation used in list responses.
type UserSummary struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsFollowing bool   `json:"is_following"`
}

func (h *Handler) follow(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	target, ok := h.resolveTarget(c)
	if !ok {
		return
	}
	if target.ID == viewerID {
		httpx.Abort(c, httpx.BadBody("cannot follow yourself"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	already, err := dao.Client().User.Query().
		Where(entuser.IDEQ(viewerID)).
		QueryFollowing().
		Where(entuser.IDEQ(target.ID)).
		Exist(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("check follow: "+err.Error()))
		return
	}
	if already {
		c.Status(http.StatusOK)
		return
	}

	if err := dao.Client().User.UpdateOneID(viewerID).
		AddFollowingIDs(target.ID).
		Exec(ctx); err != nil {
		httpx.Abort(c, httpx.Internal("follow: "+err.Error()))
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) unfollow(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	target, ok := h.resolveTarget(c)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := dao.Client().User.UpdateOneID(viewerID).
		RemoveFollowingIDs(target.ID).
		Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			c.Status(http.StatusOK)
			return
		}
		httpx.Abort(c, httpx.Internal("unfollow: "+err.Error()))
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) listFollowers(c *gin.Context) {
	target, ok := h.resolveTarget(c)
	if !ok {
		return
	}
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	users, err := target.QueryFollowers().
		Order(entuser.ByCreatedAt(sql.OrderDesc())).
		Limit(page.Limit + 1).
		All(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("list followers: "+err.Error()))
		return
	}

	summaries, nextCursor := h.toSummaries(ctx, users, viewerID, page.Limit)
	c.JSON(http.StatusOK, gin.H{
		"followers": summaries,
		"page":      httpx.PageResponse{NextCursor: nextCursor, HasMore: nextCursor != ""},
	})
}

func (h *Handler) listFollowing(c *gin.Context) {
	target, ok := h.resolveTarget(c)
	if !ok {
		return
	}
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	users, err := target.QueryFollowing().
		Order(entuser.ByCreatedAt(sql.OrderDesc())).
		Limit(page.Limit + 1).
		All(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("list following: "+err.Error()))
		return
	}

	summaries, nextCursor := h.toSummaries(ctx, users, viewerID, page.Limit)
	c.JSON(http.StatusOK, gin.H{
		"following": summaries,
		"page":      httpx.PageResponse{NextCursor: nextCursor, HasMore: nextCursor != ""},
	})
}

// resolveTarget looks up the user identified by the :username route param.
func (h *Handler) resolveTarget(c *gin.Context) (*ent.User, bool) {
	username := c.Param("username")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.UsernameEQ(username)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			httpx.Abort(c, httpx.NotFound("user not found"))
		} else {
			httpx.Abort(c, httpx.Internal("lookup user: "+err.Error()))
		}
		return nil, false
	}
	return u, true
}

// toSummaries converts ent users to UserSummary slices, applying the page
// limit and computing a cursor for the next page (user ID of last item).
// It batch-checks follow status for the viewer in a single query.
func (h *Handler) toSummaries(ctx context.Context, users []*ent.User, viewerID string, limit int) ([]UserSummary, string) {
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}

	following := map[string]bool{}
	if viewerID != "" && len(users) > 0 {
		ids := make([]string, len(users))
		for i, u := range users {
			ids[i] = u.ID
		}
		fids, _ := dao.Client().User.Query().
			Where(entuser.IDEQ(viewerID)).
			QueryFollowing().
			Where(entuser.IDIn(ids...)).
			IDs(ctx)
		for _, id := range fids {
			following[id] = true
		}
	}

	out := make([]UserSummary, len(users))
	for i, u := range users {
		out[i] = UserSummary{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			IsFollowing: following[u.ID],
		}
	}

	var nextCursor string
	if hasMore && len(users) > 0 {
		nextCursor = users[len(users)-1].ID
	}
	return out, nextCursor
}

// IsFollowing checks whether followerID follows targetID. Used by other
// packages (e.g. users handler) to populate the is_following field.
func IsFollowing(ctx context.Context, followerID, targetID string) (bool, error) {
	if followerID == "" || followerID == targetID {
		return false, nil
	}
	return dao.Client().User.Query().
		Where(entuser.IDEQ(followerID)).
		QueryFollowing().
		Where(entuser.IDEQ(targetID)).
		Exist(ctx)
}
