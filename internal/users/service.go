package users

import (
	"context"
	"fmt"

	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/dao/ent"
	entuser "github.com/kongken/ohome/internal/dao/ent/user"
)

// UserSummary is the compact user representation used in list responses
// (followers, likes, community members, notification actors, ...).
type UserSummary struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsFollowing bool   `json:"is_following"`
}

// SummariesByIDs builds UserSummary values in the given ID order,
// batch-loading users and follow status in two queries. Users that vanished
// (deleted since the referencing row was created) are silently skipped.
func SummariesByIDs(ctx context.Context, ids []string, viewerID string) ([]UserSummary, error) {
	out := make([]UserSummary, 0, len(ids))
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
			continue // user deleted since the referencing row was created
		}
		out = append(out, UserSummary{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			IsFollowing: following[u.ID],
		})
	}
	return out, nil
}

// IsFollowing checks whether followerID follows targetID.
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
