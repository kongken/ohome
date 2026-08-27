package notifications

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/httpx"
	"github.com/kongken/ohome/internal/users"
)

// Handler bundles notification HTTP handlers. All routes require auth:
// notifications are always scoped to the authenticated recipient.
type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Register wires notification routes onto the `/api/v1/notifications` group.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", auth.RequireAuth(h.issuer), h.list)
	g.GET("/unread-count", auth.RequireAuth(h.issuer), h.unreadCount)
	g.POST("/read-all", auth.RequireAuth(h.issuer), h.markAllRead)
	g.POST("/:id/read", auth.RequireAuth(h.issuer), h.markRead)
	g.DELETE("/:id", auth.RequireAuth(h.issuer), h.remove)
}

// notificationResponse mirrors proto notifications.v1.Notification field
// names; the actor uses the shared compact user summary shape.
type notificationResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Actor        *users.UserSummary `json:"actor,omitempty"`
	PostID       string             `json:"post_id,omitempty"`
	TargetUserID string             `json:"target_user_id,omitempty"`
	Text         string             `json:"text,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	ReadAt       *time.Time         `json:"read_at"`
}

func (h *Handler) list(c *gin.Context) {
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	unread := c.Query("unread") == "true"
	typeFilter := strings.TrimSpace(c.Query("type"))
	if typeFilter != "" && !ValidType(typeFilter) {
		httpx.Abort(c, httpx.BadQuery(fmt.Sprintf("type must be one of: %s", knownTypes())))
		return
	}

	filter := bson.M{"user_id": viewerID}
	if unread {
		filter["read_at"] = bson.M{"$exists": false}
	}
	if typeFilter != "" {
		filter["type"] = typeFilter
	}

	cur, err := decodeCursor(page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}
	if cur != nil {
		filter["$or"] = []bson.M{
			{"created_at": bson.M{"$lt": cur.T}},
			{"created_at": cur.T, "_id": bson.M{"$lt": cur.ID}},
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(page.Limit + 1))
	docs, err := dao.NotificationsColl().Find(ctx, filter, opts)
	if err != nil {
		httpx.Abort(c, httpx.Internal("list notifications: "+err.Error()))
		return
	}
	var found []Notification
	if err := docs.All(ctx, &found); err != nil {
		httpx.Abort(c, httpx.Internal("decode notifications: "+err.Error()))
		return
	}

	hasMore := len(found) > page.Limit
	if hasMore {
		found = found[:page.Limit]
	}
	items := enrich(ctx, found)

	var next string
	if hasMore && len(found) > 0 {
		last := found[len(found)-1]
		next = encodeCursor(cursor{ID: last.ID, T: last.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": next,
		"has_more":    hasMore,
	})
}

// enrich batch-loads actor summaries for a page of notifications.
func enrich(ctx context.Context, docs []Notification) []notificationResponse {
	actorIDs := make([]string, 0, len(docs))
	seen := map[string]bool{}
	for _, d := range docs {
		if d.ActorID != "" && !seen[d.ActorID] {
			seen[d.ActorID] = true
			actorIDs = append(actorIDs, d.ActorID)
		}
	}
	actors := map[string]*users.UserSummary{}
	if len(actorIDs) > 0 {
		summaries, err := users.SummariesByIDs(ctx, actorIDs, "")
		if err != nil {
			// Actors are decorative on notifications; degrade instead of failing.
			slog.Warn("load notification actors failed", "error", err)
		} else {
			for i := range summaries {
				actors[summaries[i].ID] = &summaries[i]
			}
		}
	}

	out := make([]notificationResponse, len(docs))
	for i, d := range docs {
		resp := notificationResponse{
			ID:           d.ID,
			Type:         d.Type,
			Actor:        actors[d.ActorID],
			PostID:       d.PostID,
			TargetUserID: d.TargetUserID,
			Text:         d.Text,
			CreatedAt:    d.CreatedAt,
			ReadAt:       d.ReadAt,
		}
		out[i] = resp
	}
	return out
}

func (h *Handler) unreadCount(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	count, err := dao.NotificationsColl().CountDocuments(ctx, bson.M{
		"user_id": viewerID,
		"read_at": bson.M{"$exists": false},
	})
	if err != nil {
		httpx.Abort(c, httpx.Internal("count notifications: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *Handler) markRead(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	res, err := dao.NotificationsColl().UpdateOne(ctx,
		bson.M{"_id": c.Param("id"), "user_id": viewerID},
		bson.M{"$set": bson.M{"read_at": time.Now()}},
	)
	if err != nil {
		httpx.Abort(c, httpx.Internal("mark read: "+err.Error()))
		return
	}
	if res.MatchedCount == 0 {
		httpx.Abort(c, httpx.NotFound("notification not found"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) markAllRead(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	_, err := dao.NotificationsColl().UpdateMany(ctx,
		bson.M{"user_id": viewerID, "read_at": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"read_at": time.Now()}},
	)
	if err != nil {
		httpx.Abort(c, httpx.Internal("mark all read: "+err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) remove(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	res, err := dao.NotificationsColl().DeleteOne(ctx,
		bson.M{"_id": c.Param("id"), "user_id": viewerID},
	)
	if err != nil {
		httpx.Abort(c, httpx.Internal("delete notification: "+err.Error()))
		return
	}
	if res.DeletedCount == 0 {
		httpx.Abort(c, httpx.NotFound("notification not found"))
		return
	}
	c.Status(http.StatusNoContent)
}

// ---- cursor codec ----

// cursor carries the keyset position across pages: creation timestamp plus
// document ID tiebreaker, matching the (created_at DESC, _id DESC) sort.
type cursor struct {
	T  time.Time `json:"t"`
	ID string    `json:"i"`
}

func encodeCursor(cr cursor) string {
	b, err := json.Marshal(cr)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("cursor is not valid base64")
	}
	var cr cursor
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, fmt.Errorf("cursor payload invalid")
	}
	if cr.ID == "" || cr.T.IsZero() {
		return nil, fmt.Errorf("cursor fields missing")
	}
	return &cr, nil
}

func knownTypes() string {
	return strings.Join([]string{
		TypeLike, TypeComment, TypeMention, TypeFollow,
		TypeConnectionRequest, TypeCommunityInvite, TypePostShare, TypeSystem,
	}, ", ")
}
