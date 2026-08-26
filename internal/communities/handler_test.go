package communities

import (
	"testing"
	"time"

	"github.com/kongken/ohome/internal/dao/ent"
)

func TestValidateSortWhitelist(t *testing.T) {
	for _, s := range []string{"", "members", "trending"} {
		if err := validateSort(s); err != nil {
			t.Fatalf("validateSort(%q) returned error: %v", s, err)
		}
	}
	if err := validateSort("popular"); err == nil {
		t.Fatal("validateSort should reject unknown values")
	}
}

func TestBuildCommunityMapsFields(t *testing.T) {
	cm := &ent.Community{
		ID:           "c_design_systems",
		Name:         "Design Systems",
		Description:  "All things design systems",
		IconURL:      "https://x/icon.png",
		CoverURL:     "https://x/cover.png",
		Category:     "design",
		MembersCount: 12400,
	}
	resp := buildCommunity(cm, false)

	if resp.ID != cm.ID || resp.Name != cm.Name || resp.Category != "design" {
		t.Fatalf("resp = %#v", resp)
	}
	if resp.MembersCount != 12400 {
		t.Fatalf("members_count = %d, want 12400", resp.MembersCount)
	}
	if resp.IsMember {
		t.Fatal("is_member should be false")
	}

	member := buildCommunity(cm, true)
	if !member.IsMember {
		t.Fatal("is_member should be true when passed")
	}
}

func TestBuildCommunityOmitsEmptyOptionals(t *testing.T) {
	resp := buildCommunity(&ent.Community{ID: "c1", Name: "N"}, true)
	if resp.Description != "" || resp.IconURL != "" || resp.CoverURL != "" || resp.Category != "" {
		t.Fatalf("optional fields should stay empty for omitempty, got %#v", resp)
	}
}

func TestMembershipCursorRowShape(t *testing.T) {
	// Guard the fields the keyset pagination relies on.
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	row := ent.Membership{ID: "m1", CreatedAt: now}
	if row.ID != "m1" || !row.CreatedAt.Equal(now) {
		t.Fatalf("membership row = %#v", row)
	}
}
