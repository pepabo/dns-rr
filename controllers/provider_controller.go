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
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	dnsv1alpha1 "github.com/ch1aki/dns-rr/api/v1alpha1"
	provider "github.com/ch1aki/dns-rr/controllers/provider"
)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	providerCacheUpdateIntervalMinute = 5
)

var (
	providerZoneCacheLock sync.RWMutex
	providerZoneCache     map[string][]types.ResourceRecordSet
)

//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=providers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=providers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.ch1aki.github.io,resources=providers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Provider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var p dnsv1alpha1.Provider
	err := r.Get(ctx, req.NamespacedName, &p)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "unable to get Provider", "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !p.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// TODO(validation):
	// TODO(reconcile):

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Start cache update gorutine
	go r.updateCacheInBackground(context.Background(), providerCacheUpdateIntervalMinute*time.Minute)

	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.Provider{}).
		Complete(r)
}

func (r *ProviderReconciler) updateCacheInBackground(ctx context.Context, interval time.Duration) {
	logger := log.FromContext(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Start initialize cache")
	providerZoneCache = make(map[string][]types.ResourceRecordSet)
	if err := r.updateCache(ctx); err != nil {
		logger.Error(err, "Failed to initialize cache")
	}
	logger.Info("Successed initialize cache!!")

	logger.Info("Started cache update in background")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.updateCache(ctx); err != nil {
				logger.Error(err, "failed to update cache in background")
			}
		}
	}
}

func (r *ProviderReconciler) updateCache(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Get custom resources
	providerList := &dnsv1alpha1.ProviderList{}
	if err := r.List(ctx, providerList); err != nil {
		return err
	}

	// update cache
	providerZoneCacheLock.Lock()
	defer providerZoneCacheLock.Unlock()
	for _, p := range providerList.Items {
		cacheKey := p.Namespace + "/" + p.Name
		var route53 provider.Route53Provider
		client, err := route53.NewClient(ctx, &p, r.Client, &providerZoneCache)
		if err != nil {
			logger.Error(err, "failed to initialize provider client at updateCache")
			return err
		}
		cacheData, err := client.AllRecords(ctx, p.Spec.Route53.HostedZoneName)
		if err != nil {
			logger.Error(err, "fialed to get AllRecords")
			return err
		}
		providerZoneCache[cacheKey] = cacheData
		logger.Info("Completed updating cache.", "cache", cacheKey)
	}

	return nil
}
