/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package centralinstance

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/stackrox/acs-fleet-manager/pkg/api/public"
	"github.com/stackrox/acs-fleet-manager/pkg/client/fleetmanager"

	"github.com/stehessel/provider-redhat/apis/rhacs/v1alpha1"
	apisv1alpha1 "github.com/stehessel/provider-redhat/apis/v1alpha1"
	"github.com/stehessel/provider-redhat/pkg/clients/rhacs"
	"github.com/stehessel/provider-redhat/pkg/controller/features"
)

const (
	errNotCentralInstance = "managed resource is not a CentralInstance custom resource"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errGetPC              = "cannot get ProviderConfig"
	errGetCreds           = "cannot get credentials"
	errGetInstance        = "cannot get central instance"
	errNewClient          = "cannot create rhacs client"
	errNewInstance        = "cannot create central instance"
)

// Setup adds a controller that reconciles CentralInstance managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.CentralInstanceGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.CentralInstanceGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.CentralInstance{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube  client.Client
	usage resource.Tracker
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return nil, errors.New(errNotCentralInstance)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cd := pc.Spec.Credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}
	stringData := string(data)

	client, err := rhacs.New(stringData, pc.Spec.Endpoint)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}
	return &external{client: client}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client fleetmanager.PublicAPI
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCentralInstance)
	}

	if cr.Status.AtProvider.ID == "" {
		return managed.ExternalObservation{}, nil
	}

	central, resp, err := c.client.GetCentralById(ctx, cr.Status.AtProvider.ID)
	fmt.Printf("\n\nObserve: %+v, central: %+v, resp: %+v, err: %+v\n\n", cr, central, resp, err)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return managed.ExternalObservation{}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetInstance)
	}

	upToDate := false
	if central.Name == cr.Spec.ForProvider.Name {
		upToDate = true
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCentralInstance)
	}

	request := public.CentralRequestPayload{
		CloudProvider: cr.Spec.ForProvider.CloudProvider,
		MultiAz:       cr.Spec.ForProvider.MultiAZ,
		Name:          cr.Spec.ForProvider.Name,
		Region:        cr.Spec.ForProvider.Region,
	}
	if _, _, err := c.client.CreateCentral(ctx, true, request); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errNewInstance)
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCentralInstance)
	}

	fmt.Printf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return errors.New(errNotCentralInstance)
	}

	fmt.Printf("Deleting: %+v", cr)

	return nil
}
