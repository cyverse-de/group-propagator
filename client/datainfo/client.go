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
	"go.opentelemetry.io/otel"
)

var log = logging.Log.WithFields(logrus.Fields{"package": "client.datainfo"})

const otelName = "github.com/cyverse-de/group-propagator/client/groups"

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
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e ServiceError
		sc := resp.StatusCode
		err = json.NewDecoder(resp.Body).Decode(&e)
		if err != nil {
			log.Error(errors.Wrap(err, "Failed decoding error response"))
		}
		// This is a little hacky but data-info returns a 500, apparently
		if e.ErrorCode == "ERR_DOES_NOT_EXIST" {
			sc = 404
		}
		return restutils.NewHTTPError(sc, fmt.Sprintf("GET %s returned %d", uri, sc))
	}

	if target != nil {
		err = json.NewDecoder(resp.Body).Decode(target)
		if err != nil {
			return errors.Wrap(err, "Failed decoding JSON")
		}
	}
	return err

}

// Use status endpoint to check our data-info URI
func (d *DataInfoClient) Check(ctx context.Context) error {
	uri, err := url.Parse(d.DataInfoBase)
	if err != nil {
		return errors.Wrap(err, "Failed to parse data-info base URL")
	}

	uri.RawQuery = "expecting=data-info"

	return d.reqJSON(ctx, http.MethodGet, uri.String(), nil, nil)
}

// Create IRODS Group
func (d *DataInfoClient) CreateGroup(ctx context.Context, name string, members []string) (Group, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "CreateGroup")
	defer span.End()

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
	ctx, span := otel.Tracer(otelName).Start(ctx, "ListGroupMembers")
	defer span.End()

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
	ctx, span := otel.Tracer(otelName).Start(ctx, "UpdateGroupMembers")
	defer span.End()

	var g Group

	uri, err := d.uriPath(ctx, "groups", name)
	if err != nil {
		return g, errors.Wrap(err, "Failed to build URL")
	}

	postGroup := Group{Members: members}
	msg, err := json.Marshal(postGroup)
	if err != nil {
		return g, errors.Wrap(err, "Failed to marshal group to create")
	}

	err = d.reqJSON(ctx, http.MethodPut, uri, bytes.NewBuffer(msg), &g)
	return g, err
}

// Delete Group
func (d *DataInfoClient) DeleteGroup(ctx context.Context, name string) error {
	ctx, span := otel.Tracer(otelName).Start(ctx, "DeleteGroup")
	defer span.End()

	uri, err := d.uriPath(ctx, "groups", name)
	if err != nil {
		return errors.Wrap(err, "Failed to build URL")
	}

	return d.reqJSON(ctx, http.MethodDelete, uri, nil, nil)
}
