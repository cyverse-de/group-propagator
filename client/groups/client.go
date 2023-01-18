package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cyverse-de/group-propagator/client"
	"github.com/cyverse-de/group-propagator/logging"

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

// for crawling:
// list groups under prefix
// list group members

// for individual:
// get group by name
// list group members

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

// List groups under a provided prefix, using the REST service
func (c *GroupsClient) ListGroupsByPrefix(ctx context.Context, prefix string) (GroupsList, error) {
	var gs GroupsList

	uri, err := c.uriPath(ctx, fmt.Sprintf("search=%s", prefix), "groups")
	if err != nil {
		return gs, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return gs, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return gs, err
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return gs, client.NewHTTPError(resp.StatusCode, fmt.Sprintf("%s returned %d", uri, resp.StatusCode))
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&gs)
	return gs, nil
}

// Get the basic group information for a group from the REST service, given a name
func (c *GroupsClient) GetGroupByName(ctx context.Context, groupName string) (Group, error) {
	var g Group

	uri, err := c.uriPath(ctx, "", "groups", groupName)
	if err != nil {
		return g, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return g, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return g, err
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return g, client.NewHTTPError(resp.StatusCode, fmt.Sprintf("%s returned %d", uri, resp.StatusCode))
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&g)
	return g, err // if err != nil this still behaves right
}

// List members of a group using the REST service, given a name
func (c *GroupsClient) GetGroupMembers(ctx context.Context, groupName string) (GroupMembers, error) {
	var gm GroupMembers
	return gm, nil
}
