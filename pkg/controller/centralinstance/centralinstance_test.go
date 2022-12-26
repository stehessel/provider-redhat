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

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/stehessel/provider-redhat/apis/rhacs/v1alpha1"
)

// Test that our Reconciler implementation satisfies the Reconciler interface.
var (
	_ managed.ExternalClient    = &external{}
	_ managed.ExternalConnecter = &connector{}
)

var (
	cloudProvider  = v1alpha1.CloudProvider("aws")
	multiAZ        = true
	name           = "test-central"
	region         = v1alpha1.Region("us-east-1")
	centralRequest = public.CentralRequest{
		Id:            "test-id",
		CloudProvider: string(cloudProvider),
		MultiAz:       multiAZ,
		Name:          name,
		Region:        string(region),
	}
)

type centralInstanceModifier func(*v1alpha1.CentralInstance)

func withCloudAccountID(id string) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Spec.ForProvider.CloudAccountID = id }
}

func withCloudProvider(provider v1alpha1.CloudProvider) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Spec.ForProvider.CloudProvider = provider }
}

func withName(name string) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Spec.ForProvider.Name = name }
}

func withMultiAZ(multi bool) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Spec.ForProvider.MultiAZ = multi }
}

func withRegion(region v1alpha1.Region) centralInstanceModifier {
	return func(c *v1alpha1.CentralInstance) { c.Spec.ForProvider.Region = region }
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
		o   managed.ExternalObservation
		err error
	}

	cases := []struct {
		name   string
		client fleetmanager.PublicAPI
		args   args
		want   want
	}{
		{
			name: "Observation no diff",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return centralRequest, nil, nil
				},
			},
			args: args{ctx: context.Background(), mg: centralInstance()},
			want: want{o: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			}, err: nil},
		},
		{
			name: "Observation diff",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					c := centralRequest
					c.Name = "new-name"
					return c, nil, nil
				},
			},
			args: args{ctx: context.Background(), mg: centralInstance()},
			want: want{o: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: false,
			}, err: nil},
		},
		{
			name: "Observation no Central found",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return public.CentralRequest{}, makeHTTPResponse(http.StatusNotFound), errors.New(errGetFailed)
				},
			},
			args: args{ctx: context.Background(), mg: centralInstance()},
			want: want{o: managed.ExternalObservation{}, err: nil},
		},
		{
			name: "Observation error",
			client: &fleetmanager.PublicAPIMock{
				GetCentralByIdFunc: func(ctx context.Context, id string) (public.CentralRequest, *http.Response, error) {
					return public.CentralRequest{}, nil, errors.New(errGetFailed)
				},
			},
			args: args{ctx: context.Background(), mg: centralInstance()},
			want: want{o: managed.ExternalObservation{}, err: cmpopts.AnyError},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{client: tc.client}
			got, err := e.Observe(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got,
				cmpopts.IgnoreFields(managed.ExternalObservation{}, "Diff")); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}
