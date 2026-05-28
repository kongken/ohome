package users

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
)

const (
	maxInterests   = 20
	maxInterestLen = 64
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Handler bundles user profile HTTP handlers.
type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Register wires users routes onto `/api/v1/users`.
func (h *Handler) Register(g *gin.RouterGroup) {
	me := g.Group("/me", auth.RequireAuth(h.issuer))
	me.GET("", h.getMe)
	me.PATCH("", h.updateMe)
	me.GET("/interests", h.getInterests)
	me.PUT("/interests", h.updateInterests)

	g.GET("/:username", h.getUser)
}

type updateMeRequest struct {
	DisplayName *string  `json:"display_name"`
	Username    *string  `json:"username"`
	Title       *string  `json:"title"`
	Bio         *string  `json:"bio"`
	Location    *string  `json:"location"`
	Interests   []string `json:"interests"`
}

type updateInterestsRequest struct {
	Interests []string `json:"interests"`
}

type profileResponse struct {
	ID          string       `json:"id"`
	Username    string       `json:"username"`
	DisplayName string       `json:"display_name,omitempty"`
	Title       string       `json:"title,omitempty"`
	Bio         string       `json:"bio,omitempty"`
	AvatarURL   string       `json:"avatar_url,omitempty"`
	CoverURL    string       `json:"cover_url,omitempty"`
	Location    string       `json:"location,omitempty"`
	Interests   []string     `json:"interests"`
	Stats       profileStats `json:"stats"`
	IsFollowing bool         `json:"is_following"`
	IsSelf      bool         `json:"is_self"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at,omitempty"`
}

type profileStats struct {
	Followers int `json:"followers"`
	Following int `json:"following"`
	Projects  int `json:"projects"`
}

func (h *Handler) getMe(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.IDEQ(uid)).
		Only(ctx)
	if err != nil {
		abortUserQuery(c, err)
		return
	}

	resp, err := toProfile(ctx, u, uid)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load user stats: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) updateMe(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req updateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	if err := normalizeUpdate(&req); err != nil {
		httpx.Abort(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	update := dao.Client().User.UpdateOneID(uid)
	applyOptionalString(update, req.DisplayName, setDisplayName, clearDisplayName)
	applyOptionalString(update, req.Title, setTitle, clearTitle)
	applyOptionalString(update, req.Bio, setBio, clearBio)
	applyOptionalString(update, req.Location, setLocation, clearLocation)
	if req.Username != nil {
		update.SetUsername(*req.Username)
	}
	if req.Interests != nil {
		update.SetInterests(req.Interests)
	}

	u, err := update.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			httpx.Abort(c, httpx.Conflict("username already in use"))
			return
		}
		abortUserQuery(c, err)
		return
	}

	resp, err := toProfile(ctx, u, uid)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load user stats: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) getInterests(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.IDEQ(uid)).
		Only(ctx)
	if err != nil {
		abortUserQuery(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"interests": safeInterests(u.Interests)})
}

func (h *Handler) updateInterests(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req updateInterestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	interests, err := normalizeInterests(req.Interests)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.UpdateOneID(uid).
		SetInterests(interests).
		Save(ctx)
	if err != nil {
		abortUserQuery(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"interests": safeInterests(u.Interests)})
}

func (h *Handler) getUser(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		httpx.Abort(c, httpx.BadQuery("username is required"))
		return
	}

	viewerID := auth.UserID(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.UsernameEQ(username)).
		Only(ctx)
	if err != nil {
		abortUserQuery(c, err)
		return
	}

	resp, err := toProfile(ctx, u, viewerID)
	if err != nil {
		httpx.Abort(c, httpx.Internal("load user stats: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, resp)
}

func normalizeUpdate(req *updateMeRequest) error {
	var err error
	req.DisplayName = normalizeOptional(req.DisplayName)
	req.Title = normalizeOptional(req.Title)
	req.Bio = normalizeOptional(req.Bio)
	req.Location = normalizeOptional(req.Location)

	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if len(username) < 3 || len(username) > 64 {
			return httpx.BadBody("username must be between 3 and 64 characters")
		}
		if !usernameRe.MatchString(username) {
			return httpx.BadBody("username must contain only alphanumeric characters, underscores, or hyphens")
		}
		req.Username = &username
	}
	if req.DisplayName != nil && len(*req.DisplayName) > 128 {
		return httpx.BadBody("display_name must be at most 128 characters")
	}
	if req.Title != nil && len(*req.Title) > 128 {
		return httpx.BadBody("title must be at most 128 characters")
	}
	if req.Location != nil && len(*req.Location) > 128 {
		return httpx.BadBody("location must be at most 128 characters")
	}
	if req.Interests != nil {
		req.Interests, err = normalizeInterests(req.Interests)
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizeOptional(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	return &s
}

func normalizeInterests(in []string) ([]string, error) {
	if len(in) > maxInterests {
		return nil, httpx.BadBody("interests must contain at most 20 items")
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if len(s) > maxInterestLen {
			return nil, httpx.BadBody("each interest must be at most 64 characters")
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

func toProfile(ctx context.Context, u *ent.User, viewerID string) (*profileResponse, error) {
	followers, err := u.QueryFollowers().Count(ctx)
	if err != nil {
		return nil, err
	}
	following, err := u.QueryFollowing().Count(ctx)
	if err != nil {
		return nil, err
	}

	return &profileResponse{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Title:       u.Title,
		Bio:         u.Bio,
		AvatarURL:   u.AvatarURL,
		CoverURL:    u.CoverURL,
		Location:    u.Location,
		Interests:   safeInterests(u.Interests),
		Stats: profileStats{
			Followers: followers,
			Following: following,
			Projects:  0,
		},
		IsFollowing: false,
		IsSelf:      viewerID != "" && viewerID == u.ID,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}, nil
}

func safeInterests(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func abortUserQuery(c *gin.Context, err error) {
	if ent.IsNotFound(err) {
		httpx.Abort(c, httpx.NotFound("user not found"))
		return
	}
	httpx.Abort(c, httpx.Internal("query user: "+err.Error()))
}

func applyOptionalString(
	update *ent.UserUpdateOne,
	value *string,
	set func(*ent.UserUpdateOne, string),
	clear func(*ent.UserUpdateOne),
) {
	if value == nil {
		return
	}
	if *value == "" {
		clear(update)
		return
	}
	set(update, *value)
}

func setDisplayName(u *ent.UserUpdateOne, v string) { u.SetDisplayName(v) }
func clearDisplayName(u *ent.UserUpdateOne)         { u.ClearDisplayName() }
func setTitle(u *ent.UserUpdateOne, v string)       { u.SetTitle(v) }
func clearTitle(u *ent.UserUpdateOne)               { u.ClearTitle() }
func setBio(u *ent.UserUpdateOne, v string)         { u.SetBio(v) }
func clearBio(u *ent.UserUpdateOne)                 { u.ClearBio() }
func setLocation(u *ent.UserUpdateOne, v string)    { u.SetLocation(v) }
func clearLocation(u *ent.UserUpdateOne)            { u.ClearLocation() }
