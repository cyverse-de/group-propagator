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
// * Fetch group details and members via the groups service
//   -> get a groups.Group and groups.GroupMembers
// * Determine iRODS group name (@grouper-<Group.ID>)
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

func (p *Propagator) getGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	m, err := p.getGroupMembersVisiting(ctx, groupID, map[string]struct{}{groupID: {}})
	if err != nil {
		return nil, err
	}
	return dedupeMembers(m), nil
}

// dedupeMembers keeps the first occurrence of each member. A user reached both
// directly and through a subgroup appears once per path, which data-info would
// take at face value and the propagation log would report as a member count
// larger than the group has.
func dedupeMembers(members []string) []string {
	seen := make(map[string]struct{}, len(members))
	deduped := make([]string, 0, len(members))
	for _, m := range members {
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		deduped = append(deduped, m)
	}
	return deduped
}

// getGroupMembersVisiting is the recursive body of getGroupMembers. The
// visited set keeps a membership cycle (impossible via the service API, but
// reachable by editing the database directly) from recursing forever; as a
// side effect it also fetches a subgroup shared by two parents only once.
func (p *Propagator) getGroupMembersVisiting(ctx context.Context, groupID string, visited map[string]struct{}) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "getGroupMembers")
	defer span.End()

	var m []string

	members, err := p.groupsClient.GetGroupMembersByID(ctx, groupID)
	if err != nil {
		return m, errors.Wrapf(err, "Failed fetching group members for %s", groupID)
	}

	// A withheld list is empty but the group is not. Propagating it would PUT
	// an empty member list to data-info, which replaces rather than merges, and
	// silently strip the iRODS group -- logged as an unremarkable "0 members".
	// This means the configured groups user is not an administrative account of
	// the groups service; it must be, or it cannot see membership at all.
	if members.Redacted {
		return m, errors.Errorf(
			"groups service withheld the member list for %s; the configured groups.user is not "+
				"an admin of the groups service, so propagating would erase the iRODS group", groupID)
	}

	for _, member := range members.Members {
		switch member.SourceID {
		case groups.SourceUser:
			m = append(m, member.ID)
		case groups.SourceGroup:
			// A nested group. Its subject ID is the nested group's own group ID,
			// so the recursion stays keyed by ID all the way down.
			// Skipping an already-visited group is deliberately silent: a
			// shared subgroup (diamond) is legitimate, and its members were
			// already collected the first time through.
			if _, seen := visited[member.ID]; seen {
				continue
			}
			visited[member.ID] = struct{}{}
			submem, err := p.getGroupMembersVisiting(ctx, member.ID, visited)
			if err != nil {
				return m, errors.Wrapf(err, "Failed recursing to fetch members of %s (%s)", member.Name, member.ID)
			}
			m = append(m, submem...)
		default:
			// Skipping the member instead would PUT a short list to data-info,
			// which replaces rather than merges, dropping people from the iRODS
			// group under a log line that reads like a successful run.
			return nil, errors.Errorf(
				"member %s (%s) of group %s has the unexpected source id %q; this usually means the "+
					"groups service gained a subject source this propagator does not know how to "+
					"resolve, and propagating without the member would remove it from the iRODS group",
				member.Name, member.ID, groupID, member.SourceID)
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
		return errors.Wrap(err, "Failed fetching group by ID")
	} else if groupID != g.ID {
		return errors.Errorf("Fetched group has an ID of %s, but was fetched using the ID %s", g.ID, groupID)
	}

	irodsMembers, err := p.getGroupMembers(ctx, groupID)
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
