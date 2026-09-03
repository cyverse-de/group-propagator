package main

import (
	"context"
	"fmt"

	"github.com/cyverse-de/group-propagator/client/groups"
	"github.com/pkg/errors"

	"go.opentelemetry.io/otel"
)

// groupPublisher is the part of the messaging client the crawl uses. The
// interface lives here because its implementation is a third-party package.
type groupPublisher interface {
	PublishContext(ctx context.Context, key string, body []byte) error
}

type Crawler struct {
	groupsClient   *groups.GroupsClient
	deUsersGroupID string

	// maybe a data-info client too for irods crawling?

	publishClient groupPublisher
}

func NewCrawler(groupsClient *groups.GroupsClient, deUsersGroupID string, publishClient groupPublisher) *Crawler {
	return &Crawler{
		groupsClient:   groupsClient,
		deUsersGroupID: deUsersGroupID,
		publishClient:  publishClient,
	}
}

// Request propagation of every group users can create, skipping the DE's own
// internal groups. The service holds one deployment's group data, so there is
// no folder or prefix left to scope by. This handles new groups and existing
// groups with updated memberships; it does not send messages for groups that no
// longer exist.
func (c *Crawler) CrawlGroups(ctx context.Context) error {
	ctx, span := otel.Tracer(otelName).Start(ctx, "CrawlGroups")
	defer span.End()

	gs, err := c.groupsClient.ListAllGroups(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed listing groups")
	}

	var overallError error
	for _, group := range gs {
		// System groups (de-users among them) are internal bookkeeping with no
		// iRODS counterpart. Propagating one would create an @grouper-<id>
		// iRODS group that nothing else knows about or ever removes.
		if group.GroupType == groups.GroupTypeSystem || group.ID == c.deUsersGroupID {
			continue
		}
		if err := c.publishClient.PublishContext(ctx, fmt.Sprintf("index.group.%s", group.ID), []byte{}); err != nil {
			log.Error(errors.Wrap(err, fmt.Sprintf("Error publishing message for group %s", group.ID)))
			overallError = err
		}
	}

	return overallError
}
