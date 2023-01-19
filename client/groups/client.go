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
)

var log = logging.Log.WithFields(logrus.Fields{"package": "client.groups"})

type GroupsClient struct {
	GroupsBase string
	GroupsUser string
}

var httpClient = http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func NewGroupsClient(base, user string) *GroupsClient {
	return &GroupsClient{base, user}
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
func (c *GroupsClient) ListGroupsByPrefix(ctx context.Context, prefix string) (GroupList, error) {
	var gs GroupList

	uri, err := c.uriPath(ctx, fmt.Sprintf("search=%s", prefix), "groups")
	if err != nil {
		return gs, err
	}

	err = c.getJSON(ctx, uri, &gs)
	return gs, err
}

// Get the basic group information for a group from the REST service, given a name
func (c *GroupsClient) GetGroupByName(ctx context.Context, groupName string) (Group, error) {
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
	var gm GroupMembers
	uri, err := c.uriPath(ctx, "", "groups", groupName, "members")
	if err != nil {
		return gm, err
	}

	err = c.getJSON(ctx, uri, &gm)
	return gm, err
}
