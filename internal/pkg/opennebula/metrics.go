// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package opennebula

import (
	"context"
	"time"
)

// MetricsRecorder is the minimal metrics surface needed by the OpenNebula adapter wrapper.
type MetricsRecorder interface {
	ObserveOpenNebulaRequest(operation, class string, duration time.Duration)
}

type instrumentedClient struct {
	inner   Client
	metrics MetricsRecorder
}

// Instrument wraps a client with request latency and classification metrics.
func Instrument(client Client, metrics MetricsRecorder) Client {
	if client == nil || metrics == nil {
		return client
	}

	return &instrumentedClient{
		inner:   client,
		metrics: metrics,
	}
}

func (c *instrumentedClient) LookupTemplateByName(ctx context.Context, name string) (TemplateRef, error) {
	return observe(c, "lookup_template", func() (TemplateRef, error) {
		return c.inner.LookupTemplateByName(ctx, name)
	})
}

func (c *instrumentedClient) LookupImageByName(ctx context.Context, name string) (ImageRef, error) {
	return observe(c, "lookup_image", func() (ImageRef, error) {
		return c.inner.LookupImageByName(ctx, name)
	})
}

func (c *instrumentedClient) LookupDatastoreByName(ctx context.Context, name string) (DatastoreRef, error) {
	return observe(c, "lookup_datastore", func() (DatastoreRef, error) {
		return c.inner.LookupDatastoreByName(ctx, name)
	})
}

func (c *instrumentedClient) LookupNetworksByName(ctx context.Context, names []string) ([]NetworkRef, error) {
	return observe(c, "lookup_networks", func() ([]NetworkRef, error) {
		return c.inner.LookupNetworksByName(ctx, names)
	})
}

func (c *instrumentedClient) CreateImage(ctx context.Context, request CreateImageRequest) (ImageRef, error) {
	return observe(c, "create_image", func() (ImageRef, error) {
		return c.inner.CreateImage(ctx, request)
	})
}

func (c *instrumentedClient) GetImage(ctx context.Context, imageID int) (ImageInfo, error) {
	return observe(c, "get_image", func() (ImageInfo, error) {
		return c.inner.GetImage(ctx, imageID)
	})
}

func (c *instrumentedClient) DeleteImage(ctx context.Context, imageID int) error {
	start := time.Now()
	err := c.inner.DeleteImage(ctx, imageID)
	c.metrics.ObserveOpenNebulaRequest("delete_image", string(ClassifyError(err)), time.Since(start))
	return err
}

func (c *instrumentedClient) InstantiateTemplate(ctx context.Context, request InstantiateRequest) (VMRef, error) {
	return observe(c, "instantiate_template", func() (VMRef, error) {
		return c.inner.InstantiateTemplate(ctx, request)
	})
}

func (c *instrumentedClient) GetVM(ctx context.Context, vmID int) (VMInfo, error) {
	return observe(c, "get_vm", func() (VMInfo, error) {
		return c.inner.GetVM(ctx, vmID)
	})
}

func (c *instrumentedClient) TerminateVM(ctx context.Context, vmID int, hard bool) error {
	start := time.Now()
	err := c.inner.TerminateVM(ctx, vmID, hard)
	c.metrics.ObserveOpenNebulaRequest("terminate_vm", string(ClassifyError(err)), time.Since(start))
	return err
}

func observe[T any](client *instrumentedClient, operation string, call func() (T, error)) (T, error) {
	start := time.Now()
	value, err := call()
	client.metrics.ObserveOpenNebulaRequest(operation, string(ClassifyError(err)), time.Since(start))
	return value, err
}
