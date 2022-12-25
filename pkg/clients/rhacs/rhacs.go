package rhacs

import (
	"github.com/pkg/errors"
	"github.com/stackrox/acs-fleet-manager/pkg/client/fleetmanager"
)

// New creates a new fleet manager client.
func New(token string, endpoint string) (fleetmanager.PublicAPI, error) {
	auth, err := fleetmanager.NewOCMAuth(fleetmanager.OCMOption{RefreshToken: token})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fleet manager authentication")
	}

	client, err := fleetmanager.NewClient(endpoint, auth, fleetmanager.WithUserAgent("crossplane"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fleet manager client")
	}

	return client.PublicAPI(), nil
}
