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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	xytestv1 "xy.io/test/api/v1"
)

// XyDaemonsetReconciler reconciles a XyDaemonset object
type XyDaemonsetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Add permissions to the controller
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// Add permissions to the controller

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the XyDaemonset object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/reconcile
func (r *XyDaemonsetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log.Log.Info("XyDaemonset Controller start reconcile")
	// fetch the XyDaemonset instance
	instance := &xytestv1.XyDaemonset{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if instance.Status.AutoScalingStatus == "Running" {
		log.Log.Info("AutoScaling Running")
		return ctrl.Result{Requeue: true}, nil
	}

	specPodLength := instance.Spec.Replicas
	//获取当前集群所有节点
	allNodeList := &corev1.NodeList{}
	if err := r.Client.List(ctx, allNodeList); err != nil {
		return ctrl.Result{}, err
	}

	currentStatus := xytestv1.XyDaemonsetStatus{}

	for _, node := range allNodeList.Items {
		existingPods, err := r.fetchPodLenAndStatus(ctx, instance, node.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		existingPodNames, existingPodLength := r.filterPods(existingPods.Items)
		if specPodLength != existingPodLength {
			if err := r.markRunning(instance, ctx); err != nil {
				return ctrl.Result{}, err
			}

			// 比期望值小，需要 scale up create
			if specPodLength > existingPodLength {
				log.Log.Info(fmt.Sprintf("creating pod, current num %d < expected num %d", existingPodLength, specPodLength))
				pod := buildPod(instance, node.Name)
				if err := controllerutil.SetControllerReference(instance, pod, r.Scheme); err != nil {
					log.Log.Error(err, "scale up failed: SetControllerReference")
					return ctrl.Result{}, err
				}
				if err := r.Client.Create(ctx, pod); err != nil {
					log.Log.Error(err, "scale up failed: create pod")
					return ctrl.Result{}, err
				}
				existingPodNames = append(existingPodNames, pod.Name)
				existingPodLength += 1
			}

			// 比期望值大，需要 scale down delete
			if specPodLength < existingPodLength {
				log.Log.Info(fmt.Sprintf("deleting pod, current num %d > expected num %d", existingPodLength, specPodLength))
				pod := existingPods.Items[0]
				existingPods.Items = existingPods.Items[1:]
				existingPodNames = removeString(existingPodNames, pod.Name)
				existingPodLength -= 1
				if err := r.Client.Delete(ctx, &pod); err != nil {
					log.Log.Error(err, "scale down faled")
					return ctrl.Result{}, err
				}
			}
		}
		currentStatus.AvailableReplicas += existingPodLength
		currentStatus.PodNames = append(currentStatus.PodNames, existingPodNames...)
	}
	// 更新当前instance状态
	if instance.Status.AvailableReplicas != currentStatus.AvailableReplicas || !(reflect.DeepEqual(instance.Status.PodNames, currentStatus.PodNames)) {
		log.Log.Info("instance.Status.AvailableReplicas")
		log.Log.Info(fmt.Sprint(instance.Status.PodNames))
		log.Log.Info("currentStatus.PodNames")
		log.Log.Info(fmt.Sprint(currentStatus.PodNames))
		log.Log.Info(fmt.Sprintf("更新当前instance状态, instance.Status.AvailableReplicas %d : currentStatus.AvailableReplicas %d", instance.Status.AvailableReplicas, currentStatus.AvailableReplicas))
		currentStatus.AutoScalingStatus = "Sleep"
		instance.Status = currentStatus
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			log.Log.Error(err, "update pod failed")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true}, nil
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func (r *XyDaemonsetReconciler) fetchPodLenAndStatus(ctx context.Context, ds *xytestv1.XyDaemonset, nodeName string) (*corev1.PodList, error) {
	// 获取当前instance用label对应的所有的 pod 的名称列表
	existingPods := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(labels.Set{"xyDs": ds.Name, "nodeName": nodeName})
	listOps := &client.ListOptions{Namespace: ds.Namespace, LabelSelector: labelSelector}

	if err := r.Client.List(ctx, existingPods, listOps); err != nil {
		log.Log.Error(err, "fetching existing pods failed")
		return existingPods, err
	}

	return existingPods, nil
}

func buildPod(ds *xytestv1.XyDaemonset, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", ds.Name),
			Namespace:    ds.Namespace,
			Labels:       map[string]string{"xyDs": ds.Name, "nodeName": nodeName},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    ds.Name + "-container",
					Image:   ds.Spec.Image,
					Command: ds.Spec.Command,
				},
			},
			NodeName: nodeName,
		},
	}
}

func (r *XyDaemonsetReconciler) markRunning(ds *xytestv1.XyDaemonset, ctx context.Context) error {
	ds.Status.AutoScalingStatus = "Running"
	if err := r.Client.Status().Update(ctx, ds); err != nil {
		log.Log.Info("Begin AutoScaling")
		return err
	}
	return nil
}
func (r *XyDaemonsetReconciler) filterPods(pods []corev1.Pod) ([]string, int) {
	var existingPodNames []string
	for _, pod := range pods {
		if pod.GetObjectMeta().GetDeletionTimestamp() != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			existingPodNames = append(existingPodNames, pod.GetObjectMeta().GetName())
		}
	}
	return existingPodNames, len(existingPodNames)
}

// SetupWithManager sets up the controller with the Manager.
func (r *XyDaemonsetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&xytestv1.XyDaemonset{}).
		Named("xydaemonset").
		Complete(r)
}
