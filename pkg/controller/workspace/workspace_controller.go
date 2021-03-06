package workspace

import (
	"context"
	"fmt"
	"github.com/che-incubator/che-workspace-operator/internal/cluster"
	workspacev1alpha1 "github.com/che-incubator/che-workspace-operator/pkg/apis/workspace/v1alpha1"
	"github.com/che-incubator/che-workspace-operator/pkg/config"
	"github.com/che-incubator/che-workspace-operator/pkg/controller/workspace/prerequisites"
	"github.com/che-incubator/che-workspace-operator/pkg/controller/workspace/provision"
	wsRuntime "github.com/che-incubator/che-workspace-operator/pkg/controller/workspace/runtime"
	"github.com/google/uuid"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	origLog "log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
)

var log = logf.Log.WithName("controller_workspace")

// Add creates a new Workspace Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileWorkspace {
	return &ReconcileWorkspace{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileWorkspace) error {
	// Create a new controller
	c, err := controller.New("workspace-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	operatorNamespace, err := k8sutil.GetOperatorNamespace()
	if err == nil {
		config.ConfigMapReference.Namespace = operatorNamespace
	} else if err == k8sutil.ErrRunLocal {
		config.ConfigMapReference.Namespace = os.Getenv("WATCH_NAMESPACE")
		log.Info(fmt.Sprintf("Running operator in local mode; watching namespace %s", config.ConfigMapReference.Namespace))
	} else if err != k8sutil.ErrNoNamespace {
		return err
	}

	err = config.WatchControllerConfig(c, mgr)
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Workspace
	err = c.Watch(&source.Kind{Type: &workspacev1alpha1.Workspace{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployments and requeue the owner Workspace
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &workspacev1alpha1.Workspace{},
	})
	if err != nil {
		return err
	}

	// Watch for changes in secondary resource Components and requeue the owner workspace
	err = c.Watch(&source.Kind{Type: &workspacev1alpha1.Component{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &workspacev1alpha1.Workspace{},
	})

	err = c.Watch(&source.Kind{Type: &workspacev1alpha1.WorkspaceRouting{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &workspacev1alpha1.Workspace{},
	})

	// Check if we're running on OpenShift
	isOS, err := cluster.IsOpenShift()
	if err != nil {
		return err
	}
	config.ControllerCfg.SetIsOpenShift(isOS)

	// Redirect standard logging to the reconcile's log
	// Necessary as e.g. the plugin broker logs to stdout
	origLog.SetOutput(r)

	return nil
}

// blank assignment to verify that ReconcileWorkspace implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileWorkspace{}

// ReconcileWorkspace reconciles a Workspace object
type ReconcileWorkspace struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Enable redirecting standard log output to the controller's log
func (r *ReconcileWorkspace) Write(p []byte) (n int, err error) {
	log.Info(string(p))
	return len(p), nil
}

// Reconcile reads that state of the cluster for a Workspace object and makes changes based on the state read
// and what is in the Workspace.Spec
func (r *ReconcileWorkspace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Workspace")
	clusterAPI := provision.ClusterAPI{
		Client: r.client,
		Scheme: r.scheme,
		Logger: reqLogger,
	}

	// Fetch the Workspace instance
	workspace := &workspacev1alpha1.Workspace{}
	err := r.client.Get(context.TODO(), request.NamespacedName, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// TODO: The rolebindings here are created namespace-wide; find a way to limit this, given that each workspace
	// needs a new serviceAccount
	err = prerequisites.CheckPrerequisites(workspace, r.client, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ensure workspaceID is set.
	if workspace.Status.WorkspaceId == "" {
		workspaceId, err := getWorkspaceId(workspace)
		if err != nil {
			return reconcile.Result{}, err
		}
		workspace.Status.WorkspaceId = workspaceId
	}

	// Step one: Create components, and wait for their states to be ready.
	componentsStatus := provision.SyncComponentsToCluster(workspace, clusterAPI)
	if !componentsStatus.Continue {
		reqLogger.Info("Waiting on components to be ready")
		return reconcile.Result{Requeue: componentsStatus.Requeue}, componentsStatus.Err
	}
	componentDescriptions := componentsStatus.ComponentDescriptions

	cheRestApisComponent := getCheRestApisComponent(workspace.Name, workspace.Status.WorkspaceId, workspace.Namespace)
	componentDescriptions = append(componentDescriptions, cheRestApisComponent)

	// Step two: Create routing, and wait for routing to be ready
	routingStatus := provision.SyncRoutingToCluster(workspace, componentDescriptions, clusterAPI)
	if !routingStatus.Continue {
		reqLogger.Info("Waiting on routing to be ready")
		return reconcile.Result{Requeue: routingStatus.Requeue}, routingStatus.Err
	}

	// Step 2.5: setup runtime annotation (TODO: use configmap)
	cheRuntime, err := wsRuntime.ConstructRuntimeAnnotation(componentDescriptions, routingStatus.ExposedEndpoints)
	workspaceStatus := provision.SyncWorkspaceStatus(workspace, cheRuntime, clusterAPI)
	if !workspaceStatus.Continue {
		reqLogger.Info("Updating workspace status")
		return reconcile.Result{Requeue: workspaceStatus.Requeue}, workspaceStatus.Err
	}

	// Step three: Collect all workspace deployment contributions
	routingPodAdditions := routingStatus.PodAdditions
	var podAdditions []workspacev1alpha1.PodAdditions
	for _, component := range componentDescriptions {
		podAdditions = append(podAdditions, component.PodAdditions)
	}
	if routingPodAdditions != nil {
		podAdditions = append(podAdditions, *routingPodAdditions)
	}

	// Step four: Prepare workspace ServiceAccount
	saAnnotations := map[string]string{}
	if routingPodAdditions != nil {
		saAnnotations = routingPodAdditions.ServiceAccountAnnotations
	}
	serviceAcctStatus := provision.SyncServiceAccount(workspace, saAnnotations, clusterAPI)
	if !serviceAcctStatus.Continue{
		reqLogger.Info("Waiting for workspace ServiceAccount")
		return reconcile.Result{Requeue: serviceAcctStatus.Requeue}, serviceAcctStatus.Err
	}
	serviceAcctName := serviceAcctStatus.ServiceAccountName

	// Step five: Create deployment and wait for it to be ready
	deploymentStatus := provision.SyncDeploymentToCluster(workspace, podAdditions, serviceAcctName, clusterAPI)
	if !deploymentStatus.Continue {
		reqLogger.Info("Waiting on deployment to be ready")
		return reconcile.Result{Requeue: deploymentStatus.Requeue}, deploymentStatus.Err
	}

	reqLogger.Info("Everything ready :)")
	return reconcile.Result{}, nil
}

func getWorkspaceId(instance *workspacev1alpha1.Workspace) (string, error) {
	uid, err := uuid.Parse(string(instance.UID))
	if err != nil {
		return "", err
	}
	return "workspace" + strings.Join(strings.Split(uid.String(), "-")[0:3], ""), nil
}
