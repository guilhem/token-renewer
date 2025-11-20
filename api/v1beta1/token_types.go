/*
Copyright 2025.

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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TokenSpec defines the desired state of Token.
type TokenSpec struct {
	// +kubebuilder:validation:Required
	Provider ProviderSpec `json:"provider"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Metadata string `json:"metadata"`
	// +kubebuilder:validation:Required
	Renewval RenewvalSpec `json:"renewval,omitempty"`
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// ProviderSpec defines the desired state of the provider.
type ProviderSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// RenewvalSpec defines the desired state of the renewval.
type RenewvalSpec struct {
	BeforeDuration metav1.Duration `json:"beforeDuration,omitempty"`
}

// TokenStatus defines the observed state of Token.
type TokenStatus struct {
	ExpirationTime metav1.Time `json:"expirationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Token is the Schema for the tokens API.
type Token struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TokenSpec   `json:"spec,omitempty"`
	Status TokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TokenList contains a list of Token.
type TokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Token `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Token{}, &TokenList{})
}
