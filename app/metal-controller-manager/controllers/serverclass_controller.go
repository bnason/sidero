// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package controllers

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	metalv1alpha1 "github.com/talos-systems/sidero/app/metal-controller-manager/api/v1alpha1"
)

// ServerClassReconciler reconciles a ServerClass object.
type ServerClassReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

type serverFilter interface {
	filterCPU([]metalv1alpha1.CPUInformation) serverFilter
	filterSysInfo([]metalv1alpha1.SystemInformation) serverFilter
	filterLabels([]map[string]string) serverFilter
	fetchItems() map[string]metalv1alpha1.Server
}

type serverResults struct {
	items map[string]metalv1alpha1.Server
}

func newServerFilter(sl *metalv1alpha1.ServerList) serverFilter {
	newSF := &serverResults{
		items: make(map[string]metalv1alpha1.Server),
	}

	// Add all servers to serverclass, but only if they've been accepted first
	for _, server := range sl.Items {
		if server.Spec.Accepted {
			newSF.items[server.Name] = server
		}
	}

	return newSF
}

func (sr *serverResults) filterCPU(filters []metalv1alpha1.CPUInformation) serverFilter {
	if len(filters) == 0 {
		return sr
	}

	for _, server := range sr.items {
		var match bool

		for _, cpu := range filters {
			if server.Spec.CPU != nil && cpu.PartialEqual(server.Spec.CPU) {
				match = true

				break
			}
		}

		if !match {
			// Remove from results list if it's there since it's not a match for this qualifier
			delete(sr.items, server.ObjectMeta.Name)
		}
	}

	return sr
}

func (sr *serverResults) filterSysInfo(filters []metalv1alpha1.SystemInformation) serverFilter {
	if len(filters) == 0 {
		return sr
	}

	for _, server := range sr.items {
		var match bool

		for _, sysInfo := range filters {
			if server.Spec.SystemInformation != nil && sysInfo.PartialEqual(server.Spec.SystemInformation) {
				match = true
				break
			}
		}

		if !match {
			// Remove from results list if it's there since it's not a match for this qualifier
			delete(sr.items, server.ObjectMeta.Name)
		}
	}

	return sr
}

func (sr *serverResults) filterLabels(filters []map[string]string) serverFilter {
	if len(filters) == 0 {
		return sr
	}

	for _, server := range sr.items {
		var match bool

		for _, label := range filters {
			for labelKey, labelVal := range label {
				if val, ok := server.ObjectMeta.Labels[labelKey]; ok {
					if labelVal == val {
						match = true
						break
					}
				}
			}
		}

		if !match {
			// Remove from results list if it's there since it's not a match for this qualifier
			delete(sr.items, server.ObjectMeta.Name)
		}
	}

	return sr
}

func (sr *serverResults) fetchItems() map[string]metalv1alpha1.Server {
	return sr.items
}

// +kubebuilder:rbac:groups=metal.sidero.dev,resources=serverclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.sidero.dev,resources=serverclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.sidero.dev,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.sidero.dev,resources=servers/status,verbs=get;update;patch

func (r *ServerClassReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	l := r.Log.WithValues("serverclass", req.NamespacedName)

	l.Info("fetching serverclass", "serverclass", req.NamespacedName)

	sc := metalv1alpha1.ServerClass{}

	if err := r.Get(ctx, req.NamespacedName, &sc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&sc, r)
	if err != nil {
		return ctrl.Result{}, err
	}

	sl := &metalv1alpha1.ServerList{}

	if err := r.List(ctx, sl); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get serverclass: %w", err)
	}

	// Create serverResults struct and seed items with all known, accepted servers
	results := newServerFilter(sl)

	// Filter servers down based on qualifiers
	results = results.filterCPU(sc.Spec.Qualifiers.CPU)
	results = results.filterSysInfo(sc.Spec.Qualifiers.SystemInformation)
	results = results.filterLabels(sc.Spec.Qualifiers.LabelSelectors)

	avail := []string{}
	used := []string{}

	for _, server := range results.fetchItems() {
		if server.Status.InUse {
			used = append(used, server.Name)
			continue
		}

		avail = append(avail, server.Name)
	}

	// sort lists to avoid spurious updates due to `map` key ordering
	sort.Strings(avail)
	sort.Strings(used)

	sc.Status.ServersAvailable = avail
	sc.Status.ServersInUse = used

	if err := patchHelper.Patch(ctx, &sc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ServerClassReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	// This mapRequests handler allows us to add a watch on server resources. Upon a server resource update,
	// we will dump all server classes and issue a reconcile against them so that they will get updated statuses
	// for available/in-use servers that match.
	mapRequests := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			reqList := []reconcile.Request{}

			scList := &metalv1alpha1.ServerClassList{}

			if err := r.List(context.Background(), scList); err != nil {
				return reqList
			}

			for _, serverClass := range scList.Items {
				reqList = append(
					reqList,
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      serverClass.Name,
							Namespace: serverClass.Namespace,
						},
					},
				)
			}

			return reqList
		})

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&metalv1alpha1.ServerClass{}).
		Watches(
			&source.Kind{Type: &metalv1alpha1.Server{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: mapRequests,
			},
		).
		Complete(r)
}
