package httpx

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// Page is the parsed cursor pagination input.
type Page struct {
	Cursor string
	Limit  int
}

// PageResponse mirrors `common.v1.PageResponse`.
type PageResponse struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ParsePage reads `cursor` and `limit` query params, applying defaults and
// the global cap. Never returns an error: invalid `limit` falls back to the
// default rather than failing the request.
func ParsePage(c *gin.Context) Page {
	p := Page{
		Cursor: c.Query("cursor"),
		Limit:  defaultPageLimit,
	}
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > maxPageLimit {
				n = maxPageLimit
			}
			p.Limit = n
		}
	}
	return p
}
