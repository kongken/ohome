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
	entpost "github.com/kongken/ohome/internal/dao/ent/post"
	"github.com/kongken/ohome/internal/dao/ent/schema"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
)

const (
	maxContentLen     = 5000
	maxHashtags       = 20
	maxHashtagLen     = 64
	maxAttachments    = 10
	visibilityPublic  = "public"
	visibilityPrivate = "private"
	visibilityFriends = "followers"
)

// Handler bundles post and feed HTTP handlers.
type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Register wires posts routes onto `/api/v1`.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/feed", auth.RequireAuth(h.issuer), h.feed)
	g.GET("/posts/:id", auth.OptionalAuth(h.issuer), h.getPost)
	g.POST("/posts", auth.RequireAuth(h.issuer), h.createPost)
	g.PATCH("/posts/:id", auth.RequireAuth(h.issuer), h.updatePost)
	g.DELETE("/posts/:id", auth.RequireAuth(h.issuer), h.deletePost)
}

// RegisterOnUsers wires profile post routes under `/api/v1/users`.
func (h *Handler) RegisterOnUsers(g *gin.RouterGroup) {
	g.GET("/:username/posts", auth.OptionalAuth(h.issuer), h.userPosts)
}

type attachment struct {
	Type    string `json:"type,omitempty"`
	MediaID string `json:"media_id,omitempty"`
	URL     string `json:"url,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

type createPostRequest struct {
	Content     string       `json:"content"`
	Attachments []attachment `json:"attachments"`
	Hashtags    []string     `json:"hashtags"`
	CommunityID string       `json:"community_id"`
	EventID     string       `json:"event_id"`
	Visibility  string       `json:"visibility"`
}

type updatePostRequest struct {
	Content     *string       `json:"content"`
	Attachments *[]attachment `json:"attachments"`
	Hashtags    *[]string     `json:"hashtags"`
	CommunityID *string       `json:"community_id"`
	EventID     *string       `json:"event_id"`
	Visibility  *string       `json:"visibility"`
}

type postResponse struct {
	ID         string         `json:"id"`
	Author     authorResponse `json:"author"`
	Community  *communityRef  `json:"community,omitempty"`
	Content    string         `json:"content"`
	Media      []attachment   `json:"media"`
	Hashtags   []string       `json:"hashtags"`
	Visibility string         `json:"visibility"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Stats      postStats      `json:"stats"`
	Viewer     viewerState    `json:"viewer"`
}

type authorResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Title     string `json:"title,omitempty"`
}

type communityRef struct {
	ID string `json:"id"`
}

type postStats struct {
	Likes    int `json:"likes"`
	Comments int `json:"comments"`
	Shares   int `json:"shares"`
}

type viewerState struct {
	Liked      bool `json:"liked"`
	Bookmarked bool `json:"bookmarked"`
	Shared     bool `json:"shared"`
}

func (h *Handler) createPost(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req createPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	if err := normalizeCreate(&req); err != nil {
		httpx.Abort(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, err := dao.Client().Post.Create().
		SetID(uuid.NewString()).
		SetAuthorID(uid).
		SetContent(req.Content).
		SetAttachments(toEntAttachments(req.Attachments)).
		SetHashtags(req.Hashtags).
		SetVisibility(req.Visibility).
		SetNillableCommunityID(optionalString(req.CommunityID)).
		SetNillableEventID(optionalString(req.EventID)).
		Save(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("create post: "+err.Error()))
		return
	}

	resp, err := h.toResponse(ctx, p, uid)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *Handler) feed(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}
	page := httpx.ParsePage(c)
	scope := c.DefaultQuery("scope", "following")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	query := basePostQuery().
		Order(entpost.ByCreatedAt(sql.OrderDesc()), entpost.ByID(sql.OrderDesc())).
		Limit(page.Limit + 1)

	switch scope {
	case "following":
		ids, err := dao.Client().User.Query().
			Where(entuser.IDEQ(viewerID)).
			QueryFollowing().
			IDs(ctx)
		if err != nil {
			httpx.Abort(c, httpx.Internal("load following: "+err.Error()))
			return
		}
		ids = append(ids, viewerID)
		query = query.Where(entpost.AuthorIDIn(ids...)).
			Where(entpost.Or(
				entpost.VisibilityIn(visibilityPublic, visibilityFriends),
				entpost.AuthorIDEQ(viewerID),
			))
	case "for_you":
		query = query.Where(entpost.VisibilityEQ(visibilityPublic))
	case "community":
		communityID := strings.TrimSpace(c.Query("community_id"))
		if communityID == "" {
			httpx.Abort(c, httpx.BadQuery("community_id is required for community feed"))
			return
		}
		query = query.Where(entpost.CommunityIDEQ(communityID), entpost.VisibilityEQ(visibilityPublic))
	default:
		httpx.Abort(c, httpx.BadQuery("scope must be one of: for_you, following, community"))
		return
	}

	posts, err := applyCursor(ctx, query, page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}
	h.writePostList(c, ctx, posts, viewerID, page.Limit)
}

