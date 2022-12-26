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
	"net/http"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stackrox/acs-fleet-manager/pkg/api/public"
	"github.com/stackrox/acs-fleet-manager/pkg/client/fleetmanager"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	errNewClient          = "cannot create rhacs client"
	errGetFailed          = "cannot get central instance"
	errCreateFailed       = "cannot create central instance"
	errUpdateFailed       = "cannot update central instance"
	errDeleteFailed       = "cannot delete central instance"
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

func generateObservation(in *public.CentralRequest) v1alpha1.CentralInstanceObservation {
	return v1alpha1.CentralInstanceObservation{
		CentralDataURL: in.CentralDataURL,
		CentralUIURL:   in.CentralUIURL,
		CloudAccountID: in.CloudAccountId,
		CloudProvider:  v1alpha1.CloudProvider(in.CloudProvider),
		CreatedAt:      metav1.NewTime(in.CreatedAt),
		FailedReason:   in.FailedReason,
		HRef:           in.Href,
		ID:             in.Id,
		InstanceType:   in.InstanceType,
		Kind:           in.Kind,
		MultiAZ:        in.MultiAz,
		Name:           in.Name,
		Owner:          in.Owner,
		Region:         v1alpha1.Region(in.Region),
		Status:         in.Status,
		UpdatedAt:      metav1.NewTime(in.UpdatedAt),
		Version:        in.Version,
	}
}

func getCondition(status string) xpv1.Condition {
	switch status {
	case rhacs.CentralRequestStatusAccepted,
		rhacs.CentralRequestStatusPreparing,
		rhacs.CentralRequestStatusProvisioning:
		return xpv1.Creating()
	case rhacs.CentralRequestStatusReady:
		return xpv1.Available()
	case rhacs.CentralRequestStatusDeprovision,
		rhacs.CentralRequestStatusDeleting:
		return xpv1.Deleting()
	default:
		return xpv1.Unavailable()
	}
}

func isUpToDate(in *v1alpha1.CentralInstance, observed *public.CentralRequest) (bool, string) {
	observedParams := v1alpha1.CentralInstanceParameters{
		Name:          observed.Name,
		CloudProvider: v1alpha1.CloudProvider(observed.CloudProvider),
		Region:        v1alpha1.Region(observed.Region),
		MultiAZ:       observed.MultiAz,
	}
	if diff := cmp.Diff(in.Spec.ForProvider, observedParams, cmpopts.EquateEmpty()); diff != "" {
		diff = "Observed difference in central instance\n" + diff
		return false, diff
	}
	return true, ""
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCentralInstance)
	}

	centralResp, resp, err := c.client.GetCentralById(ctx, meta.GetExternalName(cr))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errGetFailed)
	}

	cr.Status.AtProvider = generateObservation(&centralResp)
	condition := getCondition(cr.Status.AtProvider.Status)
	cr.SetConditions(condition)
	upToDate, diff := isUpToDate(cr, &centralResp)
	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
		Diff:             diff,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCentralInstance)
	}

	cr.SetConditions(xpv1.Creating())

	request := public.CentralRequestPayload{
		CloudAccountId: cr.Spec.ForProvider.CloudAccountID,
		CloudProvider:  string(cr.Spec.ForProvider.CloudProvider),
		MultiAz:        cr.Spec.ForProvider.MultiAZ,
		Name:           cr.Spec.ForProvider.Name,
		Region:         string(cr.Spec.ForProvider.Region),
	}
	centralResp, _, err := c.client.CreateCentral(ctx, true, request)
	if err == nil {
		meta.SetExternalName(cr, centralResp.Id)
	}
	return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCentralInstance)
	}
	if cr.GetCondition(xpv1.TypeReady) == xpv1.Creating() ||
		cr.GetCondition(xpv1.TypeReady) == xpv1.Deleting() {
		return managed.ExternalUpdate{}, nil
	}

	err := c.Delete(ctx, mg)
	return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateFailed)
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.CentralInstance)
	if !ok {
		return errors.New(errNotCentralInstance)
	}

	_, err := c.client.DeleteCentralById(ctx, cr.Status.AtProvider.ID, true)
	return errors.Wrap(err, errDeleteFailed)
}
