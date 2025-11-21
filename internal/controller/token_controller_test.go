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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tokenrenewerv1beta1 "github.com/guilhem/token-renewer/api/v1beta1"
	"github.com/guilhem/token-renewer/internal/providers"
)

// mockProvider implements the TokenProvider interface for testing
type mockProvider struct{}

func (m *mockProvider) RenewToken(ctx context.Context, metadata, token string) (newToken string, newMetadata string, expiration *time.Time, err error) {
	// Return a new token with a far future expiration
	exp := time.Now().Add(24 * time.Hour)
	return "new-test-token", metadata, &exp, nil
}

func (m *mockProvider) GetTokenValidity(ctx context.Context, metadata, token string) (expiration *time.Time, err error) {
	// Return a far future expiration time
	exp := time.Now().Add(24 * time.Hour)
	return &exp, nil
}

var _ = Describe("Token Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		token := &tokenrenewerv1beta1.Token{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Token")
			err := k8sClient.Get(ctx, typeNamespacedName, token)
			if err != nil && errors.IsNotFound(err) {
				// Create the secret first
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"token": []byte("test-token-value"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				resource := &tokenrenewerv1beta1.Token{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: tokenrenewerv1beta1.TokenSpec{
						Provider: tokenrenewerv1beta1.ProviderSpec{
							Name: "test-provider",
						},
						Metadata: "test-metadata",
						Renewval: tokenrenewerv1beta1.RenewvalSpec{},
						SecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &tokenrenewerv1beta1.Token{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Token")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			providersManager := providers.NewProvidersManager()

			// Register a mock provider
			mockProv := &mockProvider{}
			providersManager.RegisterPlugin("test-provider", mockProv)

			// Create a fake event recorder
			broadcaster := record.NewBroadcaster()
			fakeRecorder := broadcaster.NewRecorder(k8sClient.Scheme(), corev1.EventSource{Component: "token-controller"})

			controllerReconciler := &TokenReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				ProvidersManager: providersManager,
				Recorder:         fakeRecorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
