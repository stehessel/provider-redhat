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
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stackrox/acs-fleet-manager/pkg/api/public"
	"github.com/stackrox/acs-fleet-manager/pkg/client/fleetmanager"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/stehessel/provider-redhat/apis/rhacs/v1alpha1"
	"github.com/stehessel/provider-redhat/pkg/clients/rhacs"
)

// Test that our Reconciler implementation satisfies the Reconciler interface.
var (
	_ managed.ExternalClient    = &external{}
	_ managed.ExternalConnecter = &connector{}
)

var (
	cloudProvider = v1alpha1.CloudProvider("aws")
	multiAZ       = true
	name          = "test-central"
	region        = v1alpha1.Region("us-east-1")
	id            = "test-id"
)

type (
	centralRequestModifier  func(*public.CentralRequest)
	centralInstanceModifier func(*v1alpha1.CentralInstance)
)

func withRequestName(name string) centralRequestModifier {
	return func(c *public.CentralRequest) { c.Name = name }
}

func withRequestStatus(status string) centralRequestModifier {
	return func(c *public.CentralRequest) { c.Status = status }
}

func withConditions(c ...xpv1.Condition) centralInstanceModifier {
	return func(r *v1alpha1.CentralInstance) { r.Status.ConditionedStatus.Conditions = c }
}

func withName(name string) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Status.AtProvider.Name = name }
}

func withStatus(status string) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Status.AtProvider.Status = status }
}

func withExternalName(name string) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.ObjectMeta.Annotations["crossplane.io/external-name"] = name }
}

func centralRequest(mod ...centralRequestModifier) public.CentralRequest {
	c := public.CentralRequest{
		Id:            id,
		CloudProvider: string(cloudProvider),
		MultiAz:       multiAZ,
		Name:          name,
		Region:        string(region),
		Status:        rhacs.CentralRequestStatusReady,
	}
	for _, m := range mod {
		m(&c)
	}
	return c
}

func centralInstance(mod ...centralInstanceModifier) *v1alpha1.CentralInstance {
	c := &v1alpha1.CentralInstance{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.CentralInstanceSpec{
			ForProvider: v1alpha1.CentralInstanceParameters{
				CloudProvider: cloudProvider,
				MultiAZ:       multiAZ,
				Name:          name,
				Region:        region,
			},
		},
		Status: v1alpha1.CentralInstanceStatus{
			AtProvider: v1alpha1.CentralInstanceObservation{
				CloudProvider: cloudProvider,
				ID:            id,
				MultiAZ:       multiAZ,
				Name:          name,
				Region:        region,
				Status:        rhacs.CentralRequestStatusReady,
			},
		},
	}
	meta.SetExternalName(c, c.Name)
	for _, m := range mod {
		m(c)
	}
	return c
}

func makeHTTPResponse(statusCode int) *http.Response {
	response := &http.Response{
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		Header:     http.Header{},
		StatusCode: statusCode,
	}
	return response
}

