/*
Copyright 2022.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceRecordSpec defines the desired state of ResourceRecord
type ResourceRecordSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Enum=A;NS;AAAA;MX;CNAME;SRV;TXT
	Class string `json:"class"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2147483647
	Ttl int32 `json:"ttl"`

	// +optional
	// +nullable
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=255
	Weight *int64 `json:"weight,omitempty"`

	OwnerRef string `json:"ownerRef"`

	ProviderRef string `json:"providerRef"`

	// +optional
	Rdata string `json:"rdata,omitempty"`

	// +optional
	// +kubebuilder:default=false
	IsAlias bool `json:"isAlias,omitempty"`

	// +optional
	AliasTarget AliasTarget `json:"aliasTarget,omitempty"`

	// +optional
	// +nullable
	Id *string `json:"id,omitempty"`
}

type AliasTarget struct {
	Record string `json:"record"`

	// Only Route53
	EvaluateTargetHealth bool `json:"evaluateTargetHealth"`

	// +optional
	HostedZoneID string `json:"hostedZoneID,omitempty"`
}

// ResourceRecordStatus defines the observed state of ResourceRecord
type ResourceRecordStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ResourceRecord is the Schema for the resourcerecords API
type ResourceRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceRecordSpec   `json:"spec,omitempty"`
	Status ResourceRecordStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourceRecordList contains a list of ResourceRecord
type ResourceRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceRecord{}, &ResourceRecordList{})
}
