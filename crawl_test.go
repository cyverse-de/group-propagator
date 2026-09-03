package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/cyverse-de/group-propagator/client/groups"
)

// recordingPublisher stands in for the AMQP client, keeping the routing keys
// the crawl asked to publish.
type recordingPublisher struct {
	keys []string
}

func (p *recordingPublisher) PublishContext(ctx context.Context, key string, body []byte) error {
	p.keys = append(p.keys, key)
	return nil
}

// newListingServer serves a paginated /groups listing over the given groups.
func newListingServer(t *testing.T, gs []groups.Group) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups" {
			http.NotFound(w, r)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		page := []groups.Group{}
		for i := offset; i < offset+limit && i < len(gs); i++ {
			page = append(page, gs[i])
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(groups.GroupList{Groups: page}); err != nil {
			t.Errorf("encoding listing: %v", err)
		}
	}))
}

// The listing carries the DE's own internal groups alongside the ones users
// create. Publishing for a system group would have the propagator create an
// @grouper-<id> iRODS group for it -- an object that never existed while the
// listing was scoped by folder prefix, and that nothing else removes.
func TestCrawlGroupsSkipsSystemGroups(t *testing.T) {
	const deUsers = "1111111111111111111111111111aaaa"

	tests := []struct {
		name   string
		groups []groups.Group
		want   []string
	}{
		{
			name: "user-created groups only",
			groups: []groups.Group{
				{ID: "aaaa1111", Name: "Field Team", GroupType: "team"},
				{ID: "bbbb2222", Name: "default", GroupType: "collaborator_list"},
				{ID: "cccc3333", Name: "Genomics", GroupType: "community"},
			},
			want: []string{"index.group.aaaa1111", "index.group.bbbb2222", "index.group.cccc3333"},
		},
		{
			name: "system groups mixed in",
			groups: []groups.Group{
				{ID: deUsers, Name: "de-users", GroupType: "system"},
				{ID: "dddd4444", Name: "grouper-all", GroupType: "system"},
				{ID: "aaaa1111", Name: "Field Team", GroupType: "team"},
			},
			want: []string{"index.group.aaaa1111"},
		},
		{
			name: "de-users skipped by ID as well as by type",
			groups: []groups.Group{
				{ID: deUsers, Name: "de-users", GroupType: "team"},
			},
			want: []string{},
		},
		{
			name: "nothing but system groups",
			groups: []groups.Group{
				{ID: "dddd4444", Name: "grouper-all", GroupType: "system"},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newListingServer(t, tt.groups)
			defer srv.Close()

			publisher := &recordingPublisher{}
			crawler := NewCrawler(groups.NewGroupsClient(srv.URL, "de_grouper", "de-users"), deUsers, publisher)

			if err := crawler.CrawlGroups(context.Background()); err != nil {
				t.Fatalf("CrawlGroups: %v", err)
			}
			if !slices.Equal(publisher.keys, tt.want) {
				t.Fatalf("published %v, want %v", publisher.keys, tt.want)
			}
		})
	}
}
