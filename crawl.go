package main

import (
	"context"
	"fmt"

	"github.com/cyverse-de/group-propagator/client/groups"
	"github.com/cyverse-de/messaging/v9"
	"github.com/pkg/errors"

	"go.opentelemetry.io/otel"
)

type Crawler struct {
	groupsClient *groups.GroupsClient
	publicGroup  string

	// maybe a data-info client too for irods crawling?

	publishClient *messaging.Client
}

func NewCrawler(groupsClient *groups.GroupsClient, publicGroup string, publishClient *messaging.Client) *Crawler {
	return &Crawler{
		groupsClient:  groupsClient,
		publicGroup:   publicGroup,
		publishClient: publishClient,
	}
}

// Request every group the groups service knows about. The service holds one
// deployment's group data, so there is no folder or prefix left to scope by.
// This handles new groups and existing groups with updated memberships;
// it does not send messages for groups that no longer exist.
func (c *Crawler) CrawlGroups(ctx context.Context) error {
	ctx, span := otel.Tracer(otelName).Start(ctx, "CrawlGroups")
	defer span.End()

	gs, err := c.groupsClient.ListAllGroups(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed listing groups")
	}

	var overallError error
	for _, group := range gs {
		if group.ID != c.publicGroup {
			err = c.publishClient.PublishContext(ctx, fmt.Sprintf("index.group.%s", group.ID), []byte{})
		}
		if err != nil {
			log.Error(errors.Wrap(err, fmt.Sprintf("Error publishing message for group %s", group.ID)))
			overallError = err
		}
	}

	return overallError
}
