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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tokenrenewerv1beta1 "github.com/guilhem/token-renewer/api/v1beta1"
	"github.com/guilhem/token-renewer/internal/providers"
)

// TokenReconciler reconciles a Token object
type TokenReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ProvidersManager *providers.ProvidersManager
}

// +kubebuilder:rbac:groups=token-renewer.barpilot.io,resources=tokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=token-renewer.barpilot.io,resources=tokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=token-renewer.barpilot.io,resources=tokens/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *TokenReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Token")

	// Fetch the Token instance
	token := &tokenrenewerv1beta1.Token{}
	if err := r.Get(ctx, req.NamespacedName, token); err != nil {
		log.Error(err, "unable to fetch Token")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the secret reference
	secretRef := token.Spec.SecretRef
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: secretRef.Name}, secret); err != nil {
		log.Error(err, "unable to fetch Secret", "secret", secretRef.Name)
		r.Recorder.Event(token, "Warning", "SecretNotFound", "Secret not found")
		return ctrl.Result{}, fmt.Errorf("unable to fetch secret: %w", err)
	}

	tokenBytes, exists := secret.Data["token"]
	if !exists {
		log.Error(nil, "token key not found in secret", "secret", secretRef.Name, "key", "token")
		r.Recorder.Event(token, "Warning", "TokenKeyNotFound", "Secret missing 'token' key")
		return ctrl.Result{}, fmt.Errorf("token key not found in secret")
	}

	tokenValue := string(tokenBytes)
	if tokenValue == "" {
		log.Info("Token is empty, cannot use for renewal", "token", token.GetName())
		r.Recorder.Event(token, "Warning", "TokenEmpty", "Token is empty")
		return ctrl.Result{}, fmt.Errorf("token is empty")
	}

	// Get the provider for the token
	providerName := token.Spec.Provider.Name
	provider, err := r.ProvidersManager.GetProvider(providerName)
	if err != nil {
		log.Error(err, "unable to get provider", "provider", providerName)
		r.Recorder.Event(token, "Warning", "ProviderNotFound", "Provider not found")
		return ctrl.Result{}, fmt.Errorf("unable to get provider: %w", err)
	}

	if token.Status.ExpirationTime.IsZero() {
		log.Info("Token has no expiration time, setting it")

		t, err := provider.GetTokenValidity(ctx, token.Spec.Metadata, tokenValue)
		if err != nil {
			log.Error(err, "unable to get token validity", "token", token.Spec.Metadata)
			r.Recorder.Event(token, "Warning", "TokenValidityError", "Error getting token validity")
			return ctrl.Result{}, fmt.Errorf("unable to get token validity: %w", err)
		}

		if op, err := controllerutil.CreateOrPatch(ctx, r.Client, token, func() error {
			token.Status.ExpirationTime = metav1.NewTime(*t)
			return nil
		}); err != nil {
			log.Error(err, "unable to update Token", "token", token.GetName())
			r.Recorder.Event(token, "Warning", "TokenUpdateError", "Error updating token")
			return ctrl.Result{}, fmt.Errorf("unable to update token: %w", err)
		} else if op != controllerutil.OperationResultNone {
			log.Info("Token updated successfully", "operation", op)
			r.Recorder.Event(token, "Normal", "TokenUpdated", "Token updated successfully")
		}
	}

	// Check if the token is about to expire
	timeToUpdate := time.Now().Add(token.Spec.Renewval.BeforeDuration.Duration)

	if !token.Status.ExpirationTime.IsZero() && !token.Status.ExpirationTime.After(timeToUpdate) {
		log.Info("Token is about to expire, renewing", "token", token.GetName())
		newToken, newMeta, newTime, err := provider.RenewToken(ctx, token.Spec.Metadata, tokenValue)
		if err != nil {
			log.Error(err, "unable to renew token", "token", token.Spec.Metadata)
			r.Recorder.Event(token, "Warning", "TokenRenewalError", "Error renewing token")
			return ctrl.Result{}, fmt.Errorf("unable to renew token: %w", err)
		}

		log.Info("Token renewed successfully")

		// Update the secret with the new token
		if op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
			secret.StringData = make(map[string]string)
			secret.StringData["token"] = newToken
			return nil
		}); err != nil {
			r.Recorder.Event(token, "Warning", "SecretUpdateError", "Error updating secret")
			return ctrl.Result{}, fmt.Errorf("unable to update secret: %w", err)
		} else if op != controllerutil.OperationResultNone {
			r.Recorder.Event(token, "Normal", "SecretUpdated", "Secret updated successfully")
		}

		// Update the token with the new metadata and expiration time
		if op, err := controllerutil.CreateOrPatch(ctx, r.Client, token, func() error {
			token.Spec.Metadata = newMeta
			token.Status.ExpirationTime = metav1.NewTime(*newTime)
			return nil
		}); err != nil {
			r.Recorder.Event(token, "Warning", "TokenUpdateError", "Error updating token")
			return ctrl.Result{}, fmt.Errorf("unable to update token: %w", err)
		} else if op != controllerutil.OperationResultNone {
			log.Info("Token updated successfully", "operation", op)
			r.Recorder.Event(token, "Normal", "TokenUpdated", "Token updated successfully")
		}
	}

	return ctrl.Result{
		RequeueAfter: time.Until(token.Status.ExpirationTime.Add(-token.Spec.Renewval.BeforeDuration.Duration)),
	}, nil
}

// SetupWithManager sets up the controller with the Manager using a custom rate limiter.
func (r *TokenReconciler) SetupWithManager(mgr ctrl.Manager, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tokenrenewerv1beta1.Token{}).
		WithOptions(controller.Options{
			RateLimiter: rateLimiter,
		}).
		Named("token").
		Complete(r)
}
