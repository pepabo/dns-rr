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

package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/ch1aki/dns-rr/api/v1alpha1"
	"github.com/ch1aki/dns-rr/controllers/provider"
)

// ResourceRecordReconciler reconciles a ResourceRecord object
type ResourceRecordReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=resourcerecords,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=resourcerecords/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=resourcerecords/finalizers,verbs=update
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=owners,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=owners/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=owners/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func (r *ResourceRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var rr dnsv1alpha1.ResourceRecord
	err := r.Get(ctx, req.NamespacedName, &rr)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "unable to get ResourceRecord", "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !rr.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// main logic

	// get owner object
	var owner dnsv1alpha1.Owner
	err = r.Get(ctx, client.ObjectKey{Namespace: rr.Namespace, Name: rr.Spec.OwnerRef}, &owner)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "unable to get ResourceRecord", "name", rr.Namespace+"/"+rr.Spec.OwnerRef)
		return ctrl.Result{}, err
	}

	// get provider object
	var p dnsv1alpha1.Provider
	err = r.Get(ctx, client.ObjectKey{Namespace: rr.Namespace, Name: rr.Spec.ProviderRef}, &p)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "unable to get ResourceRecord", "name", rr.Namespace+"/"+rr.Spec.ProviderRef)
		return ctrl.Result{}, err
	}

	// setup client
	var route53 provider.Route53Provider
	client, err := route53.NewClient(ctx, &p, r.Client)
	if err != nil {
		logger.Error(err, "failed initialize client")
	}

	// converge
	_ = client.Converge(ctx, p.Spec.Route53.HostedZoneID, p.Spec.Route53.HostedZoneName, owner.Spec.Names, rr.Spec)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.ResourceRecord{}).
		Complete(r)
}