func (h *Handler) userPosts(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		httpx.Abort(c, httpx.BadQuery("username is required"))
		return
	}
	viewerID := auth.UserID(c)
	page := httpx.ParsePage(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.UsernameEQ(username)).
		Only(ctx)
	if err != nil {
		abortQuery(c, err, "user not found")
		return
	}

	query := basePostQuery().
		Where(entpost.AuthorIDEQ(u.ID)).
		Order(entpost.ByCreatedAt(sql.OrderDesc()), entpost.ByID(sql.OrderDesc())).
		Limit(page.Limit + 1)
	if viewerID == u.ID {
		query = query.Where(entpost.AuthorIDEQ(viewerID))
	} else if viewerID != "" {
		follows, err := isFollowing(ctx, viewerID, u.ID)
		if err != nil {
			httpx.Abort(c, httpx.Internal("check follow: "+err.Error()))
			return
		}
		if follows {
			query = query.Where(entpost.VisibilityIn(visibilityPublic, visibilityFriends))
		} else {
			query = query.Where(entpost.VisibilityEQ(visibilityPublic))
		}
	} else {
		query = query.Where(entpost.VisibilityEQ(visibilityPublic))
	}

	posts, err := applyCursor(ctx, query, page.Cursor)
	if err != nil {
		httpx.Abort(c, httpx.BadQuery("invalid cursor"))
		return
	}
	h.writePostList(c, ctx, posts, viewerID, page.Limit)
}

