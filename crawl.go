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
	groupsClient    *groups.GroupsClient
	groupBaseFolder string
	publicGroup     string

	// maybe a data-info client too for irods crawling?

	publishClient *messaging.Client
}

func NewCrawler(groupsClient *groups.GroupsClient, groupBaseFolder, publicGroup string, publishClient *messaging.Client) *Crawler {
	return &Crawler{
		groupsClient:    groupsClient,
		groupBaseFolder: groupBaseFolder,
		publicGroup:     publicGroup,
		publishClient:   publishClient,
	}
}

// Request all groups within the configured base folder/prefix
// This handles new groups and existing groups with updated memberships
// It does not send messages for groups that no longer exist in Grouper
func (c *Crawler) CrawlGrouperGroups(ctx context.Context) error {
	ctx, span := otel.Tracer(otelName).Start(ctx, "CrawlGrouperGroups")
	defer span.End()

	gs, err := c.groupsClient.ListGroupsByPrefix(ctx, c.groupBaseFolder, c.groupBaseFolder) // same thing passed twice: as prefix for group search and for folder to search within
	if err != nil {
		return errors.Wrap(err, "Failed listing groups by prefix")
	}

	var overallError error
	for _, group := range gs.Groups {
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
