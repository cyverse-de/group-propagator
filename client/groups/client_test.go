package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// The groups service caps a single listing at 1000, and production carries
// well over that, so a crawler that issues one unpaginated request propagates
// ACLs for the first page and silently ignores the rest.
func TestListAllGroupsPaginates(t *testing.T) {
	tests := []struct {
		name  string
		total int
		want  int
	}{
		{name: "empty", total: 0, want: 0},
		{name: "single short page", total: 17, want: 17},
		{name: "exactly one full page", total: pageSize, want: pageSize},
		{name: "just over one page", total: pageSize + 1, want: pageSize + 1},
		{name: "production scale", total: 2583, want: 2583},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/groups" {
					http.NotFound(w, r)
					return
				}
				requests++
				limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

				page := []Group{}
				for i := offset; i < offset+limit && i < tt.total; i++ {
					page = append(page, Group{ID: fmt.Sprintf("group-%04d", i)})
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(GroupList{Groups: page}); err != nil {
					t.Errorf("encoding page: %v", err)
				}
			}))
			defer srv.Close()

			c := NewGroupsClient(srv.URL, "de_grouper", "de-users")
			got, err := c.ListAllGroups(context.Background())
			if err != nil {
				t.Fatalf("ListAllGroups: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d groups over %d requests, want %d", len(got), requests, tt.want)
			}

			// Every group exactly once, in order: an off-by-one in the offset
			// arithmetic drops or repeats rows without changing the count.
			for i, g := range got {
				if want := fmt.Sprintf("group-%04d", i); g.ID != want {
					t.Fatalf("group %d is %s, want %s", i, g.ID, want)
				}
			}
		})
	}
}

func TestLookupDEUsersGroup(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/lookup" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(Group{ID: "abc123", Name: "de-users"}); err != nil {
			t.Errorf("encoding group: %v", err)
		}
	}))
	defer srv.Close()

	c := NewGroupsClient(srv.URL, "de_grouper", "de-users")
	if err := c.SetGroupsID(context.Background()); err != nil {
		t.Fatalf("SetGroupsID: %v", err)
	}
	if c.GroupsID != "abc123" {
		t.Fatalf("GroupsID is %q, want abc123", c.GroupsID)
	}

	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	// de-users is a system group, and the type is what keeps the lookup from
	// matching a collaborator list that happens to be named de-users.
	for key, want := range map[string]string{"group_type": "system", "name": "de-users", "user": "de_grouper"} {
		if q.Get(key) != want {
			t.Errorf("query %s is %q, want %q", key, q.Get(key), want)
		}
	}
}
