package datainfo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cyverse-de/go-mod/restutils"
	"github.com/cyverse-de/group-propagator/logging"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var log = logging.Log.WithFields(logrus.Fields{"package": "client.datainfo"})

type DataInfoClient struct {
	DataInfoBase string
	DataInfoUser string
}

var httpClient = http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func NewDataInfoClient(base, user string) *DataInfoClient {
	return &DataInfoClient{base, user}
}

func (d *DataInfoClient) uriPath(ctx context.Context, pathParts ...string) (string, error) {
	base, err := url.Parse(d.DataInfoBase)
	if err != nil {
		return "", errors.Wrap(err, "Failed to parse data-info base URL")
	}

	uri := base.JoinPath(pathParts...)
	uri.RawQuery = fmt.Sprintf("user=%s", d.DataInfoUser)

	return uri.String(), nil
}

func (d *DataInfoClient) reqJSON(ctx context.Context, method, uri string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, uri, body)
	if err != nil {
		return errors.Wrap(err, "Failed creating request with context")
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
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

// Create IRODS Group
func (d *DataInfoClient) CreateGroup(ctx context.Context, name string, members []string) (Group, error) {
	var g Group

	uri, err := d.uriPath(ctx, "groups")
	if err != nil {
		return g, errors.Wrap(err, "Failed to build URL")
	}

	postGroup := Group{Name: name, Members: members}
	msg, err := json.Marshal(postGroup)
	if err != nil {
		return g, errors.Wrap(err, "Failed to marshal group to create")
	}

	err = d.reqJSON(ctx, http.MethodPost, uri, bytes.NewBuffer(msg), &g)
	return g, err
}

// List Group Members
func (d *DataInfoClient) ListGroupMembers(ctx context.Context, name string) (Group, error) {
	var g Group

	uri, err := d.uriPath(ctx, "groups", name)
	if err != nil {
		return g, errors.Wrap(err, "Failed to build URL")
	}

	err = d.reqJSON(ctx, http.MethodGet, uri, nil, &g)
	return g, err
}

// Update Group Members
func (d *DataInfoClient) UpdateGroupMembers(ctx context.Context, name string, members []string) (Group, error) {
	var g Group

	uri, err := d.uriPath(ctx, "groups", name)
	if err != nil {
		return g, errors.Wrap(err, "Failed to build URL")
	}

	postGroup := Group{Name: name, Members: members}
	msg, err := json.Marshal(postGroup)
	if err != nil {
		return g, errors.Wrap(err, "Failed to marshal group to create")
	}

	err = d.reqJSON(ctx, http.MethodPut, uri, bytes.NewBuffer(msg), &g)
	return g, err
}

// Delete Group
func (d *DataInfoClient) DeleteGroup(ctx context.Context, name string) error {
	uri, err := d.uriPath(ctx, "groups", name)
	if err != nil {
		return errors.Wrap(err, "Failed to build URL")
	}

	return d.reqJSON(ctx, http.MethodDelete, uri, nil, nil)
}