func (h *Handler) getPost(c *gin.Context) {
	viewerID := auth.UserID(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, err := basePostQuery().
		Where(entpost.IDEQ(c.Param("id"))).
		Only(ctx)
	if err != nil {
		abortQuery(c, err, "post not found")
		return
	}
	allowed, err := canView(ctx, p, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("check visibility: "+err.Error()))
		return
	}
	if !allowed {
		httpx.Abort(c, httpx.NotFound("post not found"))
		return
	}

	resp, err := h.toResponse(ctx, p, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) updatePost(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req updatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, err := basePostQuery().
		Where(entpost.IDEQ(c.Param("id"))).
		Only(ctx)
	if err != nil {
		abortQuery(c, err, "post not found")
		return
	}
	if p.AuthorID != viewerID {
		httpx.Abort(c, httpx.Forbidden("only the author can edit this post"))
		return
	}

	hasMedia := len(p.Attachments) > 0
	if req.Attachments != nil {
		hasMedia = len(*req.Attachments) > 0
	}
	if err := normalizeUpdate(&req, hasMedia); err != nil {
		httpx.Abort(c, err)
		return
	}

	update := dao.Client().Post.UpdateOneID(p.ID)
	if req.Content != nil {
		update.SetContent(*req.Content)
	}
	if req.Attachments != nil {
		update.SetAttachments(toEntAttachments(*req.Attachments))
	}
	if req.Hashtags != nil {
		update.SetHashtags(*req.Hashtags)
	}
	applyOptional(update.SetCommunityID, update.ClearCommunityID, req.CommunityID)
	applyOptional(update.SetEventID, update.ClearEventID, req.EventID)
	if req.Visibility != nil {
		update.SetVisibility(*req.Visibility)
	}

	p, err = update.Save(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("update post: "+err.Error()))
		return
	}
	resp, err := h.toResponse(ctx, p, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load post: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) deletePost(c *gin.Context) {
	viewerID := auth.UserID(c)
	if viewerID == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	p, err := basePostQuery().
		Where(entpost.IDEQ(c.Param("id"))).
		Only(ctx)
	if err != nil {
		abortQuery(c, err, "post not found")
		return
	}
	if p.AuthorID != viewerID {
		httpx.Abort(c, httpx.Forbidden("only the author can delete this post"))
		return
	}

	if err := dao.Client().Post.UpdateOneID(p.ID).
		SetDeletedAt(time.Now()).
		Exec(ctx); err != nil {
		httpx.Abort(c, httpx.Internal("delete post: "+err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) writePostList(c *gin.Context, ctx context.Context, posts []*ent.Post, viewerID string, limit int) {
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}

	resp, err := h.toResponses(ctx, posts, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load posts: "+err.Error()))
		return
	}

	var nextCursor string
	if hasMore && len(posts) > 0 {
		nextCursor = posts[len(posts)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       resp,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

func (h *Handler) toResponses(ctx context.Context, posts []*ent.Post, viewerID string) ([]postResponse, error) {
	if len(posts) == 0 {
		return []postResponse{}, nil
	}

	authorIDs := make([]string, 0, len(posts))
	seen := map[string]bool{}
	for _, p := range posts {
		if !seen[p.AuthorID] {
			seen[p.AuthorID] = true
			authorIDs = append(authorIDs, p.AuthorID)
		}
	}
	users, err := dao.Client().User.Query().
		Where(entuser.IDIn(authorIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load authors: %w", err)
	}
	authors := make(map[string]*ent.User, len(users))
	for _, u := range users {
		authors[u.ID] = u
	}

	out := make([]postResponse, len(posts))
	for i, p := range posts {
		author, ok := authors[p.AuthorID]
		if !ok {
			return nil, fmt.Errorf("author %s not found", p.AuthorID)
		}
		out[i] = buildResponse(p, author, viewerID)
	}
	return out, nil
}

func (h *Handler) toResponse(ctx context.Context, p *ent.Post, viewerID string) (postResponse, error) {
	resps, err := h.toResponses(ctx, []*ent.Post{p}, viewerID)
	if err != nil {
		return postResponse{}, err
	}
	return resps[0], nil
}

func buildResponse(p *ent.Post, u *ent.User, viewerID string) postResponse {
	name := u.DisplayName
	if name == "" {
		name = u.Username
	}
	resp := postResponse{
		ID: p.ID,
		Author: authorResponse{
			ID:        u.ID,
			Name:      name,
			Username:  u.Username,
			AvatarURL: u.AvatarURL,
			Title:     u.Title,
		},
		Content:    p.Content,
		Media:      fromEntAttachments(p.Attachments),
		Hashtags:   safeStrings(p.Hashtags),
		Visibility: p.Visibility,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
		Stats: postStats{
			Likes:    p.LikesCount,
			Comments: p.CommentsCount,
			Shares:   p.SharesCount,
		},
		Viewer: viewerState{},
	}
	if p.CommunityID != "" {
		resp.Community = &communityRef{ID: p.CommunityID}
	}
	return resp
}

func normalizeCreate(req *createPostRequest) error {
	req.Content = strings.TrimSpace(req.Content)
	req.CommunityID = strings.TrimSpace(req.CommunityID)
	req.EventID = strings.TrimSpace(req.EventID)
	if req.Visibility == "" {
		req.Visibility = visibilityPublic
	}
	if err := validateContent(req.Content, len(req.Attachments) > 0); err != nil {
		return err
	}
	if err := validateVisibility(req.Visibility); err != nil {
		return err
	}
	var err error
	req.Hashtags, err = normalizeHashtags(req.Hashtags)
	if err != nil {
		return err
	}
	return validateAttachments(req.Attachments)
}

func normalizeUpdate(req *updatePostRequest, hasMedia bool) error {
	if req.Content != nil {
		content := strings.TrimSpace(*req.Content)
		if err := validateContent(content, hasMedia); err != nil {
			return err
		}
		req.Content = &content
	}
	if req.Visibility != nil {
		visibility := strings.TrimSpace(*req.Visibility)
		if err := validateVisibility(visibility); err != nil {
			return err
		}
		req.Visibility = &visibility
	}
	if req.Hashtags != nil {
		tags, err := normalizeHashtags(*req.Hashtags)
		if err != nil {
			return err
		}
		req.Hashtags = &tags
	}
	if req.Attachments != nil {
		if err := validateAttachments(*req.Attachments); err != nil {
			return err
		}
	}
	req.CommunityID = normalizeOptionalString(req.CommunityID)
	req.EventID = normalizeOptionalString(req.EventID)
	return nil
}

func validateContent(content string, hasMedia bool) error {
	if content == "" && !hasMedia {
		return httpx.BadBody("content is required")
	}
	if len(content) > maxContentLen {
		return httpx.BadBody(fmt.Sprintf("content must be at most %d characters", maxContentLen))
	}
	return nil
}

func validateVisibility(v string) error {
	switch v {
	case visibilityPublic, visibilityFriends, visibilityPrivate:
		return nil
	default:
		return httpx.BadBody("visibility must be one of: public, followers, private")
	}
}

func normalizeHashtags(in []string) ([]string, error) {
	if len(in) > maxHashtags {
		return nil, httpx.BadBody(fmt.Sprintf("hashtags must contain at most %d items", maxHashtags))
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, tag := range in {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}
		if len(tag) > maxHashtagLen {
			return nil, httpx.BadBody(fmt.Sprintf("hashtag must be at most %d characters", maxHashtagLen))
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
	}
	return out, nil
}

func validateAttachments(in []attachment) error {
	if len(in) > maxAttachments {
		return httpx.BadBody(fmt.Sprintf("attachments must contain at most %d items", maxAttachments))
	}
	for _, a := range in {
		if strings.TrimSpace(a.Type) == "" {
			return httpx.BadBody("attachment type is required")
		}
		if strings.TrimSpace(a.MediaID) == "" && strings.TrimSpace(a.URL) == "" {
			return httpx.BadBody("attachment media_id or url is required")
		}
		if a.Width < 0 || a.Height < 0 {
			return httpx.BadBody("attachment dimensions must be non-negative")
		}
	}
	return nil
}

func basePostQuery() *ent.PostQuery {
	return dao.Client().Post.Query().Where(entpost.DeletedAtIsNil())
}

func canView(ctx context.Context, p *ent.Post, viewerID string) (bool, error) {
	if p.Visibility == visibilityPublic || p.AuthorID == viewerID {
		return true, nil
	}
	if p.Visibility != visibilityFriends || viewerID == "" {
		return false, nil
	}
	return isFollowing(ctx, viewerID, p.AuthorID)
}

func isFollowing(ctx context.Context, followerID, targetID string) (bool, error) {
	return dao.Client().User.Query().
		Where(entuser.IDEQ(followerID)).
		QueryFollowing().
		Where(entuser.IDEQ(targetID)).
		Exist(ctx)
}

func applyCursor(ctx context.Context, query *ent.PostQuery, cursor string) ([]*ent.Post, error) {
	if cursor == "" {
		return query.All(ctx)
	}
	p, err := dao.Client().Post.Get(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor post not found: %w", err)
	}
	return query.Where(
		entpost.Or(
			entpost.CreatedAtLT(p.CreatedAt),
			entpost.And(
				entpost.CreatedAtEQ(p.CreatedAt),
				entpost.IDLT(cursor),
			),
		),
	).All(ctx)
}

func abortQuery(c *gin.Context, err error, notFound string) {
	if ent.IsNotFound(err) {
		httpx.Abort(c, httpx.NotFound(notFound))
		return
	}
	httpx.Abort(c, httpx.Internal(err.Error()))
}

func toEntAttachments(in []attachment) []schema.PostAttachment {
	out := make([]schema.PostAttachment, len(in))
	for i, a := range in {
		out[i] = schema.PostAttachment{
			Type:    strings.TrimSpace(a.Type),
			MediaID: strings.TrimSpace(a.MediaID),
			URL:     strings.TrimSpace(a.URL),
			Width:   a.Width,
			Height:  a.Height,
		}
	}
	return out
}

func fromEntAttachments(in []schema.PostAttachment) []attachment {
	out := make([]attachment, len(in))
	for i, a := range in {
		out[i] = attachment{
			Type:    a.Type,
			MediaID: a.MediaID,
			URL:     a.URL,
			Width:   a.Width,
			Height:  a.Height,
		}
	}
	return out
}

func safeStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func optionalString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func normalizeOptionalString(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	return &trimmed
}

func applyOptional(set func(string) *ent.PostUpdateOne, clear func() *ent.PostUpdateOne, value *string) {
	applyOptionalValue(func(s string) { set(s) }, func() { clear() }, value)
}

func applyOptionalValue(set func(string), clear func(), value *string) {
	if value == nil {
		return
	}
	if *value == "" {
		clear()
		return
	}
	set(*value)
}
