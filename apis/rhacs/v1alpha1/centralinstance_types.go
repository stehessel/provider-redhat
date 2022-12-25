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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// CentralInstanceParameters are the configurable fields of a CentralInstance.
type CentralInstanceParameters struct {
	// Name of the Central instance.
	Name string `json:"name"`

	// CloudProvider to which Central is deployed.
	CloudProvider string `json:"cloudProvider"`

	// Region defines the geographical region which hosts Central.
	Region string `json:"region"`

	// MultiAZ defines if Central is deployed to a cluster with multiple availability zones.
	// +kubebuilder:default=true
	MultiAZ bool `json:"multiAZ"`
}

// CentralInstanceObservation are the observable fields of a CentralInstance.
type CentralInstanceObservation struct {
	// CentralDataURL represents Central's data URL.
	CentralDataURL string `json:"centralDataURL,omitempty"`

	// CentralUIURL represents Central's UI URL.
	CentralUIURL string `json:"centralUIURL,omitempty"`

	// CloudProvider to which Central is deployed.
	CloudProvider string `json:"cloudProvider,omitempty"`

	// CreatedAt defines the timestamp at which Central was created.
	CreatedAt metav1.Time `json:"createdAt,omitempty"`

	// HRef represents the API path of Central in the RHACS fleet manager.
	HRef string `json:"href,omitempty"`

	// ID represents a unique identifier for Central.
	ID string `json:"id,omitempty"`

	// InstanceType defines the purchasing type of Central.
	InstanceType string `json:"instanceType,omitempty"`

	// Kind defines the Central kind.
	Kind string `json:"kind,omitempty"`

	// MultiAZ defines if Central is deployed to a cluster with multiple availability zones.
	MultiAZ bool `json:"multiAZ,omitempty"`

	// Name of the Central instance.
	Name string `json:"name,omitempty"`

	// Owner of the Central instance.
	Owner string `json:"owner,omitempty"`

	// Region defines the geographical region which hosts Central.
	Region string `json:"region,omitempty"`

	// Status defines the status of Central.
	Status string `json:"status,omitempty"`

	// CreatedAt defines the timestamp at which Central was last updated.
	UpdatedAt metav1.Time `json:"updatedAt,omitempty"`

	// Version represents the Central version.
	Version string `json:"version,omitempty"`
}

// A CentralInstanceSpec defines the desired state of a CentralInstance.
type CentralInstanceSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       CentralInstanceParameters `json:"forProvider"`
}

// A CentralInstanceStatus represents the observed state of a CentralInstance.
type CentralInstanceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          CentralInstanceObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A CentralInstance is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,redhat}
type CentralInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CentralInstanceSpec   `json:"spec"`
	Status CentralInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CentralInstanceList contains a list of CentralInstance
type CentralInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CentralInstance `json:"items"`
}

// CentralInstance type metadata.
var (
	CentralInstanceKind             = reflect.TypeOf(CentralInstance{}).Name()
	CentralInstanceGroupKind        = schema.GroupKind{Group: Group, Kind: CentralInstanceKind}.String()
	CentralInstanceKindAPIVersion   = CentralInstanceKind + "." + SchemeGroupVersion.String()
	CentralInstanceGroupVersionKind = SchemeGroupVersion.WithKind(CentralInstanceKind)
)

func init() {
	SchemeBuilder.Register(&CentralInstance{}, &CentralInstanceList{})
}
