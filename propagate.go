package main

import (
	"github.com/cyverse-de/group-propagator/client/datainfo"
	"github.com/cyverse-de/group-propagator/client/groups"
)

// To propagate a group:
// * Fetch group details and members via iplant-groups
//   -> get a model.GrouperGroup and model.GrouperGroupMembers, probably
// * Determine iRODS group name (@grouper-<GrouperGroup.ID>)
// * Create or update group with proper membership list via data-info, potentially validating users/etc.

type Propagator struct {
	groupsClient groups.GroupsClient
	groupPrefix  string

	dataInfoClient datainfo.DataInfoClient
}

func NewPropagator(groupsClient groups.GroupsClient, groupPrefix string, dataInfoClient datainfo.DataInfoClient) *Propagator {
	if groupPrefix == "" {
		groupPrefix = "@grouper-"
	}

	return &Propagator{
		groupsClient:   groupsClient,
		groupPrefix:    groupPrefix,
		dataInfoClient: dataInfoClient,
	}
}

/*
func (p *Propagator) PropagateGroupById(groupId string) error {
	// get group by ID
	// get group members
	// create group if necessary
	// update group memberships
}
*/
