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

func (c *GroupsClient) getDEUsersGroup(ctx context.Context) (*Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "getDEUsersGroup")
	defer span.End()

	uri, err := c.uriPath(ctx, fmt.Sprintf("group_type=system&name=%s", url.QueryEscape(c.DEUsersGroupName)),
		"groups", "lookup")
	if err != nil {
		return nil, errors.Wrap(err, "Failed to build the de-users lookup URL")
	}

	var g Group
	if err := c.getJSON(ctx, uri, &g); err != nil {
		return nil, errors.Wrap(err, "Failed to get group ID")
	}
	return &g, nil
}

func (c *GroupsClient) SetGroupsID(ctx context.Context) error {
	g, err := c.getDEUsersGroup(ctx)
	if err != nil {
		return err
	}
	c.GroupsID = g.ID
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
	defer func() { _ = resp.Body.Close() }()

	if target != nil {
		err = json.NewDecoder(resp.Body).Decode(target)
		if err != nil {
			return errors.Wrap(err, "Failed decoding JSON")
		}
	}
	return err
}

// Check reports whether the groups service is reachable and able to serve group
// data. The service answers its status endpoint even when its database is down,
// so a 200 alone is not enough to start against.
func (c *GroupsClient) Check(ctx context.Context) error {
	uri, err := url.Parse(c.GroupsBase)
	if err != nil {
		return errors.Wrap(err, "Failed to parse groups base URL")
	}

	var status Status
	if err := c.getJSON(ctx, uri.String(), &status); err != nil {
		return err
	}
	if !status.Database {
		return errors.New("the groups service cannot reach its database")
	}
	return nil
}

// pageSize is how many groups one listing request asks for. The service caps a
// single response at 1000, so anything larger would be silently trimmed.
const pageSize = 1000

// ListAllGroups returns every group the service knows about, following
// pagination to the end. The groups service is already scoped to one
// deployment's group data, so there is no folder or prefix to filter on.
func (c *GroupsClient) ListAllGroups(ctx context.Context) ([]Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "ListAllGroups")
	defer span.End()

	// The offset advances by what each page actually held, and only an empty
	// page ends the crawl. Keying either off the requested pageSize would
	// silently truncate the listing if the service ever clamps responses
	// below what we asked for.
	var all []Group
	for offset := 0; ; offset = len(all) {
		uri, err := c.uriPath(ctx, fmt.Sprintf("limit=%d&offset=%d", pageSize, offset), "groups")
		if err != nil {
			return all, err
		}
		log.Debugf("ListAllGroups uri: %s", uri)

		var page GroupList
		if err := c.getJSON(ctx, uri, &page); err != nil {
			return all, err
		}
		if len(page.Groups) == 0 {
			return all, nil
		}
		all = append(all, page.Groups...)
	}
}

// Get the basic group information for a group from the REST service, given an ID
func (c *GroupsClient) GetGroupByID(ctx context.Context, groupID string) (Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupByID")
	defer span.End()

	var g Group

	uri, err := c.uriPath(ctx, "", "groups", url.PathEscape(groupID))
	if err != nil {
		return g, err
	}

	err = c.getJSON(ctx, uri, &g)
	return g, err
}

// List members of a group using the REST service, given an ID. Membership is
// keyed by ID rather than name because a nested group's member entry carries
// its own short name, which is not a lookup key on either backend.
func (c *GroupsClient) GetGroupMembersByID(ctx context.Context, groupID string) (GroupMembers, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupMembersByID")
	defer span.End()

	var gm GroupMembers
	uri, err := c.uriPath(ctx, "", "groups", url.PathEscape(groupID), "members")
	if err != nil {
		return gm, err
	}

	err = c.getJSON(ctx, uri, &gm)
	return gm, err
}
