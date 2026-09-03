package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"

	"github.com/cyverse-de/group-propagator/client/groups"
)

// memberFixture describes one group's membership in the fake groups service.
type memberFixture struct {
	id       string
	name     string
	sourceID string
}

// newGroupsServer serves member listings keyed by group ID only. A request for
// any other path 404s, so a recursion that follows a nested group's name rather
// than its ID fails the test rather than quietly returning short membership.
// Requests are hard-capped so a recursion that stops terminating (e.g. on a
// membership cycle) fails the test quickly instead of hanging it.
func newGroupsServer(t *testing.T, membership map[string][]memberFixture) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for groupID, members := range membership {
		subjects := make([]groups.Subject, 0, len(members))
		for _, m := range members {
			subjects = append(subjects, groups.Subject{ID: m.id, Name: m.name, SourceID: m.sourceID})
		}
		body := groups.GroupMembers{Members: subjects}
		mux.HandleFunc(fmt.Sprintf("/groups/%s/members", groupID), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encoding member listing: %v", err)
			}
		})
	}

	const maxRequests = 25
	var requests int
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > maxRequests {
			t.Errorf("more than %d requests; the recursion is probably not terminating", maxRequests)
			http.Error(w, "too many requests", http.StatusInternalServerError)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func TestGetGroupMembersFollowsNestingByID(t *testing.T) {
	// Field Team -> Genomics Lab -> msmith's default, mirroring the local test
	// dataset: lchen is only reachable at depth 2.
	const (
		fieldTeam   = "1111111111111111111111111111aaaa"
		genomicsLab = "2222222222222222222222222222bbbb"
		msmithList  = "3333333333333333333333333333cccc"
	)

	tests := []struct {
		name    string
		groupID string
		want    []string
	}{
		{
			name:    "two levels of nesting",
			groupID: fieldTeam,
			want:    []string{"lchen", "msmith", "rpatel"},
		},
		{
			name:    "one level of nesting",
			groupID: genomicsLab,
			want:    []string{"lchen", "rpatel"},
		},
		{
			name:    "no nesting",
			groupID: msmithList,
			want:    []string{"lchen"},
		},
	}

	membership := map[string][]memberFixture{
		fieldTeam: {
			{id: "msmith", name: "msmith", sourceID: "ldap"},
			{id: genomicsLab, name: "Genomics Lab", sourceID: "g:gsa"},
		},
		genomicsLab: {
			{id: "rpatel", name: "rpatel", sourceID: "ldap"},
			{id: msmithList, name: "default", sourceID: "g:gsa"},
		},
		msmithList: {
			{id: "lchen", name: "lchen", sourceID: "ldap"},
		},
	}

	srv := newGroupsServer(t, membership)
	defer srv.Close()

	client := groups.NewGroupsClient(srv.URL, "de_grouper", "de-users")
	propagator := NewPropagator(client, "@grouper-", nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := propagator.getGroupMembers(context.Background(), tt.groupID)
			if err != nil {
				t.Fatalf("getGroupMembers(%s): %v", tt.groupID, err)
			}
			sort.Strings(got)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// A membership cycle (A contains B contains A) can't be built through the
// groups service API, but it is reachable by editing the database directly.
// The recursion must terminate on one and return the union of user members;
// with the AMQP consumer's concurrency of 1, an unterminated recursion would
// halt all propagation. A revisited group is skipped silently rather than
// treated as an error, because the same shape occurs legitimately in a
// diamond (two groups sharing a subgroup).
func TestGetGroupMembersTerminatesOnCycle(t *testing.T) {
	const (
		groupA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaa1111"
		groupB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbb2222"
	)

	membership := map[string][]memberFixture{
		groupA: {
			{id: "asturm", name: "asturm", sourceID: "ldap"},
			{id: groupB, name: "Group B", sourceID: "g:gsa"},
		},
		groupB: {
			{id: "bcarter", name: "bcarter", sourceID: "ldap"},
			{id: groupA, name: "Group A", sourceID: "g:gsa"},
		},
	}

	srv := newGroupsServer(t, membership)
	defer srv.Close()

	client := groups.NewGroupsClient(srv.URL, "de_grouper", "de-users")
	propagator := NewPropagator(client, "@grouper-", nil)

	got, err := propagator.getGroupMembers(context.Background(), groupA)
	if err != nil {
		t.Fatalf("getGroupMembers(%s): %v", groupA, err)
	}
	sort.Strings(got)
	want := []string{"asturm", "bcarter"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// A redacted member list must abort propagation rather than be treated as an
// empty group. data-info's member update is a replace, so propagating an empty
// list strips the iRODS group -- and the log line for that ("with 0 members")
// is indistinguishable from the benign case where iRODS has no account for the
// members. The only signal is this refusal.
func TestGetGroupMembersRefusesRedactedList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/groups/g1/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Exactly what the groups service returns to a caller that is not an
		// admin, for a public group whose membership is not public.
		if err := json.NewEncoder(w).Encode(groups.GroupMembers{
			Members:  []groups.Subject{},
			Redacted: true,
		}); err != nil {
			t.Errorf("encoding member listing: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := &Propagator{groupsClient: groups.NewGroupsClient(srv.URL, "de_grouper", "de-users")}

	members, err := p.getGroupMembers(context.Background(), "g1")
	if err == nil {
		t.Fatalf("expected a refusal, got %d members and no error", len(members))
	}
	if len(members) != 0 {
		t.Errorf("no members should be returned alongside the refusal, got %d", len(members))
	}
}

// A member whose source the propagator cannot resolve has to abort the
// propagation. Logging and skipping it hands data-info a short list, and the
// member update replaces rather than merges, so the unresolved member is
// dropped from the iRODS group under a log line that reads like an ordinary
// successful run -- the same failure the redacted-list refusal exists to stop.
func TestGetGroupMembersRefusesUnknownSource(t *testing.T) {
	const (
		parent = "4444444444444444444444444444dddd"
		child  = "5555555555555555555555555555eeee"
	)

	tests := []struct {
		name       string
		membership map[string][]memberFixture
		want       []string
		wantErr    bool
	}{
		{
			name: "known sources only",
			membership: map[string][]memberFixture{
				parent: {
					{id: "asturm", name: "asturm", sourceID: "ldap"},
					{id: child, name: "Genomics Lab", sourceID: "g:gsa"},
				},
				child: {{id: "bcarter", name: "bcarter", sourceID: "ldap"}},
			},
			want: []string{"asturm", "bcarter"},
		},
		{
			name: "unrecognized source id",
			membership: map[string][]memberFixture{
				parent: {
					{id: "asturm", name: "asturm", sourceID: "ldap"},
					{id: "svc-account", name: "svc-account", sourceID: "jdbc"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing source id",
			membership: map[string][]memberFixture{
				parent: {{id: "asturm", name: "asturm", sourceID: ""}},
			},
			wantErr: true,
		},
		{
			name: "unrecognized source id inside a nested group",
			membership: map[string][]memberFixture{
				parent: {{id: child, name: "Genomics Lab", sourceID: "g:gsa"}},
				child:  {{id: "svc-account", name: "svc-account", sourceID: "jdbc"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newGroupsServer(t, tt.membership)
			defer srv.Close()

			client := groups.NewGroupsClient(srv.URL, "de_grouper", "de-users")
			propagator := NewPropagator(client, "@grouper-", nil)

			got, err := propagator.getGroupMembers(context.Background(), parent)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected a refusal, got %v and no error", got)
				}
				if len(got) != 0 {
					t.Errorf("no members should be returned alongside the refusal, got %d", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("getGroupMembers(%s): %v", parent, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Nesting reaches the same user by more than one path whenever someone belongs
// to a group and to one of its subgroups. The visited set only keeps each group
// from being fetched twice, so the member list itself has to be deduplicated:
// otherwise data-info is handed repeats and the propagation log reports more
// members than the group has.
func TestGetGroupMembersDeduplicates(t *testing.T) {
	const (
		parent = "6666666666666666666666666666ffff"
		left   = "7777777777777777777777777777aaaa"
		right  = "8888888888888888888888888888bbbb"
	)

	tests := []struct {
		name       string
		membership map[string][]memberFixture
		// Order is first occurrence in the crawl, so a change here is a change
		// in what data-info is handed.
		want []string
	}{
		{
			name: "member of both a group and its subgroup",
			membership: map[string][]memberFixture{
				parent: {
					{id: "msmith", name: "msmith", sourceID: "ldap"},
					{id: left, name: "Genomics Lab", sourceID: "g:gsa"},
				},
				left: {
					{id: "msmith", name: "msmith", sourceID: "ldap"},
					{id: "rpatel", name: "rpatel", sourceID: "ldap"},
				},
			},
			want: []string{"msmith", "rpatel"},
		},
		{
			name: "two subgroups sharing a member",
			membership: map[string][]memberFixture{
				parent: {
					{id: left, name: "Genomics Lab", sourceID: "g:gsa"},
					{id: right, name: "Field Team", sourceID: "g:gsa"},
				},
				left: {{id: "rpatel", name: "rpatel", sourceID: "ldap"}},
				right: {
					{id: "rpatel", name: "rpatel", sourceID: "ldap"},
					{id: "lchen", name: "lchen", sourceID: "ldap"},
				},
			},
			want: []string{"rpatel", "lchen"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newGroupsServer(t, tt.membership)
			defer srv.Close()

			client := groups.NewGroupsClient(srv.URL, "de_grouper", "de-users")
			propagator := NewPropagator(client, "@grouper-", nil)

			got, err := propagator.getGroupMembers(context.Background(), parent)
			if err != nil {
				t.Fatalf("getGroupMembers(%s): %v", parent, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
