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
		// The crawl stops only on an empty page, so a crawl over N full
		// pages costs N+1 requests: the last one comes back empty.
		wantRequests int
	}{
		{name: "empty", total: 0, want: 0, wantRequests: 1},
		{name: "single short page", total: 17, want: 17, wantRequests: 2},
		{name: "exactly one full page", total: pageSize, want: pageSize, wantRequests: 2},
		{name: "just over one page", total: pageSize + 1, want: pageSize + 1, wantRequests: 3},
		{name: "production scale", total: 2583, want: 2583, wantRequests: 4},
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
				if requests > tt.wantRequests {
					// Fail fast rather than hang: a client that never
					// advances its offset would otherwise loop forever.
					t.Errorf("request %d exceeds the %d expected; the client is probably not advancing its offset", requests, tt.wantRequests)
					http.Error(w, "too many requests", http.StatusInternalServerError)
					return
				}
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
			if requests != tt.wantRequests {
				t.Errorf("crawl took %d requests, want %d", requests, tt.wantRequests)
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

// The service caps how many groups one response may carry, and the cap is the
// server's to change. If it ever drops below what the client asks for, the
// crawl must keep paging by what actually came back rather than treating the
// clamped page as the final short one and silently truncating the listing.
func TestListAllGroupsHonorsServerCappedPages(t *testing.T) {
	const (
		total     = 1200
		serverCap = 500
		// 1200 groups at 500 per page is three data pages plus the empty
		// terminator; anything past that means the offset is not advancing.
		wantRequests = 4
	)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups" {
			http.NotFound(w, r)
			return
		}
		requests++
		if requests > wantRequests {
			// Fail fast rather than hang if the client ignores page progress.
			t.Errorf("request %d exceeds the %d expected; the client is probably not advancing its offset", requests, wantRequests)
			http.Error(w, "too many requests", http.StatusInternalServerError)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit > serverCap {
			limit = serverCap
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		page := []Group{}
		for i := offset; i < offset+limit && i < total; i++ {
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
	if len(got) != total {
		t.Fatalf("got %d groups over %d requests, want %d", len(got), requests, total)
	}
	if requests != wantRequests {
		t.Errorf("crawl took %d requests, want %d", requests, wantRequests)
	}
	for i, g := range got {
		if want := fmt.Sprintf("group-%04d", i); g.ID != want {
			t.Fatalf("group %d is %s, want %s", i, g.ID, want)
		}
	}
}

// The crawl ends on an empty page, which assumes the service advances through
// the listing. A service that did not would keep answering with groups the
// crawl already has: with the AMQP consumer's concurrency of 1 that wedges the
// only propagation goroutine while the accumulated listing grows until the pod
// is OOM-killed, and nothing is logged. The crawl has to give up instead.
func TestListAllGroupsBoundsTheCrawl(t *testing.T) {
	tests := []struct {
		name string
		// page answers a listing request made at the given offset.
		page         func(offset int) []Group
		wantRequests int
	}{
		{
			name:         "offset ignored entirely",
			page:         func(int) []Group { return []Group{{ID: "group-0000"}, {ID: "group-0001"}} },
			wantRequests: 2,
		},
		{
			name:         "listing that never runs out",
			page:         func(offset int) []Group { return []Group{{ID: fmt.Sprintf("group-%06d", offset)}} },
			wantRequests: maxListPages,
		},
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
				if requests > tt.wantRequests {
					t.Errorf("request %d exceeds the %d expected; the crawl is unbounded", requests, tt.wantRequests)
					http.Error(w, "too many requests", http.StatusInternalServerError)
					return
				}
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(GroupList{Groups: tt.page(offset)}); err != nil {
					t.Errorf("encoding page: %v", err)
				}
			}))
			defer srv.Close()

			c := NewGroupsClient(srv.URL, "de_grouper", "de-users")
			if _, err := c.ListAllGroups(context.Background()); err == nil {
				t.Fatal("expected the crawl to give up, got no error")
			}
			if requests != tt.wantRequests {
				t.Errorf("crawl took %d requests, want %d", requests, tt.wantRequests)
			}
		})
	}
}

// The groups service answers a non-admin's listing with a 200 and an
// access-filtered page -- there is no marker distinguishing it from a complete
// one. A propagator running as such a user would crawl an empty-ish listing
// forever and groups would silently stop propagating, so startup has to prove
// admin standing by checking that de-users shows up in the same kind of
// listing the crawl uses.
func TestVerifyAdminListing(t *testing.T) {
	tests := []struct {
		name   string
		groups []Group
		// serverCap is how many groups the fake will return in one response,
		// standing in for a service whose own cap is smaller than what the
		// check asks for; 0 means it answers with everything at once.
		serverCap int
		wantErr   bool
	}{
		{
			name: "de-users visible",
			groups: []Group{
				{ID: "abc123", Name: "de-users", GroupType: "system"},
				{ID: "def456", Name: "grouper-all", GroupType: "system"},
			},
			wantErr: false,
		},
		{
			name: "de-users past the first page",
			groups: []Group{
				{ID: "def456", Name: "grouper-all", GroupType: "system"},
				{ID: "abc123", Name: "de-users", GroupType: "system"},
			},
			serverCap: 1,
			wantErr:   false,
		},
		{
			name: "listing access-filtered",
			groups: []Group{
				{ID: "def456", Name: "grouper-all", GroupType: "system"},
			},
			wantErr: true,
		},
		{
			name:    "listing empty",
			groups:  []Group{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/groups" {
					http.NotFound(w, r)
					return
				}
				gotQuery = r.URL.Query()
				limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
				if tt.serverCap > 0 && limit > tt.serverCap {
					limit = tt.serverCap
				}
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

				page := []Group{}
				for i := offset; i < offset+limit && i < len(tt.groups); i++ {
					page = append(page, tt.groups[i])
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(GroupList{Groups: page}); err != nil {
					t.Errorf("encoding listing: %v", err)
				}
			}))
			defer srv.Close()

			c := NewGroupsClient(srv.URL, "de_grouper", "de-users")
			c.GroupsID = "abc123"

			err := c.VerifyAdminListing(context.Background())
			if tt.wantErr && err == nil {
				t.Fatal("expected an error for a listing without de-users, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("VerifyAdminListing: %v", err)
			}
			// System groups are the smallest slice of the listing that must
			// contain de-users, so the check filters to them.
			if got := gotQuery.Get("group_type"); got != "system" {
				t.Errorf("group_type is %q, want system", got)
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