func TestObserve(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		obs managed.ExternalObservation
		mg  resource.Managed
		err error
	}

	cases := []struct {
		name   string
		client fleetmanager.PublicAPI
		args   args
		want   want
	}{
		{
			name: "observation no diff",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest(), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Available())),
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				mg:  centralInstance(withConditions(xpv1.Available())),
				err: nil,
			},
		},
		{
			name: "observation diff",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest(withRequestName("new-name")), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Available())),
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				mg:  centralInstance(withName("new-name"), withConditions(xpv1.Available())),
				err: nil,
			},
		},
		{
			name: "observation while creating",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest(withRequestStatus(rhacs.CentralRequestStatusAccepted)), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Creating()), withStatus(rhacs.CentralRequestStatusAccepted)),
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				mg:  centralInstance(withConditions(xpv1.Creating()), withStatus(rhacs.CentralRequestStatusAccepted)),
				err: nil,
			},
		},
		{
			name: "observation while available",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest(withRequestStatus(rhacs.CentralRequestStatusReady)), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Creating()), withStatus(rhacs.CentralRequestStatusAccepted)),
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				mg:  centralInstance(withConditions(xpv1.Available())),
				err: nil,
			},
		},
		{
			name: "observation while deleting",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest(withRequestStatus(rhacs.CentralRequestStatusDeleting)), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Creating()), withStatus(rhacs.CentralRequestStatusAccepted)),
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				mg:  centralInstance(withConditions(xpv1.Deleting()), withStatus(rhacs.CentralRequestStatusDeleting)),
				err: nil,
			},
		},
		{
			name: "observation no central found",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return public.CentralRequest{}, makeHTTPResponse(http.StatusNotFound), errors.New(errGetFailed)
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				obs: managed.ExternalObservation{},
				mg:  centralInstance(),
				err: nil,
			},
		},
		{
			name: "observation error during get",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return public.CentralRequest{}, nil, errors.New(errGetFailed)
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				obs: managed.ExternalObservation{},
				mg:  centralInstance(),
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{client: tc.client}
			got, err := e.Observe(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got,
				cmpopts.IgnoreFields(managed.ExternalObservation{}, "Diff")); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		obs managed.ExternalCreation
		mg  resource.Managed
		err error
	}

	cases := []struct {
		name   string
		client fleetmanager.PublicAPI
		args   args
		want   want
	}{
		{
			name: "creation success",
			client: &fleetmanager.PublicAPIMock{
				CreateCentralFunc: func(ctx context.Context, async bool, request public.CentralRequestPayload) (public.CentralRequest, *http.Response, error) {
					return centralRequest(), nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				obs: managed.ExternalCreation{},
				mg:  centralInstance(withConditions(xpv1.Creating()), withExternalName(id)),
				err: nil,
			},
		},
		{
			name: "creation error",
			client: &fleetmanager.PublicAPIMock{
				CreateCentralFunc: func(ctx context.Context, async bool, request public.CentralRequestPayload) (public.CentralRequest, *http.Response, error) {
					return centralRequest(), nil, errors.New(errCreateFailed)
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				obs: managed.ExternalCreation{},
				mg:  centralInstance(withConditions(xpv1.Creating())),
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{client: tc.client}
			got, err := e.Create(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\ne.Create(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Create(...): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\ne.Create(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		obs managed.ExternalUpdate
		mg  resource.Managed
		err error
	}

	cases := []struct {
		name   string
		client fleetmanager.PublicAPI
		args   args
		want   want
	}{
		{
			name: "update success",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withStatus(rhacs.CentralRequestStatusReady)),
			},
			want: want{
				mg:  centralInstance(withStatus(rhacs.CentralRequestStatusReady), withConditions(xpv1.Deleting())),
				err: nil,
			},
		},
		{
			name: "update not ready",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, errors.New("should never reach this error")
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withStatus(rhacs.CentralRequestStatusDeprovision)),
			},
			want: want{
				mg:  centralInstance(withConditions(xpv1.Deleting()), withStatus(rhacs.CentralRequestStatusDeprovision)),
				err: nil,
			},
		},
		{
			name: "update error",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, errors.New(errUpdateFailed)
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withStatus(rhacs.CentralRequestStatusReady)),
			},
			want: want{
				mg:  centralInstance(withStatus(rhacs.CentralRequestStatusReady), withConditions(xpv1.Deleting())),
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{client: tc.client}
			got, err := e.Update(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\ne.Update(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Update(...): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\ne.Update(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		mg  resource.Managed
		err error
	}

	cases := []struct {
		name   string
		client fleetmanager.PublicAPI
		args   args
		want   want
	}{
		{
			name: "delete success",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, nil
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				mg:  centralInstance(withConditions(xpv1.Deleting())),
				err: nil,
			},
		},
		{
			name: "delete already in progress",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, errors.New("should never reach this error")
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(withConditions(xpv1.Deleting()), withStatus(rhacs.CentralRequestStatusDeprovision)),
			},
			want: want{
				mg:  centralInstance(withConditions(xpv1.Deleting()), withStatus(rhacs.CentralRequestStatusDeprovision)),
				err: nil,
			},
		},
		{
			name: "delete error",
			client: &fleetmanager.PublicAPIMock{
				DeleteCentralByIdFunc: func(ctx context.Context, id string, async bool) (*http.Response, error) {
					return nil, errors.New(errDeleteFailed)
				},
			},
			args: args{
				ctx: context.Background(),
				mg:  centralInstance(),
			},
			want: want{
				mg:  centralInstance(withConditions(xpv1.Deleting())),
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{client: tc.client}
			err := e.Delete(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\ne.Delete(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\ne.Delete(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}
