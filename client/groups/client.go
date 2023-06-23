package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cyverse-de/go-mod/restutils"
	"github.com/cyverse-de/group-propagator/logging"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

var log = logging.Log.WithFields(logrus.Fields{"package": "client.groups"})

const otelName = "github.com/cyverse-de/group-propagator/client/groups"

type GroupsClient struct {
	GroupsBase       string
	GroupsUser       string
	DEUsersGroupName string
	GroupsID         string
}

var httpClient = http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func NewGroupsClient(base string, user string, name string) *GroupsClient {
	return &GroupsClient{GroupsBase: base, GroupsUser: user, DEUsersGroupName: name}
}

func (c *GroupsClient) getDEUsersGroupID(ctx context.Context) (*group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "getGroupID")
	defer span.End()

	fullURL, err := url.Parse(c.GroupsBase)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to parse iplant-groups base URL")
	}
	fullURL = fullURL.JoinPath("groups", c.DEUsersGroupName)

	q := fullURL.Query()
	q.Set("user", c.GroupsUser)

	fullURL.RawQuery = q.Encode()

	var group group
	err = c.getJSON(ctx, fullURL.String(), &group)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get group ID")
	}
	return &group, nil
}

func (c *GroupsClient) SetGroupsID(ctx context.Context) error {
	groups, err := c.getDEUsersGroupID(ctx)
	if err != nil {
		return err
	}
	c.GroupsID = *groups.ID
	return nil
}

func (c *GroupsClient) uriPath(ctx context.Context, rawQuery string, pathParts ...string) (string, error) {
	base, err := url.Parse(c.GroupsBase)
	if err != nil {
		return "", err
	}
	uri := base.JoinPath(pathParts...)
	if rawQuery == "" {
		uri.RawQuery = fmt.Sprintf("user=%s", c.GroupsUser)
	} else {
		uri.RawQuery = fmt.Sprintf("%s&user=%s", rawQuery, c.GroupsUser)
	}

	return uri.String(), nil
}

func (c *GroupsClient) getJSON(ctx context.Context, uri string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return errors.Wrap(err, "Failed creating request with context")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Failed requesting URL")
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return restutils.NewHTTPError(resp.StatusCode, fmt.Sprintf("GET %s returned %d", uri, resp.StatusCode))
	}
	defer resp.Body.Close()

	if target != nil {
		err = json.NewDecoder(resp.Body).Decode(target)
		if err != nil {
			return errors.Wrap(err, "Failed decoding JSON")
		}
	}
	return err
}

// Use status endpoint to check our iplant-groups URI
func (c *GroupsClient) Check(ctx context.Context) error {
	uri, err := url.Parse(c.GroupsBase)
	if err != nil {
		return errors.Wrap(err, "Failed to parse iplant-groups base URL")
	}

	uri.RawQuery = "expecting=iplant-groups"

	return c.getJSON(ctx, uri.String(), nil)
}

// List groups under a provided prefix, using the REST service
func (c *GroupsClient) ListGroupsByPrefix(ctx context.Context, prefix, folder string) (GroupList, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "ListGroupsByPrefix")
	defer span.End()

	var gs GroupList
	var uri string
	var err error

	if folder != "" {
		uri, err = c.uriPath(ctx, fmt.Sprintf("search=%s&folder=%s", prefix, folder), "groups")
	} else {
		uri, err = c.uriPath(ctx, fmt.Sprintf("search=%s", prefix), "groups")
	}
	log.Debugf("ListGroupsByPrefix uri: %s", uri)

	if err != nil {
		return gs, err
	}

	err = c.getJSON(ctx, uri, &gs)
	return gs, err
}

// Get the basic group information for a group from the REST service, given a name
func (c *GroupsClient) GetGroupByName(ctx context.Context, groupName string) (Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupByName")
	defer span.End()

	var g Group

	uri, err := c.uriPath(ctx, "", "groups", groupName)
	if err != nil {
		return g, err
	}

	err = c.getJSON(ctx, uri, &g)
	return g, err
}

// Get the basic group information for a group from the REST service, given an ID
func (c *GroupsClient) GetGroupByID(ctx context.Context, groupID string) (Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupByID")
	defer span.End()

	var g Group

	uri, err := c.uriPath(ctx, "", "groups", "id", groupID)
	if err != nil {
		return g, err
	}

	err = c.getJSON(ctx, uri, &g)
	return g, err
}

// List members of a group using the REST service, given a name
func (c *GroupsClient) GetGroupMembers(ctx context.Context, groupName string) (GroupMembers, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupMembers")
	defer span.End()

	var gm GroupMembers
	uri, err := c.uriPath(ctx, "", "groups", groupName, "members")
	if err != nil {
		return gm, err
	}

	err = c.getJSON(ctx, uri, &gm)
	return gm, err
}
