package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"

	"github.com/cyverse-de/go-mod/restutils"
	"github.com/cyverse-de/group-propagator/client/datainfo"
	"github.com/cyverse-de/group-propagator/client/groups"
)

// To propagate a group:
// * Fetch group details and members via iplant-groups
//   -> get a model.GrouperGroup and model.GrouperGroupMembers, probably
// * Determine iRODS group name (@grouper-<GrouperGroup.ID>)
// * Create or update group with proper membership list via data-info, potentially validating users/etc.

type Propagator struct {
	groupsClient *groups.GroupsClient
	groupPrefix  string

	dataInfoClient *datainfo.DataInfoClient
}

func NewPropagator(groupsClient *groups.GroupsClient, groupPrefix string, dataInfoClient *datainfo.DataInfoClient) *Propagator {
	if groupPrefix == "" {
		groupPrefix = "@grouper-"
	}

	return &Propagator{
		groupsClient:   groupsClient,
		groupPrefix:    groupPrefix,
		dataInfoClient: dataInfoClient,
	}
}

func (p *Propagator) getGroupMembers(ctx context.Context, groupName string) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "getGroupMembers")
	defer span.End()

	var m []string

	members, err := p.groupsClient.GetGroupMembers(ctx, groupName)
	if err != nil {
		return m, errors.Wrapf(err, "Failed fetching Grouper group members for %s", groupName)
	}

	for _, member := range members.Members {
		if member.SourceID == "ldap" {
			m = append(m, member.ID)
		} else if member.SourceID == "g:gsa" {
			// this is a group that is a member of a group
			submem, err := p.getGroupMembers(ctx, member.Name)
			if err != nil {
				return m, errors.Wrapf(err, "Failed recursing to fetch members of %s", member.Name)
			}
			m = append(m, submem...)
		} else {
			log.Errorf("Could not add group member %+v", member)
		}
	}

	return m, nil
}

func (p *Propagator) PropagateGroupById(ctx context.Context, groupID string) error {
	ctx, span := otel.Tracer(otelName).Start(ctx, "PropagateGroupByID")
	defer span.End()

	// Don't propagate the de-users group.
	if groupID == p.groupsClient.GroupsID {
		log.Infof("Skipping a propagation request for the de-users group: %s", groupID)
		return nil
	}

	irodsName := fmt.Sprintf("%s%s", p.groupPrefix, groupID)

	g, err := p.groupsClient.GetGroupByID(ctx, groupID)
	if restutils.GetStatusCode(err) == 404 {
		err = p.dataInfoClient.DeleteGroup(ctx, irodsName)
		if err != nil {
			err = errors.Wrap(err, "Error deleting group")
		}
		return err
	} else if err != nil {
		return errors.Wrap(err, "Failed fetching Grouper group by ID")
	} else if groupID != g.ID {
		return errors.New(fmt.Sprintf("Fetched Grouper group has an ID of %s, but was fetched using the ID %s", g.ID, groupID))
	}

	irodsMembers, err := p.getGroupMembers(ctx, g.Name)
	if err != nil {
		return errors.Wrap(err, "Failed getting group members")
	}

	irodsGroupExists := true

	// Check if group exists/has members, but we don't need to care what members
	_, err = p.dataInfoClient.ListGroupMembers(ctx, irodsName)
	if restutils.GetStatusCode(err) == 404 {
		irodsGroupExists = false
	} else if err != nil {
		return errors.Wrap(err, "Failed fetching existing iRODS group members")
	}

	if !irodsGroupExists {
		initialGroup, err := p.dataInfoClient.CreateGroup(ctx, irodsName, []string{})
		if err != nil {
			return errors.Wrapf(err, "Failed creating group %s (%s) -> %s", g.Name, groupID, initialGroup.Name)
		}
	}

	finalGroup, err := p.dataInfoClient.UpdateGroupMembers(ctx, irodsName, irodsMembers)

	if err != nil {
		return errors.Wrapf(err, "Failed updating group %s (%s) -> %s with %d members", g.Name, groupID, finalGroup.Name, len(irodsMembers))
	}

	log.Infof("Updated group %s (%s) -> %s with %d members", g.Name, groupID, finalGroup.Name, len(finalGroup.Members))

	return nil
}
