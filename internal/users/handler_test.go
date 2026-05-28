package users

import "testing"

func TestNormalizeUpdate(t *testing.T) {
	displayName := "  Alex Smith  "
	username := "alex_smith"
	title := "  Product Designer  "
	location := "  San Francisco  "
	req := updateMeRequest{
		DisplayName: &displayName,
		Username:    &username,
		Title:       &title,
		Location:    &location,
		Interests:   []string{" Design ", "design", "", "Photography"},
	}

	if err := normalizeUpdate(&req); err != nil {
		t.Fatalf("normalizeUpdate returned error: %v", err)
	}
	if got := *req.DisplayName; got != "Alex Smith" {
		t.Fatalf("display_name = %q", got)
	}
	if got := *req.Title; got != "Product Designer" {
		t.Fatalf("title = %q", got)
	}
	if got := *req.Location; got != "San Francisco" {
		t.Fatalf("location = %q", got)
	}
	if got := *req.Username; got != "alex_smith" {
		t.Fatalf("username = %q", got)
	}
	want := []string{"Design", "Photography"}
	if len(req.Interests) != len(want) {
		t.Fatalf("interests len = %d, want %d: %#v", len(req.Interests), len(want), req.Interests)
	}
	for i := range want {
		if req.Interests[i] != want[i] {
			t.Fatalf("interests[%d] = %q, want %q", i, req.Interests[i], want[i])
		}
	}
}

func TestNormalizeUpdateRejectsInvalidUsername(t *testing.T) {
	username := "bad name"
	req := updateMeRequest{Username: &username}

	if err := normalizeUpdate(&req); err == nil {
		t.Fatal("normalizeUpdate returned nil error for invalid username")
	}
}

func TestNormalizeInterestsRejectsTooManyItems(t *testing.T) {
	interests := make([]string, maxInterests+1)
	for i := range interests {
		interests[i] = "tag"
	}

	if _, err := normalizeInterests(interests); err == nil {
		t.Fatal("normalizeInterests returned nil error for too many interests")
	}
}
