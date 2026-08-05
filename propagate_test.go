package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	return httptest.NewServer(mux)
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
