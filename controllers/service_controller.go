package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"spx-lb-controller/pkg/api"
)

const (
	lbFinalizer          = "superphenix.net/lb-finalizer"
	lbEIDAnnotation      = "superphenix.net/lb-eid"
	lbVIPAnnotation      = "superphenix.net/lb-vip"
	lbExposeAnnotation   = "superphenix.net/lb-expose"
	eipEIDAnnotation     = "superphenix.net/eip-eid"
	subnetEIDAnnotation  = "superphenix.net/subnet-eid"
	clusterNameAnno      = "cluster.x-k8s.io/cluster-name"
	clusterNamespaceAnno = "cluster.x-k8s.io/cluster-namespace"
	vipCIDR              = "198.18.0.0/16"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	APIClient *api.Client
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var service corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !service.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &service)
	}

	// Check if the service is of type LoadBalancer
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		// If it was previously a LoadBalancer, we might want to delete it from external API
		if controllerutil.ContainsFinalizer(&service, lbFinalizer) {
			return r.reconcileDelete(ctx, &service)
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&service, lbFinalizer) {
		controllerutil.AddFinalizer(&service, lbFinalizer)
		if err := r.Update(ctx, &service); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Prepare API body
	vip, err := r.getOrGenerateVIP(ctx, &service)
	if err != nil {
		l.Error(err, "Failed to get or generate VIP")
		return ctrl.Result{}, err
	}

	subnetID, err := r.getOrComputeSubnetID(ctx, &service)
	if err != nil {
		l.Error(err, "Failed to get subnet ID")
		return ctrl.Result{}, err
	}

	body, err := r.prepareLBBody(ctx, &service, vip, subnetID)
	if err != nil {
		l.Error(err, "Failed to prepare load balancer body")
		return ctrl.Result{}, err
	}

	eid := service.Annotations[lbEIDAnnotation]
	var lbInfo *api.LBFullResponse
	if eid == "" {
		// Create
		l.Info("Creating external load balancer", "service", service.Name, "vip", vip)
		newEID, err := r.APIClient.CreateLoadBalancer(body)
		if err != nil {
			l.Error(err, "Failed to create external load balancer")
			return ctrl.Result{}, err
		}
		eid = newEID
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		service.Annotations[lbEIDAnnotation] = eid
		service.Annotations[lbVIPAnnotation] = vip
		if err := r.Update(ctx, &service); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Update only when the desired state differs from the external one
		lbInfo, err = r.APIClient.GetLoadBalancer(eid)
		if err != nil {
			l.Error(err, "Failed to get load balancer info")
			return ctrl.Result{}, err
		}
		if lbUpToDate(body, lbInfo) {
			l.V(1).Info("External load balancer is up to date", "service", service.Name, "eid", eid)
		} else {
			l.Info("Updating external load balancer", "service", service.Name, "eid", eid)
			if err := r.APIClient.UpdateLoadBalancer(eid, body); err != nil {
				l.Error(err, "Failed to update external load balancer")
				return ctrl.Result{}, err
			}
			lbInfo = nil
		}
	}

	// Get the latest LB info (after a creation or an update) to have the actual VIP
	if lbInfo == nil {
		lbInfo, err = r.APIClient.GetLoadBalancer(eid)
		if err != nil {
			l.Error(err, "Failed to get load balancer info")
			return ctrl.Result{}, err
		}
	}

	// Handle EIP for public exposure
	expose := service.Annotations[lbExposeAnnotation]
	eipEID := service.Annotations[eipEIDAnnotation]

	var ingressIP string
	if expose == "public" {
		if eipEID == "" {
			l.Info("Creating EIP for public load balancer", "service", service.Name)

			eipBody := api.CreateEIPBody{}
			eipBody.General.ProductName = service.Name
			eipBody.General.SubnetEID = subnetID
			eipBody.Spec.InternalIP = lbInfo.LoadBalancer.Spec.VIP

			newEipEID, err := r.APIClient.CreateEIP(eipBody)
			if err != nil {
				l.Error(err, "Failed to create EIP")
				return ctrl.Result{}, err
			}
			if service.Annotations == nil {
				service.Annotations = make(map[string]string)
			}
			service.Annotations[eipEIDAnnotation] = newEipEID
			if err := r.Update(ctx, &service); err != nil {
				return ctrl.Result{}, err
			}
			// EIP created, requeue to check its status
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// EIP exists, check its status
		eipInfo, err := r.APIClient.GetEIP(eipEID)
		if err != nil {
			l.Error(err, "Failed to get EIP info", "eipEID", eipEID)
			return ctrl.Result{}, err
		}

		if !eipInfo.EIP.Status.Ready || eipInfo.ExternalIP() == "" {
			l.Info("EIP is not ready yet, retrying...", "service", service.Name, "eipEID", eipEID)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		ingressIP = eipInfo.ExternalIP()
		l.Info("EIP is ready", "service", service.Name, "externalIP", ingressIP)
	} else {
		// Not public, if EIP exists, delete it
		if eipEID != "" {
			l.Info("Deleting EIP as service is no longer public", "service", service.Name, "eipEID", eipEID)
			if err := r.APIClient.DeleteEIP(eipEID); err != nil {
				l.Error(err, "Failed to delete EIP")
				return ctrl.Result{}, err
			}
			delete(service.Annotations, eipEIDAnnotation)
			if err := r.Update(ctx, &service); err != nil {
				return ctrl.Result{}, err
			}
			// Requeue for status update in next iteration
			return ctrl.Result{Requeue: true}, nil
		}
		ingressIP = lbInfo.LoadBalancer.Spec.VIP
	}

	// Update status with determined IP
	if ingressIP != "" {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{IP: ingressIP},
		}
		if err := r.Status().Update(ctx, &service); err != nil {
			l.Error(err, "Failed to update service status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) reconcileDelete(ctx context.Context, service *corev1.Service) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	eipEID := service.Annotations[eipEIDAnnotation]
	if eipEID != "" {
		l.Info("Deleting external EIP", "service", service.Name, "eipEID", eipEID)
		if err := r.APIClient.DeleteEIP(eipEID); err != nil {
			l.Error(err, "Failed to delete external EIP")
			return ctrl.Result{}, err
		}
	}

	eid := service.Annotations[lbEIDAnnotation]
	if eid != "" {
		l.Info("Deleting external load balancer", "service", service.Name, "eid", eid)
		if err := r.APIClient.DeleteLoadBalancer(eid); err != nil {
			l.Error(err, "Failed to delete external load balancer")
			return ctrl.Result{}, err
		}
	}

	if controllerutil.ContainsFinalizer(service, lbFinalizer) {
		controllerutil.RemoveFinalizer(service, lbFinalizer)
		if err := r.Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// getOrComputeSubnetID returns the subnet EID stored on the service, or computes
// it from the cluster info and stores it in the subnet-eid annotation.
func (r *ServiceReconciler) getOrComputeSubnetID(ctx context.Context, service *corev1.Service) (string, error) {
	if subnetID := service.Annotations[subnetEIDAnnotation]; subnetID != "" {
		return subnetID, nil
	}

	log.FromContext(ctx).Info("Calculating subnet ID", "service", service.Name)
	subnetID, err := r.getSubnetID(ctx)
	if err != nil {
		return "", err
	}

	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}
	service.Annotations[subnetEIDAnnotation] = subnetID
	if err := r.Update(ctx, service); err != nil {
		return "", err
	}
	return subnetID, nil
}

func (r *ServiceReconciler) getSubnetID(ctx context.Context) (string, error) {
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, client.Limit(1)); err != nil {
		return "", err
	}
	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("no nodes found to retrieve cluster ID")
	}

	clusterID := nodes.Items[0].Annotations[clusterNameAnno]
	if clusterID == "" {
		return "", fmt.Errorf("node %s does not have annotation %s", nodes.Items[0].Name, clusterNameAnno)
	}

	clusterInfo, err := r.APIClient.GetClusterInfo(clusterID)
	if err != nil {
		return "", err
	}

	if len(clusterInfo.Spec.Groups) == 0 || len(clusterInfo.Spec.Groups[0].Subnets) == 0 {
		return "", fmt.Errorf("cluster info does not contain subnet info")
	}

	subnetLocalID := clusterInfo.Spec.Groups[0].Subnets[0].ID
	projectID := r.APIClient.ProjectID

	// The subnet EID is computed like SPX effective IDs:
	// "spx-" + uuidv5(namespace: projectID, value: subnetLocalID)
	ns, err := uuid.Parse(projectID)
	if err != nil {
		return "", fmt.Errorf("projectID %s is not a valid UUID: %v", projectID, err)
	}

	u := uuid.NewSHA1(ns, []byte(subnetLocalID))
	return fmt.Sprintf("spx-%s", u.String()), nil
}

func (r *ServiceReconciler) prepareLBBody(ctx context.Context, service *corev1.Service, vip, subnetID string) (api.CreateLoadBalancerBody, error) {
	body := api.CreateLoadBalancerBody{}
	body.General.ProductName = fmt.Sprintf("k8s-lb-%s-%s", service.Namespace, service.Name)

	body.Spec.VIP = vip
	body.Spec.SubnetEID = subnetID

	for _, p := range service.Spec.Ports {
		body.Spec.Ports = append(body.Spec.Ports, api.RulePort{
			Port:       int(p.Port),
			Protocol:   string(p.Protocol),
			TargetPort: int(p.NodePort),
		})
	}

	// Fetch EndpointSlices for this service
	var epsList discoveryv1.EndpointSliceList
	if err := r.List(ctx, &epsList, client.InNamespace(service.Namespace), client.MatchingLabels{"kubernetes.io/service-name": service.Name}); err != nil {
		return body, err
	}

	nodeIPs := make(map[string]struct{})
	for _, eps := range epsList.Items {
		for _, endpoint := range eps.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				if endpoint.NodeName != nil && *endpoint.NodeName != "" {
					var node corev1.Node
					if err := r.Get(ctx, client.ObjectKey{Name: *endpoint.NodeName}, &node); err != nil {
						continue
					}
					for _, addr := range node.Status.Addresses {
						if addr.Type == corev1.NodeInternalIP {
							nodeIPs[addr.Address] = struct{}{}
							break
						}
					}
				}
			}
		}
	}

	for ip := range nodeIPs {
		body.Spec.Endpoints = append(body.Spec.Endpoints, ip)
	}
	// Map iteration order is random: keep the body deterministic
	slices.Sort(body.Spec.Endpoints)

	return body, nil
}

// lbUpToDate reports whether the external load balancer already matches the
// desired body, so that no update is needed. Endpoints and ports are compared
// regardless of their order. The subnet is only compared when the API reports it.
func lbUpToDate(desired api.CreateLoadBalancerBody, current *api.LBFullResponse) bool {
	cur := current.LoadBalancer.Spec
	if current.ProductName != desired.General.ProductName || cur.VIP != desired.Spec.VIP {
		return false
	}

	currentSubnet := current.LoadBalancer.SubnetEID
	if currentSubnet == "" {
		currentSubnet = cur.SubnetEID
	}
	if currentSubnet != "" && currentSubnet != desired.Spec.SubnetEID {
		return false
	}

	return sameStrings(cur.Endpoints, desired.Spec.Endpoints) && samePorts(cur.Ports, desired.Spec.Ports)
}

func sameStrings(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func samePorts(a, b []api.RulePort) bool {
	key := func(ports []api.RulePort) []string {
		keys := make([]string, 0, len(ports))
		for _, p := range ports {
			keys = append(keys, fmt.Sprintf("%d/%s/%d", p.Port, strings.ToUpper(p.Protocol), p.TargetPort))
		}
		slices.Sort(keys)
		return keys
	}
	return slices.Equal(key(a), key(b))
}

func (r *ServiceReconciler) getOrGenerateVIP(ctx context.Context, service *corev1.Service) (string, error) {
	if vip, ok := service.Annotations[lbVIPAnnotation]; ok && vip != "" {
		return vip, nil
	}

	// Generate a new VIP in 198.18.0.0/16
	_, ipnet, err := net.ParseCIDR(vipCIDR)
	if err != nil {
		return "", err
	}

	ones, bits := ipnet.Mask.Size()
	numHosts := big.NewInt(1)
	numHosts.Lsh(numHosts, uint(bits-ones))

	// We want to avoid .0 and .255 usually, although in a /16 there are many such IPs.
	// For simplicity, let's just pick a random number between 1 and numHosts-2
	// and add it to the base IP.

	// range = numHosts - 2
	maxRange := new(big.Int).Sub(numHosts, big.NewInt(2))
	if maxRange.Cmp(big.NewInt(0)) <= 0 {
		return "", fmt.Errorf("CIDR too small")
	}

	randOffset, err := rand.Int(rand.Reader, maxRange)
	if err != nil {
		return "", err
	}
	// offset = randOffset + 1 (to avoid .0)
	offset := new(big.Int).Add(randOffset, big.NewInt(1))

	ipInt := big.NewInt(0).SetBytes(ipnet.IP.To4())
	ipInt.Add(ipInt, offset)

	ipBytes := ipInt.Bytes()
	// Ensure we have 4 bytes
	if len(ipBytes) < 4 {
		padded := make([]byte, 4)
		copy(padded[4-len(ipBytes):], ipBytes)
		ipBytes = padded
	}

	return net.IP(ipBytes).String(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Watches(
			&discoveryv1.EndpointSlice{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				eps, ok := obj.(*discoveryv1.EndpointSlice)
				if !ok {
					return nil
				}
				svcName := eps.Labels["kubernetes.io/service-name"]
				if svcName == "" {
					return nil
				}
				return []reconcile.Request{
					{NamespacedName: client.ObjectKey{
						Namespace: eps.Namespace,
						Name:      svcName,
					}},
				}
			}),
		).
		Complete(r)
}
