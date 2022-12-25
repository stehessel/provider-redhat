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
	ConfigurableField string `json:"configurableField"`
}

// CentralInstanceObservation are the observable fields of a CentralInstance.
type CentralInstanceObservation struct {
	ObservableField string `json:"observableField,omitempty"`
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
