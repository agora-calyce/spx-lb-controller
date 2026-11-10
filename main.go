package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-auth-plugin works.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"spx-lb-controller/controllers"
	"spx-lb-controller/pkg/api"
)

var (
	scheme      = runtime.NewScheme()
	setupLog    = ctrl.Log.WithName("setup")
	development = "true"
)

// projectIDAnnotation is set by Cluster API on Nodes; its value is the SPX
// project namespace ("spx-" + ProjectID).
const projectIDAnnotation = "cluster.x-k8s.io/cluster-namespace"

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var apiURL string
	var apiToken string
	var apiOrgaID string
	var apiAZ string
	var debug bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&apiURL, "api-url", getEnv("API_URL", "https://api.superphenix.net"), "The base URL of the LB API.")
	flag.StringVar(&apiToken, "api-token", getEnv("API_TOKEN", ""), "The api-token for the LB API.")
	flag.StringVar(&apiOrgaID, "api-orga-id", getEnv("API_ORGA_ID", ""), "The organization ID for the LB API.")
	flag.StringVar(&apiAZ, "api-az", getEnv("API_AZ", ""), "The AZ code for the LB API.")
	flag.BoolVar(&debug, "debug", getEnv("DEBUG", "false") == "true", "Enable debug mode for full error logs and API tracing.")
	opts := zap.Options{
		Development: development == "true" || debug,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "spx-lb-controller-lock",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create clientset for project discovery")
		os.Exit(1)
	}
	projectID, err := discoverProjectID(context.Background(), clientset)
	if err != nil {
		setupLog.Error(err, "unable to discover ProjectID")
		os.Exit(1)
	}
	setupLog.Info("discovered ProjectID from node annotation", "projectID", projectID)

	apiClient := api.NewClient(apiURL, apiToken, apiOrgaID, apiAZ, projectID, debug)

	if err = (&controllers.ServiceReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		APIClient: apiClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// discoverProjectID retrieves the SPX ProjectID from the
// cluster.x-k8s.io/cluster-namespace annotation of a Node, without its "spx-"
// prefix. It returns an error if no Node, no annotation or no valid UUID is found.
func discoverProjectID(ctx context.Context, clientset kubernetes.Interface) (string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("unable to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return "", errors.New("no nodes found")
	}

	node := nodes.Items[0]
	projectIDRaw := node.Annotations[projectIDAnnotation]
	if projectIDRaw == "" {
		return "", fmt.Errorf("node %s does not have the %s annotation", node.Name, projectIDAnnotation)
	}

	projectID := strings.TrimPrefix(projectIDRaw, "spx-")
	if _, err := uuid.Parse(projectID); err != nil {
		return "", fmt.Errorf("annotation %s=%q on node %s is not a valid ProjectID: %w", projectIDAnnotation, projectIDRaw, node.Name, err)
	}

	return projectID, nil
}
