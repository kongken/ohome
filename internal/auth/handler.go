package auth

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
	"github.com/kongken/ohome/internal/httpx"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Handler bundles the auth-related HTTP handlers. It depends on the ent
// client for user persistence and an Issuer for JWT signing/verification.
type Handler struct {
	issuer *Issuer
}

func NewHandler(issuer *Issuer) *Handler {
	return &Handler{issuer: issuer}
}

// Register wires every auth route onto the given group. The group is
// expected to be `/api/v1/auth`. Routes that need authentication use
// RequireAuth on a per-route basis.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/register", h.register)
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.GET("/me", RequireAuth(h.issuer), h.me)
}

// --- requests / responses ---------------------------------------------------

type registerRequest struct {
	FullName     string `json:"full_name" binding:"required,max=128"`
	Email        string `json:"email" binding:"required,email,max=254"`
	Username     string `json:"username" binding:"required,min=3,max=64"`
	Password     string `json:"password" binding:"required,min=8,max=72"`
	AcceptTerms  bool   `json:"accept_terms"`
}

type loginRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"remember_me"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type tokenPair struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	TokenType    string         `json:"token_type"`
	ExpiresIn    int64          `json:"expires_in"`
	User         *userPublic    `json:"user,omitempty"`
}

type userPublic struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"display_name,omitempty"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

func toPublic(u *ent.User) *userPublic {
	return &userPublic{
		ID:            u.ID,
		Username:      u.Username,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		AvatarURL:     u.AvatarURL,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt,
	}
}

// --- handlers ---------------------------------------------------------------

func (h *Handler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	if !req.AcceptTerms {
		httpx.Abort(c, httpx.Unprocessable("terms must be accepted"))
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	if !usernameRe.MatchString(req.Username) {
		httpx.Abort(c, httpx.BadBody("username must contain only alphanumeric characters, underscores, or hyphens"))
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.Abort(c, httpx.Internal("failed to hash password"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	displayName := req.FullName
	if displayName == "" {
		displayName = req.Username
	}

	u, err := dao.Client().User.Create().
		SetID(uuid.NewString()).
		SetUsername(req.Username).
		SetEmail(req.Email).
		SetPasswordHash(hash).
		SetDisplayName(displayName).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			httpx.Abort(c, httpx.Conflict("username or email already in use"))
			return
		}
		httpx.Abort(c, httpx.Internal("create user: "+err.Error()))
		return
	}

	access, refresh, expiresIn, err := h.issuer.IssuePair(u.ID, u.Username)
	if err != nil {
		httpx.Abort(c, httpx.Internal("issue token: "+err.Error()))
		return
	}

	c.JSON(http.StatusCreated, tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         toPublic(u),
	})
}

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.EmailEQ(req.Email)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			httpx.Abort(c, httpx.New(http.StatusUnauthorized, httpx.CodeAuthInvalidCreds, "invalid email or password"))
			return
		}
		httpx.Abort(c, httpx.Internal("query user: "+err.Error()))
		return
	}

	if !VerifyPassword(u.PasswordHash, req.Password) {
		httpx.Abort(c, httpx.New(http.StatusUnauthorized, httpx.CodeAuthInvalidCreds, "invalid email or password"))
		return
	}

	access, refresh, expiresIn, err := h.issuer.IssuePair(u.ID, u.Username)
	if err != nil {
		httpx.Abort(c, httpx.Internal("issue token: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         toPublic(u),
	})
}

func (h *Handler) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}

	claims, err := h.issuer.Parse(req.RefreshToken, RefreshToken)
	if err != nil {
		code := httpx.CodeAuthTokenInvalid
		msg := "invalid refresh token"
		if errors.Is(err, jwt.ErrTokenExpired) {
			code = httpx.CodeAuthTokenExpired
			msg = "refresh token expired"
		}
		httpx.Abort(c, httpx.New(http.StatusUnauthorized, code, msg))
		return
	}

	access, refresh, expiresIn, err := h.issuer.IssuePair(claims.Subject, claims.Username)
	if err != nil {
		httpx.Abort(c, httpx.Internal("issue token: "+err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	u, err := dao.Client().User.Query().
		Where(entuser.IDEQ(claims.Subject)).
		Only(ctx)
	if err != nil {
		httpx.Abort(c, httpx.Internal("query user: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         toPublic(u),
	})
}

func (h *Handler) me(c *gin.Context) {
	uid := UserID(c)
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
		if ent.IsNotFound(err) {
			httpx.Abort(c, httpx.NotFound("user not found"))
			return
		}
		httpx.Abort(c, httpx.Internal("query user: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, toPublic(u))
}
